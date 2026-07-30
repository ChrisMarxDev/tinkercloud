package client

import (
	"context"
	"encoding/json"
	"net/url"
)

// DataEntry and DataDocument intentionally describe Tinker's bounded data
// primitives, rather than SQLite rows. The control API never exposes a
// database path, schema, or query language.
type DataEntry struct {
	Key       string          `json:"key"`
	Value     json.RawMessage `json:"value"`
	Version   uint64          `json:"version"`
	UpdatedAt string          `json:"updated_at"`
}

type DataKVList struct {
	Entries    []DataEntry `json:"entries"`
	NextCursor string      `json:"next_cursor,omitempty"`
}

type DataDocument struct {
	ID        string          `json:"id"`
	Data      json.RawMessage `json:"data"`
	Version   uint64          `json:"version"`
	CreatedAt string          `json:"created_at"`
	UpdatedAt string          `json:"updated_at"`
}

type DataDocumentList struct {
	Documents  []DataDocument `json:"documents"`
	NextCursor string         `json:"next_cursor,omitempty"`
	Revision   uint64         `json:"revision"`
}

type DataCollectionList struct {
	Collections []string `json:"collections"`
	NextCursor  string   `json:"next_cursor,omitempty"`
}

func dataPath(slug string) string { return "/api/v1/apps/" + url.PathEscape(slug) + "/data" }
func dataKVPath(slug, key string) string {
	return dataPath(slug) + "/kv/" + url.PathEscape(key)
}
func dataDocumentsPath(slug, collection string) string {
	return dataPath(slug) + "/collections/" + url.PathEscape(collection) + "/documents"
}
func dataDocumentPath(slug, collection, id string) string {
	return dataDocumentsPath(slug, collection) + "/" + url.PathEscape(id)
}

func (c Client) ListDataKV(ctx context.Context, slug, prefix, cursor string, limit int) (DataKVList, error) {
	var out DataKVList
	q := url.Values{}
	if prefix != "" {
		q.Set("prefix", prefix)
	}
	if cursor != "" {
		q.Set("cursor", cursor)
	}
	if limit > 0 {
		q.Set("limit", stringInt(limit))
	}
	err := c.dataGet(ctx, dataPath(slug)+"/kv", q, &out)
	return out, err
}

// Do does not take a query parameter; dataGet keeps query construction
// centrally encoded and prevents callers from hand-concatenating values.
func (c Client) dataGet(ctx context.Context, path string, q url.Values, out any) error {
	if len(q) != 0 {
		path += "?" + q.Encode()
	}
	return c.Do(ctx, "GET", path, "", nil, out)
}

func (c Client) GetDataKV(ctx context.Context, slug, key string) (DataEntry, error) {
	var out DataEntry
	err := c.Do(ctx, "GET", dataKVPath(slug, key), "", nil, &out)
	return out, err
}

func (c Client) SetDataKV(ctx context.Context, slug, key string, value json.RawMessage, expected *uint64, idempotencyKey string) (DataEntry, error) {
	var out DataEntry
	in := map[string]any{"value": value}
	if expected != nil {
		in["expected_version"] = *expected
	}
	err := c.Do(ctx, "PUT", dataKVPath(slug, key), idempotencyKey, in, &out)
	return out, err
}

func (c Client) DeleteDataKV(ctx context.Context, slug, key string, expected uint64, idempotencyKey string) error {
	return c.Do(ctx, "DELETE", dataKVPath(slug, key), idempotencyKey, map[string]uint64{"expected_version": expected}, nil)
}

func (c Client) ListDataCollections(ctx context.Context, slug, cursor string, limit int) (DataCollectionList, error) {
	var out DataCollectionList
	q := url.Values{}
	if cursor != "" {
		q.Set("cursor", cursor)
	}
	if limit > 0 {
		q.Set("limit", stringInt(limit))
	}
	err := c.dataGet(ctx, dataPath(slug)+"/collections", q, &out)
	return out, err
}

func (c Client) ListDataDocuments(ctx context.Context, slug, collection, cursor string, limit int) (DataDocumentList, error) {
	var out DataDocumentList
	q := url.Values{}
	if cursor != "" {
		q.Set("cursor", cursor)
	}
	if limit > 0 {
		q.Set("limit", stringInt(limit))
	}
	err := c.dataGet(ctx, dataDocumentsPath(slug, collection), q, &out)
	return out, err
}

func (c Client) GetDataDocument(ctx context.Context, slug, collection, id string) (DataDocument, error) {
	var out DataDocument
	err := c.Do(ctx, "GET", dataDocumentPath(slug, collection, id), "", nil, &out)
	return out, err
}

func (c Client) CreateDataDocument(ctx context.Context, slug, collection string, data json.RawMessage, idempotencyKey string) (DataDocument, error) {
	var out DataDocument
	err := c.Do(ctx, "POST", dataDocumentsPath(slug, collection), idempotencyKey, map[string]any{"data": data}, &out)
	return out, err
}

func (c Client) UpdateDataDocument(ctx context.Context, slug, collection, id string, data json.RawMessage, expected uint64, idempotencyKey string) (DataDocument, error) {
	var out DataDocument
	err := c.Do(ctx, "PUT", dataDocumentPath(slug, collection, id), idempotencyKey, map[string]any{"data": data, "expected_version": expected}, &out)
	return out, err
}

func (c Client) DeleteDataDocument(ctx context.Context, slug, collection, id string, expected uint64, idempotencyKey string) error {
	return c.Do(ctx, "DELETE", dataDocumentPath(slug, collection, id), idempotencyKey, map[string]uint64{"expected_version": expected}, nil)
}

// stringInt avoids accepting a CLI-supplied query fragment as a raw path.
func stringInt(v int) string {
	const digits = "0123456789"
	if v == 0 {
		return "0"
	}
	var out [20]byte
	i := len(out)
	for v > 0 {
		i--
		out[i] = digits[v%10]
		v /= 10
	}
	return string(out[i:])
}
