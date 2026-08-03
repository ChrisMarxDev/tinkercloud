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

func TestFetcherAllowsOneCanonicalGitHubReleaseAssetRedirect(t *testing.T) {
	for _, name := range []string{
		"tinkercloud-linux-amd64",
		"tinkercloud-linux-amd64.metadata.json",
		"tinkercloud-linux-amd64.signature",
		"release-manifest.json",
		"release-manifest.json.metadata.json",
		"release-manifest.json.signature",
	} {
		t.Run(name, func(t *testing.T) {
			initial := "https://github.com/ChrisMarxDev/tinkercloud/releases/download/v0.1.6/" + name
			asset := "https://release-assets.githubusercontent.com/repos/1/releases/2/assets/3?token=opaque"
			var validated, requested []string
			f := Fetcher{
				Client: fetchDoer(func(r *http.Request) (*http.Response, error) {
					requested = append(requested, r.URL.String())
					switch r.URL.String() {
					case initial:
						return &http.Response{StatusCode: http.StatusFound, Header: http.Header{"Location": []string{asset}}, Body: io.NopCloser(strings.NewReader("")), Request: r}, nil
					case asset:
						return response(r, http.StatusOK, "signed binary"), nil
					default:
						t.Fatalf("unexpected request %q", r.URL)
						return nil, nil
					}
				}),
				ValidateURL: func(raw string) error {
					validated = append(validated, raw)
					return nil
				},
			}

			got, err := f.Get(context.Background(), initial, 1024)
			if err != nil || string(got) != "signed binary" {
				t.Fatalf("Get() = %q, %v", got, err)
			}
			if strings.Join(requested, ",") != initial+","+asset {
				t.Fatalf("requests = %v", requested)
			}
			if strings.Join(validated, ",") != initial+","+asset {
				t.Fatalf("validated URLs = %v", validated)
			}
		})
	}
}

func TestFetcherRejectsGitHubRedirectOutsideExactReleaseAssetBoundary(t *testing.T) {
	initial := "https://github.com/ChrisMarxDev/tinkercloud/releases/download/v0.1.6/tinkercloud-linux-amd64"
	for name, location := range map[string]string{
		"second redirect":        "https://release-assets.githubusercontent.com/second",
		"wrong destination host": "https://github-releases.githubusercontent.com/asset",
		"credentials":            "https://user@release-assets.githubusercontent.com/asset",
		"fragment":               "https://release-assets.githubusercontent.com/asset#fragment",
		"downgrade":              "http://release-assets.githubusercontent.com/asset",
		"port":                   "https://release-assets.githubusercontent.com:444/asset",
	} {
		t.Run(name, func(t *testing.T) {
			calls := 0
			f := permissiveFetcher(fetchDoer(func(r *http.Request) (*http.Response, error) {
				calls++
				if calls == 1 {
					return &http.Response{StatusCode: http.StatusFound, Header: http.Header{"Location": []string{location}}, Body: io.NopCloser(strings.NewReader("")), Request: r}, nil
				}
				return response(r, http.StatusFound, ""), nil
			}))
			if _, err := f.Get(context.Background(), initial, 1024); !errors.Is(err, ErrFetch) {
				t.Fatalf("redirect to %q accepted: %v", location, err)
			}
			if calls > 2 {
				t.Fatalf("followed more than one redirect: %d requests", calls)
			}
		})
	}
	for _, raw := range []string{
		"https://github.com/ChrisMarxDev/tinkercloud/releases/latest/download/tinkercloud-linux-amd64",
		"https://github.com/ChrisMarxDev/tinkercloud/releases/download/v01.2.3/tinkercloud-linux-amd64",
		"https://github.com/ChrisMarxDev/tinkercloud/releases/download/v0.1.6/tinkercloud-linux-amd64#fragment",
		"https://github.com:444/ChrisMarxDev/tinkercloud/releases/download/v0.1.6/tinkercloud-linux-amd64",
		"https://github.com/other/tinkercloud/releases/download/v0.1.6/tinkercloud-linux-amd64",
		"https://github.com/ChrisMarxDev/tinkercloud/releases/download/v0.1.6/not-an-updater-artifact",
	} {
		t.Run(raw, func(t *testing.T) {
			calls := 0
			f := permissiveFetcher(fetchDoer(func(r *http.Request) (*http.Response, error) {
				calls++
				return &http.Response{StatusCode: http.StatusFound, Header: http.Header{"Location": []string{"https://release-assets.githubusercontent.com/asset"}}, Body: io.NopCloser(strings.NewReader("")), Request: r}, nil
			}))
			if _, err := f.Get(context.Background(), raw, 1024); !errors.Is(err, ErrFetch) {
				t.Fatalf("noncanonical URL accepted: %v", err)
			}
			if calls != 1 {
				t.Fatalf("noncanonical URL followed redirect: %d requests", calls)
			}
		})
	}
}

func TestFetcherRejectsRedirectFinalURLMismatch(t *testing.T) {
	initial := "https://github.com/ChrisMarxDev/tinkercloud/releases/download/v0.1.6/tinkercloud-linux-amd64"
	asset := "https://release-assets.githubusercontent.com/asset"
	calls := 0
	f := permissiveFetcher(fetchDoer(func(r *http.Request) (*http.Response, error) {
		calls++
		if calls == 1 {
			return &http.Response{StatusCode: http.StatusFound, Header: http.Header{"Location": []string{asset}}, Body: io.NopCloser(strings.NewReader("")), Request: r}, nil
		}
		wrong, err := http.NewRequest(http.MethodGet, "https://release-assets.githubusercontent.com/other", nil)
		if err != nil {
			t.Fatal(err)
		}
		return response(wrong, http.StatusOK, "signed binary"), nil
	}))
	if _, err := f.Get(context.Background(), initial, 1024); !errors.Is(err, ErrFetch) {
		t.Fatalf("final URL mismatch accepted: %v", err)
	}
}

func TestFetcherRejectsRedirectTargetFragmentBeforeSecondRequest(t *testing.T) {
	initial := "https://github.com/ChrisMarxDev/tinkercloud/releases/download/v0.1.6/tinkercloud-linux-amd64"
	requests := 0
	f := permissiveFetcher(fetchDoer(func(r *http.Request) (*http.Response, error) {
		requests++
		return &http.Response{StatusCode: http.StatusFound, Header: http.Header{"Location": []string{"https://release-assets.githubusercontent.com/asset#fragment"}}, Body: io.NopCloser(strings.NewReader("")), Request: r}, nil
	}))
	if _, err := f.Get(context.Background(), initial, 1024); !errors.Is(err, ErrFetch) {
		t.Fatalf("fragment redirect target accepted: %v", err)
	}
	if requests != 1 {
		t.Fatalf("fragment redirect target made %d requests, want 1", requests)
	}
}

func TestFetcherRejectsRedirectTargetWhenPublicDNSValidationFails(t *testing.T) {
	initial := "https://github.com/ChrisMarxDev/tinkercloud/releases/download/v0.1.6/tinkercloud-linux-amd64"
	asset := "https://release-assets.githubusercontent.com/asset"
	requests := 0
	f := Fetcher{
		Client: fetchDoer(func(r *http.Request) (*http.Response, error) {
			requests++
			return &http.Response{StatusCode: http.StatusFound, Header: http.Header{"Location": []string{asset}}, Body: io.NopCloser(strings.NewReader("")), Request: r}, nil
		}),
		ValidateURL: func(raw string) error {
			if raw == asset {
				return ErrFetch
			}
			return nil
		},
	}
	if _, err := f.Get(context.Background(), initial, 1024); !errors.Is(err, ErrFetch) {
		t.Fatalf("DNS-denied redirect target accepted: %v", err)
	}
	if requests != 1 {
		t.Fatalf("requests = %d, want 1", requests)
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
