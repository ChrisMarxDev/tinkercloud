package client

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestProbePublicStaticAcceptsSizedDocumentAndAssetAndDeniesRegistry(t *testing.T) {
	document := []byte(strings.Repeat("document-", 5000))
	asset := []byte(strings.Repeat("asset-", 6000))
	hash := func(b []byte) string { return fmt.Sprintf("%x", sha256.Sum256(b)) }
	evidence := &PublicStaticEvidence{RootSHA256: hash(document), RootBytes: int64(len(document)), AssetPath: "assets/app.js", AssetSHA256: hash(asset), AssetBytes: int64(len(asset))}
	root, _ := url.Parse("https://demo.apps.test/")
	if !safePublicReservedDenial(publicReservedResponse(&http.Request{})) {
		t.Fatal("test denial fixture is not safe")
	}
	var routes []string
	c := New("https://control.test", "token")
	c.HTTP = &http.Client{Transport: rt(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/" {
			return publicStaticResponse(r, document, nil), nil
		}
		if r.URL.Path == "/assets/app.js" {
			return publicStaticResponse(r, asset, nil), nil
		}
		routes = append(routes, r.Method+" "+r.URL.Path)
		return publicReservedResponse(r), nil
	})}
	if err := c.probePublicStaticOnce(context.Background(), root, evidence); err != nil {
		t.Fatalf("%v after routes=%v", err, routes)
	}
	if got, want := strings.Join(routes, ","), "GET /_tinker/auth/login,GET /_tinker/auth/callback,POST /_tinker/auth/logout,GET /_tinker/api/v1/me,GET /_tinker/api/v1/app,GET /_tinker/api/v1/capabilities,GET /_tinker/api/v1/kv,GET /_tinker/api/v1/db,GET /_tinker/api/v1/blobs,GET /_tinker/ws/v1,POST /_tinker/api/v1/llm/chat"; got != want {
		t.Fatalf("routes=%s", got)
	}
}

func TestPublicStaticEvidenceRejectsUnsafeReceipt(t *testing.T) {
	hash := strings.Repeat("a", 64)
	for _, evidence := range []*PublicStaticEvidence{
		{RootSHA256: strings.ToUpper(hash), RootBytes: 1},
		{RootSHA256: hash, RootBytes: -1},
		{RootSHA256: hash, RootBytes: 1, AssetPath: "../private", AssetSHA256: hash, AssetBytes: 1},
		{RootSHA256: hash, RootBytes: 1, AssetPath: "asset.js", AssetSHA256: hash, AssetBytes: maxPublicStaticEvidenceBytes + 1},
	} {
		if validPublicStaticEvidence(evidence) {
			t.Fatalf("unsafe evidence accepted: %#v", evidence)
		}
	}
}

func TestProbePublicStaticRejectsUnsafeResponses(t *testing.T) {
	document, asset := []byte("document"), []byte("asset")
	hash := func(b []byte) string { return fmt.Sprintf("%x", sha256.Sum256(b)) }
	for _, tc := range []struct {
		name     string
		indexing bool
		mutate   func(*http.Request, *http.Response)
	}{
		{"wrong root hash", false, func(r *http.Request, response *http.Response) {
			if r.URL.Path == "/" {
				response.Body = io.NopCloser(strings.NewReader("otherdoc"))
			}
		}},
		{"short root", false, func(r *http.Request, response *http.Response) {
			if r.URL.Path == "/" {
				response.Body = io.NopCloser(strings.NewReader("short"))
			}
		}},
		{"long root", false, func(r *http.Request, response *http.Response) {
			if r.URL.Path == "/" {
				response.Body = io.NopCloser(strings.NewReader("document+"))
			}
		}},
		{"redirect", false, func(r *http.Request, response *http.Response) {
			if r.URL.Path == "/" {
				response.StatusCode = http.StatusFound
				response.Header.Set("Location", "/other")
			}
		}},
		{"cookie", false, func(r *http.Request, response *http.Response) {
			if r.URL.Path == "/" {
				response.Header.Set("Set-Cookie", "unsafe=1")
			}
		}},
		{"missing noindex", false, func(r *http.Request, response *http.Response) {
			if r.URL.Path == "/" {
				response.Header.Del("X-Robots-Tag")
			}
		}},
		{"wrong indexing", true, func(r *http.Request, response *http.Response) {
			if r.URL.Path == "/" {
				response.Header.Set("X-Robots-Tag", "noindex, nofollow")
			}
		}},
		{"unsafe denial status", false, func(r *http.Request, response *http.Response) {
			if r.URL.Path == "/_tinker/auth/login" {
				response.StatusCode = http.StatusUnauthorized
			}
		}},
		{"unsafe denial marker", false, func(r *http.Request, response *http.Response) {
			if r.URL.Path == "/_tinker/auth/login" {
				response.Body = io.NopCloser(strings.NewReader(`{"error":{"code":"not_found","message":"This request is not authorized.","request_id":"req_0123456789abcdef01234567","marker":"document"}}`))
			}
		}},
		{"unsafe denial code", false, func(r *http.Request, response *http.Response) {
			if r.URL.Path == "/_tinker/auth/login" {
				response.Body = io.NopCloser(strings.NewReader(`{"error":{"code":"not_authorized","message":"This request is not authorized.","request_id":"req_0123456789abcdef01234567"}}`))
			}
		}},
		{"unsafe denial request id", false, func(r *http.Request, response *http.Response) {
			if r.URL.Path == "/_tinker/auth/login" {
				response.Header.Set("X-Request-ID", "req_bad")
			}
		}},
		{"unsafe denial header", false, func(r *http.Request, response *http.Response) {
			if r.URL.Path == "/_tinker/auth/login" {
				response.Header.Del("X-Content-Type-Options")
			}
		}},
		{"asset mismatch", false, func(r *http.Request, response *http.Response) {
			if r.URL.Path == "/assets/app.js" {
				response.Body = io.NopCloser(strings.NewReader("other"))
			}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			evidence := &PublicStaticEvidence{RootSHA256: hash(document), RootBytes: int64(len(document)), AssetPath: "assets/app.js", AssetSHA256: hash(asset), AssetBytes: int64(len(asset)), Indexing: tc.indexing}
			root, _ := url.Parse("https://demo.apps.test/")
			c := New("https://control.test", "token")
			c.HTTP = &http.Client{Transport: rt(func(r *http.Request) (*http.Response, error) {
				var response *http.Response
				switch r.URL.Path {
				case "/":
					response = publicStaticResponse(r, document, nil)
					if tc.indexing {
						response.Header.Del("X-Robots-Tag")
					}
				case "/assets/app.js":
					response = publicStaticResponse(r, asset, nil)
				default:
					response = publicReservedResponse(r)
				}
				tc.mutate(r, response)
				return response, nil
			})}
			if err := c.probePublicStaticOnce(context.Background(), root, evidence); err == nil {
				t.Fatal("unsafe response accepted")
			}
		})
	}
}

func publicStaticResponse(r *http.Request, body []byte, extra http.Header) *http.Response {
	h := make(http.Header)
	h.Set("Cache-Control", "no-store")
	if r.URL.Path == "/" {
		h.Set("X-Robots-Tag", "noindex, nofollow")
	}
	for k, v := range extra {
		h[k] = v
	}
	return &http.Response{StatusCode: http.StatusOK, Header: h, Body: io.NopCloser(strings.NewReader(string(body))), Request: r}
}

func publicReservedResponse(r *http.Request) *http.Response {
	id := "req_0123456789abcdef01234567"
	h := make(http.Header)
	h.Set("Cache-Control", "no-store")
	h.Set("Content-Type", "application/json; charset=utf-8")
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Referrer-Policy", "same-origin")
	h.Set("X-Request-ID", id)
	return &http.Response{StatusCode: http.StatusNotFound, Header: h, Body: io.NopCloser(strings.NewReader(`{"error":{"code":"not_found","message":"This request is not authorized.","request_id":"` + id + `"}}`)), Request: r}
}
