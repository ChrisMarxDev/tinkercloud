package main

import (
	"context"
	"crypto/ed25519"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"syscall"
	"time"

	"github.com/ChrisMarxDev/tinkercloud/internal/compatibility"
	"github.com/ChrisMarxDev/tinkercloud/internal/config"
	"github.com/ChrisMarxDev/tinkercloud/internal/persistence"
	"github.com/ChrisMarxDev/tinkercloud/internal/update"
)

var updateServiceRestart = func(ctx context.Context) error {
	return systemctlRunner("restart", "tinkercloud.service")
}

// Redirects are a release-origin change, not a convenience.  Do not make this
// client follow one: Fetcher also checks the final response URL as a second
// guard for injected clients.
var updateHTTPClient update.HTTPDoer = update.NewSecureHTTPClient(30 * time.Second)

var updateFetchValidator = update.ValidatePublicHTTPS

var updateSlug = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`)

const updateAnonymousDenyPath = "/_tinker/api/v1/capabilities"

func runUpdate(args []string, out io.Writer) error {
	if effectiveUID() != 0 {
		return errors.New("tinkercloud: root_required")
	}
	fs := flag.NewFlagSet("update", flag.ContinueOnError)
	cfgPath := fs.String("config", defaultConfigPath, "")
	binary := fs.String("binary", "", "")
	metadata := fs.String("metadata", "", "")
	signature := fs.String("signature", "", "")
	releaseManifest := fs.String("release-manifest", "", "")
	releaseManifestMetadata := fs.String("release-manifest-metadata", "", "")
	releaseManifestSignature := fs.String("release-manifest-signature", "", "")
	releaseBase := fs.String("release-base", "", "")
	metadataURL := fs.String("metadata-url", "", "")
	artifactName := fs.String("artifact", "tinkercloud-linux-amd64", "")
	target := fs.String("target", "/usr/local/bin/tinkercloud", "")
	appSlug := fs.String("app-slug", "", "")
	rollback := fs.Bool("rollback", false, "")
	if fs.Parse(args) != nil || *target == "" || (*appSlug != "" && !updateSlug.MatchString(*appSlug)) {
		return errors.New("tinkercloud: invalid_arguments")
	}
	cfg, err := config.LoadYAML(*cfgPath)
	if err != nil {
		return errors.New("tinkercloud: config_invalid")
	}
	unlock, err := acquireUpdateLock(cfg.DataDirectory)
	if err != nil {
		return errors.New("tinkercloud: update_in_progress")
	}
	defer unlock()
	installer := update.FileInstaller{Target: *target, RollbackDir: filepath.Join(cfg.DataDirectory, "update-rollback")}
	if *rollback {
		if *binary != "" || *metadata != "" || *signature != "" || *releaseManifest != "" || *releaseManifestMetadata != "" || *releaseManifestSignature != "" || *releaseBase != "" || *metadataURL != "" {
			return errors.New("tinkercloud: invalid_arguments")
		}
		// Restore precedes every DB/app check. Root rollback remains available if
		// the candidate cannot open the database or resolve an active app.
		if err := restoreBeforeProbe(context.Background(), installer, restarterFunc(updateServiceRestart)); err != nil {
			return errors.New("tinkercloud: rollback_failed")
		}
		if *appSlug == "" {
			*appSlug, err = firstActiveProbeApp(context.Background(), cfg)
			if err != nil {
				return errors.New("tinkercloud: rollback_failed")
			}
		}
		if err := requireActiveProbeApp(context.Background(), cfg, *appSlug); err != nil || !allHealthy(context.Background(), updateChecks(cfg, *appSlug, *target, *cfgPath)) || installer.Commit(context.Background()) != nil {
			return errors.New("tinkercloud: rollback_failed")
		}
		_, err := io.WriteString(out, "rollback restored and healthy\n")
		return err
	}
	noActive := false
	if *appSlug == "" {
		*appSlug, noActive, err = updateProbeApp(context.Background(), cfg)
		if err != nil {
			return errors.New("tinkercloud: active_app_required")
		}
	} else if err := requireActiveProbeApp(context.Background(), cfg, *appSlug); err != nil {
		return errors.New("tinkercloud: active_app_required")
	}
	checks := updateChecksForState(cfg, *appSlug, noActive, *target, *cfgPath)
	a, key, err := updateArtifact(context.Background(), *binary, *metadata, *signature, *releaseBase, *metadataURL, cfg.UpdateReleaseBase, *artifactName)
	if err != nil || !compatibility.CompatibleArtifact(a.Version, a.API, a.Schema) {
		return errors.New("tinkercloud: verification_failed")
	}
	artifactLocal := *binary != "" || *metadata != "" || *signature != ""
	manifestLocal := *releaseManifest != "" || *releaseManifestMetadata != "" || *releaseManifestSignature != ""
	if artifactLocal != manifestLocal {
		return errors.New("tinkercloud: verification_failed")
	}
	if _, err = updateReleaseCompatibility(context.Background(), a, key, *releaseManifest, *releaseManifestMetadata, *releaseManifestSignature, *releaseBase, *metadataURL, cfg.UpdateReleaseBase); err != nil {
		return errors.New("tinkercloud: verification_failed")
	}
	state, err := update.ApplyAfterRestart(context.Background(), key, a, installer, restarterFunc(updateServiceRestart), checks...)
	if err != nil || state != update.Healthy {
		return errors.New("tinkercloud: update_failed")
	}
	if err := installer.Commit(context.Background()); err != nil {
		// A snapshot that cannot be cleared is not a completed update: restore
		// the known-good binary while recovery state is still available.
		_ = installer.Restore(context.Background())
		_ = updateServiceRestart(context.Background())
		return errors.New("tinkercloud: update_failed")
	}
	_, err = io.WriteString(out, "update verified and healthy\n")
	return err
}

func updateReleaseCompatibility(ctx context.Context, server update.Artifact, key ed25519.PublicKey, manifest, metadata, signature, releaseBase, metadataURL, configuredBase string) (compatibility.Matrix, error) {
	local := manifest != "" || metadata != "" || signature != ""
	remote := releaseBase != "" || metadataURL != ""
	if local {
		if remote || manifest == "" || metadata == "" || signature == "" {
			return compatibility.Matrix{}, errors.New("invalid compatibility source")
		}
		raw, err := readUpdateInput(manifest, update.MaxManifestBytes)
		if err != nil {
			return compatibility.Matrix{}, err
		}
		meta, err := readUpdateInput(metadata, update.MaxMetadataBytes)
		if err != nil {
			return compatibility.Matrix{}, err
		}
		sig, err := readUpdateInput(signature, update.MaxSignatureBytes)
		if err != nil {
			return compatibility.Matrix{}, err
		}
		return update.VerifyReleaseManifest(key, raw, meta, sig, server)
	}
	if releaseBase == "" && metadataURL == "" {
		releaseBase = configuredBase
	}
	urls, err := update.ManifestURLsFor(releaseBase, metadataURL)
	if err != nil {
		return compatibility.Matrix{}, err
	}
	raw, meta, sig, err := (update.Fetcher{Client: updateHTTPClient, ValidateURL: updateFetchValidator}).Manifest(ctx, urls)
	if err != nil {
		return compatibility.Matrix{}, err
	}
	return update.VerifyReleaseManifest(key, raw, meta, sig, server)
}

func readUpdateInput(path string, limit int64) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	value, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil || int64(len(value)) > limit {
		return nil, errors.New("update input unavailable")
	}
	return value, nil
}

func updateArtifact(ctx context.Context, binary, metadata, signature, releaseBase, metadataURL, configuredBase, artifact string) (update.Artifact, ed25519.PublicKey, error) {
	local := binary != "" || metadata != "" || signature != ""
	remote := releaseBase != "" || metadataURL != ""
	if local {
		if remote || binary == "" || metadata == "" || signature == "" {
			return update.Artifact{}, nil, errors.New("invalid source")
		}
		a, key, err := loadVerifiedArtifact(binary, metadata, signature)
		return a, key, err
	}
	if releaseBase == "" && metadataURL == "" {
		releaseBase = configuredBase
	}
	urls, err := update.ReleaseURLsFor(releaseBase, metadataURL, artifact)
	if err != nil {
		return update.Artifact{}, nil, err
	}
	b, m, s, err := (update.Fetcher{Client: updateHTTPClient, ValidateURL: updateFetchValidator}).Release(ctx, urls)
	if err != nil {
		return update.Artifact{}, nil, err
	}
	key, err := loadPinnedKey()
	if err != nil {
		return update.Artifact{}, nil, err
	}
	return verifiedArtifactBytes(b, m, s, key)
}

func updateChecks(cfg config.Config, slug, target, cfgPath string) []update.Health {
	return updateChecksForState(cfg, slug, false, target, cfgPath)
}

func updateChecksForState(cfg config.Config, slug string, noActive bool, target, cfgPath string) []update.Health {
	client := updateHTTPClient
	checks := []update.Health{
		update.ListenerHealth{Addresses: []string{cfg.ListenHTTP, cfg.ListenHTTPS}},
		update.ExecHealth{Binary: target, Config: cfgPath, Env: os.Environ()},
		update.HTTPHealth{URL: "https://" + cfg.PlatformHost() + "/api/v1/version", Client: client},
	}
	if noActive {
		// The reserved label cannot be a valid app slug and is derived locally.
		return append(checks, update.UnknownHostDenyHealth{HTTPHealth: update.HTTPHealth{URL: "https://unknown-update-probe." + cfg.AppSuffix() + updateAnonymousDenyPath, Client: client}})
	}
	return append(checks, update.AnonymousDenyHealth{HTTPHealth: update.HTTPHealth{URL: "https://" + slug + "." + cfg.AppSuffix() + updateAnonymousDenyPath, Client: client}})
}

func allHealthy(ctx context.Context, checks []update.Health) bool {
	for _, check := range checks {
		if check == nil || check.Check(ctx) != nil {
			return false
		}
	}
	return true
}

func restoreBeforeProbe(ctx context.Context, installer update.Installer, restarter update.Restarter) error {
	if installer == nil || restarter == nil {
		return errors.New("rollback unavailable")
	}
	if err := installer.Restore(ctx); err != nil {
		return err
	}
	return restarter.Restart(ctx)
}

// requireActiveProbeApp prevents a typo or deleted app from converting the
// denial probe into a harmless unknown-host 404.  The probe's app identity is
// thus locally derived, never supplied as a URL by an operator or a client.
func requireActiveProbeApp(ctx context.Context, cfg config.Config, slug string) error {
	store, err := persistence.OpenSQLite(ctx, filepath.Join(cfg.DataDirectory, "tinkercloud.db"))
	if err != nil {
		return err
	}
	defer store.Close()
	_, err = store.ResolveActive(ctx, slug)
	return err
}

func firstActiveProbeApp(ctx context.Context, cfg config.Config) (string, error) {
	store, err := persistence.OpenSQLite(ctx, filepath.Join(cfg.DataDirectory, "tinkercloud.db"))
	if err != nil {
		return "", err
	}
	defer store.Close()
	return store.FirstActiveAppSlug(ctx)
}

// updateProbeApp distinguishes an exact empty active-app set from a database
// read failure.  A failed/ambiguous query must never be treated as empty.
func updateProbeApp(ctx context.Context, cfg config.Config) (string, bool, error) {
	store, err := persistence.OpenSQLite(ctx, filepath.Join(cfg.DataDirectory, "tinkercloud.db"))
	if err != nil {
		return "", false, err
	}
	defer store.Close()
	var count int
	if err := store.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM applications a JOIN deployments d ON d.id=a.current_deployment_id AND d.app_id=a.id WHERE a.status='active' AND d.state='active' AND d.release_hash <> ''").Scan(&count); err != nil || count < 0 {
		return "", false, fmt.Errorf("active apps unavailable")
	}
	if count == 0 {
		return "", true, nil
	}
	slug, err := store.FirstActiveAppSlug(ctx)
	return slug, false, err
}

type restarterFunc func(context.Context) error

func (f restarterFunc) Restart(ctx context.Context) error { return f(ctx) }

func acquireUpdateLock(dataDir string) (func(), error) {
	dir := filepath.Join(dataDir, "update")
	if !pathHasNoSymlink(dir, true) {
		return nil, errors.New("unsafe update lock directory")
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	path := filepath.Join(dir, ".lock")
	if st, err := os.Lstat(path); err == nil && (st.Mode()&os.ModeSymlink != 0 || !st.Mode().IsRegular() || st.Mode().Perm()&0077 != 0) {
		return nil, errors.New("unsafe update lock")
	} else if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|syscall.O_NOFOLLOW, 0600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = f.Close()
		return nil, err
	}
	return func() {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		_ = f.Close()
	}, nil
}
