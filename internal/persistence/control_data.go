package persistence

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/tinyhost/tiny/internal/collections"
	"github.com/tinyhost/tiny/internal/controlapi"
	"github.com/tinyhost/tiny/internal/kv"
)

// The deployer data methods are intentionally small adapters over the same
// per-app repositories used by the SDK. They do not accept SQL, a database
// path, or an immutable app ID from the control request.

func (s ControlService) DataKVList(ctx context.Context, a controlapi.Actor, slug, prefix, cursor string, limit int) (any, error) {
	scope, err := s.dataScope(ctx, a, slug, false)
	if err != nil || s.DataKV == nil || len([]byte(prefix)) > kv.DefaultLimits().KeyBytes || len([]byte(cursor)) > kv.DefaultLimits().KeyBytes || limit < 1 || limit > kv.DefaultLimits().ListLimit {
		if err != nil {
			return nil, err
		}
		return nil, kv.ErrInvalidListLimit
	}
	return s.DataKV.List(ctx, scope.appID, prefix, cursor, limit)
}

func (s ControlService) DataKVGet(ctx context.Context, a controlapi.Actor, slug, key string) (any, error) {
	scope, err := s.dataScope(ctx, a, slug, false)
	if err != nil {
		return nil, err
	}
	if s.DataKV == nil {
		return nil, ErrUnavailable
	}
	if !kv.ValidKey(key) {
		return nil, kv.ErrInvalidKey
	}
	v, err := s.DataKV.Get(ctx, scope.appID, key)
	if err != nil {
		return nil, err
	}
	if v == nil {
		return nil, sql.ErrNoRows
	}
	return v, nil
}

func (s ControlService) DataKVSet(ctx context.Context, a controlapi.Actor, slug, key string, value json.RawMessage, expected *uint64, requestID string) (any, error) {
	scope, err := s.dataScope(ctx, a, slug, true)
	if err != nil {
		return nil, err
	}
	if s.DataKV == nil {
		return nil, ErrUnavailable
	}
	if !kv.ValidKey(key) {
		return nil, kv.ErrInvalidKey
	}
	if !kv.ValidValue(value) {
		return nil, kv.ErrInvalidValue
	}
	if requestID == "" {
		return nil, ErrUnavailable
	}
	digest := dataDigest("set", key, string(value), versionText(expected))
	fresh, completed, err := s.dataMutationIntent(ctx, scope, "data.kv.set", key, requestID, digest)
	if err != nil {
		return nil, err
	}
	if completed {
		current, getErr := s.DataKV.Get(ctx, scope.appID, key)
		if getErr != nil || current == nil || !bytes.Equal(current.Value, value) || (expected != nil && current.Version != *expected+1) {
			return nil, ErrUnavailable
		}
		return current, nil
	}
	if !fresh {
		// Without an expected version, equal current bytes cannot prove that
		// this attempt performed the write rather than observing older state.
		if expected == nil {
			return nil, ErrUnavailable
		}
		current, getErr := s.DataKV.Get(ctx, scope.appID, key)
		if getErr != nil || current == nil || current.Version != *expected+1 || !bytes.Equal(current.Value, value) {
			return nil, ErrUnavailable
		}
		if err = s.dataMutationComplete(ctx, scope, "data.kv.set", requestID); err != nil {
			return nil, err
		}
		if s.DataEvents != nil {
			s.DataEvents.PublishKVChange(ctx, scope.auth, kv.Mutation{Key: key, Value: append(json.RawMessage(nil), value...), Version: current.Version})
		}
		return current, nil
	}
	result, err := s.DataKV.Set(ctx, scope.appID, key, append(json.RawMessage(nil), value...), expected)
	if err != nil {
		return nil, err
	}
	if err = s.dataMutationComplete(ctx, scope, "data.kv.set", requestID); err != nil {
		return nil, err
	}
	if s.DataEvents != nil {
		s.DataEvents.PublishKVChange(ctx, scope.auth, kv.Mutation{Key: key, Value: append(json.RawMessage(nil), value...), Version: result.Version})
	}
	return result, nil
}

func (s ControlService) DataKVDelete(ctx context.Context, a controlapi.Actor, slug, key string, expected *uint64, requestID string) (any, error) {
	scope, err := s.dataScope(ctx, a, slug, true)
	if err != nil {
		return nil, err
	}
	if s.DataKV == nil || requestID == "" {
		return nil, ErrUnavailable
	}
	if !kv.ValidKey(key) {
		return nil, kv.ErrInvalidKey
	}
	if expected == nil {
		return nil, kv.ErrVersionConflict
	}
	fresh, completed, err := s.dataMutationIntent(ctx, scope, "data.kv.delete", key, requestID, dataDigest("delete", key, versionText(expected)))
	if err != nil {
		return nil, err
	}
	if completed {
		return map[string]bool{"deleted": true}, nil
	}
	if !fresh {
		current, getErr := s.DataKV.Get(ctx, scope.appID, key)
		if getErr != nil || current != nil {
			return nil, ErrUnavailable
		}
		if err = s.dataMutationComplete(ctx, scope, "data.kv.delete", requestID); err != nil {
			return nil, err
		}
		if s.DataEvents != nil {
			s.DataEvents.PublishKVChange(ctx, scope.auth, kv.Mutation{Key: key, Version: *expected + 1, Deleted: true})
		}
		return map[string]bool{"deleted": true}, nil
	}
	deleted, _, err := s.DataKV.Delete(ctx, scope.appID, key, expected)
	if err != nil {
		return nil, err
	}
	if !deleted {
		return nil, sql.ErrNoRows
	}
	if err = s.dataMutationComplete(ctx, scope, "data.kv.delete", requestID); err != nil {
		return nil, err
	}
	if s.DataEvents != nil {
		s.DataEvents.PublishKVChange(ctx, scope.auth, kv.Mutation{Key: key, Version: *expected + 1, Deleted: true})
	}
	return map[string]bool{"deleted": true}, nil
}

func (s ControlService) DataCollections(ctx context.Context, a controlapi.Actor, slug, cursor string, limit int) (any, error) {
	scope, err := s.dataScope(ctx, a, slug, false)
	if err != nil {
		return nil, err
	}
	if s.AppDatabases == nil || limit < 1 || limit > collections.DefaultLimits().ListLimit || (cursor != "" && !collections.ValidCollection(cursor)) {
		return nil, collections.ErrInvalidListLimit
	}
	db, release, err := s.AppDatabases.Acquire(ctx, scope.appID)
	if err != nil {
		return nil, err
	}
	defer release()
	rows, err := db.DB.QueryContext(ctx, `SELECT collection FROM (SELECT DISTINCT collection FROM documents) WHERE collection>? ORDER BY collection LIMIT ?`, cursor, limit+1)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []string{}
	next := ""
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		if len(items) == limit {
			next = items[len(items)-1]
			break
		}
		items = append(items, name)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return map[string]any{"collections": items, "next_cursor": next}, nil
}

func (s ControlService) DataDocumentsList(ctx context.Context, a controlapi.Actor, slug, collection, cursor string, limit int) (any, error) {
	scope, err := s.dataScope(ctx, a, slug, false)
	if err != nil {
		return nil, err
	}
	if s.DataDocuments == nil {
		return nil, ErrUnavailable
	}
	if !collections.ValidCollection(collection) {
		return nil, collections.ErrInvalidCollection
	}
	if cursor != "" && !collections.ValidDocumentID(cursor) {
		return nil, collections.ErrInvalidDocumentID
	}
	if limit < 1 || limit > collections.DefaultLimits().ListLimit {
		return nil, collections.ErrInvalidListLimit
	}
	return s.DataDocuments.List(ctx, scope.appID, collection, cursor, limit)
}

func (s ControlService) DataDocumentGet(ctx context.Context, a controlapi.Actor, slug, collection, id string) (any, error) {
	scope, err := s.dataScope(ctx, a, slug, false)
	if err != nil {
		return nil, err
	}
	if s.DataDocuments == nil {
		return nil, ErrUnavailable
	}
	if !collections.ValidCollection(collection) {
		return nil, collections.ErrInvalidCollection
	}
	if !collections.ValidDocumentID(id) {
		return nil, collections.ErrInvalidDocumentID
	}
	v, err := s.DataDocuments.Get(ctx, scope.appID, collection, id)
	if err != nil {
		return nil, err
	}
	if v == nil {
		return nil, sql.ErrNoRows
	}
	return v, nil
}

func (s ControlService) DataDocumentCreate(ctx context.Context, a controlapi.Actor, slug, collection string, data json.RawMessage, requestID string) (any, error) {
	scope, err := s.dataScope(ctx, a, slug, true)
	if err != nil {
		return nil, err
	}
	if s.DataDocuments == nil || requestID == "" {
		return nil, ErrUnavailable
	}
	if !collections.ValidCollection(collection) {
		return nil, collections.ErrInvalidCollection
	}
	if !collections.ValidDocument(data) {
		return nil, collections.ErrInvalidDocument
	}
	id, fresh, completed, err := s.dataCreateIntent(ctx, scope, requestID, dataDigest("create", collection, string(data)))
	if err != nil {
		return nil, err
	}
	if !fresh {
		v, getErr := s.DataDocuments.Get(ctx, scope.appID, collection, id)
		if getErr != nil || v == nil || !bytes.Equal(v.Data, data) {
			return nil, ErrUnavailable
		}
		if !completed {
			if err = s.dataMutationComplete(ctx, scope, "data.document.create", requestID); err != nil {
				return nil, err
			}
			if s.DataEvents != nil {
				s.DataEvents.PublishCollectionChange(ctx, scope.auth, collections.Mutation{Collection: collection, ID: id, Version: v.Version})
			}
		}
		return v, nil
	}
	doc, mutation, err := s.DataDocuments.Create(ctx, scope.appID, collection, collections.Document{ID: id, Data: append(json.RawMessage(nil), data...)})
	if err != nil {
		return nil, err
	}
	if err = s.dataMutationComplete(ctx, scope, "data.document.create", requestID); err != nil {
		return nil, err
	}
	if s.DataEvents != nil {
		s.DataEvents.PublishCollectionChange(ctx, scope.auth, mutation)
	}
	return doc, nil
}

func (s ControlService) DataDocumentUpdate(ctx context.Context, a controlapi.Actor, slug, collection, id string, data json.RawMessage, expected *uint64, requestID string) (any, error) {
	scope, err := s.dataScope(ctx, a, slug, true)
	if err != nil {
		return nil, err
	}
	if s.DataDocuments == nil || requestID == "" {
		return nil, ErrUnavailable
	}
	if !collections.ValidCollection(collection) {
		return nil, collections.ErrInvalidCollection
	}
	if !collections.ValidDocumentID(id) {
		return nil, collections.ErrInvalidDocumentID
	}
	if !collections.ValidDocument(data) {
		return nil, collections.ErrInvalidDocument
	}
	if expected == nil {
		return nil, collections.ErrVersionConflict
	}
	fresh, completed, err := s.dataMutationIntent(ctx, scope, "data.document.update", id, requestID, dataDigest("update", collection, id, string(data), versionText(expected)))
	if err != nil {
		return nil, err
	}
	if completed {
		current, getErr := s.DataDocuments.Get(ctx, scope.appID, collection, id)
		if getErr != nil || current == nil || current.Version != *expected+1 || !bytes.Equal(current.Data, data) {
			return nil, ErrUnavailable
		}
		return current, nil
	}
	if !fresh {
		current, getErr := s.DataDocuments.Get(ctx, scope.appID, collection, id)
		if getErr != nil || current == nil || current.Version != *expected+1 || !bytes.Equal(current.Data, data) {
			return nil, ErrUnavailable
		}
		if err = s.dataMutationComplete(ctx, scope, "data.document.update", requestID); err != nil {
			return nil, err
		}
		if s.DataEvents != nil {
			s.DataEvents.PublishCollectionChange(ctx, scope.auth, collections.Mutation{Collection: collection, ID: id, Version: current.Version})
		}
		return current, nil
	}
	doc, mutation, err := s.DataDocuments.Update(ctx, scope.appID, collection, id, append(json.RawMessage(nil), data...), expected)
	if err != nil {
		return nil, err
	}
	if err = s.dataMutationComplete(ctx, scope, "data.document.update", requestID); err != nil {
		return nil, err
	}
	if s.DataEvents != nil {
		s.DataEvents.PublishCollectionChange(ctx, scope.auth, mutation)
	}
	return doc, nil
}

func (s ControlService) DataDocumentDelete(ctx context.Context, a controlapi.Actor, slug, collection, id string, expected *uint64, requestID string) (any, error) {
	scope, err := s.dataScope(ctx, a, slug, true)
	if err != nil {
		return nil, err
	}
	if s.DataDocuments == nil || requestID == "" {
		return nil, ErrUnavailable
	}
	if !collections.ValidCollection(collection) {
		return nil, collections.ErrInvalidCollection
	}
	if !collections.ValidDocumentID(id) {
		return nil, collections.ErrInvalidDocumentID
	}
	if expected == nil {
		return nil, collections.ErrVersionConflict
	}
	fresh, completed, err := s.dataMutationIntent(ctx, scope, "data.document.delete", id, requestID, dataDigest("delete", collection, id, versionText(expected)))
	if err != nil {
		return nil, err
	}
	if completed {
		return map[string]bool{"deleted": true}, nil
	}
	if !fresh {
		current, getErr := s.DataDocuments.Get(ctx, scope.appID, collection, id)
		if getErr != nil || current != nil {
			return nil, ErrUnavailable
		}
		if err = s.dataMutationComplete(ctx, scope, "data.document.delete", requestID); err != nil {
			return nil, err
		}
		if s.DataEvents != nil {
			s.DataEvents.PublishCollectionChange(ctx, scope.auth, collections.Mutation{Collection: collection, ID: id, Version: *expected + 1, Deleted: true})
		}
		return map[string]bool{"deleted": true}, nil
	}
	deleted, mutation, err := s.DataDocuments.Delete(ctx, scope.appID, collection, id, expected)
	if err != nil {
		return nil, err
	}
	if !deleted {
		return nil, sql.ErrNoRows
	}
	if err = s.dataMutationComplete(ctx, scope, "data.document.delete", requestID); err != nil {
		return nil, err
	}
	if s.DataEvents != nil {
		s.DataEvents.PublishCollectionChange(ctx, scope.auth, mutation)
	}
	return map[string]bool{"deleted": true}, nil
}

// dataCreateIntent reserves the server-assigned document ID before the app DB
// mutation. Retrying an already accepted key returns the original document
// instead of allocating a second ID.
func (s ControlService) dataCreateIntent(ctx context.Context, scope deployerDataScope, requestID, digest string) (id string, fresh, completed bool, resultErr error) {
	if requestID == "" {
		return "", false, false, ErrUnavailable
	}
	resultErr = s.Store.Write(ctx, func(tx *sql.Tx) error {
		var metadata, outcome string
		err := tx.QueryRowContext(ctx, "SELECT target_id,COALESCE(metadata_json,''),outcome FROM audit_events WHERE action='data.document.create' AND actor_id=? AND request_id=?", scope.actorID, requestID).Scan(&id, &metadata, &outcome)
		if err == nil {
			if metadata != digest {
				return ErrUnavailable
			}
			switch outcome {
			case "succeeded":
				completed = true
				return nil
			case "attempted":
				return nil
			}
			return ErrUnavailable
		}
		if err != sql.ErrNoRows {
			return err
		}
		id, err = collections.NewDocumentID()
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, "INSERT INTO audit_events(id,occurred_at,actor_kind,actor_id,app_id,action,outcome,target_kind,target_id,request_id,metadata_json) VALUES(lower(hex(randomblob(16))),datetime('now'),'deployer',?,?,'data.document.create','attempted','app_data',?,?,?)", scope.actorID, scope.appID, id, requestID, digest)
		fresh = err == nil
		return err
	})
	return id, fresh, completed, resultErr
}

func dataDigest(parts ...string) string {
	h := sha256.New()
	for _, part := range parts {
		_, _ = fmt.Fprintf(h, "%d:%s", len(part), part)
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}
func versionText(value *uint64) string {
	if value == nil {
		return ""
	}
	return fmt.Sprintf("%d", *value)
}
