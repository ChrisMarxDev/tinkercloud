// Package client is the narrow deployer HTTP contract; it has no server internals.
package client

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/ChrisMarxDev/tinkercloud/internal/compatibility"
)

var ErrUnauthorized = errors.New("not authorized")
var ErrNotFound = errors.New("not found")
var ErrConflict = errors.New("conflict")
var ErrValidation = errors.New("validation failed")
var ErrQuotaExceeded = errors.New("quota exceeded")

// BuildVersion is injected into released tinker binaries. Development builds
// omit compatibility headers so older V1 servers retain the documented
// migration behavior.
var BuildVersion = "dev"

// ErrRateLimited is intentionally response-detail free. Authentication clients
// can react consistently to a retryable throttling result without exposing a
// server body, request metadata, or the submitted email address.
var ErrRateLimited = errors.New("rate limited")

// ErrIncompatibleServer is deliberately detail-free: first-run setup can tell
// a deployer that a host was not verified without reflecting a response body,
// redirect target, or transport detail into terminal output.
var ErrIncompatibleServer = errors.New("incompatible server")

func IdempotencyKey() (string, error) {
	b := make([]byte, 24)
	if _, e := rand.Read(b); e != nil {
		return "", e
	}
	return "idem_" + hex.EncodeToString(b), nil
}

var ErrDeploymentEvidence = errors.New("deployment verification incomplete")
var ErrAppUnavailable = errors.New("app is unavailable to this deployer")

const (
	maxAnonymousDenyEvidenceBytes int64 = 32 << 10
	deploymentRequestTimeout            = 60 * time.Second
	publicProbeBudget                   = 45 * time.Second
	publicProbeAttemptTimeout           = 5 * time.Second
	publicProbeRetryDelay               = time.Second
	publicProbeMaxAttempts              = 10
)

var gatewayRequestID = regexp.MustCompile(`^req_[0-9a-f]{24}$`)

type Store interface {
	Get(string) (string, error)
	Put(string, string) error
}

// CredentialDeleter is intentionally separate from Store so normal commands
// cannot delete a bearer merely by receiving a broader storage dependency.
type CredentialDeleter interface{ Delete(string) error }

// DefaultServerStore persists only the non-secret normalized platform URL.
// Keeping it separate from bearer storage makes its mutation explicit in the
// successful interactive-login path.
type DefaultServerStore interface {
	DefaultServer() (string, error)
	SetDefaultServer(string) error
}
type Client struct {
	Base, Token string
	HTTP        *http.Client
	// PublicAcknowledged is explicit deployer confirmation carried to the
	// control request; a public manifest never implies this value.
	PublicAcknowledged bool

	// These private controls keep production retry bounds fixed while allowing
	// package tests to exercise retries without sleeping for real-world DNS/TLS
	// propagation windows.
	publicProbeBudget         time.Duration
	publicProbeAttemptTimeout time.Duration
	publicProbeRetryDelay     time.Duration
	publicProbeMaxAttempts    int
}

// AppSummary is deliberately ownership-scoped: the control API only returns
// apps belonging to the authenticated deployer, so its absence is never used
// to learn whether another deployer owns a slug.
type AppSummary struct {
	Slug   string `json:"slug"`
	Status string `json:"status"`
}

func (c Client) ListApps(ctx context.Context) ([]AppSummary, error) {
	var out []AppSummary
	if err := c.Do(ctx, "GET", "/api/v1/apps", "", nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c Client) CreateApp(ctx context.Context, slug, key string) error {
	if slug == "" || key == "" {
		return ErrAppUnavailable
	}
	return c.Do(ctx, "POST", "/api/v1/apps", key, map[string]string{"slug": slug}, nil)
}

// EnsureApp makes `tinker deploy` self-contained without turning a conflicting
// foreign slug into success. A create conflict is accepted only after a fresh,
// server-scoped list proves the authenticated deployer owns that exact slug.
func (c Client) EnsureApp(ctx context.Context, slug string) error {
	apps, err := c.ListApps(ctx)
	if err != nil {
		return ErrAppUnavailable
	}
	for _, app := range apps {
		if app.Slug == slug && app.Status == "active" {
			return nil
		}
	}
	key, err := IdempotencyKey()
	if err != nil {
		return err
	}
	if err = c.CreateApp(ctx, slug, key); err == nil {
		return nil
	}
	// A concurrent request by this deployer may have created the app first.
	// Check only our own list; any other conflict remains denied.
	apps, listErr := c.ListApps(ctx)
	if listErr == nil {
		for _, app := range apps {
			if app.Slug == slug && app.Status == "active" {
				return nil
			}
		}
	}
	return ErrAppUnavailable
}

// TokenInput deliberately carries only the least-privilege, server-validated
// controls a deployer can select. The raw token never appears in a list result.
type TokenInput struct {
	Scopes           []string `json:"scopes"`
	ExpiresInSeconds int64    `json:"expires_in_seconds"`
}

// TokenResult is returned only by token creation. Token is display-once data:
// callers must not persist or log it after handing it to the user.
type TokenResult struct {
	ID        string   `json:"id"`
	Token     string   `json:"token"`
	Scopes    []string `json:"scopes"`
	ExpiresAt string   `json:"expires_at"`
}

// TokenSummary is safe to print or persist in command output.
type TokenSummary struct {
	ID         string   `json:"id"`
	Scopes     []string `json:"scopes"`
	ExpiresAt  string   `json:"expires_at"`
	LastUsedAt *string  `json:"last_used_at"`
	Revoked    bool     `json:"revoked"`
}

func (c *Client) ListTokens(ctx context.Context, slug string) ([]TokenSummary, error) {
	var out []TokenSummary
	if err := c.Do(ctx, "GET", "/api/v1/apps/"+url.PathEscape(slug)+"/tokens", "", nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) CreateToken(ctx context.Context, slug string, in TokenInput, key string) (TokenResult, error) {
	var out TokenResult
	if err := c.Do(ctx, "POST", "/api/v1/apps/"+url.PathEscape(slug)+"/tokens", key, in, &out); err != nil {
		return TokenResult{}, err
	}
	return out, nil
}

func (c *Client) RevokeToken(ctx context.Context, slug, id, key string) error {
	return c.Do(ctx, "DELETE", "/api/v1/apps/"+url.PathEscape(slug)+"/tokens/"+url.PathEscape(id), key, nil, nil)
}

type Release struct {
	ID          string `json:"id"`
	ReleaseHash string `json:"release_hash,omitempty"`
	State       string `json:"state"`
	CreatedAt   string `json:"created_at"`
	VerifiedAt  string `json:"verified_at,omitempty"`
	ActivatedAt string `json:"activated_at,omitempty"`
}

func (c *Client) ListReleases(ctx context.Context, slug string) ([]Release, error) {
	var out []Release
	if err := c.Do(ctx, "GET", "/api/v1/apps/"+url.PathEscape(slug)+"/releases", "", nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// DeleteApp sends the exact server-validated confirmation binding. Its caller
// must obtain explicit user approval before invoking this irreversible action.
func (c *Client) DeleteApp(ctx context.Context, slug, key string) error {
	return c.Do(ctx, "DELETE", "/api/v1/apps/"+url.PathEscape(slug), key, map[string]string{"confirmation": "delete:" + slug}, nil)
}

// Logout revokes exactly the CLI bearer on this request. It has no app target,
// does not receive cookies, and cannot affect browser or viewer sessions.
func (c Client) Logout(ctx context.Context) error {
	return c.Do(ctx, "POST", "/api/v1/auth/logout", "", nil, nil)
}

func NormalizeServer(raw string, allowHTTP bool) (string, error) {
	u, e := url.Parse(raw)
	if e != nil || u.Host == "" || (u.Scheme != "https" && !(allowHTTP && u.Scheme == "http")) {
		return "", errors.New("invalid server URL")
	}
	u.Path = ""
	u.RawQuery = ""
	u.Fragment = ""
	return strings.TrimRight(u.String(), "/"), nil
}
func New(base, token string) Client {
	u, _ := url.Parse(base)
	h := &http.Client{Timeout: 15 * time.Second}
	h.CheckRedirect = func(r *http.Request, _ []*http.Request) error {
		if r.URL.Scheme != u.Scheme || r.URL.Host != u.Host {
			return errors.New("cross-origin redirect denied")
		}
		return nil
	}
	return Client{
		Base:                      base,
		Token:                     token,
		HTTP:                      h,
		publicProbeBudget:         publicProbeBudget,
		publicProbeAttemptTimeout: publicProbeAttemptTimeout,
		publicProbeRetryDelay:     publicProbeRetryDelay,
		publicProbeMaxAttempts:    publicProbeMaxAttempts,
	}
}

type Prompt interface{ Ask(string) (string, error) }
type LoginResult struct {
	Token      string `json:"token"`
	Email      string `json:"email"`
	APIVersion int    `json:"api_version"`
}
type DeploymentResult struct {
	DeploymentID         string                `json:"deployment_id"`
	URL                  string                `json:"url"`
	Domain               string                `json:"domain"`
	PolicyReady          bool                  `json:"policy_ready"`
	TLSReady             bool                  `json:"tls_ready"`
	AnonymousDenied      bool                  `json:"anonymous_denied"`
	AuthenticatedHealthy bool                  `json:"authenticated_healthy"`
	Posture              DeploymentPosture     `json:"posture,omitempty"`
	PublicStatic         *PublicStaticEvidence `json:"public_static,omitempty"`
}

type DeploymentPosture string

const (
	PosturePrivate      DeploymentPosture = "private"
	PosturePublicStatic DeploymentPosture = "public_static"
)

// PublicStaticEvidence contains only immutable candidate evidence, never a
// release path or credential. Hashes are lowercase SHA-256 hex.
type PublicStaticEvidence struct {
	RootSHA256  string `json:"root_sha256"`
	AssetPath   string `json:"asset_path,omitempty"`
	AssetSHA256 string `json:"asset_sha256,omitempty"`
	Indexing    bool   `json:"indexing"`
}

var ErrDeploymentFailed = errors.New("deployment failed")

// DeploymentEvidenceReason is a deliberately small, safe category suitable for
// CLI output. It never includes a raw URL error, response body, header, policy
// detail, path, bearer, or cookie.
type DeploymentEvidenceReason string

const (
	EvidenceActivationIncomplete DeploymentEvidenceReason = "activation_evidence_incomplete"
	EvidencePublicProbeCancelled DeploymentEvidenceReason = "public_probe_cancelled"
	EvidencePublicProbeInvalid   DeploymentEvidenceReason = "public_probe_invalid_response"
	EvidencePublicProbeNotReady  DeploymentEvidenceReason = "public_probe_not_ready"
	EvidencePublicProbeTransport DeploymentEvidenceReason = "public_probe_transport"
	EvidencePublicProbeURL       DeploymentEvidenceReason = "public_probe_untrusted_url"
)

// ActiveButUnverifiedError means the server returned a successful activation,
// but the deployer-side independent public proof did not complete. Callers must
// not report success, but they may safely show this activation receipt.
type ActiveButUnverifiedError struct {
	Deployment DeploymentResult
	Reason     DeploymentEvidenceReason
}

func (e *ActiveButUnverifiedError) Error() string {
	return "active deployment public verification incomplete"
}
func (e *ActiveButUnverifiedError) Unwrap() error { return ErrDeploymentEvidence }

// ActivationFailureReason is the small, public diagnostic category returned
// when a verified candidate cannot be activated. It is intentionally an
// allowlist: server messages, headers, transport errors, and policy detail are
// never carried into deployer output.
type ActivationFailureReason string

const (
	ActivationPolicyNotReady       ActivationFailureReason = "activation_policy_not_ready"
	ActivationCertificateNotReady  ActivationFailureReason = "activation_certificate_not_ready"
	ActivationCandidateProbeFailed ActivationFailureReason = "activation_candidate_probe_failed"
	ActivationCapabilityNotReady   ActivationFailureReason = "activation_capability_not_ready"
	ActivationCommitFailed         ActivationFailureReason = "activation_commit_failed"
)

// ActivationFailedError means the server did not commit activation for a
// previously verified candidate. It is not an active receipt: State is always
// the literal pre-activation state, verified.
type ActivationFailedError struct {
	DeploymentID string
	State        string
	Reason       ActivationFailureReason
	RequestID    string
}

func (e *ActivationFailedError) Error() string { return "deployment activation failed" }
func (e *ActivationFailedError) Unwrap() error { return ErrDeploymentFailed }

type publicVerificationError struct {
	reason    DeploymentEvidenceReason
	retryable bool
}

func (e *publicVerificationError) Error() string { return string(e.reason) }
func (e *publicVerificationError) Unwrap() error { return ErrDeploymentEvidence }

func (c Client) Deploy(ctx context.Context, slug string, archive io.Reader, size int64, key string) (DeploymentResult, error) {
	if slug == "" || key == "" || size < 0 {
		return DeploymentResult{}, ErrDeploymentFailed
	}
	u, e := url.Parse(c.Base)
	if e != nil || u.Scheme != "https" {
		return DeploymentResult{}, errors.New("HTTPS required")
	}
	req, e := http.NewRequestWithContext(ctx, "POST", c.Base+"/api/v1/apps/"+url.PathEscape(slug)+"/deployments", io.LimitReader(archive, size))
	if e != nil {
		return DeploymentResult{}, e
	}
	req.Header.Set("Content-Type", "application/gzip")
	setCompatibilityHeaders(req)
	req.Header.Set("Idempotency-Key", key)
	req.Header.Set("Authorization", "Bearer "+c.Token)
	if c.PublicAcknowledged {
		req.Header.Set("X-Tinker-Public-Acknowledged", "true")
	}
	h := c.deploymentHTTPClient()
	res, e := h.Do(req)
	if e != nil {
		return DeploymentResult{}, e
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode > 299 {
		return DeploymentResult{}, ErrDeploymentFailed
	}
	var out struct {
		DeploymentResult
		StatusURL string `json:"status_url"`
		State     string `json:"state"`
	}
	if e = json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&out); e != nil {
		return DeploymentResult{}, e
	}
	activateVerified := func(deploymentID string) (DeploymentResult, error) {
		if deploymentID == "" {
			return DeploymentResult{}, ErrDeploymentFailed
		}
		k, e := IdempotencyKey()
		if e != nil {
			return DeploymentResult{}, e
		}
		path := "/api/v1/apps/" + url.PathEscape(slug) + "/deployments/" + url.PathEscape(deploymentID) + "/activate"
		req, e := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(c.Base, "/")+path, nil)
		if e != nil {
			return DeploymentResult{}, e
		}
		setCompatibilityHeaders(req)
		req.Header.Set("Idempotency-Key", k)
		req.Header.Set("Authorization", "Bearer "+c.Token)
		res, e := h.Do(req)
		if e != nil {
			return DeploymentResult{}, e
		}
		defer res.Body.Close()
		if res.StatusCode != http.StatusOK {
			if failed := activationFailure(res, deploymentID); failed != nil {
				return DeploymentResult{DeploymentID: deploymentID}, failed
			}
			return DeploymentResult{}, ErrDeploymentFailed
		}
		var activated DeploymentResult
		if e = json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&activated); e != nil {
			return DeploymentResult{}, e
		}
		if e = c.verifyPublicDeployment(ctx, slug, activated); e != nil {
			return activated, activeButUnverified(activated, e)
		}
		return activated, nil
	}
	if out.State == "verified" {
		return activateVerified(out.DeploymentID)
	}
	if out.StatusURL == "" {
		return DeploymentResult{}, out.DeploymentResult.Verified()
	}
	statusURL := out.StatusURL
	su, e := url.Parse(statusURL)
	if e != nil || su.Scheme != u.Scheme || su.Host != u.Host {
		return DeploymentResult{}, errors.New("cross-origin status URL")
	}
	for i := 0; i < 8; i++ {
		if ctx.Err() != nil {
			return DeploymentResult{}, ctx.Err()
		}
		q, e := http.NewRequestWithContext(ctx, "GET", statusURL, nil)
		if e != nil {
			return DeploymentResult{}, e
		}
		r, e := h.Do(q)
		if e != nil {
			return DeploymentResult{}, e
		}
		e = json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&out)
		r.Body.Close()
		if e != nil {
			return DeploymentResult{}, e
		}
		if out.State == "active" {
			if e = c.verifyPublicDeployment(ctx, slug, out.DeploymentResult); e != nil {
				return out.DeploymentResult, activeButUnverified(out.DeploymentResult, e)
			}
			return out.DeploymentResult, nil
		}
		if out.State == "verified" {
			return activateVerified(out.DeploymentID)
		}
		if out.State == "failed" || out.State == "rejected" {
			return DeploymentResult{}, ErrDeploymentFailed
		}
	}
	return DeploymentResult{}, ErrDeploymentFailed
}

func activationFailure(res *http.Response, deploymentID string) error {
	if res == nil || res.StatusCode != http.StatusConflict || res.Body == nil || !gatewayRequestID.MatchString(res.Header.Get("X-Request-ID")) {
		return nil
	}
	body, err := io.ReadAll(io.LimitReader(res.Body, 32<<10+1))
	if err != nil || len(body) > 32<<10 || !uniqueActivationErrorJSON(body) {
		return nil
	}
	var envelope map[string]json.RawMessage
	if json.Unmarshal(body, &envelope) != nil || len(envelope) != 1 {
		return nil
	}
	raw, ok := envelope["error"]
	if !ok {
		return nil
	}
	var detail map[string]json.RawMessage
	if json.Unmarshal(raw, &detail) != nil || len(detail) != 3 {
		return nil
	}
	var code, message, requestID string
	if json.Unmarshal(detail["code"], &code) != nil ||
		json.Unmarshal(detail["message"], &message) != nil ||
		json.Unmarshal(detail["request_id"], &requestID) != nil ||
		message == "" || !gatewayRequestID.MatchString(requestID) || requestID != res.Header.Get("X-Request-ID") {
		return nil
	}
	reason, ok := activationFailureReasons[code]
	if !ok {
		return nil
	}
	return &ActivationFailedError{
		DeploymentID: deploymentID,
		State:        "verified",
		Reason:       reason,
		RequestID:    requestID,
	}
}

// uniqueActivationErrorJSON rejects duplicate object members before the error
// envelope is decoded into maps. json.Unmarshal otherwise keeps the later
// value, which could turn an ambiguous server response into a trusted receipt.
func uniqueActivationErrorJSON(raw []byte) bool {
	d := json.NewDecoder(bytes.NewReader(raw))
	if !consumeActivationErrorJSON(d, 0) {
		return false
	}
	var trailing any
	return d.Decode(&trailing) == io.EOF
}

func consumeActivationErrorJSON(d *json.Decoder, depth int) bool {
	if depth > 16 {
		return false
	}
	token, err := d.Token()
	if err != nil {
		return false
	}
	switch token {
	case json.Delim('{'):
		seen := map[string]struct{}{}
		for d.More() {
			key, err := d.Token()
			name, ok := key.(string)
			if err != nil || !ok {
				return false
			}
			if _, duplicate := seen[name]; duplicate {
				return false
			}
			seen[name] = struct{}{}
			if !consumeActivationErrorJSON(d, depth+1) {
				return false
			}
		}
		end, err := d.Token()
		return err == nil && end == json.Delim('}')
	case json.Delim('['):
		for d.More() {
			if !consumeActivationErrorJSON(d, depth+1) {
				return false
			}
		}
		end, err := d.Token()
		return err == nil && end == json.Delim(']')
	default:
		return true
	}
}

var activationFailureReasons = map[string]ActivationFailureReason{
	string(ActivationPolicyNotReady):       ActivationPolicyNotReady,
	string(ActivationCertificateNotReady):  ActivationCertificateNotReady,
	string(ActivationCandidateProbeFailed): ActivationCandidateProbeFailed,
	string(ActivationCapabilityNotReady):   ActivationCapabilityNotReady,
	string(ActivationCommitFailed):         ActivationCommitFailed,
}

// verifyPublicDeployment is the deployer's independent, network-facing gate.
// Server activation evidence is necessary but cannot prove the public DNS/TLS
// gateway path that a viewer will reach. The probe therefore uses the returned
// app URL with a fresh anonymous client; in particular, it never reuses the
// deployer client's bearer token or cookie jar.
func (c Client) verifyPublicDeployment(ctx context.Context, slug string, result DeploymentResult) error {
	if err := result.Verified(); err != nil {
		return publicEvidenceError(EvidenceActivationIncomplete, false)
	}
	probeURL, err := expectedAppURL(slug, result.Domain, result.URL)
	if err != nil {
		return publicEvidenceError(EvidencePublicProbeURL, false)
	}
	budget := c.publicProbeBudget
	if budget <= 0 {
		budget = publicProbeBudget
	}
	attempts := c.publicProbeMaxAttempts
	if attempts <= 0 {
		attempts = publicProbeMaxAttempts
	}
	probeCtx, cancel := context.WithTimeout(ctx, budget)
	defer cancel()

	var last *publicVerificationError
	for attempt := 0; attempt < attempts; attempt++ {
		if probeCtx.Err() != nil {
			return publicEvidenceError(EvidencePublicProbeCancelled, false)
		}
		if result.Posture == PosturePublicStatic {
			err = c.probePublicStaticOnce(probeCtx, probeURL, result.PublicStatic)
		} else if result.Posture == "" || result.Posture == PosturePrivate {
			err = c.probePublicDeploymentOnce(probeCtx, probeURL)
		} else {
			return publicEvidenceError(EvidencePublicProbeInvalid, false)
		}
		if err == nil {
			return nil
		}
		if !errors.As(err, &last) || !last.retryable {
			return err
		}
		if attempt == attempts-1 {
			return last
		}
		delay := c.publicProbeRetryDelay
		if delay <= 0 {
			delay = publicProbeRetryDelay
		}
		timer := time.NewTimer(delay)
		select {
		case <-probeCtx.Done():
			timer.Stop()
			return publicEvidenceError(EvidencePublicProbeCancelled, false)
		case <-timer.C:
		}
	}
	return last
}

func (c Client) probePublicStaticOnce(ctx context.Context, root *url.URL, evidence *PublicStaticEvidence) error {
	if evidence == nil || len(evidence.RootSHA256) != 64 || (evidence.AssetPath == "") != (evidence.AssetSHA256 == "") || (evidence.AssetSHA256 != "" && len(evidence.AssetSHA256) != 64) {
		return publicEvidenceError(EvidencePublicProbeInvalid, false)
	}
	check := func(u *url.URL, hash string, document bool) error {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
		if err != nil {
			return publicEvidenceError(EvidencePublicProbeURL, false)
		}
		resp, err := c.anonymousHTTPClient().Do(req)
		if err != nil {
			return publicEvidenceError(EvidencePublicProbeTransport, true)
		}
		defer resp.Body.Close()
		if resp.Request == nil || resp.Request.URL.String() != req.URL.String() || resp.StatusCode != http.StatusOK || resp.Header.Get("Set-Cookie") != "" || resp.Header.Get("Location") != "" {
			return publicEvidenceError(EvidencePublicProbeInvalid, false)
		}
		if document {
			want := "noindex, nofollow"
			if evidence.Indexing {
				want = ""
			}
			if resp.Header.Get("X-Robots-Tag") != want {
				return publicEvidenceError(EvidencePublicProbeInvalid, false)
			}
		}
		body, err := io.ReadAll(io.LimitReader(resp.Body, maxAnonymousDenyEvidenceBytes+1))
		if err != nil || len(body) > int(maxAnonymousDenyEvidenceBytes) {
			return publicEvidenceError(EvidencePublicProbeInvalid, false)
		}
		got := fmt.Sprintf("%x", sha256.Sum256(body))
		if got != hash {
			return publicEvidenceError(EvidencePublicProbeInvalid, false)
		}
		return nil
	}
	if err := check(root, evidence.RootSHA256, true); err != nil {
		return err
	}
	if evidence.AssetPath != "" {
		asset := *root
		asset.Path = "/" + strings.TrimPrefix(evidence.AssetPath, "/")
		if err := check(&asset, evidence.AssetSHA256, false); err != nil {
			return err
		}
	}
	for _, p := range []string{"/_tinker/auth/login", "/_tinker/api/v1/app", "/_tinker/api/v1/kv", "/_tinker/ws/v1"} {
		u := *root
		u.Path = p
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
		resp, err := c.anonymousHTTPClient().Do(req)
		if err != nil {
			return publicEvidenceError(EvidencePublicProbeTransport, true)
		}
		if resp.Body != nil {
			io.Copy(io.Discard, io.LimitReader(resp.Body, maxAnonymousDenyEvidenceBytes))
			resp.Body.Close()
		}
		if resp.StatusCode < 400 || resp.StatusCode >= 500 || resp.Header.Get("Set-Cookie") != "" || resp.Header.Get("Location") != "" {
			return publicEvidenceError(EvidencePublicProbeInvalid, false)
		}
	}
	return nil
}

func (c Client) probePublicDeploymentOnce(ctx context.Context, probeURL *url.URL) error {
	attemptTimeout := c.publicProbeAttemptTimeout
	if attemptTimeout <= 0 {
		attemptTimeout = publicProbeAttemptTimeout
	}
	attemptCtx, cancel := context.WithTimeout(ctx, attemptTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(attemptCtx, http.MethodGet, probeURL.String(), nil)
	if err != nil {
		return publicEvidenceError(EvidencePublicProbeURL, false)
	}
	// The zero values are intentional assertions for custom transports as well
	// as documentation for future callers: a platform deployer token must never
	// cross onto an untrusted app origin.
	req.Header.Del("Authorization")
	req.Header.Del("Cookie")
	resp, err := c.anonymousHTTPClient().Do(req)
	if resp != nil && resp.Body != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		if resp != nil && resp.StatusCode >= 300 && resp.StatusCode < 400 {
			return publicEvidenceError(EvidencePublicProbeInvalid, false)
		}
		return publicEvidenceError(EvidencePublicProbeTransport, true)
	}
	if resp == nil || resp.Body == nil || resp.Request == nil || resp.Request.URL == nil {
		return publicEvidenceError(EvidencePublicProbeInvalid, false)
	}
	if resp.Request.URL.String() != req.URL.String() {
		return publicEvidenceError(EvidencePublicProbeInvalid, false)
	}
	if resp != nil && (resp.StatusCode == http.StatusNotFound ||
		resp.StatusCode == http.StatusBadGateway ||
		resp.StatusCode == http.StatusServiceUnavailable ||
		resp.StatusCode == http.StatusGatewayTimeout) {
		return publicEvidenceError(EvidencePublicProbeNotReady, true)
	}
	if resp.StatusCode != http.StatusUnauthorized ||
		resp.Header.Get("Cache-Control") != "no-store" ||
		resp.Header.Get("X-Content-Type-Options") != "nosniff" {
		return publicEvidenceError(EvidencePublicProbeInvalid, false)
	}
	contentType, _, err := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if err != nil || contentType != "application/json" {
		return publicEvidenceError(EvidencePublicProbeInvalid, false)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxAnonymousDenyEvidenceBytes+1))
	if err != nil || int64(len(body)) > maxAnonymousDenyEvidenceBytes {
		return publicEvidenceError(EvidencePublicProbeInvalid, false)
	}
	var envelope map[string]json.RawMessage
	if err = json.Unmarshal(body, &envelope); err != nil || len(envelope) != 1 {
		return publicEvidenceError(EvidencePublicProbeInvalid, false)
	}
	var denial struct {
		Code      string `json:"code"`
		Message   string `json:"message"`
		RequestID string `json:"request_id"`
	}
	raw, ok := envelope["error"]
	if !ok || json.Unmarshal(raw, &denial) != nil {
		return publicEvidenceError(EvidencePublicProbeInvalid, false)
	}
	if denial.Code != "not_authorized" ||
		denial.Message != "This request is not authorized." ||
		!gatewayRequestID.MatchString(denial.RequestID) ||
		denial.RequestID != resp.Header.Get("X-Request-ID") {
		return publicEvidenceError(EvidencePublicProbeInvalid, false)
	}
	return nil
}

func publicEvidenceError(reason DeploymentEvidenceReason, retryable bool) error {
	return &publicVerificationError{reason: reason, retryable: retryable}
}

func activeButUnverified(result DeploymentResult, err error) error {
	reason := EvidenceActivationIncomplete
	var verification *publicVerificationError
	if errors.As(err, &verification) {
		reason = verification.reason
	}
	return &ActiveButUnverifiedError{Deployment: result, Reason: reason}
}

func expectedAppURL(slug, suffix, raw string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil || slug == "" || suffix == "" || u.Scheme != "https" || u.User != nil || u.Host == "" || u.RawQuery != "" || u.Fragment != "" || u.Path != "/" {
		return nil, errors.New("invalid app URL")
	}
	if !strings.EqualFold(u.Hostname(), slug+"."+suffix) || u.Port() != "" {
		return nil, errors.New("unexpected app URL")
	}
	return u, nil
}

func (c Client) anonymousHTTPClient() *http.Client {
	base := c.HTTP
	if base == nil {
		base = http.DefaultClient
	}
	// Copy the selected transport (including its TLS settings) but deliberately
	// drop the cookie jar and disable all redirects. The latter ensures a 401
	// cannot be manufactured by a different final origin.
	anonymous := *base
	anonymous.Jar = nil
	anonymous.CheckRedirect = func(*http.Request, []*http.Request) error {
		return errors.New("deployment probe redirect denied")
	}
	attemptTimeout := c.publicProbeAttemptTimeout
	if attemptTimeout <= 0 {
		attemptTimeout = publicProbeAttemptTimeout
	}
	if anonymous.Timeout <= 0 || anonymous.Timeout > attemptTimeout {
		anonymous.Timeout = attemptTimeout
	}
	return &anonymous
}

func (c Client) deploymentHTTPClient() *http.Client {
	base := c.HTTP
	if base == nil {
		base = http.DefaultClient
	}
	deployment := *base
	if deployment.Timeout <= 0 || deployment.Timeout < deploymentRequestTimeout {
		deployment.Timeout = deploymentRequestTimeout
	}
	return &deployment
}

func (r DeploymentResult) Verified() error {
	if r.DeploymentID == "" || r.URL == "" || !r.PolicyReady || !r.TLSReady {
		return ErrDeploymentEvidence
	}
	if r.Posture == PosturePublicStatic {
		if r.PublicStatic == nil || len(r.PublicStatic.RootSHA256) != 64 {
			return ErrDeploymentEvidence
		}
		return nil
	}
	if r.Posture != "" && r.Posture != PosturePrivate {
		return ErrDeploymentEvidence
	}
	if !r.AnonymousDenied || !r.AuthenticatedHealthy {
		return ErrDeploymentEvidence
	}
	return nil
}

// VerifyLoginSession proves both server compatibility and the bearer token's
// currently authenticated deployer identity. It never treats a dependency
// failure as an authorization failure: callers may fall back to OTP only for a
// definite ErrUnauthorized result.
func VerifyLoginSession(ctx context.Context, c Client) (LoginResult, error) {
	var v struct {
		APIVersion int `json:"api_version"`
	}
	if e := c.Do(ctx, "GET", "/api/v1/version", "", nil, &v); e != nil {
		return LoginResult{}, e
	}
	if v.APIVersion != 1 {
		return LoginResult{}, errors.New("incompatible server API")
	}
	var me struct {
		Email string `json:"email"`
	}
	if e := c.Do(ctx, "GET", "/api/v1/whoami", "", nil, &me); e != nil {
		return LoginResult{}, e
	}
	if me.Email == "" {
		return LoginResult{}, ErrUnauthorized
	}
	return LoginResult{Token: c.Token, Email: me.Email, APIVersion: v.APIVersion}, nil
}

// VerifyServerCompatibility is the unauthenticated first-run server proof. It
// accepts only the exact HTTPS control origin, a non-redirected 200 response,
// and a bounded complete API-v1 JSON document. It deliberately does not prove
// deployer authorization; callers still handle a missing local bearer as
// Login required after the selected platform is safely persisted.
func VerifyServerCompatibility(ctx context.Context, c Client) error {
	base, err := NormalizeServer(c.Base, false)
	if err != nil || base != c.Base {
		return ErrIncompatibleServer
	}
	h := c.HTTP
	if h == nil {
		h = http.DefaultClient
	}
	strict := *h
	strict.CheckRedirect = func(*http.Request, []*http.Request) error {
		return ErrIncompatibleServer
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/api/v1/version", nil)
	if err != nil {
		return ErrIncompatibleServer
	}
	res, err := strict.Do(req)
	if err != nil {
		return ErrIncompatibleServer
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return ErrIncompatibleServer
	}
	body, err := io.ReadAll(io.LimitReader(res.Body, 32<<10+1))
	if err != nil || len(body) > 32<<10 {
		return ErrIncompatibleServer
	}
	var out struct {
		APIVersion int `json:"api_version"`
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	if decoder.Decode(&out) != nil || decoder.Decode(&struct{}{}) != io.EOF || out.APIVersion != 1 {
		return ErrIncompatibleServer
	}
	return nil
}

// LoginWithClient completes an interactive login using the supplied client.
// The post-verification whoami call is mandatory: the server, rather than the
// OTP response, is authoritative for the persisted credential's identity.
func LoginWithClient(ctx context.Context, c Client, p Prompt) (LoginResult, error) {
	var v struct {
		APIVersion int `json:"api_version"`
	}
	if e := c.Do(ctx, "GET", "/api/v1/version", "", nil, &v); e != nil {
		return LoginResult{}, e
	}
	if v.APIVersion != 1 {
		return LoginResult{}, errors.New("incompatible server API")
	}
	email, e := p.Ask("Email: ")
	if e != nil {
		return LoginResult{}, e
	}
	var tx struct {
		Transaction string `json:"transaction"`
	}
	if e = c.Do(ctx, "POST", "/api/v1/auth/otp", "", map[string]string{"email": email}, &tx); e != nil {
		return LoginResult{}, e
	}
	code, e := p.Ask("Code: ")
	if e != nil {
		return LoginResult{}, e
	}
	var out LoginResult
	if e = c.Do(ctx, "POST", "/api/v1/auth/verify", "", map[string]string{"transaction": tx.Transaction, "code": code}, &out); e != nil {
		return LoginResult{}, e
	}
	if out.Token == "" {
		return LoginResult{}, ErrUnauthorized
	}
	verifiedClient := c
	verifiedClient.Token = out.Token
	verified, e := VerifyLoginSession(ctx, verifiedClient)
	if e != nil {
		return LoginResult{}, e
	}
	return verified, nil
}

// Login preserves the small package-level entry point used by external callers
// while command code can inject a client transport through LoginWithClient.
func Login(ctx context.Context, base string, p Prompt) (LoginResult, error) {
	return LoginWithClient(ctx, New(base, ""), p)
}

func (c Client) Do(ctx context.Context, method, path, key string, in, out any) error {
	var b bytes.Buffer
	if in != nil {
		if e := json.NewEncoder(&b).Encode(in); e != nil {
			return e
		}
	}
	r, e := http.NewRequestWithContext(ctx, method, strings.TrimRight(c.Base, "/")+path, &b)
	if e != nil {
		return e
	}
	r.Header.Set("Content-Type", "application/json")
	setCompatibilityHeaders(r)
	if c.Token != "" {
		r.Header.Set("Authorization", "Bearer "+c.Token)
	}
	if key != "" {
		r.Header.Set("Idempotency-Key", key)
	}
	h := c.HTTP
	if h == nil {
		h = http.DefaultClient
	}
	res, e := h.Do(r)
	if e != nil {
		return e
	}
	defer res.Body.Close()
	if res.StatusCode == 401 || res.StatusCode == 403 {
		return ErrUnauthorized
	}
	if res.StatusCode == http.StatusNotFound {
		return ErrNotFound
	}
	if res.StatusCode == http.StatusConflict {
		return ErrConflict
	}
	if res.StatusCode == http.StatusBadRequest {
		return ErrValidation
	}
	if res.StatusCode == http.StatusTooManyRequests {
		var envelope struct {
			Error struct {
				Code string `json:"code"`
			} `json:"error"`
		}
		if json.NewDecoder(io.LimitReader(res.Body, 32<<10)).Decode(&envelope) == nil && envelope.Error.Code == "quota_exceeded" {
			return ErrQuotaExceeded
		}
		return ErrRateLimited
	}
	if res.StatusCode < 200 || res.StatusCode > 299 {
		return errors.New("control request failed")
	}
	if out != nil {
		return json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(out)
	}
	return nil
}

func setCompatibilityHeaders(r *http.Request) {
	if _, err := compatibility.Parse(BuildVersion); err != nil {
		return
	}
	r.Header.Set("X-Tinker-CLI-Version", BuildVersion)
	r.Header.Set("X-Tinker-Control-API-Version", compatibility.ControlAPIVersion)
}
