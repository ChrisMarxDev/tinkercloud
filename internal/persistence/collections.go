package persistence

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/ChrisMarxDev/tinkercloud/internal/collections"
	"github.com/ChrisMarxDev/tinkercloud/internal/operations"
)

// CollectionRepository maps the app-scoped document primitive onto a private
// per-app SQLite file. App identity reaches it only after the collection
// service has extracted it from a sealed authorization context.
type CollectionRepository struct {
	Apps      *AppDatabaseManager
	Limits    collections.Limits
	WriteGate operations.WriteGate
	Clock     func() time.Time
}

func (r CollectionRepository) limits() collections.Limits {
	if r.Limits.CollectionBytes == 0 {
		return collections.DefaultLimits()
	}
	return r.Limits
}

func (r CollectionRepository) now() time.Time {
	if r.Clock != nil {
		return r.Clock().UTC()
	}
	return time.Now().UTC()
}

func (r CollectionRepository) withApp(ctx context.Context, appID string, fn func(*AppDatabase) error) error {
	if r.Apps == nil {
		return errors.New("app database manager unavailable")
	}
	db, release, err := r.Apps.Acquire(ctx, appID)
	if err != nil {
		return err
	}
	defer release()
	return fn(db)
}

func (r CollectionRepository) Create(ctx context.Context, appID, collection string, in collections.Document) (collections.Document, collections.Mutation, error) {
	var out collections.Document
	var mutation collections.Mutation
	if r.WriteGate != nil {
		if err := r.WriteGate.AllowWrite(ctx, operations.WriteKV); err != nil {
			return out, mutation, err
		}
	}
	err := r.withApp(ctx, appID, func(db *AppDatabase) error {
		return db.Write(ctx, func(tx *sql.Tx) error {
			var count, total, activeCollections int
			if err := tx.QueryRowContext(ctx, `SELECT COUNT(*),COALESCE(SUM(size_bytes),0) FROM documents WHERE collection=?`, collection).Scan(&count, &total); err != nil {
				return err
			}
			// Revision rows deliberately outlive an empty collection so that a
			// later recreation continues its monotonic revision. The collection
			// quota, however, limits live namespaces with documents—not historical
			// names—so delete/recreate cycles cannot exhaust it permanently.
			if err := tx.QueryRowContext(ctx, `SELECT COUNT(DISTINCT collection) FROM documents`).Scan(&activeCollections); err != nil {
				return err
			}
			var appTotal int
			if err := tx.QueryRowContext(ctx, `SELECT COALESCE(SUM(size_bytes),0) FROM documents`).Scan(&appTotal); err != nil {
				return err
			}
			if count >= r.limits().DocumentsPerCollection || appTotal+len(in.Data) > r.limits().TotalBytesPerApp {
				return collections.ErrQuotaExceeded
			}
			if count == 0 && activeCollections >= r.limits().CollectionsPerApp {
				return collections.ErrQuotaExceeded
			}
			now := r.now()
			out = collections.Document{ID: in.ID, Data: append(json.RawMessage(nil), in.Data...), Version: 1, CreatedAt: now, UpdatedAt: now}
			if _, err := tx.ExecContext(ctx, `INSERT INTO documents(collection,id,data_json,version,size_bytes,created_at,updated_at) VALUES(?,?,?,?,?,?,?)`, collection, out.ID, string(out.Data), out.Version, len(out.Data), now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano)); err != nil {
				return err
			}
			revision, err := incrementCollectionRevision(ctx, tx, collection)
			if err != nil {
				return err
			}
			mutation = collections.Mutation{Collection: collection, ID: out.ID, Version: out.Version, Revision: revision}
			return nil
		})
	})
	return out, mutation, err
}

func (r CollectionRepository) Get(ctx context.Context, appID, collection, id string) (*collections.Document, error) {
	var out *collections.Document
	err := r.withApp(ctx, appID, func(db *AppDatabase) error {
		var doc collections.Document
		var data, created, updated string
		err := db.DB.QueryRowContext(ctx, `SELECT data_json,version,created_at,updated_at FROM documents WHERE collection=? AND id=?`, collection, id).Scan(&data, &doc.Version, &created, &updated)
		if err == sql.ErrNoRows {
			return nil
		}
		if err != nil {
			return err
		}
		doc.ID, doc.Data = id, json.RawMessage(data)
		if doc.CreatedAt, err = time.Parse(time.RFC3339Nano, created); err != nil {
			return err
		}
		if doc.UpdatedAt, err = time.Parse(time.RFC3339Nano, updated); err != nil {
			return err
		}
		out = &doc
		return nil
	})
	return out, err
}

func (r CollectionRepository) Update(ctx context.Context, appID, collection, id string, data json.RawMessage, expected *uint64) (collections.Document, collections.Mutation, error) {
	var out collections.Document
	var mutation collections.Mutation
	if r.WriteGate != nil {
		if err := r.WriteGate.AllowWrite(ctx, operations.WriteKV); err != nil {
			return out, mutation, err
		}
	}
	err := r.withApp(ctx, appID, func(db *AppDatabase) error {
		return db.Write(ctx, func(tx *sql.Tx) error {
			var oldVersion uint64
			var oldBytes int
			var created string
			err := tx.QueryRowContext(ctx, `SELECT version,size_bytes,created_at FROM documents WHERE collection=? AND id=?`, collection, id).Scan(&oldVersion, &oldBytes, &created)
			if err == sql.ErrNoRows || (expected != nil && oldVersion != *expected) {
				return collections.ErrVersionConflict
			}
			if err != nil {
				return err
			}
			var total int
			if err := tx.QueryRowContext(ctx, `SELECT COALESCE(SUM(size_bytes),0) FROM documents`).Scan(&total); err != nil {
				return err
			}
			if total-oldBytes+len(data) > r.limits().TotalBytesPerApp {
				return collections.ErrQuotaExceeded
			}
			now := r.now()
			out = collections.Document{ID: id, Data: append(json.RawMessage(nil), data...), Version: oldVersion + 1, UpdatedAt: now}
			if out.CreatedAt, err = time.Parse(time.RFC3339Nano, created); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `UPDATE documents SET data_json=?,version=?,size_bytes=?,updated_at=? WHERE collection=? AND id=?`, string(data), out.Version, len(data), now.Format(time.RFC3339Nano), collection, id); err != nil {
				return err
			}
			revision, err := incrementCollectionRevision(ctx, tx, collection)
			if err != nil {
				return err
			}
			mutation = collections.Mutation{Collection: collection, ID: id, Version: out.Version, Revision: revision}
			return nil
		})
	})
	return out, mutation, err
}

func (r CollectionRepository) Delete(ctx context.Context, appID, collection, id string, expected *uint64) (bool, collections.Mutation, error) {
	var deleted bool
	var mutation collections.Mutation
	if r.WriteGate != nil {
		if err := r.WriteGate.AllowWrite(ctx, operations.WriteKV); err != nil {
			return false, mutation, err
		}
	}
	err := r.withApp(ctx, appID, func(db *AppDatabase) error {
		return db.Write(ctx, func(tx *sql.Tx) error {
			var version uint64
			err := tx.QueryRowContext(ctx, `SELECT version FROM documents WHERE collection=? AND id=?`, collection, id).Scan(&version)
			if err == sql.ErrNoRows {
				if expected != nil {
					return collections.ErrVersionConflict
				}
				return nil
			}
			if err != nil {
				return err
			}
			if expected != nil && version != *expected {
				return collections.ErrVersionConflict
			}
			if _, err := tx.ExecContext(ctx, `DELETE FROM documents WHERE collection=? AND id=?`, collection, id); err != nil {
				return err
			}
			revision, err := incrementCollectionRevision(ctx, tx, collection)
			if err != nil {
				return err
			}
			deleted = true
			mutation = collections.Mutation{Collection: collection, ID: id, Version: version + 1, Deleted: true, Revision: revision}
			return nil
		})
	})
	return deleted, mutation, err
}

func (r CollectionRepository) List(ctx context.Context, appID, collection, cursor string, limit int) (collections.ListResult, error) {
	out := collections.ListResult{Documents: []collections.Document{}}
	err := r.withApp(ctx, appID, func(db *AppDatabase) error {
		rows, err := db.DB.QueryContext(ctx, `SELECT id,data_json,version,created_at,updated_at FROM documents WHERE collection=? AND id>? ORDER BY id LIMIT ?`, collection, cursor, limit+1)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			doc, err := scanDocument(rows)
			if err != nil {
				return err
			}
			if len(out.Documents) == limit {
				out.NextCursor = out.Documents[len(out.Documents)-1].ID
				break
			}
			out.Documents = append(out.Documents, doc)
		}
		if err := rows.Err(); err != nil {
			return err
		}
		out.Revision, err = collectionRevision(ctx, db.DB, collection)
		return err
	})
	return out, err
}

func (r CollectionRepository) Snapshot(ctx context.Context, appID, collection string, limit int) (collections.Snapshot, error) {
	list, err := r.List(ctx, appID, collection, "", limit)
	if err != nil {
		return collections.Snapshot{}, err
	}
	if list.NextCursor != "" {
		return collections.Snapshot{}, collections.ErrSnapshotTooLarge
	}
	return collections.Snapshot{Collection: collection, Documents: list.Documents, Revision: list.Revision}, nil
}

type rowScanner interface{ Scan(...any) error }

func scanDocument(row rowScanner) (collections.Document, error) {
	var doc collections.Document
	var data, created, updated string
	if err := row.Scan(&doc.ID, &data, &doc.Version, &created, &updated); err != nil {
		return doc, err
	}
	var err error
	doc.Data = json.RawMessage(data)
	if doc.CreatedAt, err = time.Parse(time.RFC3339Nano, created); err != nil {
		return doc, err
	}
	if doc.UpdatedAt, err = time.Parse(time.RFC3339Nano, updated); err != nil {
		return doc, err
	}
	return doc, nil
}

func incrementCollectionRevision(ctx context.Context, tx *sql.Tx, collection string) (uint64, error) {
	var revision uint64
	err := tx.QueryRowContext(ctx, `SELECT revision FROM collection_revisions WHERE collection=?`, collection).Scan(&revision)
	if err == sql.ErrNoRows {
		revision = 1
		_, err = tx.ExecContext(ctx, `INSERT INTO collection_revisions(collection,revision) VALUES(?,?)`, collection, revision)
		return revision, err
	}
	if err != nil {
		return 0, err
	}
	revision++
	_, err = tx.ExecContext(ctx, `UPDATE collection_revisions SET revision=? WHERE collection=?`, revision, collection)
	return revision, err
}

func collectionRevision(ctx context.Context, db *sql.DB, collection string) (uint64, error) {
	var revision uint64
	err := db.QueryRowContext(ctx, `SELECT revision FROM collection_revisions WHERE collection=?`, collection).Scan(&revision)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return revision, err
}

var _ collections.Repository = CollectionRepository{}
var _ = fmt.Sprintf
