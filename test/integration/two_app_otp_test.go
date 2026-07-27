package integration

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tinyhost/tiny/internal/appapi"
	"github.com/tinyhost/tiny/internal/appauth"
	"github.com/tinyhost/tiny/internal/apps"
	"github.com/tinyhost/tiny/internal/compose"
	"github.com/tinyhost/tiny/internal/config"
	"github.com/tinyhost/tiny/internal/gateway"
	"github.com/tinyhost/tiny/internal/kv"
	"github.com/tinyhost/tiny/internal/otp"
	"github.com/tinyhost/tiny/internal/policies"
	"github.com/tinyhost/tiny/internal/releases"
	"github.com/tinyhost/tiny/internal/sessions"
)

type mail struct{ messages []otp.Message }

func (m *mail) EnqueueOTP(_ context.Context, x otp.Message) error {
	m.messages = append(m.messages, x)
	return nil
}
func TestTwoAppOTPBoundary(t *testing.T) {
	mk := func(body string) string {
		d := t.TempDir()
		if e := os.WriteFile(filepath.Join(d, "index.html"), []byte(body), 0600); e != nil {
			t.Fatal(e)
		}
		return d
	}
	aRoot, bRoot := mk("APP-A-SECRET"), mk("APP-B-SECRET")
	aEvidence, e := releases.Inspect(aRoot)
	if e != nil {
		t.Fatal(e)
	}
	bEvidence, e := releases.Inspect(bRoot)
	if e != nil {
		t.Fatal(e)
	}
	repo := apps.NewMemoryRepository(apps.App{ID: "a", Slug: "alpha", ReleaseRoot: aRoot, ReleaseEvidence: aEvidence, Status: apps.Active, SPAFallback: true, KVEnabled: true}, apps.App{ID: "b", Slug: "beta", ReleaseRoot: bRoot, ReleaseEvidence: bEvidence, Status: apps.Active, SPAFallback: true, KVEnabled: true})
	ps := &policies.MemoryStore{Policies: map[string]policies.Policy{"a": {AppID: "a", OwnerIdentityID: "owner@example.com", Revision: 1, Emails: map[string]struct{}{"alice@example.com": {}}, Valid: true}, "b": {AppID: "b", OwnerIdentityID: "owner@example.com", Revision: 1, Domains: map[string]struct{}{"example.com": {}}, Valid: true}}}
	ss := sessions.NewMemoryStore()
	out := &mail{}
	login := compose.Login{OTP: otp.Service{Store: otp.NewMemoryStore(), Outbox: out, Key: []byte("integration-key")}, Sessions: compose.MemorySessions{Store: ss}, Policies: ps, SessionTTL: time.Hour}
	api := compose.NewAppDispatcher(appapi.Dispatcher{KV: kv.New(kv.DefaultLimits(), nil)})
	g := gateway.Gateway{Config: config.Config{PlatformHost: "tiny.test", AppSuffix: "apps.tiny.test", SessionCookie: sessions.AppCookieName}, Apps: repo, Authorizer: appauth.Authorizer{Sessions: ss, Policies: ps}, Protected: api, PreAuth: login}
	s := httptest.NewServer(g)
	defer s.Close()
	c := s.Client()
	c.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	request := func(method, host, path, body, cookie string) *http.Response {
		r, _ := http.NewRequest(method, s.URL+path, strings.NewReader(body))
		r.Host = host
		if body != "" {
			r.Header.Set("Content-Type", "application/json")
		}
		if method == "PUT" || method == "DELETE" {
			r.Header.Set("Origin", "http://"+host)
		}
		if cookie != "" {
			r.AddCookie(&http.Cookie{Name: sessions.AppCookieName, Value: cookie})
		}
		q, e := c.Do(r)
		if e != nil {
			t.Fatal(e)
		}
		return q
	}
	read := func(r *http.Response) string { b, _ := io.ReadAll(r.Body); r.Body.Close(); return string(b) }
	// Generic response does not disclose whether membership is allowed.
	r := request("POST", "alpha.apps.tiny.test", "/_tiny/auth/otp", `{"email":"alice@example.com"}`, "")
	if r.StatusCode != 202 {
		t.Fatal(r.StatusCode)
	}
	read(r)
	if len(out.messages) != 1 {
		t.Fatal("test outbox missing")
	}
	m := out.messages[0]
	r = request("POST", "alpha.apps.tiny.test", "/_tiny/auth/verify", `{"email":"alice@example.com","transaction":"`+m.ChallengeID+`","code":"`+m.Code+`","return":"/safe"}`, "")
	if r.StatusCode != 303 || r.Header.Get("Location") != "/safe" {
		t.Fatal(r.StatusCode, r.Header.Get("Location"))
	}
	ck := r.Cookies()[0]
	if !ck.Secure || !ck.HttpOnly || ck.SameSite != http.SameSiteLaxMode || ck.Path != "/" || ck.Domain != "" {
		t.Fatal("unsafe cookie")
	}
	read(r)
	r = request("GET", "alpha.apps.tiny.test", "/", "", ck.Value)
	if r.StatusCode != 200 || read(r) != "APP-A-SECRET" {
		t.Fatal("app A content")
	}
	r = request("GET", "alpha.apps.tiny.test", "/_tiny/api/v1/me", "", ck.Value)
	if r.StatusCode != 200 || !strings.Contains(read(r), "alpha") {
		t.Fatal("me denied")
	}
	r = request("PUT", "alpha.apps.tiny.test", "/_tiny/api/v1/kv/k", `{"value":true}`, ck.Value)
	if r.StatusCode != 200 {
		t.Fatal("kv write", r.StatusCode, read(r))
	}
	r = request("GET", "beta.apps.tiny.test", "/", "", ck.Value)
	if r.StatusCode == 200 || strings.Contains(read(r), "APP-B-SECRET") {
		t.Fatal("cross app leaked")
	}
	for _, p := range []string{"/_tiny/ws/v1", "/_tiny/unknown", "/.hidden", "/x.map"} {
		r = request("GET", "alpha.apps.tiny.test", p, "", ck.Value)
		if r.StatusCode == 200 || strings.Contains(read(r), "APP-A-SECRET") {
			t.Fatalf("reserved/path leaked %s", p)
		}
	}
	ps.Policies["a"] = policies.Policy{AppID: "a", Valid: true}
	r = request("GET", "alpha.apps.tiny.test", "/", "", ck.Value)
	if r.StatusCode == 200 || strings.Contains(read(r), "APP-A-SECRET") {
		t.Fatal("policy removal leaked")
	}
}
