package update

// Remote update retrieval is deliberately a tiny, separate boundary.  The
// updater never treats a URL as a filesystem path and it never follows a
// redirect: an operator-selected release origin must be the origin that sends
// every signed release component.

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"path"
	"strings"
	"time"
)

var ErrFetch = errors.New("release download failed")

const (
	MaxBinaryBytes    int64 = 100 << 20
	MaxMetadataBytes  int64 = 16 << 10
	MaxSignatureBytes int64 = 4 << 10
)

type ReleaseURLs struct {
	Binary, Metadata, Signature string
}

// ReleaseURLsFor returns the three immutable files from either a release
// directory or a metadata URL.  V1 intentionally has one server artifact;
// accepting a relative artifact name or a different origin would turn this
// root command into an SSRF/redirect primitive.
func ReleaseURLsFor(releaseBase, metadataURL, artifact string) (ReleaseURLs, error) {
	if artifact == "" {
		artifact = "tinyhost-linux-amd64"
	}
	if artifact != "tinyhost-linux-amd64" || strings.ContainsAny(artifact, "/\\?#") {
		return ReleaseURLs{}, ErrFetch
	}
	if (releaseBase == "") == (metadataURL == "") {
		return ReleaseURLs{}, ErrFetch
	}
	if releaseBase != "" {
		u, err := parseReleaseURL(releaseBase)
		if err != nil || !strings.HasSuffix(u.Path, "/") {
			return ReleaseURLs{}, ErrFetch
		}
		return ReleaseURLs{Binary: u.String() + artifact, Metadata: u.String() + artifact + ".metadata.json", Signature: u.String() + artifact + ".signature"}, nil
	}
	u, err := parseReleaseURL(metadataURL)
	if err != nil || !strings.HasSuffix(u.Path, artifact+".metadata.json") {
		return ReleaseURLs{}, ErrFetch
	}
	prefix := strings.TrimSuffix(u.String(), ".metadata.json")
	return ReleaseURLs{Binary: prefix, Metadata: u.String(), Signature: prefix + ".signature"}, nil
}

func parseReleaseURL(raw string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Port() != "" {
		return nil, ErrFetch
	}
	if strings.Contains(u.Hostname(), "%") || path.Clean(u.EscapedPath()) == "." && u.Path != "/" {
		return nil, ErrFetch
	}
	return u, nil
}

// ValidatePublicHTTPS rejects local, link-local, private and special-use
// release origins before any request is made.  Callers that need a proxy or an
// air-gapped mirror use the local-file flags instead of weakening this check.
func ValidatePublicHTTPS(raw string) error {
	u, err := parseReleaseURL(raw)
	if err != nil {
		return ErrFetch
	}
	_, err = publicIPs(context.Background(), u.Hostname())
	if err != nil {
		return ErrFetch
	}
	return nil
}

func publicIPs(ctx context.Context, host string) ([]netip.Addr, error) {
	ips, err := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
	if err != nil || len(ips) == 0 {
		return nil, ErrFetch
	}
	for _, ip := range ips {
		if !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsUnspecified() || ip.IsMulticast() {
			return nil, ErrFetch
		}
	}
	return ips, nil
}

// NewSecureHTTPClient dials the public IPs resolved for each connection and
// disables environment proxy discovery. That prevents both DNS rebinding and
// an operator environment variable from turning a signed-update fetch into a
// request to a local management endpoint.
func NewSecureHTTPClient(timeout time.Duration) *http.Client {
	return &http.Client{Timeout: timeout, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }, Transport: &http.Transport{
		Proxy: nil,
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(address)
			if err != nil {
				return nil, ErrFetch
			}
			ips, err := publicIPs(ctx, host)
			if err != nil {
				return nil, ErrFetch
			}
			dialer := net.Dialer{Timeout: 10 * time.Second}
			var last error
			for _, ip := range ips {
				conn, err := dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
				if err == nil {
					return conn, nil
				}
				last = err
			}
			if last == nil {
				last = ErrFetch
			}
			return nil, last
		},
	}}
}

type Fetcher struct {
	Client      HTTPDoer
	ValidateURL func(string) error
}

func (f Fetcher) Get(ctx context.Context, raw string, limit int64) ([]byte, error) {
	if f.Client == nil || limit < 1 {
		return nil, ErrFetch
	}
	if f.ValidateURL == nil {
		f.ValidateURL = ValidatePublicHTTPS
	}
	if err := f.ValidateURL(raw); err != nil {
		return nil, ErrFetch
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	if err != nil {
		return nil, ErrFetch
	}
	resp, err := f.Client.Do(req)
	if err != nil || resp == nil {
		return nil, ErrFetch
	}
	defer resp.Body.Close()
	// A client that followed a redirect is rejected too.  The command's client
	// also stops redirects, so this is defense in depth for injected clients.
	if resp.StatusCode != http.StatusOK || resp.Request == nil || resp.Request.URL.String() != req.URL.String() || resp.ContentLength > limit {
		return nil, ErrFetch
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil || int64(len(b)) > limit {
		return nil, ErrFetch
	}
	return b, nil
}

func (f Fetcher) Release(ctx context.Context, urls ReleaseURLs) (binary, metadata, signature []byte, err error) {
	if !sameOrigin(urls.Binary, urls.Metadata) || !sameOrigin(urls.Binary, urls.Signature) {
		return nil, nil, nil, ErrFetch
	}
	if binary, err = f.Get(ctx, urls.Binary, MaxBinaryBytes); err != nil {
		return nil, nil, nil, ErrFetch
	}
	if metadata, err = f.Get(ctx, urls.Metadata, MaxMetadataBytes); err != nil {
		return nil, nil, nil, ErrFetch
	}
	if signature, err = f.Get(ctx, urls.Signature, MaxSignatureBytes); err != nil {
		return nil, nil, nil, ErrFetch
	}
	return binary, metadata, signature, nil
}

func sameOrigin(a, b string) bool {
	ua, ea := parseReleaseURL(a)
	ub, eb := parseReleaseURL(b)
	return ea == nil && eb == nil && ua.Scheme == ub.Scheme && ua.Host == ub.Host
}
