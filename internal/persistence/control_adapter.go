package persistence

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/tinyhost/tiny/internal/controlapi"
	"github.com/tinyhost/tiny/internal/deployments"
	"github.com/tinyhost/tiny/internal/identity"
	"github.com/tinyhost/tiny/internal/operations"
	"github.com/tinyhost/tiny/internal/releases"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

type ControlAuthenticator struct {
	Store *SQLiteStore
	Clock func() time.Time
}

func (a ControlAuthenticator) AuthenticateControl(ctx context.Context, r *http.Request) (controlapi.Actor, error) {
	h := r.Header.Get("Authorization")
	if !strings.HasPrefix(h, "Bearer ") || strings.Count(h, " ") != 1 {
		return controlapi.Actor{}, ErrToken
	}
	scope, slug, ok := classifyControlRoute(r.Method, r.URL.Path)
	if !ok || a.Store == nil {
		return controlapi.Actor{}, ErrToken
	}
	appID := ""
	if slug != "" {
		statuses := "'active'"
		// A delete retry must still be able to authenticate an app-scoped token
		// after the target became deleted. The service below permits only the
		// matching idempotent deletion outcome for that state.
		if scope == "app:delete" {
			statuses = "'active','suspended','deleting','deleted'"
		}
		if err := a.Store.DB.QueryRowContext(ctx, "SELECT id FROM applications WHERE slug=? AND status IN ("+statuses+")", slug).Scan(&appID); err != nil {
			return controlapi.Actor{}, ErrToken
		}
	}
	now := time.Now()
	if a.Clock != nil {
		now = a.Clock()
	}
	return a.Store.AuthenticateToken(ctx, strings.TrimPrefix(h, "Bearer "), scope, appID, now)
}

// AuthenticatePlatform deliberately accepts only the host-only control-session
// cookie. Browser traffic cannot reuse a bearer token, while CLI traffic cannot
// turn into an ambient browser session.
func (a ControlAuthenticator) AuthenticatePlatform(ctx context.Context, r *http.Request) (controlapi.Actor, error) {
	if a.Store == nil {
		return controlapi.Actor{}, ErrToken
	}
	c, err := r.Cookie(controlapi.ControlCookieName)
	if err != nil || c.Value == "" {
		return controlapi.Actor{}, ErrToken
	}
	now := time.Now()
	if a.Clock != nil {
		now = a.Clock()
	}
	return a.Store.AuthenticateControlSession(ctx, c.Value, now)
}
func routeScope(m, p string) string {
	s, _, ok := classifyControlRoute(m, p)
	if !ok {
		return ""
	}
	return s
}
func classifyControlRoute(m, p string) (string, string, bool) {
	if strings.Contains(p, "%2f") || strings.Contains(p, "%2F") {
		return "", "", false
	}
	if m == "GET" && (p == "/api/v1/whoami" || p == "/api/v1/apps") {
		return "app:read", "", true
	}
	if m == "POST" && p == "/api/v1/apps" {
		return "app:create", "", true
	}
	const pre = "/api/v1/apps/"
	if !strings.HasPrefix(p, pre) {
		return "", "", false
	}
	x := strings.TrimPrefix(p, pre)
	parts := strings.Split(x, "/")
	if len(parts) < 1 || !releases.ValidSlug(parts[0]) {
		return "", "", false
	}
	slug := parts[0]
	if len(parts) == 2 && parts[1] == "tokens" {
		if m == "GET" {
			return "app:read", slug, true
		}
		if m == "POST" {
			return "token:create", slug, true
		}
	}
	if len(parts) == 3 && parts[1] == "tokens" && m == "DELETE" && safeTokenRouteID(parts[2]) {
		return "token:revoke", slug, true
	}
	if len(parts) == 2 && parts[1] == "access" {
		if m == "GET" {
			return "access:read", slug, true
		}
		if m == "PUT" {
			return "access:write", slug, true
		}
	}
	if len(parts) == 2 && parts[1] == "releases" && m == "GET" {
		return "app:read", slug, true
	}
	if len(parts) == 1 && m == "DELETE" {
		return "app:delete", slug, true
	}
	if len(parts) == 2 && parts[1] == "deployments" && m == "POST" {
		return "deploy:create", slug, true
	}
	if len(parts) == 4 && parts[1] == "deployments" && parts[2] != "" && parts[3] == "activate" && m == "POST" {
		return "deploy:activate", slug, true
	}
	if len(parts) == 2 && parts[1] == "rollback" && m == "POST" {
		return "deploy:activate", slug, true
	}
	return "", "", false
}
func safeTokenRouteID(id string) bool {
	if id == "" || len(id) > 128 {
		return false
	}
	for _, c := range id {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_' || c == '-') {
			return false
		}
	}
	return true
}

type ControlService struct {
	Store       *SQLiteStore
	Live        interface{ Revoke(string, string) }
	Deployments *deployments.Service
	AppSuffix   string
	// WriteGate protects only resource-growing mutations. It is deliberately
	// not consulted by access changes, token revocation, or app suspension.
	WriteGate       operations.WriteGate
	AppsPerDeployer int
}

// SetDeployerStatus is the browser-safe counterpart to the root bootstrap
// command. It retains the same single-writer transaction, but only a currently
// active operator record can invoke it remotely. A suspended or revoked
// deployer loses every control credential in that same transaction.
func (s ControlService) SetDeployerStatus(ctx context.Context, a controlapi.Actor, email, status, key string) error {
	if s.Store == nil || !a.Active || a.Role != "operator" || key == "" || (status != "active" && status != "suspended" && status != "revoked") {
		return ErrUnavailable
	}
	normalized, err := identity.Normalize(email)
	if err != nil {
		return ErrUnavailable
	}
	return s.Store.Write(ctx, func(tx *sql.Tx) error {
		var priorTarget, priorAction string
		e := tx.QueryRowContext(ctx, "SELECT target_id,action FROM audit_events WHERE actor_id=? AND request_id=?", a.ID, key).Scan(&priorTarget, &priorAction)
		if e == nil {
			if priorTarget == normalized && priorAction == "deployer."+status {
				return nil
			}
			return ErrUnavailable
		}
		if e != sql.ErrNoRows {
			return e
		}
		var operatorRole, operatorStatus string
		if e = tx.QueryRowContext(ctx, "SELECT role,status FROM users WHERE id=?", a.ID).Scan(&operatorRole, &operatorStatus); e != nil || operatorRole != "operator" || operatorStatus != "active" {
			return ErrUnavailable
		}
		var id, role string
		e = tx.QueryRowContext(ctx, "SELECT id,role FROM users WHERE normalized_email=?", normalized).Scan(&id, &role)
		if e == sql.ErrNoRows {
			if status != "active" {
				return ErrUnavailable
			}
			id = "usr_" + fmt.Sprintf("%x", sha256.Sum256([]byte(normalized)))[:16]
			if _, e = tx.ExecContext(ctx, "INSERT INTO users(id,normalized_email,role,status,created_at) VALUES(?,?, 'deployer','active',datetime('now'))", id, normalized); e != nil {
				return e
			}
		} else if e != nil || role != "deployer" {
			return ErrUnavailable
		} else if _, e = tx.ExecContext(ctx, "UPDATE users SET status=? WHERE id=?", status, id); e != nil {
			return e
		}
		if status != "active" {
			if _, e = tx.ExecContext(ctx, "UPDATE api_tokens SET revoked_at=datetime('now') WHERE user_id=? AND revoked_at IS NULL", id); e != nil {
				return e
			}
		}
		_, e = tx.ExecContext(ctx, "INSERT INTO audit_events(id,occurred_at,actor_kind,actor_id,action,outcome,target_kind,target_id,request_id,metadata_json) VALUES(lower(hex(randomblob(16))),datetime('now'),'user',?,?,'success','user',?,?,?)", a.ID, "deployer."+status, normalized, key, normalized)
		return e
	})
}

// SetAppStatus atomically makes a suspension effective before acknowledging it.
// All app tokens, viewer sessions and live connections are revoked; resuming
// does not resurrect credentials, so a new login/token is required.
func (s ControlService) SetAppStatus(ctx context.Context, a controlapi.Actor, slug, status, key string) error {
	if s.Store == nil || !a.Active || key == "" || (status != "active" && status != "suspended") {
		return ErrUnavailable
	}
	var appID string
	fresh := false
	err := s.Store.Write(ctx, func(tx *sql.Tx) error {
		var target, action string
		e := tx.QueryRowContext(ctx, "SELECT target_id,action FROM audit_events WHERE actor_id=? AND request_id=?", a.ID, key).Scan(&target, &action)
		if e == nil {
			if target == slug && action == "app."+status {
				return nil
			}
			return ErrUnavailable
		}
		if e != sql.ErrNoRows {
			return e
		}
		q := "SELECT id,status FROM applications WHERE slug=? AND owner_user_id=?"
		args := []any{slug, a.ID}
		if a.Role == "operator" {
			q, args = "SELECT id,status FROM applications WHERE slug=?", []any{slug}
		}
		var current string
		if e = tx.QueryRowContext(ctx, q, args...).Scan(&appID, &current); e != nil {
			return ErrUnavailable
		}
		if current == status {
			return ErrUnavailable
		}
		if current != "active" && current != "suspended" {
			return ErrUnavailable
		}
		if _, e = tx.ExecContext(ctx, "UPDATE applications SET status=?,updated_at=datetime('now') WHERE id=?", status, appID); e != nil {
			return e
		}
		if status == "suspended" {
			if _, e = tx.ExecContext(ctx, "UPDATE api_tokens SET revoked_at=datetime('now') WHERE app_id=? AND revoked_at IS NULL", appID); e != nil {
				return e
			}
			if _, e = tx.ExecContext(ctx, "UPDATE sessions SET revoked_at=datetime('now') WHERE app_id=? AND scope='app' AND revoked_at IS NULL", appID); e != nil {
				return e
			}
		}
		if _, e = tx.ExecContext(ctx, "INSERT INTO audit_events(id,occurred_at,actor_kind,actor_id,app_id,action,outcome,target_kind,target_id,request_id) VALUES(lower(hex(randomblob(16))),datetime('now'),'user',?,?,?,'success','app',?,?)", a.ID, appID, "app."+status, slug, key); e != nil {
			return e
		}
		fresh = true
		return nil
	})
	if err == nil && fresh && status == "suspended" && s.Live != nil {
		s.Live.Revoke(appID, "")
	}
	return err
}

func (s ControlService) Whoami(_ context.Context, a controlapi.Actor) any {
	return map[string]string{"id": a.ID, "email": a.Email}
}
func (s ControlService) Apps(ctx context.Context, a controlapi.Actor) any {
	rows, e := s.Store.DB.QueryContext(ctx, "SELECT slug,status FROM applications WHERE owner_user_id=? ORDER BY slug", a.ID)
	if e != nil {
		return []any{}
	}
	defer rows.Close()
	out := []map[string]string{}
	for rows.Next() {
		var slug, status string
		if rows.Scan(&slug, &status) == nil {
			out = append(out, map[string]string{"slug": slug, "status": status})
		}
	}
	return out
}

// Dashboard is deliberately a bounded metadata-only read model for the
// platform UI. It uses server-derived ownership and never returns raw token
// values, hashes, release paths, configuration secrets, or provider details.
func (s ControlService) Dashboard(ctx context.Context, a controlapi.Actor) (controlapi.DashboardView, error) {
	if !a.Active || s.Store == nil {
		return controlapi.DashboardView{}, ErrUnavailable
	}
	v := controlapi.DashboardView{Health: []controlapi.DashboardHealth{{Name: "host diagnostics", State: "local", Detail: "Run tinyhost doctor on the VPS for database, disk, DNS, TLS, and email diagnostics."}}}
	query, args := "SELECT a.id,a.owner_user_id,a.slug,a.status,a.policy_revision,p.mode,a.current_deployment_id FROM applications a LEFT JOIN access_policies p ON p.app_id=a.id AND p.revision=a.policy_revision WHERE a.owner_user_id=? ORDER BY a.slug LIMIT 100", []any{a.ID}
	if a.Role == "operator" {
		query, args = "SELECT a.id,a.owner_user_id,a.slug,a.status,a.policy_revision,p.mode,a.current_deployment_id FROM applications a LEFT JOIN access_policies p ON p.app_id=a.id AND p.revision=a.policy_revision ORDER BY a.slug LIMIT 100", nil
	}
	rows, err := s.Store.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return v, err
	}
	type appRow struct {
		id, owner, currentDeploymentID string
		app                            controlapi.DashboardApp
	}
	var owned []appRow
	for rows.Next() {
		var x appRow
		var mode sql.NullString
		var current sql.NullString
		if err := rows.Scan(&x.id, &x.owner, &x.app.Slug, &x.app.Status, &x.app.Access.Revision, &mode, &current); err != nil {
			rows.Close()
			return v, err
		}
		if !mode.Valid || mode.String != "private" {
			rows.Close()
			return v, ErrUnavailable
		}
		x.app.Access.Mode = mode.String
		if current.Valid {
			x.currentDeploymentID = current.String
		}
		owned = append(owned, x)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return v, err
	}
	rows.Close()
	for _, x := range owned {
		app := x.app
		// The active pointer is independent from the bounded release history: a
		// rollback can select an older release that is no longer in the latest
		// 100 rows. Read it directly so its summary and stable launch link do not
		// disappear or degrade into a guessed description.
		if x.currentDeploymentID != "" {
			var state string
			var manifest []byte
			if e := s.Store.DB.QueryRowContext(ctx, "SELECT state,COALESCE(manifest_json,X'') FROM deployments WHERE id=? AND app_id=?", x.currentDeploymentID, x.id).Scan(&state, &manifest); e != nil || state != string(releases.Active) || app.Status != "active" {
				return v, ErrUnavailable
			}
			description, e := dashboardManifestDescription(manifest, state, app.Slug)
			if e != nil {
				return v, ErrUnavailable
			}
			app.Description = description
			app.StableURL = stableAppURL(app.Slug, s.AppSuffix)
		}
		accessRows, e := s.Store.DB.QueryContext(ctx, "SELECT kind,normalized_value FROM access_rules WHERE app_id=? AND policy_revision=? ORDER BY kind,normalized_value", x.id, app.Access.Revision)
		if e != nil {
			return v, e
		}
		for accessRows.Next() {
			var kind, value string
			if e := accessRows.Scan(&kind, &value); e != nil {
				accessRows.Close()
				return v, e
			}
			switch kind {
			case "email":
				app.Access.Emails = append(app.Access.Emails, value)
			case "domain":
				app.Access.Domains = append(app.Access.Domains, value)
			case "owner":
			default:
				accessRows.Close()
				return v, ErrUnavailable
			}
		}
		if e := accessRows.Err(); e != nil {
			accessRows.Close()
			return v, e
		}
		accessRows.Close()
		releasesRows, e := s.Store.DB.QueryContext(ctx, "SELECT id,COALESCE(release_hash,''),COALESCE(manifest_json,X''),state,created_at,COALESCE(verified_at,''),COALESCE(activated_at,'') FROM deployments WHERE app_id=? ORDER BY created_at DESC,id DESC LIMIT 100", x.id)
		if e != nil {
			return v, e
		}
		for releasesRows.Next() {
			var r controlapi.DashboardRelease
			var manifest []byte
			if e := releasesRows.Scan(&r.ID, &r.ReleaseHash, &manifest, &r.State, &r.CreatedAt, &r.VerifiedAt, &r.ActivatedAt); e != nil {
				releasesRows.Close()
				return v, e
			}
			description, e := dashboardManifestDescription(manifest, r.State, app.Slug)
			if e != nil {
				releasesRows.Close()
				return v, ErrUnavailable
			}
			r.Description = description
			app.Releases = append(app.Releases, r)
		}
		if e := releasesRows.Err(); e != nil {
			releasesRows.Close()
			return v, e
		}
		releasesRows.Close()
		tokensRows, e := s.Store.DB.QueryContext(ctx, "SELECT id,scopes,expires_at,last_used_at,revoked_at FROM api_tokens WHERE app_id=? AND user_id=? ORDER BY id LIMIT 100", x.id, x.owner)
		if e != nil {
			return v, e
		}
		for tokensRows.Next() {
			var t controlapi.DashboardToken
			var scopes string
			var lastUsed, revoked sql.NullString
			if e := tokensRows.Scan(&t.ID, &scopes, &t.ExpiresAt, &lastUsed, &revoked); e != nil {
				tokensRows.Close()
				return v, e
			}
			t.Scopes = strings.Split(scopes, ",")
			if lastUsed.Valid {
				value := lastUsed.String
				t.LastUsedAt = &value
			}
			t.Revoked = revoked.Valid
			app.Tokens = append(app.Tokens, t)
		}
		if e := tokensRows.Err(); e != nil {
			tokensRows.Close()
			return v, e
		}
		tokensRows.Close()
		v.Apps = append(v.Apps, app)
	}
	if a.Role != "operator" {
		return v, nil
	}
	users, err := s.Store.DB.QueryContext(ctx, "SELECT normalized_email,status FROM users WHERE role='deployer' ORDER BY normalized_email LIMIT 100")
	if err != nil {
		return v, err
	}
	for users.Next() {
		var d controlapi.DashboardDeployer
		if err := users.Scan(&d.Email, &d.Status); err != nil {
			users.Close()
			return v, err
		}
		v.Deployers = append(v.Deployers, d)
	}
	if err := users.Err(); err != nil {
		users.Close()
		return v, err
	}
	users.Close()
	audit, err := s.Store.DB.QueryContext(ctx, "SELECT occurred_at,action,outcome,COALESCE(target_id,'') FROM audit_events ORDER BY occurred_at DESC,id DESC LIMIT 100")
	if err != nil {
		return v, err
	}
	for audit.Next() {
		var x controlapi.DashboardAudit
		if err := audit.Scan(&x.OccurredAt, &x.Action, &x.Outcome, &x.Target); err != nil {
			audit.Close()
			return v, err
		}
		v.Audit = append(v.Audit, x)
	}
	err = audit.Err()
	audit.Close()
	return v, err
}

func dashboardManifestDescription(manifest []byte, state, slug string) (string, error) {
	if len(manifest) == 0 {
		if state == string(releases.Verified) || state == string(releases.Active) || state == string(releases.Superseded) {
			return "", ErrUnavailable
		}
		return "", nil
	}
	var parsed releases.Manifest
	if json.Unmarshal(manifest, &parsed) != nil || parsed.Version != 1 || !releases.ValidSlug(parsed.Name) || parsed.Name != slug {
		return "", ErrUnavailable
	}
	description, err := releases.NormalizeDescription(parsed.Description)
	if err != nil || description != parsed.Description {
		return "", ErrUnavailable
	}
	return description, nil
}

// stableAppURL admits only the configured stable gateway origin. It never
// constructs a release path or turns deployment metadata into a link.
func stableAppURL(slug, suffix string) string {
	if !releases.ValidSlug(slug) || !validDNSSuffix(suffix) {
		return ""
	}
	raw := "https://" + slug + "." + suffix + "/"
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Host != slug+"."+suffix || u.Path != "/" || u.RawQuery != "" || u.Fragment != "" {
		return ""
	}
	return raw
}

func validDNSSuffix(value string) bool {
	if len(value) == 0 || len(value) > 253 || strings.HasSuffix(value, ".") {
		return false
	}
	for _, label := range strings.Split(value, ".") {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, c := range label {
			if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-') {
				return false
			}
		}
	}
	return true
}

var ErrUnavailable = errors.New("control operation unavailable")

func (s ControlService) CreateApp(ctx context.Context, a controlapi.Actor, slug, key string) error {
	if !a.Active || key == "" || !releases.ValidSlug(slug) {
		return ErrUnavailable
	}
	if s.WriteGate != nil {
		if err := s.WriteGate.AllowWrite(ctx, operations.WriteAppCreate); err != nil {
			return err
		}
	}
	limit := s.AppsPerDeployer
	if limit == 0 {
		limit = 20
	}
	return s.Store.Write(ctx, func(tx *sql.Tx) error {
		var target string
		err := tx.QueryRowContext(ctx, "SELECT target_id FROM audit_events WHERE action='app.created' AND actor_id=? AND request_id=?", a.ID, key).Scan(&target)
		if err == nil {
			if target == slug {
				return nil
			}
			return ErrUnavailable
		}
		if err != sql.ErrNoRows {
			return err
		}
		var status string
		if err = tx.QueryRowContext(ctx, "SELECT status FROM users WHERE id=?", a.ID).Scan(&status); err != nil || status != "active" {
			return ErrUnavailable
		}
		var count int
		if err = tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM applications WHERE owner_user_id=? AND status NOT IN ('deleting','deleted')", a.ID).Scan(&count); err != nil {
			return err
		}
		if count >= limit {
			return ErrUnavailable
		}
		b := make([]byte, 16)
		if _, err = rand.Read(b); err != nil {
			return err
		}
		id := "app_" + base64.RawURLEncoding.EncodeToString(b)
		if _, err = tx.ExecContext(ctx, "INSERT INTO applications(id,owner_user_id,slug,status,policy_revision,created_at,updated_at) VALUES(?,?,?,'active',1,datetime('now'),datetime('now'))", id, a.ID, slug); err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, "INSERT INTO access_policies(app_id,revision,mode,created_by,created_at) VALUES(?,1,'private',?,datetime('now'))", id, a.ID); err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, "INSERT INTO audit_events(id,occurred_at,actor_kind,actor_id,app_id,action,outcome,target_kind,target_id,request_id) VALUES(?,datetime('now'),'user',?,?,'app.created','success','app',?,?)", id+"_audit", a.ID, id, slug, key)
		return err
	})
}

type AccessView struct {
	Mode     string `json:"mode"`
	Revision uint64 `json:"revision"`
	Allow    struct {
		Emails  []string `json:"emails"`
		Domains []string `json:"domains"`
	} `json:"allow"`
}

func (s ControlService) Access(ctx context.Context, a controlapi.Actor, slug string) (any, error) {
	if !a.Active || s.Store == nil {
		return nil, ErrUnavailable
	}
	var id, mode string
	var rev uint64
	e := s.Store.DB.QueryRowContext(ctx, "SELECT a.id,a.policy_revision,p.mode FROM applications a JOIN access_policies p ON p.app_id=a.id AND p.revision=a.policy_revision WHERE a.slug=? AND a.owner_user_id=? AND a.status='active'", slug, a.ID).Scan(&id, &rev, &mode)
	if e != nil || mode != "private" {
		return nil, ErrUnavailable
	}
	rows, e := s.Store.DB.QueryContext(ctx, "SELECT kind,normalized_value FROM access_rules WHERE app_id=? AND policy_revision=? ORDER BY kind,normalized_value", id, rev)
	if e != nil {
		return nil, ErrUnavailable
	}
	defer rows.Close()
	v := AccessView{Mode: "private", Revision: rev}
	for rows.Next() {
		var k, x string
		if e = rows.Scan(&k, &x); e != nil {
			return nil, ErrUnavailable
		}
		if k == "email" {
			v.Allow.Emails = append(v.Allow.Emails, x)
		} else if k == "domain" {
			v.Allow.Domains = append(v.Allow.Domains, x)
		} else if k != "owner" {
			return nil, ErrUnavailable
		}
	}
	if rows.Err() != nil {
		return nil, ErrUnavailable
	}
	sort.Strings(v.Allow.Emails)
	sort.Strings(v.Allow.Domains)
	return v, nil
}
func (s ControlService) ReplaceAccess(ctx context.Context, a controlapi.Actor, slug string, in controlapi.AccessPolicyInput, key string) error {
	if !a.Active || key == "" || in.ExpectedRevision == 0 {
		return ErrUnavailable
	}
	b, _ := json.Marshal(in)
	digest := fmt.Sprintf("%x", sha256.Sum256(b))
	fresh := false
	var appID string
	err := s.Store.Write(ctx, func(tx *sql.Tx) error {
		var target, meta string
		e := tx.QueryRowContext(ctx, "SELECT target_id,metadata_json FROM audit_events WHERE action='policy.replaced' AND actor_id=? AND request_id=?", a.ID, key).Scan(&target, &meta)
		if e == nil {
			if target == slug && meta == digest {
				return nil
			}
			return ErrUnavailable
		}
		if e != sql.ErrNoRows {
			return e
		}
		var rev uint64
		e = tx.QueryRowContext(ctx, "SELECT id,policy_revision FROM applications WHERE slug=? AND owner_user_id=? AND status='active'", slug, a.ID).Scan(&appID, &rev)
		if e != nil {
			return ErrUnavailable
		}
		if rev != in.ExpectedRevision {
			return controlapi.ErrPolicyRevision
		}
		currentEmails, currentDomains := map[string]bool{}, map[string]bool{}
		rows, e := tx.QueryContext(ctx, "SELECT kind,normalized_value FROM access_rules WHERE app_id=? AND policy_revision=?", appID, rev)
		if e != nil {
			return e
		}
		for rows.Next() {
			var kind, value string
			if e = rows.Scan(&kind, &value); e != nil {
				rows.Close()
				return e
			}
			switch kind {
			case "email":
				currentEmails[value] = true
			case "domain":
				currentDomains[value] = true
			case "owner":
			default:
				rows.Close()
				return ErrUnavailable
			}
		}
		if e = rows.Err(); e != nil {
			rows.Close()
			return e
		}
		rows.Close()
		broadening := false
		for _, value := range in.Allow.Emails {
			broadening = broadening || !currentEmails[value]
		}
		for _, value := range in.Allow.Domains {
			broadening = broadening || !currentDomains[value]
		}
		if broadening && !in.ConfirmBroadening {
			return ErrUnavailable
		}
		rev++
		if _, e = tx.ExecContext(ctx, "INSERT INTO access_policies(app_id,revision,mode,created_by,created_at) VALUES(?,?,'private',?,datetime('now'))", appID, rev, a.ID); e != nil {
			return e
		}
		for _, x := range in.Allow.Emails {
			if _, e = tx.ExecContext(ctx, "INSERT INTO access_rules(id,app_id,policy_revision,kind,normalized_value,created_by,created_at) VALUES(lower(hex(randomblob(16))),?,?,'email',?, ?,datetime('now'))", appID, rev, x, a.ID); e != nil {
				return e
			}
		}
		for _, x := range in.Allow.Domains {
			if _, e = tx.ExecContext(ctx, "INSERT INTO access_rules(id,app_id,policy_revision,kind,normalized_value,created_by,created_at) VALUES(lower(hex(randomblob(16))),?,?,'domain',?, ?,datetime('now'))", appID, rev, x, a.ID); e != nil {
				return e
			}
		}
		if _, e = tx.ExecContext(ctx, "UPDATE applications SET policy_revision=? WHERE id=?", rev, appID); e != nil {
			return e
		}
		_, e = tx.ExecContext(ctx, "INSERT INTO audit_events(id,occurred_at,actor_kind,actor_id,app_id,action,outcome,target_id,request_id,metadata_json) VALUES(lower(hex(randomblob(16))),datetime('now'),'user',?,?,'policy.replaced','success',?,?,?)", a.ID, appID, slug, key, digest)
		fresh = e == nil
		return e
	})
	if err == nil && fresh && s.Live != nil {
		s.Live.Revoke(appID, "")
	}
	return err
}
func (s ControlService) Tokens(ctx context.Context, a controlapi.Actor, slug string) (any, error) {
	if !a.Active {
		return nil, ErrUnavailable
	}
	var id string
	if e := s.Store.DB.QueryRowContext(ctx, "SELECT id FROM applications WHERE slug=? AND owner_user_id=? AND status='active'", slug, a.ID).Scan(&id); e != nil {
		return nil, ErrUnavailable
	}
	rows, e := s.Store.DB.QueryContext(ctx, "SELECT id,scopes,expires_at,last_used_at,revoked_at FROM api_tokens WHERE app_id=? AND user_id=? ORDER BY id", id, a.ID)
	if e != nil {
		return nil, ErrUnavailable
	}
	defer rows.Close()
	type view struct {
		ID         string   `json:"id"`
		Scopes     []string `json:"scopes"`
		ExpiresAt  string   `json:"expires_at"`
		LastUsedAt *string  `json:"last_used_at"`
		Revoked    bool     `json:"revoked"`
	}
	out := []view{}
	for rows.Next() {
		var v view
		var scopes string
		var lastUsed, revoked sql.NullString
		if e = rows.Scan(&v.ID, &scopes, &v.ExpiresAt, &lastUsed, &revoked); e != nil {
			return nil, ErrUnavailable
		}
		v.Scopes = strings.Split(scopes, ",")
		if lastUsed.Valid {
			value := lastUsed.String
			v.LastUsedAt = &value
		}
		v.Revoked = revoked.Valid
		out = append(out, v)
	}
	if rows.Err() != nil {
		return nil, ErrUnavailable
	}
	return out, nil
}

type ReleaseView struct {
	ID          string `json:"id"`
	ReleaseHash string `json:"release_hash,omitempty"`
	State       string `json:"state"`
	CreatedAt   string `json:"created_at"`
	VerifiedAt  string `json:"verified_at,omitempty"`
	ActivatedAt string `json:"activated_at,omitempty"`
}

// Releases deliberately returns database metadata only. Immutable release
// directories remain gateway-private and are never exposed through control.
func (s ControlService) Releases(ctx context.Context, a controlapi.Actor, slug string) (any, error) {
	if !a.Active || s.Store == nil {
		return nil, ErrUnavailable
	}
	var appID string
	if err := s.Store.DB.QueryRowContext(ctx, "SELECT id FROM applications WHERE slug=? AND owner_user_id=? AND status IN ('active','suspended')", slug, a.ID).Scan(&appID); err != nil {
		return nil, ErrUnavailable
	}
	rows, err := s.Store.DB.QueryContext(ctx, "SELECT id,COALESCE(release_hash,''),state,created_at,COALESCE(verified_at,''),COALESCE(activated_at,'') FROM deployments WHERE app_id=? ORDER BY created_at DESC,id DESC LIMIT 100", appID)
	if err != nil {
		return nil, ErrUnavailable
	}
	defer rows.Close()
	out := make([]ReleaseView, 0)
	for rows.Next() {
		var v ReleaseView
		if err := rows.Scan(&v.ID, &v.ReleaseHash, &v.State, &v.CreatedAt, &v.VerifiedAt, &v.ActivatedAt); err != nil {
			return nil, ErrUnavailable
		}
		out = append(out, v)
	}
	if err := rows.Err(); err != nil {
		return nil, ErrUnavailable
	}
	return out, nil
}

// DeleteApp is intentionally a state transition and revocation operation only.
// Files are immutable evidence and are cleaned asynchronously after retention
// policy evaluates them; this request must never walk or remove release paths.
func (s ControlService) DeleteApp(ctx context.Context, a controlapi.Actor, slug, key string) error {
	if !a.Active || s.Store == nil || key == "" {
		return ErrUnavailable
	}
	var appID string
	fresh := false
	err := s.Store.Write(ctx, func(tx *sql.Tx) error {
		var target string
		err := tx.QueryRowContext(ctx, "SELECT target_id FROM audit_events WHERE action='app.deletion_requested' AND actor_id=? AND request_id=?", a.ID, key).Scan(&target)
		if err == nil {
			if target == slug {
				return nil
			}
			return ErrUnavailable
		}
		if err != sql.ErrNoRows {
			return err
		}
		var status string
		if err = tx.QueryRowContext(ctx, "SELECT id,status FROM applications WHERE slug=? AND owner_user_id=?", slug, a.ID).Scan(&appID, &status); err != nil {
			return ErrUnavailable
		}
		if status != "active" && status != "suspended" {
			return ErrUnavailable
		}
		res, err := tx.ExecContext(ctx, "UPDATE applications SET status='deleting',updated_at=datetime('now') WHERE id=? AND status IN ('active','suspended')", appID)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n != 1 {
			return ErrUnavailable
		}
		if _, err = tx.ExecContext(ctx, "UPDATE api_tokens SET revoked_at=datetime('now') WHERE app_id=? AND revoked_at IS NULL", appID); err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, "UPDATE sessions SET revoked_at=datetime('now') WHERE app_id=? AND scope='app' AND revoked_at IS NULL", appID); err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, "UPDATE applications SET status='deleted',current_deployment_id=NULL,updated_at=datetime('now') WHERE id=? AND status='deleting'", appID); err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, "INSERT INTO audit_events(id,occurred_at,actor_kind,actor_id,app_id,action,outcome,target_kind,target_id,request_id) VALUES(lower(hex(randomblob(16))),datetime('now'),'user',?,?,'app.deletion_requested','success','app',?,?)", a.ID, appID, slug, key); err != nil {
			return err
		}
		fresh = true
		return nil
	})
	if err == nil && fresh && s.Live != nil {
		s.Live.Revoke(appID, "")
	}
	return err
}
func (s ControlService) CreateToken(ctx context.Context, a controlapi.Actor, slug string, in controlapi.TokenInput, key string) (controlapi.TokenResult, error) {
	var out controlapi.TokenResult
	if !a.Active || key == "" {
		return out, ErrUnavailable
	}
	err := s.Store.Write(ctx, func(tx *sql.Tx) error {
		var seen string
		e := tx.QueryRowContext(ctx, "SELECT id FROM audit_events WHERE action='token.created' AND actor_id=? AND request_id=?", a.ID, key).Scan(&seen)
		if e == nil {
			return ErrUnavailable
		}
		if e != sql.ErrNoRows {
			return e
		}
		var appID string
		e = tx.QueryRowContext(ctx, "SELECT id FROM applications WHERE slug=? AND owner_user_id=? AND status='active'", slug, a.ID).Scan(&appID)
		if e != nil {
			return ErrUnavailable
		}
		b := make([]byte, 32)
		if _, e = rand.Read(b); e != nil {
			return e
		}
		raw := "tiny_" + base64.RawURLEncoding.EncodeToString(b)
		h := sha256.Sum256([]byte(raw))
		id := "tok_" + base64.RawURLEncoding.EncodeToString(h[:12])
		expiry := time.Now().Add(time.Duration(in.ExpiresInSeconds) * time.Second).UTC()
		scopes := strings.Join(in.Scopes, ",")
		if _, e = tx.ExecContext(ctx, "INSERT INTO api_tokens(id,user_id,app_id,secret_hash,scopes,expires_at) VALUES(?,?,?,?,?,?)", id, a.ID, appID, h[:], scopes, expiry.Format(time.RFC3339Nano)); e != nil {
			return e
		}
		if _, e = tx.ExecContext(ctx, "INSERT INTO audit_events(id,occurred_at,actor_kind,actor_id,app_id,action,outcome,target_id,request_id) VALUES(?,datetime('now'),'user',?,?,'token.created','success',?,?)", id+"_audit", a.ID, appID, id, key); e != nil {
			return e
		}
		out = controlapi.TokenResult{ID: id, Token: raw, Scopes: append([]string(nil), in.Scopes...), ExpiresAt: expiry.Format(time.RFC3339Nano)}
		return nil
	})
	return out, err
}
func (s ControlService) RevokeToken(ctx context.Context, a controlapi.Actor, slug, tokenID, key string) error {
	if !a.Active || key == "" {
		return ErrUnavailable
	}
	return s.Store.Write(ctx, func(tx *sql.Tx) error {
		var prior string
		e := tx.QueryRowContext(ctx, "SELECT target_id FROM audit_events WHERE action='token.revoked' AND actor_id=? AND request_id=?", a.ID, key).Scan(&prior)
		if e == nil {
			if prior == tokenID {
				return nil
			}
			return ErrUnavailable
		}
		if e != sql.ErrNoRows {
			return e
		}
		var appID string
		if e = tx.QueryRowContext(ctx, "SELECT id FROM applications WHERE slug=? AND owner_user_id=? AND status='active'", slug, a.ID).Scan(&appID); e != nil {
			return ErrUnavailable
		}
		res, e := tx.ExecContext(ctx, "UPDATE api_tokens SET revoked_at=datetime('now') WHERE id=? AND user_id=? AND app_id=? AND revoked_at IS NULL", tokenID, a.ID, appID)
		if e != nil {
			return e
		}
		n, _ := res.RowsAffected()
		if n != 1 {
			return ErrUnavailable
		}
		_, e = tx.ExecContext(ctx, "INSERT INTO audit_events(id,occurred_at,actor_kind,actor_id,app_id,action,outcome,target_id,request_id) VALUES(?,datetime('now'),'user',?,?,'token.revoked','success',?,?)", tokenID+"_revoke", a.ID, appID, tokenID, key)
		return e
	})
}
func (s ControlService) CreateDeployment(ctx context.Context, a controlapi.Actor, slug, key string, u controlapi.Upload) (any, error) {
	if s.Deployments == nil || !a.Active {
		return nil, ErrUnavailable
	}
	var id string
	if e := s.Store.DB.QueryRowContext(ctx, "SELECT id FROM applications WHERE slug=? AND owner_user_id=? AND status='active'", slug, a.ID).Scan(&id); e != nil {
		return nil, ErrUnavailable
	}
	b := make([]byte, 16)
	if _, e := rand.Read(b); e != nil {
		return nil, e
	}
	did := "dep_" + base64.RawURLEncoding.EncodeToString(b)
	if _, e := s.Deployments.Create(ctx, deployments.Actor{ID: a.ID, Active: a.Active}, id, slug, did, key); e != nil {
		return nil, e
	}
	if e := s.Deployments.Upload(ctx, deployments.Actor{ID: a.ID, Active: a.Active}, did, u.ContentType, u.Reader); e != nil {
		return nil, e
	}
	r, e := s.Deployments.Repo.Get(ctx, did)
	if e != nil {
		return nil, e
	}
	return map[string]string{"deployment_id": r.ID, "state": string(r.State)}, nil
}
func (s ControlService) Activate(ctx context.Context, a controlapi.Actor, slug, id, key string) (controlapi.ActivationResult, error) {
	if s.Deployments == nil || !a.Active || key == "" {
		return controlapi.ActivationResult{}, ErrUnavailable
	}
	var appID string
	if e := s.Store.DB.QueryRowContext(ctx, "SELECT id FROM applications WHERE slug=? AND owner_user_id=? AND status='active'", slug, a.ID).Scan(&appID); e != nil {
		return controlapi.ActivationResult{}, ErrUnavailable
	}
	r, e := s.Deployments.Repo.Get(ctx, id)
	if e != nil || r.AppID != appID || r.OwnerID != a.ID {
		return controlapi.ActivationResult{}, ErrUnavailable
	}
	e = s.Deployments.Activate(ctx, deployments.Actor{ID: a.ID, Active: true}, id, key)
	if e != nil {
		return controlapi.ActivationResult{}, e
	}
	// The repository has committed the release pointer and its candidate policy
	// together. Existing sockets must not retain the policy that was replaced.
	if s.Live != nil {
		s.Live.Revoke(appID, "")
	}
	return controlapi.ActivationResult{DeploymentID: id, URL: "https://" + slug + "." + s.AppSuffix + "/", AppSuffix: s.AppSuffix, PolicyReady: true, TLSReady: true, AnonymousDenied: true, AuthenticatedHealthy: true}, nil
}
func (s ControlService) Rollback(ctx context.Context, a controlapi.Actor, slug, id, key string) error {
	if s.Deployments == nil || !a.Active || key == "" {
		return ErrUnavailable
	}
	var appID string
	if e := s.Store.DB.QueryRowContext(ctx, "SELECT id FROM applications WHERE slug=? AND owner_user_id=? AND status='active'", slug, a.ID).Scan(&appID); e != nil {
		return ErrUnavailable
	}
	r, e := s.Deployments.Repo.Get(ctx, id)
	if e != nil || r.AppID != appID || r.OwnerID != a.ID {
		return ErrUnavailable
	}
	if e = s.Deployments.Rollback(ctx, deployments.Actor{ID: a.ID, Active: true}, id, key); e != nil {
		return e
	}
	if s.Live != nil {
		s.Live.Revoke(appID, "")
	}
	return nil
}
