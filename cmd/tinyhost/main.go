// tinyhost is the single public-listener composition root. Production adapters
// (SQLite, mail, TLS and privileged service installation) are intentionally not
// selected in this standard-library development slice.
package main

import (
	"context"
	"crypto/tls"
	"errors"
	"flag"
	"fmt"
	"github.com/tinyhost/tiny/internal/certificates"
	"github.com/tinyhost/tiny/internal/compose"
	"github.com/tinyhost/tiny/internal/config"
	"github.com/tinyhost/tiny/internal/controlapi"
	"github.com/tinyhost/tiny/internal/deployments"
	"github.com/tinyhost/tiny/internal/email"
	"github.com/tinyhost/tiny/internal/jobs"
	"github.com/tinyhost/tiny/internal/live"
	"github.com/tinyhost/tiny/internal/operations"
	"github.com/tinyhost/tiny/internal/persistence"
	"github.com/tinyhost/tiny/internal/ratelimit"
	"github.com/tinyhost/tiny/internal/requestlog"
	"github.com/tinyhost/tiny/internal/verification"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const hstsValue = "max-age=63072000; includeSubDomains"

// httpRedirectHandler is deliberately narrower than the HTTPS gateway. Port
// 80 exists only to let autocert answer HTTP-01 and to move recognised hosts
// to their HTTPS origin. It must never receive an app, control, or login
// handler as its fallback.
func httpRedirectHandler(c config.Config, knownHost func(string) bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host, ok := canonicalHTTPHost(r.Host)
		if !ok || !knownHost(host) {
			http.NotFound(w, r)
			return
		}
		location := "https://" + host + r.URL.RequestURI()
		http.Redirect(w, r, location, http.StatusMovedPermanently)
	})
}

// canonicalHTTPHost accepts an HTTP Host header only when it can be reduced to
// a DNS hostname without retaining browser-controlled syntax. In particular,
// userinfo, paths, control characters, IP literals, and non-HTTP ports are
// rejected before the value is placed in a Location header.
func canonicalHTTPHost(raw string) (string, bool) {
	if raw == "" || strings.ContainsAny(raw, " \t\r\n\x00/@\\?#[]") {
		return "", false
	}
	host := raw
	if strings.Contains(raw, ":") {
		var port string
		var err error
		host, port, err = net.SplitHostPort(raw)
		if err != nil || port != "80" {
			return "", false
		}
	}
	host = strings.TrimSuffix(strings.ToLower(host), ".")
	if !validDNSHost(host) {
		return "", false
	}
	return host, true
}

func validDNSHost(host string) bool {
	if len(host) == 0 || len(host) > 253 || net.ParseIP(host) != nil {
		return false
	}
	for _, label := range strings.Split(host, ".") {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, r := range label {
			if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '-' {
				return false
			}
		}
	}
	return true
}

func httpsSecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Strict-Transport-Security", hstsValue)
		next.ServeHTTP(w, r)
	})
}

type acmeHTTPHandler interface {
	HTTPHandler(http.Handler) http.Handler
}

// publicHandlers keeps the plaintext and HTTPS request graphs structurally
// separate. The supplied gateway is deliberately reachable only through the
// HTTPS return value.
func publicHandlers(c config.Config, acme acmeHTTPHandler, knownHost func(string) bool, gateway http.Handler) (http.Handler, http.Handler) {
	return acme.HTTPHandler(httpRedirectHandler(c, knownHost)), httpsSecurityHeaders(gateway)
}

type hostResolver struct {
	suffix string
	store  *persistence.SQLiteStore
}
type denyDeploymentGates struct{}

func (denyDeploymentGates) Policy(context.Context, deployments.Record) bool      { return false }
func (denyDeploymentGates) Certificate(context.Context, deployments.Record) bool { return false }
func (denyDeploymentGates) Probe(context.Context, deployments.Record) bool       { return false }

func (h hostResolver) ActiveAppHost(host string) bool {
	suffix := "." + h.suffix
	if !strings.HasSuffix(host, suffix) {
		return false
	}
	slug := strings.TrimSuffix(host, suffix)
	if slug == "" || strings.Contains(slug, ".") {
		return false
	}
	return h.store.EligibleAppHost(context.Background(), slug)
}

type resendCredential struct{ key string }

func (c resendCredential) ResendAPIKey() string { return c.key }
func buildHandler(c config.Config, secrets config.Secrets, store *persistence.SQLiteStore, gates deployments.Gates) (http.Handler, *live.Hub, error) {
	hub := live.New(live.DefaultLimits())
	out := email.Resend{Credential: resendCredential{secrets.ResendAPIKey}, From: c.EmailFrom}
	atomic := persistence.AppLogin{Store: store, HMACKey: []byte(secrets.HMACKey), Outbox: out, TTL: c.OTPExpiry, MaxAttempts: c.OTPMaxAttempts}
	liveSessions := compose.LiveSessions{Sessions: store, Hub: hub}
	login := compose.Login{Atomic: atomic, Sessions: liveSessions, Policies: store, SessionTTL: c.SessionExpiry}
	limits := ratelimit.New([]byte(secrets.HMACKey), ratelimit.DefaultConfig())
	login.RateLimits = limits
	if gates == nil {
		gates = denyDeploymentGates{}
	}
	// Config is validated before this composition root is called. The resource
	// seam intentionally reaches only growth paths, never static reads or
	// revocation handlers.
	resources, _ := compose.NewResourceControls(c, operations.StaticDiskSource{Path: c.DataDirectory})
	deploy := &deployments.Service{Repo: persistence.DeploymentRepository{Store: store}, Root: c.DataDirectory, Gates: gates}
	resources.ConfigureDeployments(deploy)
	if err := deploy.RecoverStartup(context.Background(), deployments.FilesystemEvidence{Root: c.DataDirectory}); err != nil {
		return nil, hub, err
	}
	blobs := resources.BlobRepository(store)
	controlAuth := persistence.ControlAuthenticator{Store: store}
	controlService := persistence.ControlService{Store: store, Live: hub, BlobCleanup: blobs, Deployments: deploy, AppSuffix: c.AppSuffix}
	resources.ConfigureControl(&controlService)
	controlLogin := persistence.ControlLogin{Store: store, HMACKey: []byte(secrets.HMACKey), Outbox: out, TTL: c.OTPExpiry, MaxAttempts: c.OTPMaxAttempts}
	platform := controlapi.Platform{API: controlapi.Dispatcher{Auth: controlAuth, Service: controlService, Login: controlLogin, RateLimits: limits, ArchiveUploadBytes: resources.Limits.ArchiveUploadBytes}, Auth: controlAuth, Views: controlService, Actions: controlService, Login: controlLogin, RateLimits: limits}
	if err := blobs.Reconcile(context.Background()); err != nil {
		return nil, hub, err
	}
	return compose.AppPlaneWithPlatformAndBlobs(c, store, liveSessions, store, resources.KVRepository(store), blobs, hub, login, platform), hub, nil
}

var effectiveUID = os.Geteuid
var loadDoctorCredentials = readDoctorCredentials

type serviceIdentity struct{ uid, gid int }

var lookupTinyhostIdentity = func() (serviceIdentity, error) {
	u, err := user.Lookup("tinyhost")
	if err != nil {
		return serviceIdentity{}, err
	}
	uid, err := strconv.Atoi(u.Uid)
	if err != nil || uid < 1 {
		return serviceIdentity{}, errors.New("invalid service uid")
	}
	gid, err := strconv.Atoi(u.Gid)
	if err != nil || gid < 1 {
		return serviceIdentity{}, errors.New("invalid service gid")
	}
	return serviceIdentity{uid: uid, gid: gid}, nil
}

var databaseArtifactOwner = func(info os.FileInfo) (uint32, uint32, bool) {
	stat, ok := info.Sys().(*syscall.Stat_t)
	return stat.Uid, stat.Gid, ok
}

// Lchown avoids following a service-writable directory entry if it is swapped
// after the no-symlink check but before the root-owned handoff syscall.
var chownDeployerDatabaseArtifact = os.Lchown

// prepareDeployerDatabaseOwnership repairs only the known root-created SQLite
// artifacts from the old recovery path. It rejects links, foreign ownership,
// and group/other-writable modes rather than widening any permission.
var prepareDeployerDatabaseOwnership = func(databasePath string) error {
	identity, err := lookupTinyhostIdentity()
	if err != nil {
		return err
	}
	for _, path := range []string{databasePath, databasePath + "-wal", databasePath + "-shm"} {
		if !pathHasNoSymlink(path, true) {
			return errors.New("unsafe database artifact")
		}
		info, err := os.Lstat(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0022 != 0 {
			return errors.New("unsafe database artifact")
		}
		uid, gid, ok := databaseArtifactOwner(info)
		if !ok || (uid != 0 && uid != uint32(identity.uid)) {
			return errors.New("unsafe database artifact")
		}
		if uid != uint32(identity.uid) || gid != uint32(identity.gid) {
			if err = chownDeployerDatabaseArtifact(path, identity.uid, identity.gid); err != nil {
				return err
			}
		}
	}
	return nil
}

// dropToTinyhostIdentity is called only after a root-only command has completed
// its privileged argument/configuration work. It permanently removes root
// before SQLite can create a database, WAL, or SHM artifact.
var dropToTinyhostIdentity = func() error {
	identity, err := lookupTinyhostIdentity()
	if err != nil {
		return err
	}
	if err = syscall.Setgroups([]int{identity.gid}); err != nil {
		return err
	}
	if err = syscall.Setgid(identity.gid); err != nil {
		return err
	}
	return syscall.Setuid(identity.uid)
}

var openDeployerSQLite = persistence.OpenSQLite

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
}
func run(args []string, out, errout *os.File) error {
	if len(args) == 0 {
		return errors.New("tinyhost: usage")
	}
	switch args[0] {
	case "install-service":
		return runInstallService(args[1:])
	case "update":
		if effectiveUID() != 0 {
			return errors.New("tinyhost: root_required")
		}
		return runUpdate(args[1:], out)
	case "verify-artifact":
		return runVerifyArtifact(args[1:], out)
	case "recover":
		if effectiveUID() != 0 {
			return errors.New("tinyhost: root_required")
		}
		fs := flag.NewFlagSet("recover", flag.ContinueOnError)
		cfgPath := fs.String("config", "", "")
		email := fs.String("email", "", "")
		if fs.Parse(args[1:]) != nil || len(fs.Args()) != 1 || fs.Args()[0] != "operator" || *email == "" {
			return errors.New("tinyhost: invalid_arguments")
		}
		cfg, e := config.LoadYAML(*cfgPath)
		if e != nil {
			return errors.New("tinyhost: config_invalid")
		}
		s, e := persistence.OpenSQLite(context.Background(), filepath.Join(cfg.DataDirectory, "tinyhost.db"))
		if e != nil {
			return e
		}
		defer s.Close()
		if e = s.RecoverOperator(context.Background(), *email, "recover_operator"); e != nil {
			return errors.New("tinyhost: recovery_failed")
		}
		fmt.Fprintln(out, "operator recovery complete")
		return nil
	case "deployers":
		if effectiveUID() != 0 {
			return errors.New("tinyhost: root_required")
		}
		if len(args) < 2 {
			return errors.New("tinyhost: invalid_arguments")
		}
		action := args[1]
		status := map[string]string{"authorize": "active", "suspend": "suspended", "revoke": "revoked"}[action]
		if status == "" {
			return errors.New("tinyhost: invalid_action")
		}
		fs := flag.NewFlagSet("deployers", flag.ContinueOnError)
		fs.SetOutput(io.Discard)
		cfgPath := fs.String("config", defaultConfigPath, "")
		if fs.Parse(args[2:]) != nil || len(fs.Args()) != 1 {
			return errors.New("tinyhost: invalid_arguments")
		}
		email := fs.Args()[0]
		cfg, e := config.LoadYAML(*cfgPath)
		if e != nil {
			return errors.New("tinyhost: config_invalid")
		}
		// Root is authorized to begin this recovery action, but it must not own
		// SQLite artifacts afterwards. No subsequent step needs host privilege.
		databasePath := filepath.Join(cfg.DataDirectory, "tinyhost.db")
		if e = prepareDeployerDatabaseOwnership(databasePath); e != nil {
			return errors.New("tinyhost: service_identity_failed")
		}
		if e = dropToTinyhostIdentity(); e != nil {
			return errors.New("tinyhost: service_identity_failed")
		}
		s, e := openDeployerSQLite(context.Background(), databasePath)
		if e != nil {
			return errors.New("tinyhost: deployer_failed")
		}
		mutationErr := s.SetDeployerStatus(context.Background(), email, status, "root_"+action)
		closeErr := s.Close()
		if mutationErr != nil || closeErr != nil {
			return errors.New("tinyhost: deployer_failed")
		}
		fmt.Fprintf(out, "deployer %s: %s\n", action, email)
		return nil
	case "init":
		return runInit(args[1:], out, productionInitRuntime)
	case "serve":
		fs := flag.NewFlagSet("serve", flag.ContinueOnError)
		cfgPath := fs.String("config", "", "")
		if fs.Parse(args[1:]) != nil {
			return errors.New("tinyhost: invalid_arguments")
		}
		cfg, err := config.LoadYAML(*cfgPath)
		if err != nil {
			return err
		}
		secrets, err := cfg.ResolveSecrets(os.Getenv)
		if err != nil {
			return err
		}
		if err = os.MkdirAll(cfg.DataDirectory, 0700); err != nil {
			return err
		}
		store, err := persistence.OpenSQLite(context.Background(), filepath.Join(cfg.DataDirectory, "tinyhost.db"))
		if err != nil {
			return err
		}
		defer store.Close()
		gates := deployments.GateFuncs{PolicyFunc: func(ctx context.Context, r deployments.Record) bool {
			return store.CandidatePolicyReady(ctx, r)
		}, CertificateFunc: func(ctx context.Context, r deployments.Record) bool {
			return certificateReady(ctx, r.AppSlug+"."+cfg.AppSuffix, &http.Client{Timeout: 5 * time.Second})
		}, ProbeFunc: func(ctx context.Context, r deployments.Record) bool {
			p, e := verification.ProbeCandidate(ctx, cfg, cfg.DataDirectory, r)
			return e == nil && p.Passed()
		}}
		h, _, err := buildHandler(cfg, secrets, store, gates)
		if err != nil {
			return err
		}
		logger := slog.New(slog.NewJSONHandler(errout, nil))
		resolver := hostResolver{suffix: cfg.AppSuffix, store: store}
		cm := certificates.NewAutocert(cfg.ACMECachedir, cfg.ACMEEmail, cfg.PlatformHost, resolver, func(string) bool { return true })
		if err = os.MkdirAll(cfg.ACMECachedir, 0700); err != nil {
			return err
		}
		lh, err := net.Listen("tcp", cfg.ListenHTTP)
		if err != nil {
			return err
		}
		defer lh.Close()
		lt, err := net.Listen("tcp", cfg.ListenHTTPS)
		if err != nil {
			return err
		}
		defer lt.Close()
		serverOptions := func(handler http.Handler) *http.Server {
			return &http.Server{Handler: handler, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 << 10}
		}
		// The plaintext listener has no gateway fallback. autocert gets the
		// HTTP-01 route and every other request is either a safe redirect for a
		// configured host or a fail-closed denial.
		plainHTTP, secureHTTPS := publicHandlers(cfg, cm, func(host string) bool {
			return host == cfg.PlatformHost || resolver.ActiveAppHost(host)
		}, h)
		hs := serverOptions(requestlog.Middleware(plainHTTP, logger))
		ts := serverOptions(requestlog.Middleware(secureHTTPS, logger))
		serveCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		cleanup := &jobs.Scheduler{Interval: 6 * time.Hour, Timeout: 2 * time.Minute}
		cleanup.Run = func(ctx context.Context) error {
			removed, e := store.ExecuteCleanup(ctx, cfg.DataDirectory, cfg.Limits.ReleaseRetention, jobs.Executor{})
			outcome := "succeeded"
			if e != nil {
				outcome = "failed"
			}
			// The audit record deliberately carries only a bounded count; it never
			// names release paths, hashes, or any browser/client input.
			if auditErr := store.RecordCleanupOutcome(ctx, outcome, len(removed)); auditErr != nil && e == nil {
				requestlog.Service(logger, "release_cleanup", "failed", len(removed))
				return auditErr
			}
			requestlog.Service(logger, "release_cleanup", outcome, len(removed))
			return e
		}
		cleanupDone := make(chan struct{}, 2)
		go func() { cleanup.Start(serveCtx); cleanupDone <- struct{}{} }()
		go func() { cleanup.RunOnce(serveCtx); cleanupDone <- struct{}{} }()
		waitCleanup := func(ctx context.Context) {
			for range 2 {
				select {
				case <-cleanupDone:
				case <-ctx.Done():
					return
				}
			}
		}
		errs := make(chan error, 2)
		go func() { errs <- hs.Serve(lh) }()
		go func() { errs <- ts.Serve(tls.NewListener(lt, cm.TLSConfig())) }()
		select {
		case <-serveCtx.Done():
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			_ = hs.Shutdown(shutdownCtx)
			_ = ts.Shutdown(shutdownCtx)
			waitCleanup(shutdownCtx)
			return nil
		case e := <-errs:
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			_ = hs.Shutdown(shutdownCtx)
			_ = ts.Shutdown(shutdownCtx)
			stop()
			waitCleanup(shutdownCtx)
			if e == http.ErrServerClosed {
				return nil
			}
			return e
		}
	case "status", "doctor":
		fs := flag.NewFlagSet(args[0], flag.ContinueOnError)
		cfgPath := fs.String("config", defaultConfigPath, "")
		credentialPath := fs.String("credentials", defaultCredentialPath, "")
		candidateHealthCheck := fs.Bool("update-health-check", false, "")
		if fs.Parse(args[1:]) != nil || len(fs.Args()) != 0 || (args[0] != "doctor" && *candidateHealthCheck) {
			return errors.New("tinyhost: invalid_arguments")
		}
		if args[0] == "doctor" && effectiveUID() != 0 {
			return errors.New("tinyhost: root_required")
		}
		cfg, e := config.LoadYAML(*cfgPath)
		if e != nil {
			return errors.New("tinyhost: config_invalid")
		}
		statePath := filepath.Join(filepath.Dir(*cfgPath), "init-state.json")
		checks := diagnose(cfg, statePath)
		if args[0] == "doctor" {
			credentials, credentialErr := loadDoctorCredentials(cfg, *credentialPath)
			if credentialErr != nil {
				credentials = doctorCredentials{}
			}
			if *candidateHealthCheck {
				checks = candidateDoctor(cfg, credentials, statePath)
			} else {
				checks = doctor(cfg, credentials, statePath)
			}
		}
		for _, c := range checks {
			fmt.Fprintf(out, "%s: %s\n", c.Name, c.SafeDetail())
		}
		for _, c := range checks {
			if !c.Healthy {
				return errors.New("tinyhost: degraded")
			}
		}
		return nil
	default:
		return errors.New("tinyhost: unknown_command")
	}
}
