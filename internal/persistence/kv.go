package persistence

import (
	"context"
	"database/sql"
	"encoding/json"
	"github.com/tinyhost/tiny/internal/kv"
	"github.com/tinyhost/tiny/internal/operations"
	"time"
)

type KVRepository struct {
	Store     *SQLiteStore
	Limits    kv.Limits
	WriteGate operations.WriteGate
}

func (r KVRepository) limits() kv.Limits {
	if r.Limits.KeyBytes == 0 {
		return kv.DefaultLimits()
	}
	return r.Limits
}
func (r KVRepository) Get(ctx context.Context, app, key string) (*kv.Entry, error) {
	var e kv.Entry
	var value, updated string
	err := r.Store.DB.QueryRowContext(ctx, "SELECT value_json,version,updated_at FROM app_kv WHERE app_id=? AND key=?", app, key).Scan(&value, &e.Version, &updated)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	e.Key = key
	e.Value = json.RawMessage(value)
	e.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated)
	return &e, nil
}
func (r KVRepository) Set(ctx context.Context, app, key string, value json.RawMessage, expected *uint64) (kv.Entry, error) {
	var out kv.Entry
	if r.WriteGate != nil {
		if err := r.WriteGate.AllowWrite(ctx, operations.WriteKV); err != nil {
			return out, err
		}
	}
	l := r.limits()
	err := r.Store.Write(ctx, func(tx *sql.Tx) error {
		var old uint64
		var size int
		err := tx.QueryRowContext(ctx, "SELECT version,size_bytes FROM app_kv WHERE app_id=? AND key=?", app, key).Scan(&old, &size)
		missing := err == sql.ErrNoRows
		if err != nil && !missing {
			return err
		}
		if expected != nil && (missing || old != *expected) {
			return kv.ErrVersionConflict
		}
		var count, total int
		if e := tx.QueryRowContext(ctx, "SELECT COUNT(*),COALESCE(SUM(size_bytes),0) FROM app_kv WHERE app_id=?", app).Scan(&count, &total); e != nil {
			return e
		}
		if (missing && count >= l.KeysPerApp) || total-size+len(value) > l.TotalBytesPerApp {
			return kv.ErrQuotaExceeded
		}
		out = kv.Entry{Key: key, Value: append(json.RawMessage(nil), value...), Version: old + 1, UpdatedAt: time.Now().UTC()}
		_, err = tx.ExecContext(ctx, "INSERT INTO app_kv(app_id,key,value_json,version,size_bytes,created_at,updated_at) VALUES(?,?,?,?,?,?,?) ON CONFLICT(app_id,key) DO UPDATE SET value_json=excluded.value_json,version=excluded.version,size_bytes=excluded.size_bytes,updated_at=excluded.updated_at", app, key, string(value), out.Version, len(value), out.UpdatedAt.Format(time.RFC3339Nano), out.UpdatedAt.Format(time.RFC3339Nano))
		return err
	})
	return out, err
}
func (r KVRepository) Delete(ctx context.Context, app, key string, expected *uint64) (bool, kv.Mutation, error) {
	var deleted bool
	var m kv.Mutation
	if r.WriteGate != nil {
		if err := r.WriteGate.AllowWrite(ctx, operations.WriteKV); err != nil {
			return false, m, err
		}
	}
	err := r.Store.Write(ctx, func(tx *sql.Tx) error {
		var ver uint64
		e := tx.QueryRowContext(ctx, "SELECT version FROM app_kv WHERE app_id=? AND key=?", app, key).Scan(&ver)
		if e == sql.ErrNoRows {
			if expected != nil {
				return kv.ErrVersionConflict
			}
			return nil
		}
		if e != nil {
			return e
		}
		if expected != nil && ver != *expected {
			return kv.ErrVersionConflict
		}
		_, e = tx.ExecContext(ctx, "DELETE FROM app_kv WHERE app_id=? AND key=?", app, key)
		deleted = e == nil
		m = kv.Mutation{Key: key, Version: ver + 1, Deleted: true}
		return e
	})
	return deleted, m, err
}
func (r KVRepository) List(ctx context.Context, app, prefix, cursor string, limit int) (kv.ListResult, error) {
	rows, e := r.Store.DB.QueryContext(ctx, "SELECT key,value_json,version,updated_at FROM app_kv WHERE app_id=? AND substr(key,1,?)=? AND key>? ORDER BY key LIMIT ?", app, len(prefix), prefix, cursor, limit+1)
	if e != nil {
		return kv.ListResult{}, e
	}
	defer rows.Close()
	o := kv.ListResult{Entries: []kv.Entry{}}
	for rows.Next() {
		var x kv.Entry
		var v, u string
		if e = rows.Scan(&x.Key, &v, &x.Version, &u); e != nil {
			return o, e
		}
		x.Value = json.RawMessage(v)
		x.UpdatedAt, _ = time.Parse(time.RFC3339Nano, u)
		if len(o.Entries) == limit {
			o.NextCursor = o.Entries[len(o.Entries)-1].Key
			break
		}
		o.Entries = append(o.Entries, x)
	}
	return o, rows.Err()
}
