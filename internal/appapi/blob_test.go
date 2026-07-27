package appapi

import (
	"bytes"
	"context"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/tinyhost/tiny/internal/appauth"
	"github.com/tinyhost/tiny/internal/apps"
	"github.com/tinyhost/tiny/internal/blob"
	"github.com/tinyhost/tiny/internal/identity"
	"github.com/tinyhost/tiny/internal/policies"
	"github.com/tinyhost/tiny/internal/sessions"
)

func blobAuth(t *testing.T, app string, enabled bool) appauth.AuthorizationContext {
	t.Helper()
	s := sessions.NewMemoryStore()
	token, _, _ := s.Create(app, identity.Identity{ID: "viewer", Email: "v@example.com"}, time.Now().Add(time.Hour))
	a, e := appauth.Authorizer{Sessions: s, Policies: &policies.MemoryStore{Policies: map[string]policies.Policy{app: {AppID: app, OwnerIdentityID: "viewer", Valid: true}}}}.Authorize(context.Background(), apps.App{ID: app, BlobsEnabled: enabled}, token, "req_blob")
	if e != nil {
		t.Fatal(e)
	}
	return a
}

type blobSpy struct {
	calls int
	body  []byte
}

func (s *blobSpy) Upload(_ context.Context, _ appauth.AuthorizationContext, _ string, _ string, r io.Reader) (blob.Metadata, error) {
	s.calls++
	b, e := io.ReadAll(r)
	s.body = b
	if e != nil {
		return blob.Metadata{}, e
	}
	return blob.Metadata{ID: "blb_0123456789abcdef0123456789abcdef", Name: "a.txt", Size: int64(len(b)), ContentType: "text/plain", CreatedAt: time.Now()}, nil
}
func (s *blobSpy) Get(context.Context, appauth.AuthorizationContext, string) (io.ReadCloser, blob.Metadata, error) {
	return nil, blob.Metadata{}, io.EOF
}
func (s *blobSpy) List(context.Context, appauth.AuthorizationContext, string, int) (blob.ListResult, error) {
	s.calls++
	return blob.ListResult{}, nil
}
func (s *blobSpy) Delete(context.Context, appauth.AuthorizationContext, string) (bool, error) {
	s.calls++
	return false, nil
}
func multipartBlob(t *testing.T, extra bool) *http.Request {
	t.Helper()
	var b bytes.Buffer
	w := multipart.NewWriter(&b)
	p, e := w.CreateFormFile("file", "a.txt")
	if e != nil {
		t.Fatal(e)
	}
	_, _ = p.Write([]byte("hello"))
	if extra {
		_ = w.WriteField("evil", "1")
	}
	_ = w.Close()
	r := httptest.NewRequest(http.MethodPost, "/_tiny/api/v1/blobs", &b)
	r.Header.Set("Content-Type", w.FormDataContentType())
	r.Header.Set("Origin", "http://example.com")
	r.Host = "example.com"
	return r
}
func TestBlobUploadStreamsFirstPartAndRejectsExtraPartsBeforeReady(t *testing.T) {
	t.Run("one file", func(t *testing.T) {
		s := &blobSpy{}
		w := httptest.NewRecorder()
		Dispatcher{Blobs: s}.Dispatch(blobAuth(t, "a", true), w, multipartBlob(t, false))
		if w.Code != 201 || string(s.body) != "hello" {
			t.Fatalf("status=%d body=%q response=%s", w.Code, s.body, w.Body.String())
		}
	})
	t.Run("extra part", func(t *testing.T) {
		s := &blobSpy{}
		w := httptest.NewRecorder()
		Dispatcher{Blobs: s}.Dispatch(blobAuth(t, "a", true), w, multipartBlob(t, true))
		if w.Code != 400 || s.calls != 1 || !strings.Contains(w.Body.String(), "validation_failed") {
			t.Fatalf("status=%d calls=%d body=%s", w.Code, s.calls, w.Body.String())
		}
	})
}
func TestDisabledBlobNeverTouchesRepository(t *testing.T) {
	s := &blobSpy{}
	w := httptest.NewRecorder()
	Dispatcher{Blobs: s}.Dispatch(blobAuth(t, "a", false), w, httptest.NewRequest(http.MethodGet, "/_tiny/api/v1/blobs", nil))
	if w.Code != 403 || s.calls != 0 {
		t.Fatalf("status=%d calls=%d", w.Code, s.calls)
	}
}

func TestBlobMutationRequiresInjectedProductionHTTPSOrigin(t *testing.T) {
	s := &blobSpy{}
	d := Dispatcher{Blobs: s, Origin: func(r *http.Request) bool { return r.Header.Get("Origin") == "https://"+r.Host }}
	r := multipartBlob(t, false)
	w := httptest.NewRecorder()
	d.Dispatch(blobAuth(t, "a", true), w, r)
	if w.Code != http.StatusForbidden || s.calls != 0 {
		t.Fatalf("http origin mutation status=%d calls=%d", w.Code, s.calls)
	}
	r = multipartBlob(t, false)
	r.Header.Set("Origin", "https://example.com")
	w = httptest.NewRecorder()
	d.Dispatch(blobAuth(t, "a", true), w, r)
	if w.Code != http.StatusCreated {
		t.Fatalf("https origin mutation status=%d body=%s", w.Code, w.Body.String())
	}
}
