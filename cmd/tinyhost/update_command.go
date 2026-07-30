package main

import (
	"context"
	"crypto/ed25519"
	"errors"
	"flag"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"github.com/tinyhost/tiny/internal/compatibility"
	"github.com/tinyhost/tiny/internal/config"
	"github.com/tinyhost/tiny/internal/persistence"
	"github.com/tinyhost/tiny/internal/update"
)

var updateServiceRestart = func(ctx context.Context) error {
	return systemctlRunner("restart", "tinyhost.service")
}

// Redirects are a release-origin change, not a convenience.  Do not make this
// client follow one: Fetcher also checks the final response URL as a second
// guard for injected clients.
var updateHTTPClient update.HTTPDoer = update.NewSecureHTTPClient(30 * time.Second)

var updateFetchValidator = update.ValidatePublicHTTPS

var updateSlug = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`)

const updateAnonymousDenyPath = "/_tiny/api/v1/capabilities"

func runUpdate(args []string, out io.Writer) error {
	if effectiveUID() != 0 {
		return errors.New("tinyhost: root_required")
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
	artifactName := fs.String("artifact", "tinyhost-linux-amd64", "")
	target := fs.String("target", "/usr/local/bin/tinyhost", "")
	appSlug := fs.String("app-slug", "", "")
	rollback := fs.Bool("rollback", false, "")
	if fs.Parse(args) != nil || *target == "" || (*appSlug != "" && !updateSlug.MatchString(*appSlug)) {
		return errors.New("tinyhost: invalid_arguments")
	}
	cfg, err := config.LoadYAML(*cfgPath)
	if err != nil {
		return errors.New("tinyhost: config_invalid")
	}
	installer := update.FileInstaller{Target: *target, RollbackDir: filepath.Join(cfg.DataDirectory, "update-rollback")}
	if *rollback {
		if *binary != "" || *metadata != "" || *signature != "" || *releaseManifest != "" || *releaseManifestMetadata != "" || *releaseManifestSignature != "" || *releaseBase != "" || *metadataURL != "" {
			return errors.New("tinyhost: invalid_arguments")
		}
		// Restore precedes every DB/app check. Root rollback remains available if
		// the candidate cannot open the database or resolve an active app.
		if err := restoreBeforeProbe(context.Background(), installer, restarterFunc(updateServiceRestart)); err != nil {
			return errors.New("tinyhost: rollback_failed")
		}
		if *appSlug == "" {
			*appSlug, err = firstActiveProbeApp(context.Background(), cfg)
			if err != nil {
				return errors.New("tinyhost: rollback_failed")
			}
		}
		if err := requireActiveProbeApp(context.Background(), cfg, *appSlug); err != nil || !allHealthy(context.Background(), updateChecks(cfg, *appSlug, *target, *cfgPath)) || installer.Commit(context.Background()) != nil {
			return errors.New("tinyhost: rollback_failed")
		}
		_, err := io.WriteString(out, "rollback restored and healthy\n")
		return err
	}
	if *appSlug == "" {
		*appSlug, err = firstActiveProbeApp(context.Background(), cfg)
		if err != nil {
			return errors.New("tinyhost: active_app_required")
		}
	}
	if err := requireActiveProbeApp(context.Background(), cfg, *appSlug); err != nil {
		return errors.New("tinyhost: active_app_required")
	}
	checks := updateChecks(cfg, *appSlug, *target, *cfgPath)
	a, key, err := updateArtifact(context.Background(), *binary, *metadata, *signature, *releaseBase, *metadataURL, cfg.UpdateReleaseBase, *artifactName)
	if err != nil || !compatibility.CompatibleArtifact(a.Version, a.API, a.Schema) {
		return errors.New("tinyhost: verification_failed")
	}
	artifactLocal := *binary != "" || *metadata != "" || *signature != ""
	manifestLocal := *releaseManifest != "" || *releaseManifestMetadata != "" || *releaseManifestSignature != ""
	if artifactLocal != manifestLocal {
		return errors.New("tinyhost: verification_failed")
	}
	if _, err = updateReleaseCompatibility(context.Background(), a, key, *releaseManifest, *releaseManifestMetadata, *releaseManifestSignature, *releaseBase, *metadataURL, cfg.UpdateReleaseBase); err != nil {
		return errors.New("tinyhost: verification_failed")
	}
	state, err := update.ApplyAfterRestart(context.Background(), key, a, installer, restarterFunc(updateServiceRestart), checks...)
	if err != nil || state != update.Healthy {
		return errors.New("tinyhost: update_failed")
	}
	if err := installer.Commit(context.Background()); err != nil {
		// A snapshot that cannot be cleared is not a completed update: restore
		// the known-good binary while recovery state is still available.
		_ = installer.Restore(context.Background())
		_ = updateServiceRestart(context.Background())
		return errors.New("tinyhost: update_failed")
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
	client := updateHTTPClient
	return []update.Health{
		update.ListenerHealth{Addresses: []string{cfg.ListenHTTP, cfg.ListenHTTPS}},
		update.ExecHealth{Binary: target, Config: cfgPath, Env: os.Environ()},
		update.HTTPHealth{URL: "https://" + cfg.PlatformHost() + "/api/v1/version", Client: client},
		update.AnonymousDenyHealth{HTTPHealth: update.HTTPHealth{URL: "https://" + slug + "." + cfg.AppSuffix() + updateAnonymousDenyPath, Client: client}},
	}
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
	store, err := persistence.OpenSQLite(ctx, filepath.Join(cfg.DataDirectory, "tinyhost.db"))
	if err != nil {
		return err
	}
	defer store.Close()
	_, err = store.ResolveActive(ctx, slug)
	return err
}

func firstActiveProbeApp(ctx context.Context, cfg config.Config) (string, error) {
	store, err := persistence.OpenSQLite(ctx, filepath.Join(cfg.DataDirectory, "tinyhost.db"))
	if err != nil {
		return "", err
	}
	defer store.Close()
	return store.FirstActiveAppSlug(ctx)
}

type restarterFunc func(context.Context) error

func (f restarterFunc) Restart(ctx context.Context) error { return f(ctx) }
