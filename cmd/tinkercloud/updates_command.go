package main

// The scheduled updater has a deliberately tiny surface: it stores only an
// opt-in channel, installs fixed unit bytes, and invokes this same binary with
// fixed arguments.  It is not a general systemd wrapper.

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/ChrisMarxDev/tinkercloud/internal/compatibility"
	"github.com/ChrisMarxDev/tinkercloud/internal/config"
	"github.com/ChrisMarxDev/tinkercloud/internal/update"
)

var updatesUnitPath = "/etc/systemd/system/tinkercloud-updates.service"
var updatesTimerPath = "/etc/systemd/system/tinkercloud-updates.timer"
var updatesNow = time.Now
var updatesWriteFile = os.WriteFile

const updatesUnit = "[Unit]\nDescription=Tinkercloud update check\n\n[Service]\nType=oneshot\nExecStart=/usr/local/bin/tinkercloud updates run\nNoNewPrivileges=true\nPrivateTmp=true\nProtectSystem=strict\nProtectHome=true\nReadWritePaths=/var/lib/tinkercloud\n"
const updatesTimer = "[Unit]\nDescription=Tinkercloud update timer\n\n[Timer]\nOnCalendar=daily\nRandomizedDelaySec=1h\nPersistent=true\nUnit=tinkercloud-updates.service\n\n[Install]\nWantedBy=timers.target\n"

type updateScheduleState struct {
	Channel          string `json:"channel"`
	Enabled          bool   `json:"enabled"`
	InstalledVersion string `json:"installed_version,omitempty"`
	Updated          string `json:"updated"`
}

func runUpdates(args []string, out io.Writer) error {
	if effectiveUID() != 0 {
		return errors.New("tinkercloud: root_required")
	}
	if len(args) == 0 {
		return errors.New("tinkercloud: invalid_arguments")
	}
	fs := flag.NewFlagSet("updates", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	cfgPath := fs.String("config", defaultConfigPath, "")
	channel := fs.String("channel", "", "")
	if fs.Parse(args[1:]) != nil || len(fs.Args()) != 0 {
		return errors.New("tinkercloud: invalid_arguments")
	}
	if args[0] != "enable" && *channel != "" {
		return errors.New("tinkercloud: invalid_arguments")
	}
	cfg, err := config.LoadYAML(*cfgPath)
	if err != nil {
		return errors.New("tinkercloud: config_invalid")
	}
	statePath := filepath.Join(cfg.DataDirectory, "update", "schedule.json")
	switch args[0] {
	case "enable":
		if *channel != "beta" && *channel != "stable" {
			return errors.New("tinkercloud: invalid_arguments")
		}
		if !pathHasNoSymlink(updatesUnitPath, true) || !pathHasNoSymlink(updatesTimerPath, true) {
			return errors.New("tinkercloud: unsafe_unit_path")
		}
		if err := os.MkdirAll(filepath.Dir(statePath), 0700); err != nil {
			return errors.New("tinkercloud: updates_failed")
		}
		if err := updatesWriteFile(updatesUnitPath, []byte(updatesUnit), 0644); err != nil {
			return errors.New("tinkercloud: updates_failed")
		}
		if err := updatesWriteFile(updatesTimerPath, []byte(updatesTimer), 0644); err != nil {
			return errors.New("tinkercloud: updates_failed")
		}
		if err := systemctlRunner("daemon-reload"); err != nil {
			return errors.New("tinkercloud: service_failed")
		}
		if err := systemctlRunner("enable", "--now", "tinkercloud-updates.timer"); err != nil {
			return errors.New("tinkercloud: service_failed")
		}
		b, _ := json.Marshal(updateScheduleState{Channel: *channel, Enabled: true, Updated: updatesNow().UTC().Format(time.RFC3339)})
		if err := writeAtomicPrivate(statePath, b); err != nil {
			_ = systemctlRunner("disable", "--now", "tinkercloud-updates.timer")
			return errors.New("tinkercloud: updates_failed")
		}
		_, err = fmt.Fprintf(out, "updates enabled: %s\n", *channel)
		return err
	case "disable":
		if err := systemctlRunner("disable", "--now", "tinkercloud-updates.timer"); err != nil {
			return errors.New("tinkercloud: service_failed")
		}
		b, _ := json.Marshal(updateScheduleState{Enabled: false, Updated: updatesNow().UTC().Format(time.RFC3339)})
		if err := os.MkdirAll(filepath.Dir(statePath), 0700); err != nil || writeAtomicPrivate(statePath, b) != nil {
			// The durable state must never claim the timer is disabled while it
			// may still run. Best-effort compensation restores the old behavior.
			_ = systemctlRunner("enable", "--now", "tinkercloud-updates.timer")
			return errors.New("tinkercloud: updates_failed")
		}
		_, err = io.WriteString(out, "updates disabled\n")
		return err
	case "status":
		if _, statErr := os.Lstat(statePath); os.IsNotExist(statErr) {
			_, err = io.WriteString(out, "updates disabled\n")
			return err
		}
		state, err := readUpdateScheduleState(statePath)
		if err != nil {
			return errors.New("tinkercloud: updates_failed")
		}
		_, err = fmt.Fprintf(out, "updates %s\n", map[bool]string{true: "enabled (" + state.Channel + ")", false: "disabled"}[state.Enabled])
		return err
	case "run":
		state, err := readUpdateScheduleState(statePath)
		if err != nil || !state.Enabled {
			return errors.New("tinkercloud: updates_disabled")
		}
		// Discovery is intentionally not delegated to arbitrary unit arguments.
		return runDiscoveredUpdate(context.Background(), cfg, state, *cfgPath, out)
	default:
		return errors.New("tinkercloud: invalid_arguments")
	}
}

func readUpdateScheduleState(path string) (updateScheduleState, error) {
	var state updateScheduleState
	st, err := os.Lstat(path)
	if err != nil || st.Mode()&os.ModeSymlink != 0 || st.Mode().Perm() != 0600 || !st.Mode().IsRegular() || st.Size() > 512 {
		return state, errors.New("state unavailable")
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return state, errors.New("state unavailable")
	}
	decoder := json.NewDecoder(strings.NewReader(string(b)))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&state) != nil || decoder.Decode(&struct{}{}) != io.EOF || (state.Enabled && state.Channel != "beta" && state.Channel != "stable") || (!state.Enabled && (state.Channel != "" || state.InstalledVersion != "")) {
		return updateScheduleState{}, errors.New("state invalid")
	}
	return state, nil
}

// runDiscoveredUpdate is kept separate so discovery remains an untrusted input
// boundary. The current release contract has no committed public GitHub origin
// in this repository; refusing to guess one is safer than silently widening
// the root timer into a network downloader.
const githubReleasesURL = "https://api.github.com/repos/ChrisMarxDev/tinkercloud/releases"

var githubDiscoveryValidator = validateGitHubDiscoveryURL

type githubRelease struct {
	TagName    string        `json:"tag_name"`
	Prerelease bool          `json:"prerelease"`
	Draft      bool          `json:"draft"`
	Assets     []githubAsset `json:"assets"`
}
type githubAsset struct {
	BrowserDownloadURL string `json:"browser_download_url"`
}

func runDiscoveredUpdate(ctx context.Context, cfg config.Config, state updateScheduleState, cfgPath string, out io.Writer) error {
	current, err := compatibility.Parse(buildVersion)
	if err != nil {
		return errors.New("tinkercloud: verification_failed")
	}
	if state.InstalledVersion != "" {
		if v, e := compatibility.Parse(state.InstalledVersion); e != nil || v.Compare(current) > 0 {
			return errors.New("tinkercloud: verification_failed")
		}
	}
	release, version, err := discoverGitHubRelease(ctx, state.Channel, current)
	if err != nil {
		return errors.New("tinkercloud: verification_failed")
	}
	// Unattended migration is deliberately forbidden even when the signed
	// candidate later claims compatibility; runUpdate will authenticate it too.
	base := "https://github.com/ChrisMarxDev/tinkercloud/releases/download/v" + version + "/"
	if err := runUpdate([]string{"--config", cfgPath, "--release-base", base}, out); err != nil {
		return err
	}
	state.InstalledVersion, state.Updated = version, updatesNow().UTC().Format(time.RFC3339)
	b, _ := json.Marshal(state)
	if err := writeAtomicPrivate(filepath.Join(cfg.DataDirectory, "update", "schedule.json"), b); err != nil {
		return errors.New("tinkercloud: updates_failed")
	}
	_ = release // selected release has already had its complete exact asset set checked.
	return nil
}

func discoverGitHubRelease(ctx context.Context, channel string, current compatibility.Version) (githubRelease, string, error) {
	if channel != "beta" && channel != "stable" {
		return githubRelease{}, "", errors.New("channel")
	}
	client := update.Fetcher{Client: updateHTTPClient, ValidateURL: githubDiscoveryValidator}
	b, err := client.Get(ctx, githubReleasesURL, update.MaxManifestBytes)
	if err != nil {
		return githubRelease{}, "", err
	}
	var releases []githubRelease
	if json.Unmarshal(b, &releases) != nil || len(releases) == 0 || len(releases) > 100 {
		return githubRelease{}, "", errors.New("discovery")
	}
	type candidate struct {
		release githubRelease
		version compatibility.Version
		raw     string
	}
	var candidates []candidate
	for _, r := range releases {
		if r.Draft || r.Prerelease != (channel == "beta") || !strings.HasPrefix(r.TagName, "v") {
			continue
		}
		v, e := compatibility.Parse(strings.TrimPrefix(r.TagName, "v"))
		if e != nil || v.Compare(current) <= 0 || !githubAssetsExact(r.Assets, strings.TrimPrefix(r.TagName, "v")) {
			continue
		}
		candidates = append(candidates, candidate{r, v, strings.TrimPrefix(r.TagName, "v")})
	}
	if len(candidates) == 0 {
		return githubRelease{}, "", errors.New("no candidate")
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].version.Compare(candidates[j].version) > 0 })
	return candidates[0].release, candidates[0].raw, nil
}

func validateGitHubDiscoveryURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.String() != githubReleasesURL || u.Scheme != "https" || u.Host != "api.github.com" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return update.ErrFetch
	}
	return update.ValidatePublicHTTPS(raw)
}

func githubAssetsExact(assets []githubAsset, version string) bool {
	want := map[string]bool{}
	for _, n := range []string{"tinkercloud-linux-amd64", "tinkercloud-linux-amd64.metadata.json", "tinkercloud-linux-amd64.signature", "release-manifest.json", "release-manifest.json.metadata.json", "release-manifest.json.signature"} {
		want["https://github.com/ChrisMarxDev/tinkercloud/releases/download/v"+version+"/"+n] = false
	}
	for _, a := range assets {
		if seen, ok := want[a.BrowserDownloadURL]; ok {
			if seen {
				return false
			}
			want[a.BrowserDownloadURL] = true
		}
	}
	for _, ok := range want {
		if !ok {
			return false
		}
	}
	return true
}
