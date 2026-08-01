package persistence

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"os"
	"sync"
	"time"

	"github.com/ChrisMarxDev/tinkercloud/internal/blob"
	"github.com/ChrisMarxDev/tinkercloud/internal/operations"
)

// BlobRepository owns catalog transitions; LocalStore owns only private bytes.
// The mutex deliberately spans staging/finalization so the single-node quota is
// exact even while a streamed upload is outside SQLite's write transaction.
type BlobBytes interface {
	Put(string, string, io.Reader, int64) (int64, string, error)
	Open(string, string) (io.ReadCloser, int64, string, error)
	Delete(string, string) error
	Keys(blob.Key, int) ([]blob.Key, bool, error)
}
type BlobRepository struct {
	Store             *SQLiteStore
	Bytes             BlobBytes
	WriteGate         operations.WriteGate
	BeforeReadyCommit func() error // test/failure-injection seam; production nil
	mu                sync.Mutex
	locks             map[string]*blobAppLock
}
type blobAppLock struct {
	mu   sync.Mutex
	refs int
}

// lockApp serializes transitions only within one app. A global upload mutex
// would allow one slow tenant to block every other tenant's quota path.
func (r *BlobRepository) lockApp(app string) func() {
	r.mu.Lock()
	if r.locks == nil {
		r.locks = map[string]*blobAppLock{}
	}
	l := r.locks[app]
	if l == nil {
		l = &blobAppLock{}
		r.locks[app] = l
	}
	l.refs++
	r.mu.Unlock()
	l.mu.Lock()
	return func() {
		l.mu.Unlock()
		r.mu.Lock()
		l.refs--
		if l.refs == 0 {
			delete(r.locks, app)
		}
		r.mu.Unlock()
	}
}

func (r *BlobRepository) Upload(ctx context.Context, app, identity string, m blob.Metadata, src io.Reader, limits blob.Limits) (blob.Metadata, error) {
	if r.Store == nil {
		return blob.Metadata{}, blob.ErrUnavailable
	}
	if r.WriteGate != nil {
		if e := r.WriteGate.AllowWrite(ctx, operations.WriteBlob); e != nil {
			return blob.Metadata{}, e
		}
	}
	unlock := r.lockApp(app)
	defer unlock()
	now := time.Now().UTC()
	m.CreatedAt = now
	if err := r.Store.Write(ctx, func(tx *sql.Tx) error {
		// An authorization context may have been issued before a concurrent app
		// deletion. The catalog transition itself is the final authority: only
		// an application still active in this transaction may acquire staging.
		res, e := tx.ExecContext(ctx, `INSERT INTO app_blobs(id,app_id,state,display_name,content_type,size_bytes,content_hash,created_by_identity_id,created_at,updated_at)
			SELECT ?,?, 'staging',?,?,0,'',?,?,?
			WHERE EXISTS (SELECT 1 FROM applications WHERE id=? AND status='active')`,
			m.ID, app, m.Name, m.ContentType, identity, now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano), app)
		if e != nil {
			return e
		}
		n, e := res.RowsAffected()
		if e != nil {
			return e
		}
		if n != 1 {
			return blob.ErrUnavailable
		}
		return nil
	}); err != nil {
		return blob.Metadata{}, err
	}
	size, hash, err := r.Bytes.Put(app, m.ID, src, limits.BlobBytes)
	if err != nil {
		_ = r.Store.Write(context.Background(), func(tx *sql.Tx) error {
			_, e := tx.ExecContext(context.Background(), "DELETE FROM app_blobs WHERE app_id=? AND id=? AND state='staging'", app, m.ID)
			return e
		})
		_ = r.Bytes.Delete(app, m.ID)
		return blob.Metadata{}, err
	}
	if r.BeforeReadyCommit != nil {
		if err = r.BeforeReadyCommit(); err != nil {
			_ = r.Bytes.Delete(app, m.ID)
			_ = r.Store.Write(context.Background(), func(tx *sql.Tx) error {
				_, e := tx.ExecContext(context.Background(), "DELETE FROM app_blobs WHERE app_id=? AND id=? AND state='staging'", app, m.ID)
				return e
			})
			return blob.Metadata{}, err
		}
	}
	err = r.Store.Write(ctx, func(tx *sql.Tx) error {
		var count int
		var total int64
		if e := tx.QueryRowContext(ctx, "SELECT COUNT(*),COALESCE(SUM(size_bytes),0) FROM app_blobs WHERE app_id=? AND state='ready'", app).Scan(&count, &total); e != nil {
			return e
		}
		if count >= limits.BlobsPerApp || total+size > limits.TotalBytesPerApp {
			return blob.ErrQuotaExceeded
		}
		// Recheck lifecycle state in the same transaction as finalization. This
		// prevents a pre-deletion upload authorization from publishing bytes
		// after DeleteApp has revoked the app.
		res, e := tx.ExecContext(ctx, `UPDATE app_blobs SET state='ready',size_bytes=?,content_hash=?,updated_at=?
			WHERE app_id=? AND id=? AND state='staging'
			AND EXISTS (SELECT 1 FROM applications WHERE id=? AND status='active')`, size, hash, time.Now().UTC().Format(time.RFC3339Nano), app, m.ID, app)
		if e != nil {
			return e
		}
		n, e := res.RowsAffected()
		if e != nil || n != 1 {
			return blob.ErrUnavailable
		}
		return nil
	})
	if err != nil {
		_ = r.Bytes.Delete(app, m.ID)
		_ = r.Store.Write(context.Background(), func(tx *sql.Tx) error {
			_, e := tx.ExecContext(context.Background(), "DELETE FROM app_blobs WHERE app_id=? AND id=? AND state='staging'", app, m.ID)
			return e
		})
		return blob.Metadata{}, err
	}
	m.Size = size
	return m, nil
}
func (r *BlobRepository) Open(ctx context.Context, app, id string) (io.ReadCloser, blob.Metadata, error) {
	var m blob.Metadata
	var created string
	var hash string
	e := r.Store.DB.QueryRowContext(ctx, "SELECT display_name,content_type,size_bytes,content_hash,created_at FROM app_blobs WHERE app_id=? AND id=? AND state='ready'", app, id).Scan(&m.Name, &m.ContentType, &m.Size, &hash, &created)
	if e == sql.ErrNoRows {
		return nil, blob.Metadata{}, os.ErrNotExist
	}
	if e != nil {
		return nil, blob.Metadata{}, e
	}
	m.ID = id
	m.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
	rd, size, actual, e := r.Bytes.Open(app, id)
	if e != nil || size != m.Size || actual != hash {
		if rd != nil {
			rd.Close()
		}
		return nil, blob.Metadata{}, blob.ErrUnavailable
	}
	return rd, m, nil
}
func (r *BlobRepository) List(ctx context.Context, app, cursor string, limit int) (blob.ListResult, error) {
	rows, e := r.Store.DB.QueryContext(ctx, "SELECT id,display_name,content_type,size_bytes,content_hash,created_at FROM app_blobs WHERE app_id=? AND state='ready' AND id>? ORDER BY id LIMIT ?", app, cursor, limit+1)
	if e != nil {
		return blob.ListResult{}, e
	}
	defer rows.Close()
	out := blob.ListResult{Blobs: []blob.Metadata{}}
	for rows.Next() {
		var m blob.Metadata
		var created, hash string
		if e = rows.Scan(&m.ID, &m.Name, &m.ContentType, &m.Size, &hash, &created); e != nil {
			return out, e
		}
		m.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
		rd, size, actual, openErr := r.Bytes.Open(app, m.ID)
		if rd != nil {
			_ = rd.Close()
		}
		if openErr != nil || size != m.Size || actual != hash {
			// A ready catalog record without matching private bytes is neither
			// listable nor quota-bearing. Cleanup retries byte removal later.
			if markErr := r.markUnavailable(ctx, app, m.ID); markErr != nil {
				return out, markErr
			}
			return out, blob.ErrUnavailable
		}
		if len(out.Blobs) == limit {
			out.NextCursor = out.Blobs[len(out.Blobs)-1].ID
			break
		}
		out.Blobs = append(out.Blobs, m)
	}
	return out, rows.Err()
}
func (r *BlobRepository) markUnavailable(ctx context.Context, app, id string) error {
	unlock := r.lockApp(app)
	defer unlock()
	return r.Store.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "UPDATE app_blobs SET state='deleting',updated_at=? WHERE app_id=? AND id=? AND state='ready'", time.Now().UTC().Format(time.RFC3339Nano), app, id)
		return err
	})
}
func (r *BlobRepository) Delete(ctx context.Context, app, id string) (bool, error) {
	if r.Store == nil {
		return false, blob.ErrUnavailable
	}
	unlock := r.lockApp(app)
	defer unlock()
	var exists bool
	e := r.Store.Write(ctx, func(tx *sql.Tx) error {
		res, e := tx.ExecContext(ctx, "UPDATE app_blobs SET state='deleting',updated_at=? WHERE app_id=? AND id=? AND state='ready'", time.Now().UTC().Format(time.RFC3339Nano), app, id)
		if e != nil {
			return e
		}
		n, e := res.RowsAffected()
		exists = n == 1
		return e
	})
	if e != nil {
		return false, e
	}
	if !exists {
		return false, nil
	}
	if e = r.Bytes.Delete(app, id); e != nil {
		return false, e
	}
	e = r.Store.Write(ctx, func(tx *sql.Tx) error {
		_, e := tx.ExecContext(ctx, "DELETE FROM app_blobs WHERE app_id=? AND id=? AND state='deleting'", app, id)
		return e
	})
	return e == nil, e
}
func (r *BlobRepository) Reconcile(ctx context.Context) error {
	if r.Store == nil {
		return blob.ErrUnavailable
	}
	type key struct{ app, id string }
	// Process finite pages. This is restart-safe and does not turn a long-lived
	// catalog into an unbounded in-memory cleanup list.
	for {
		rows, e := r.Store.DB.QueryContext(ctx, "SELECT b.app_id,b.id FROM app_blobs b LEFT JOIN applications a ON a.id=b.app_id WHERE b.state IN ('staging','deleting') OR a.status IN ('deleting','deleted') ORDER BY b.app_id,b.id LIMIT 256")
		if e != nil {
			return e
		}
		keys := make([]key, 0, 256)
		for rows.Next() {
			var k key
			if e = rows.Scan(&k.app, &k.id); e != nil {
				rows.Close()
				return e
			}
			keys = append(keys, k)
		}
		if e = rows.Err(); e != nil {
			rows.Close()
			return e
		}
		rows.Close()
		if len(keys) == 0 {
			break
		}
		for _, k := range keys {
			unlock := r.lockApp(k.app)
			// Deleted apps can still have ready rows. Hide them before byte work so
			// a partial cleanup never leaves list/quota ghosts.
			if _, e = r.Store.DB.ExecContext(ctx, "UPDATE app_blobs SET state='deleting',updated_at=? WHERE app_id=? AND id=?", time.Now().UTC().Format(time.RFC3339Nano), k.app, k.id); e != nil {
				unlock()
				return e
			}
			if e = r.Bytes.Delete(k.app, k.id); e != nil {
				unlock()
				return e
			}
			if _, e = r.Store.DB.ExecContext(ctx, "DELETE FROM app_blobs WHERE app_id=? AND id=? AND state='deleting'", k.app, k.id); e != nil {
				unlock()
				return e
			}
			unlock()
		}
	}
	// The local adapter is never a public catalog, but recovery may enumerate its
	// own bounded private namespace to remove installed bytes lacking ready
	// metadata (for example, after an interrupted final metadata commit).
	var after blob.Key
	for {
		objects, more, e := r.Bytes.Keys(after, 256)
		if e != nil {
			return e
		}
		for _, object := range objects {
			after = object
			var state string
			e = r.Store.DB.QueryRowContext(ctx, "SELECT state FROM app_blobs WHERE app_id=? AND id=?", object.App, object.ID).Scan(&state)
			if e == sql.ErrNoRows {
				if e = r.Bytes.Delete(object.App, object.ID); e != nil {
					return e
				}
				continue
			}
			if e != nil {
				return e
			}
			if state != "ready" {
				continue
			}
			// Verify catalog and byte evidence before retaining a ready object.
			if rd, _, openErr := r.Open(ctx, object.App, object.ID); openErr != nil {
				if rd != nil {
					_ = rd.Close()
				}
				unlock := r.lockApp(object.App)
				_, _ = r.Store.DB.ExecContext(ctx, "UPDATE app_blobs SET state='deleting',updated_at=? WHERE app_id=? AND id=? AND state='ready'", time.Now().UTC().Format(time.RFC3339Nano), object.App, object.ID)
				if deleteErr := r.Bytes.Delete(object.App, object.ID); deleteErr != nil {
					unlock()
					return deleteErr
				}
				if _, deleteErr := r.Store.DB.ExecContext(ctx, "DELETE FROM app_blobs WHERE app_id=? AND id=? AND state='deleting'", object.App, object.ID); deleteErr != nil {
					unlock()
					return deleteErr
				}
				unlock()
			} else {
				_ = rd.Close()
			}
		}
		if !more {
			break
		}
	}
	// A missing object has no adapter key to enumerate, so independently walk
	// ready catalog rows. Both walks are paged and use a stable app-first key.
	var catalogAfter key
	for {
		rows, e := r.Store.DB.QueryContext(ctx, "SELECT app_id,id FROM app_blobs WHERE state='ready' AND (app_id>? OR (app_id=? AND id>?)) ORDER BY app_id,id LIMIT 256", catalogAfter.app, catalogAfter.app, catalogAfter.id)
		if e != nil {
			return e
		}
		page := make([]key, 0, 256)
		for rows.Next() {
			var k key
			if e = rows.Scan(&k.app, &k.id); e != nil {
				rows.Close()
				return e
			}
			page = append(page, k)
		}
		if e = rows.Err(); e != nil {
			rows.Close()
			return e
		}
		rows.Close()
		if len(page) == 0 {
			break
		}
		for _, k := range page {
			catalogAfter = k
			rd, _, openErr := r.Open(ctx, k.app, k.id)
			if rd != nil {
				_ = rd.Close()
			}
			if openErr == nil {
				continue
			}
			unlock := r.lockApp(k.app)
			if _, e = r.Store.DB.ExecContext(ctx, "UPDATE app_blobs SET state='deleting',updated_at=? WHERE app_id=? AND id=? AND state='ready'", time.Now().UTC().Format(time.RFC3339Nano), k.app, k.id); e != nil {
				unlock()
				return e
			}
			if e = r.Bytes.Delete(k.app, k.id); e != nil {
				unlock()
				return e
			}
			if _, e = r.Store.DB.ExecContext(ctx, "DELETE FROM app_blobs WHERE app_id=? AND id=? AND state='deleting'", k.app, k.id); e != nil {
				unlock()
				return e
			}
			unlock()
		}
	}
	return nil
}

// RemoveAppNamespace removes an app's now-empty private blob directory after
// Reconcile has proved that no cataloged or orphaned bytes remain.
func (r *BlobRepository) RemoveAppNamespace(app string) error {
	if r == nil || r.Bytes == nil {
		return blob.ErrUnavailable
	}
	remover, ok := r.Bytes.(interface{ RemoveAppNamespace(string) error })
	if !ok {
		return blob.ErrUnavailable
	}
	unlock := r.lockApp(app)
	defer unlock()
	return remover.RemoveAppNamespace(app)
}

var _ blob.Repository = (*BlobRepository)(nil)
var _ = errors.Is
