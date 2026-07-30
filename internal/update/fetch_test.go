package update

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

type fetchDoer func(*http.Request) (*http.Response, error)

func (f fetchDoer) Do(r *http.Request) (*http.Response, error) { return f(r) }

func response(r *http.Request, status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body)), ContentLength: int64(len(body)), Request: r}
}

func permissiveFetcher(d HTTPDoer) Fetcher {
	return Fetcher{Client: d, ValidateURL: func(string) error { return nil }}
}

func TestReleaseURLsRejectsNonHTTPSAndUnexpectedArtifact(t *testing.T) {
	for _, raw := range []string{"http://example.com/r/", "https://example.com/r", "https://example.com/r/?q=1"} {
		if _, err := ReleaseURLsFor(raw, "", "tinkercloud-linux-amd64"); !errors.Is(err, ErrFetch) {
			t.Fatalf("%q accepted: %v", raw, err)
		}
	}
	if _, err := ReleaseURLsFor("https://example.com/r/", "", "../tinkercloud"); !errors.Is(err, ErrFetch) {
		t.Fatal("unsafe artifact accepted")
	}
}

func TestReleaseURLsFromMetadataKeepsOrigin(t *testing.T) {
	u, err := ReleaseURLsFor("", "https://releases.example/r/tinkercloud-linux-amd64.metadata.json", "tinkercloud-linux-amd64")
	if err != nil || u.Binary != "https://releases.example/r/tinkercloud-linux-amd64" || u.Signature != u.Binary+".signature" {
		t.Fatal(u, err)
	}
}

func TestManifestURLsRemainOnReleaseOrigin(t *testing.T) {
	urls, err := ManifestURLsFor("https://releases.example/r/", "")
	if err != nil || urls.Binary != "https://releases.example/r/release-manifest.json" {
		t.Fatalf("base manifest URLs = %#v, %v", urls, err)
	}
	urls, err = ManifestURLsFor("", "https://releases.example/r/tinkercloud-linux-amd64.metadata.json")
	if err != nil || urls.Metadata != "https://releases.example/r/release-manifest.json.metadata.json" {
		t.Fatalf("metadata-derived manifest URLs = %#v, %v", urls, err)
	}
	for _, raw := range []string{"http://releases.example/r/", "https://user@releases.example/r/", "https://releases.example/r/file"} {
		if _, err = ManifestURLsFor(raw, ""); !errors.Is(err, ErrFetch) {
			t.Fatalf("accepted manifest base %q", raw)
		}
	}
}

func TestFetcherRejectsOversizeRedirectAndNetworkFailure(t *testing.T) {
	url := "https://releases.example/r/tinkercloud-linux-amd64"
	for name, do := range map[string]HTTPDoer{
		"oversize": fetchDoer(func(r *http.Request) (*http.Response, error) { return response(r, 200, "12345"), nil }),
		"redirect": fetchDoer(func(r *http.Request) (*http.Response, error) {
			return response(&http.Request{URL: r.URL.ResolveReference(r.URL)}, 302, ""), nil
		}),
		"network": fetchDoer(func(*http.Request) (*http.Response, error) { return nil, errors.New("offline") }),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := permissiveFetcher(do).Get(context.Background(), url, 4); !errors.Is(err, ErrFetch) {
				t.Fatal(err)
			}
		})
	}
}

func TestFetcherReleaseRejectsMixedOrigin(t *testing.T) {
	f := permissiveFetcher(fetchDoer(func(r *http.Request) (*http.Response, error) { return response(r, 200, "x"), nil }))
	_, _, _, err := f.Release(context.Background(), ReleaseURLs{Binary: "https://a.example/tinkercloud-linux-amd64", Metadata: "https://b.example/tinkercloud-linux-amd64.metadata.json", Signature: "https://a.example/tinkercloud-linux-amd64.signature"})
	if !errors.Is(err, ErrFetch) {
		t.Fatal(err)
	}
}

func TestValidatePublicHTTPSRejectsLoopbackSSRF(t *testing.T) {
	if err := ValidatePublicHTTPS("https://127.0.0.1/releases/"); !errors.Is(err, ErrFetch) {
		t.Fatal(err)
	}
}
