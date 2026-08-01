// Package emulator provides a deliberately local-only Tinkercloud app development server.
// It is not a Tinkercloud gateway and must never be used to expose an app to a network.
package emulator

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/ChrisMarxDev/tinkercloud/internal/compatibility"
	"github.com/coder/websocket"
	_ "modernc.org/sqlite"
)

const apiPrefix = "/_tinker/api/v1"

// The emulator is loopback-only, but it still runs untrusted app JavaScript.
// Keep its websocket accounting bounded so local development cannot turn one
// malformed app into an unbounded memory/socket sink or hide a difference from
// the hosted live protocol.
const (
	emulatorMaxFrame         = 32 << 10
	emulatorMaxConnections   = 100
	emulatorMaxSubscriptions = 32
)

// Config identifies one app directory and its explicitly local state directory.
// Empty StateDir selects a deterministic in-memory database, useful only for tests.
type Config struct {
	AppDir      string
	StateDir    string
	Slug        string
	ViewerID    string
	ViewerEmail string
	Listen      string
}

// Server serves an app and its local development capability shim.
type Server struct {
	cfg     Config
	db      *sql.DB
	mu      sync.Mutex
	clients map[*websocket.Conn]map[string]struct{}
}

func New(ctx context.Context, cfg Config) (*Server, error) {
	if cfg.AppDir == "" {
		return nil, errors.New("emulator app directory is required")
	}
	root, err := filepath.Abs(cfg.AppDir)
	if err != nil {
		return nil, err
	}
	if resolved, e := filepath.EvalSymlinks(root); e == nil {
		root = resolved
	}
	info, err := os.Stat(root)
	if err != nil || !info.IsDir() {
		return nil, fmt.Errorf("emulator app directory %q is not a directory", root)
	}
	cfg.AppDir = root
	if cfg.Slug == "" {
		cfg.Slug = "local-app"
	}
	if cfg.ViewerID == "" {
		cfg.ViewerID = "local-viewer"
	}
	if cfg.ViewerEmail == "" {
		cfg.ViewerEmail = "local@example.test"
	}
	if cfg.Listen == "" {
		cfg.Listen = "127.0.0.1:8787"
	}
	if !loopbackAddress(cfg.Listen) {
		return nil, fmt.Errorf("emulator refuses non-loopback listener %q", cfg.Listen)
	}
	dsn := "file:tinker-emulator?mode=memory&cache=shared"
	if cfg.StateDir != "" {
		state, e := filepath.Abs(cfg.StateDir)
		if e != nil {
			return nil, e
		}
		if e = os.MkdirAll(state, 0700); e != nil {
			return nil, fmt.Errorf("create emulator state directory: %w", e)
		}
		cfg.StateDir = state
		dsn = filepath.Join(state, "tinker-emulator.db")
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	for _, q := range []string{"PRAGMA journal_mode=WAL", "PRAGMA busy_timeout=5000", `CREATE TABLE IF NOT EXISTS kv (key TEXT PRIMARY KEY, value BLOB NOT NULL CHECK(json_valid(value)), version INTEGER NOT NULL, updated_at TEXT NOT NULL)`, `CREATE TABLE IF NOT EXISTS documents (collection TEXT NOT NULL, id TEXT NOT NULL, data BLOB NOT NULL CHECK(json_valid(data)), version INTEGER NOT NULL, created_at TEXT NOT NULL, updated_at TEXT NOT NULL, PRIMARY KEY(collection,id))`, `CREATE TABLE IF NOT EXISTS collection_revisions (collection TEXT PRIMARY KEY, revision INTEGER NOT NULL)`} {
		if _, err = db.ExecContext(ctx, q); err != nil {
			db.Close()
			return nil, fmt.Errorf("initialize emulator database: %w", err)
		}
	}
	return &Server{cfg: cfg, db: db, clients: map[*websocket.Conn]map[string]struct{}{}}, nil
}

func loopbackAddress(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return false
	}
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
func (s *Server) Close() error {
	s.mu.Lock()
	for c := range s.clients {
		_ = c.Close(websocket.StatusGoingAway, "emulator stopping")
	}
	s.clients = map[*websocket.Conn]map[string]struct{}{}
	s.mu.Unlock()
	return s.db.Close()
}
func (s *Server) Handler() http.Handler { return http.HandlerFunc(s.serveHTTP) }
func (s *Server) ListenAndServe() error {
	l, err := net.Listen("tcp", s.cfg.Listen)
	if err != nil {
		return fmt.Errorf("tinker emulator cannot listen on %s (choose --listen with a free loopback port): %w", s.cfg.Listen, err)
	}
	return s.Serve(l)
}
func (s *Server) Serve(l net.Listener) error {
	if l == nil || !loopbackAddress(l.Addr().String()) {
		return errors.New("emulator refuses a non-loopback listener")
	}
	return http.Serve(l, s.Handler())
}

func (s *Server) serveHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if r.URL.Path == "/_tinker/ws/v1" {
		s.websocket(w, r)
		return
	}
	if strings.HasPrefix(r.URL.Path, apiPrefix) {
		s.api(w, r)
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", 405)
		return
	}
	s.static(w, r)
}
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func errJSON(w http.ResponseWriter, status int, code string) {
	writeJSON(w, status, map[string]any{"error": map[string]string{"code": code, "message": "Tinker emulator request failed."}})
}
func (s *Server) api(w http.ResponseWriter, r *http.Request) {
	p := r.URL.EscapedPath()
	switch {
	case r.Method == "GET" && p == apiPrefix+"/me":
		writeJSON(w, 200, map[string]any{"identity": map[string]string{"id": s.cfg.ViewerID, "email": s.cfg.ViewerEmail}, "app": map[string]string{"slug": s.cfg.Slug}})
	case r.Method == "GET" && p == apiPrefix+"/app":
		writeJSON(w, 200, map[string]any{"slug": s.cfg.Slug, "features": map[string]bool{"kv": true, "db": true, "realtime": true}})
	case r.Method == "GET" && p == apiPrefix+"/capabilities":
		writeJSON(w, 200, map[string]any{"capabilities": []map[string]any{{"name": "kv", "version": 1}, {"name": "live", "version": 1}, {"name": "db", "version": 1}}})
	case p == apiPrefix+"/kv":
		s.listKV(w, r)
	case strings.HasPrefix(p, apiPrefix+"/kv/"):
		s.kv(w, r, strings.TrimPrefix(p, apiPrefix+"/kv/"))
	case strings.HasPrefix(p, apiPrefix+"/db/"):
		s.collections(w, r, strings.TrimPrefix(p, apiPrefix+"/db/"))
	default:
		errJSON(w, 404, "not_found")
	}
}
func validName(v string) bool { return v != "" && len(v) <= 256 && !strings.ContainsAny(v, "\x00/\\") }

var emulatorCollectionName = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9_-]{0,62}[a-z0-9])?$`)
var emulatorDocumentID = regexp.MustCompile(`^doc_[A-Za-z0-9_-]{22}$`)

func validCollection(v string) bool { return emulatorCollectionName.MatchString(v) }
func validDocumentID(v string) bool { return emulatorDocumentID.MatchString(v) }
func decodeBody(r *http.Request, dst any) bool {
	if r.Body == nil {
		return false
	}
	d := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 128<<10))
	d.DisallowUnknownFields()
	if d.Decode(dst) != nil {
		return false
	}
	// A body such as `{...}{...}` must not be accepted as one valid request.
	// Production's dispatcher has the same one-value boundary.
	return d.Decode(&struct{}{}) == io.EOF
}
func (s *Server) kv(w http.ResponseWriter, r *http.Request, key string) {
	k, e := url.PathUnescape(key)
	if e != nil || !validName(k) {
		errJSON(w, 400, "validation_failed")
		return
	}
	switch r.Method {
	case "GET":
		if r.URL.RawQuery != "" {
			errJSON(w, 400, "validation_failed")
			return
		}
		var value json.RawMessage
		var version int
		var at string
		e = s.db.QueryRowContext(r.Context(), "SELECT value,version,updated_at FROM kv WHERE key=?", k).Scan(&value, &version, &at)
		if errors.Is(e, sql.ErrNoRows) {
			errJSON(w, 404, "not_found")
			return
		}
		if e != nil {
			errJSON(w, 503, "temporarily_unavailable")
			return
		}
		writeJSON(w, 200, map[string]any{"key": k, "value": value, "version": version, "updated_at": at})
	case "PUT":
		var in struct {
			Value    json.RawMessage `json:"value"`
			Expected *int            `json:"expected_version"`
		}
		if !decodeBody(r, &in) || !json.Valid(in.Value) {
			errJSON(w, 400, "validation_failed")
			return
		}
		tx, e := s.db.BeginTx(r.Context(), nil)
		if e != nil {
			errJSON(w, 503, "temporarily_unavailable")
			return
		}
		defer tx.Rollback()
		var old int
		e = tx.QueryRowContext(r.Context(), "SELECT version FROM kv WHERE key=?", k).Scan(&old)
		missing := errors.Is(e, sql.ErrNoRows)
		if e != nil && !missing {
			errJSON(w, 503, "temporarily_unavailable")
			return
		}
		if in.Expected != nil && (missing || old != *in.Expected) {
			errJSON(w, 409, "version_conflict")
			return
		}
		ver := old + 1
		at := time.Now().UTC().Format(time.RFC3339Nano)
		_, e = tx.ExecContext(r.Context(), "INSERT INTO kv(key,value,version,updated_at) VALUES(?,?,?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value,version=excluded.version,updated_at=excluded.updated_at", k, in.Value, ver, at)
		if e != nil {
			errJSON(w, 503, "temporarily_unavailable")
			return
		}
		if e = tx.Commit(); e != nil {
			errJSON(w, 503, "temporarily_unavailable")
			return
		}
		writeJSON(w, 200, map[string]any{"key": k, "value": in.Value, "version": ver, "updated_at": at})
		s.broadcast(map[string]any{"v": 1, "type": "kv.changed", "key": k, "version": ver})
	case "DELETE":
		var in struct {
			Expected *int `json:"expected_version"`
		}
		if !decodeBody(r, &in) {
			errJSON(w, 400, "validation_failed")
			return
		}
		tx, e := s.db.BeginTx(r.Context(), nil)
		if e != nil {
			errJSON(w, 503, "temporarily_unavailable")
			return
		}
		defer tx.Rollback()
		var old int
		e = tx.QueryRowContext(r.Context(), "SELECT version FROM kv WHERE key=?", k).Scan(&old)
		missing := errors.Is(e, sql.ErrNoRows)
		if e != nil && !missing {
			errJSON(w, 503, "temporarily_unavailable")
			return
		}
		if in.Expected != nil && (missing || old != *in.Expected) {
			errJSON(w, 409, "version_conflict")
			return
		}
		result, e := tx.ExecContext(r.Context(), "DELETE FROM kv WHERE key=?", k)
		if e != nil {
			errJSON(w, 503, "temporarily_unavailable")
			return
		}
		n, _ := result.RowsAffected()
		if e = tx.Commit(); e != nil {
			errJSON(w, 503, "temporarily_unavailable")
			return
		}
		writeJSON(w, 200, map[string]bool{"deleted": n > 0})
		if n > 0 {
			s.broadcast(map[string]any{"v": 1, "type": "kv.changed", "key": k, "version": old + 1, "deleted": true})
		}
	default:
		errJSON(w, 405, "validation_failed")
	}
}
func (s *Server) listKV(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		errJSON(w, 405, "validation_failed")
		return
	}
	prefix := r.URL.Query().Get("prefix")
	rows, e := s.db.QueryContext(r.Context(), "SELECT key,value,version,updated_at FROM kv WHERE key >= ? AND key < ? ORDER BY key LIMIT 100", prefix, prefix+"\uffff")
	if e != nil {
		errJSON(w, 503, "temporarily_unavailable")
		return
	}
	defer rows.Close()
	entries := []any{}
	for rows.Next() {
		var k, at string
		var v json.RawMessage
		var ver int
		if rows.Scan(&k, &v, &ver, &at) == nil {
			entries = append(entries, map[string]any{"key": k, "value": v, "version": ver, "updated_at": at})
		}
	}
	writeJSON(w, 200, map[string]any{"entries": entries})
}

func (s *Server) collections(w http.ResponseWriter, r *http.Request, rest string) {
	parts := strings.Split(rest, "/")
	if len(parts) < 1 || len(parts) > 2 {
		errJSON(w, 400, "validation_failed")
		return
	}
	collection, e := url.PathUnescape(parts[0])
	if e != nil || !validCollection(collection) {
		errJSON(w, 400, "validation_failed")
		return
	}
	if len(parts) == 1 {
		if r.Method == "POST" {
			s.createDocument(w, r, collection)
		} else {
			s.listDocuments(w, r, collection)
		}
		return
	}
	id, e := url.PathUnescape(parts[1])
	if e != nil || !validDocumentID(id) {
		errJSON(w, 400, "validation_failed")
		return
	}
	switch r.Method {
	case "GET":
		var data json.RawMessage
		var version int
		var created, at string
		e = s.db.QueryRowContext(r.Context(), "SELECT data,version,created_at,updated_at FROM documents WHERE collection=? AND id=?", collection, id).Scan(&data, &version, &created, &at)
		if errors.Is(e, sql.ErrNoRows) {
			errJSON(w, 404, "not_found")
			return
		}
		if e != nil {
			errJSON(w, 503, "temporarily_unavailable")
			return
		}
		writeJSON(w, 200, map[string]any{"id": id, "data": data, "version": version, "created_at": created, "updated_at": at})
	case "PUT":
		var in struct {
			Data     json.RawMessage `json:"data"`
			Expected *int            `json:"expected_version"`
		}
		if !decodeBody(r, &in) || !jsonObject(in.Data) {
			errJSON(w, 400, "validation_failed")
			return
		}
		tx, e := s.db.BeginTx(r.Context(), nil)
		if e != nil {
			errJSON(w, 503, "temporarily_unavailable")
			return
		}
		defer tx.Rollback()
		var old int
		var created string
		e = tx.QueryRowContext(r.Context(), "SELECT version,created_at FROM documents WHERE collection=? AND id=?", collection, id).Scan(&old, &created)
		if errors.Is(e, sql.ErrNoRows) || (e == nil && in.Expected != nil && old != *in.Expected) {
			errJSON(w, 409, "version_conflict")
			return
		}
		if e != nil {
			errJSON(w, 503, "temporarily_unavailable")
			return
		}
		ver := old + 1
		at := time.Now().UTC().Format(time.RFC3339Nano)
		_, e = tx.ExecContext(r.Context(), "UPDATE documents SET data=?,version=?,updated_at=? WHERE collection=? AND id=?", in.Data, ver, at, collection, id)
		if e != nil {
			errJSON(w, 503, "temporarily_unavailable")
			return
		}
		revision, e := incrementRevisionTx(r.Context(), tx, collection)
		if e != nil {
			errJSON(w, 503, "temporarily_unavailable")
			return
		}
		if e = tx.Commit(); e != nil {
			errJSON(w, 503, "temporarily_unavailable")
			return
		}
		writeJSON(w, 200, map[string]any{"id": id, "data": in.Data, "version": ver, "created_at": created, "updated_at": at})
		s.broadcast(map[string]any{"v": 1, "type": "collection.changed", "collection": collection, "id": id, "version": ver, "revision": revision})
	case "DELETE":
		var in struct {
			Expected *int `json:"expected_version"`
		}
		if !decodeBody(r, &in) {
			errJSON(w, 400, "validation_failed")
			return
		}
		tx, e := s.db.BeginTx(r.Context(), nil)
		if e != nil {
			errJSON(w, 503, "temporarily_unavailable")
			return
		}
		defer tx.Rollback()
		var oldVersion int
		found := tx.QueryRowContext(r.Context(), "SELECT version FROM documents WHERE collection=? AND id=?", collection, id).Scan(&oldVersion)
		missing := errors.Is(found, sql.ErrNoRows)
		if found != nil && !missing {
			errJSON(w, 503, "temporarily_unavailable")
			return
		}
		if in.Expected != nil && (missing || oldVersion != *in.Expected) {
			errJSON(w, 409, "version_conflict")
			return
		}
		if missing {
			if e = tx.Commit(); e != nil {
				errJSON(w, 503, "temporarily_unavailable")
				return
			}
			writeJSON(w, 200, map[string]bool{"deleted": false})
			return
		}
		result, e := tx.ExecContext(r.Context(), "DELETE FROM documents WHERE collection=? AND id=?", collection, id)
		if e != nil {
			errJSON(w, 503, "temporarily_unavailable")
			return
		}
		n, _ := result.RowsAffected()
		revision, e := incrementRevisionTx(r.Context(), tx, collection)
		if e != nil {
			errJSON(w, 503, "temporarily_unavailable")
			return
		}
		if e = tx.Commit(); e != nil {
			errJSON(w, 503, "temporarily_unavailable")
			return
		}
		writeJSON(w, 200, map[string]bool{"deleted": n > 0})
		s.broadcast(map[string]any{"v": 1, "type": "collection.changed", "collection": collection, "id": id, "version": oldVersion + 1, "deleted": true, "revision": revision})
	default:
		errJSON(w, 405, "validation_failed")
	}
}
func jsonObject(raw json.RawMessage) bool {
	var v map[string]json.RawMessage
	return len(raw) > 0 && json.Unmarshal(raw, &v) == nil && v != nil
}
func (s *Server) createDocument(w http.ResponseWriter, r *http.Request, c string) {
	var in struct {
		Data json.RawMessage `json:"data"`
	}
	if !decodeBody(r, &in) || !jsonObject(in.Data) {
		errJSON(w, 400, "validation_failed")
		return
	}
	b := make([]byte, 16)
	if _, e := rand.Read(b); e != nil {
		errJSON(w, 503, "temporarily_unavailable")
		return
	}
	id := "doc_" + base64.RawURLEncoding.EncodeToString(b)
	at := time.Now().UTC().Format(time.RFC3339Nano)
	tx, e := s.db.BeginTx(r.Context(), nil)
	if e != nil {
		errJSON(w, 503, "temporarily_unavailable")
		return
	}
	defer tx.Rollback()
	_, e = tx.ExecContext(r.Context(), "INSERT INTO documents(collection,id,data,version,created_at,updated_at) VALUES(?,?,?,?,?,?)", c, id, in.Data, 1, at, at)
	if e != nil {
		errJSON(w, 503, "temporarily_unavailable")
		return
	}
	revision, e := incrementRevisionTx(r.Context(), tx, c)
	if e != nil {
		errJSON(w, 503, "temporarily_unavailable")
		return
	}
	if e = tx.Commit(); e != nil {
		errJSON(w, 503, "temporarily_unavailable")
		return
	}
	writeJSON(w, 201, map[string]any{"id": id, "data": in.Data, "version": 1, "created_at": at, "updated_at": at})
	s.broadcast(map[string]any{"v": 1, "type": "collection.changed", "collection": c, "id": id, "version": 1, "revision": revision})
}
func incrementRevisionTx(ctx context.Context, tx *sql.Tx, c string) (int, error) {
	var revision int
	err := tx.QueryRowContext(ctx, "SELECT revision FROM collection_revisions WHERE collection=?", c).Scan(&revision)
	if errors.Is(err, sql.ErrNoRows) {
		_, err = tx.ExecContext(ctx, "INSERT INTO collection_revisions(collection,revision) VALUES(?,1)", c)
		return 1, err
	}
	if err != nil {
		return 0, err
	}
	revision++
	_, err = tx.ExecContext(ctx, "UPDATE collection_revisions SET revision=? WHERE collection=?", revision, c)
	return revision, err
}
func (s *Server) listDocuments(w http.ResponseWriter, r *http.Request, c string) {
	if r.Method != "GET" {
		errJSON(w, 405, "validation_failed")
		return
	}
	q := r.URL.Query()
	for key, values := range q {
		if (key != "cursor" && key != "limit" && key != "snapshot") || len(values) != 1 {
			errJSON(w, 400, "validation_failed")
			return
		}
	}
	if q.Get("snapshot") != "" && q.Get("snapshot") != "1" {
		errJSON(w, 400, "validation_failed")
		return
	}
	if q.Get("snapshot") == "1" && (q.Get("cursor") != "" || q.Get("limit") != "") {
		errJSON(w, 400, "validation_failed")
		return
	}
	limit := 100
	if q.Get("snapshot") == "1" {
		limit = 1000
	}
	if q.Get("limit") != "" {
		if _, e := fmt.Sscan(q.Get("limit"), &limit); e != nil || limit < 1 || limit > 100 {
			errJSON(w, 400, "validation_failed")
			return
		}
	}
	cursor := q.Get("cursor")
	rows, e := s.db.QueryContext(r.Context(), "SELECT id,data,version,created_at,updated_at FROM documents WHERE collection=? AND id>? ORDER BY id LIMIT ?", c, cursor, limit+1)
	if e != nil {
		errJSON(w, 503, "temporarily_unavailable")
		return
	}
	defer rows.Close()
	documents := []any{}
	for rows.Next() {
		var id, created, at string
		var data json.RawMessage
		var version int
		if rows.Scan(&id, &data, &version, &created, &at) == nil {
			documents = append(documents, map[string]any{"id": id, "data": data, "version": version, "created_at": created, "updated_at": at})
		}
	}
	var revision int
	_ = s.db.QueryRowContext(r.Context(), "SELECT revision FROM collection_revisions WHERE collection=?", c).Scan(&revision)
	out := map[string]any{"documents": documents, "revision": revision}
	if len(documents) > limit {
		out["documents"] = documents[:limit]
		out["next_cursor"] = documents[limit-1].(map[string]any)["id"]
	}
	writeJSON(w, 200, out)
}

func (s *Server) static(w http.ResponseWriter, r *http.Request) {
	path := filepath.Clean("/" + r.URL.Path)
	if strings.Contains(path, "\\") {
		http.NotFound(w, r)
		return
	}
	rel := strings.TrimPrefix(path, "/")
	target := filepath.Join(s.cfg.AppDir, rel)
	// filepath.Rel is the final traversal guard even on unusual path inputs.
	if relative, e := filepath.Rel(s.cfg.AppDir, target); e != nil || strings.HasPrefix(relative, "..") || filepath.IsAbs(relative) {
		http.NotFound(w, r)
		return
	}
	// Resolve a present file before serving it: a symlink inside the app
	// directory must not turn the static server into a local file reader.
	if resolved, e := filepath.EvalSymlinks(target); e == nil {
		if relative, e := filepath.Rel(s.cfg.AppDir, resolved); e != nil || strings.HasPrefix(relative, "..") || filepath.IsAbs(relative) {
			http.NotFound(w, r)
			return
		}
		target = resolved
	}
	if info, e := os.Stat(target); e == nil && !info.IsDir() {
		http.ServeFile(w, r, target)
		return
	}
	index := filepath.Join(s.cfg.AppDir, "index.html")
	if _, e := os.Stat(index); e == nil {
		http.ServeFile(w, r, index)
		return
	}
	http.NotFound(w, r)
}

func (s *Server) websocket(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" || !loopbackOrigin(r.Header.Get("Origin")) {
		http.Error(w, "not authorized", http.StatusForbidden)
		return
	}
	protocol, ok := emulatorSubprotocol(r.Header.Get("Sec-WebSocket-Protocol"))
	if !ok {
		http.Error(w, "SDK version incompatible", http.StatusUpgradeRequired)
		return
	}
	options := &websocket.AcceptOptions{InsecureSkipVerify: true}
	if protocol != "" {
		options.Subprotocols = []string{protocol}
	}
	c, e := websocket.Accept(w, r, options)
	if e != nil {
		return
	}
	defer c.CloseNow()
	c.SetReadLimit(emulatorMaxFrame)
	s.mu.Lock()
	if len(s.clients) >= emulatorMaxConnections {
		s.mu.Unlock()
		_ = c.Close(websocket.StatusPolicyViolation, "connection limit")
		return
	}
	s.clients[c] = map[string]struct{}{}
	s.mu.Unlock()
	defer func() { s.mu.Lock(); delete(s.clients, c); s.mu.Unlock() }()
	for {
		_, raw, e := c.Read(r.Context())
		if e != nil {
			return
		}
		var frame struct {
			V                                 int `json:"v"`
			Type, Channel, Prefix, Collection string
			Event                             string
			Payload                           json.RawMessage
		}
		if json.Unmarshal(raw, &frame) != nil || frame.V != 1 {
			_ = c.Close(websocket.StatusPolicyViolation, "invalid frame")
			return
		}
		s.mu.Lock()
		subs, present := s.clients[c]
		if !present {
			s.mu.Unlock()
			return
		}
		switch frame.Type {
		case "subscribe":
			if validName(frame.Channel) {
				if !addSubscription(subs, "channel:"+frame.Channel) {
					_ = c.Close(websocket.StatusPolicyViolation, "subscription limit")
					s.mu.Unlock()
					return
				}
			}
		case "unsubscribe":
			delete(subs, "channel:"+frame.Channel)
		case "subscribe_kv":
			if len([]byte(frame.Prefix)) <= 256 && !strings.ContainsRune(frame.Prefix, '\x00') {
				if !addSubscription(subs, "kv:"+frame.Prefix) {
					_ = c.Close(websocket.StatusPolicyViolation, "subscription limit")
					s.mu.Unlock()
					return
				}
			}
		case "unsubscribe_kv":
			if len(frame.Prefix) <= 256 {
				delete(subs, "kv:"+frame.Prefix)
			}
		case "subscribe_collection":
			if validCollection(frame.Collection) {
				if !addSubscription(subs, "collection:"+frame.Collection) {
					_ = c.Close(websocket.StatusPolicyViolation, "subscription limit")
					s.mu.Unlock()
					return
				}
			} else {
				_ = c.Close(websocket.StatusPolicyViolation, "invalid frame")
				s.mu.Unlock()
				return
			}
		case "unsubscribe_collection":
			if validCollection(frame.Collection) {
				delete(subs, "collection:"+frame.Collection)
			} else {
				_ = c.Close(websocket.StatusPolicyViolation, "invalid frame")
				s.mu.Unlock()
				return
			}
		case "publish":
			if validName(frame.Channel) && validName(frame.Event) && json.Valid(frame.Payload) {
				s.mu.Unlock()
				s.broadcast(map[string]any{"v": 1, "type": "event", "channel": frame.Channel, "event": frame.Event, "payload": frame.Payload})
				continue
			}
		default:
			_ = c.Close(websocket.StatusPolicyViolation, "invalid frame")
			s.mu.Unlock()
			return
		}
		s.mu.Unlock()
	}
}

func addSubscription(subscriptions map[string]struct{}, key string) bool {
	if _, exists := subscriptions[key]; exists {
		return true
	}
	if len(subscriptions) >= emulatorMaxSubscriptions {
		return false
	}
	subscriptions[key] = struct{}{}
	return true
}

// emulatorSubprotocol intentionally mirrors the hosted current-SDK boundary.
// The loopback emulator is not a compatibility escape hatch: a browser client
// still has to speak the current Tinkercloud app API before it can receive live data.
func emulatorSubprotocol(raw string) (string, bool) {
	if raw == "" {
		return "", true
	}
	if strings.Contains(raw, ",") {
		return "", false
	}
	const prefix = "tinker.sdk."
	const separator = ".api."
	if !strings.HasPrefix(raw, prefix) {
		return "", false
	}
	parts := strings.Split(strings.TrimPrefix(raw, prefix), separator)
	if len(parts) != 2 || parts[1] != compatibility.AppAPIVersion || !compatibility.Current("0.1.0").AppAPI.Client.Contains(parts[0]) {
		return "", false
	}
	return raw, true
}

func loopbackOrigin(raw string) bool {
	if raw == "" {
		return true
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" {
		return false
	}
	if strings.EqualFold(u.Hostname(), "localhost") {
		return true
	}
	return net.ParseIP(u.Hostname()) != nil && net.ParseIP(u.Hostname()).IsLoopback()
}
func (s *Server) broadcast(message map[string]any) {
	raw, e := json.Marshal(message)
	if e != nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for c, subs := range s.clients {
		deliver := false
		if typ, _ := message["type"].(string); typ == "event" {
			if channel, _ := message["channel"].(string); channel != "" {
				_, deliver = subs["channel:"+channel]
			}
		} else if typ == "kv.changed" {
			key, _ := message["key"].(string)
			for sub := range subs {
				if strings.HasPrefix(sub, "kv:") && strings.HasPrefix(key, strings.TrimPrefix(sub, "kv:")) {
					deliver = true
					break
				}
			}
		} else if typ == "collection.changed" {
			if collection, _ := message["collection"].(string); collection != "" {
				_, deliver = subs["collection:"+collection]
			}
		}
		if deliver {
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			e = c.Write(ctx, websocket.MessageText, raw)
			cancel()
			if e != nil {
				_ = c.Close(websocket.StatusGoingAway, "slow consumer")
				delete(s.clients, c)
			}
		}
	}
}

// AppDir returns the normalized static application directory for diagnostics.
func (s *Server) AppDir() string   { return s.cfg.AppDir }
func (s *Server) StateDir() string { return s.cfg.StateDir }
func (s *Server) Config() Config   { return s.cfg }
