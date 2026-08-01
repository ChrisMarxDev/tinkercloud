package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"

	"github.com/ChrisMarxDev/tinkercloud/internal/config"
)

const (
	defaultDataDirectory      = "/var/lib/tinkercloud"
	defaultACMECacheDirectory = "/var/lib/tinkercloud-acme"
	defaultUnitPath           = "/etc/systemd/system/tinkercloud.service"
	defaultBinaryPath         = "/usr/local/bin/tinkercloud"
)

// uninstallRuntime makes the intentionally fixed destructive sequence
// testable. It accepts no caller-controlled filesystem paths or subprocess
// arguments; preserving the ACME directory is a security and rate-limit
// default, not an optional destructive flag.
type uninstallRuntime struct {
	Lstat      func(string) (os.FileInfo, error)
	RemoveAll  func(string) error
	Remove     func(string) error
	Run        func(context.Context, string, ...string) error
	LookupUser func(string) (*user.User, error)
	LoadConfig func(string) (config.Config, error)
	MountInfo  func() ([]byte, error)
}

var productionUninstallRuntime = uninstallRuntime{
	Lstat:     os.Lstat,
	RemoveAll: os.RemoveAll,
	Remove:    os.Remove,
	Run: func(ctx context.Context, name string, args ...string) error {
		return exec.CommandContext(ctx, name, args...).Run()
	},
	LookupUser: user.Lookup,
	LoadConfig: config.LoadYAML,
	MountInfo:  func() ([]byte, error) { return os.ReadFile("/proc/self/mountinfo") },
}

func runUninstall(args []string, out io.Writer, rt uninstallRuntime) error {
	if effectiveUID() != 0 {
		return errors.New("tinkercloud: root_required")
	}
	if len(args) != 1 || args[0] != "--confirm-uninstall" {
		return errors.New("tinkercloud: uninstall_confirmation_required")
	}
	if err := validateUninstallPaths(rt); err != nil {
		return errors.New("tinkercloud: uninstall_unsafe_state")
	}
	ctx := context.Background()
	unitPresent, err := pathPresent(rt, defaultUnitPath)
	if err != nil {
		return errors.New("tinkercloud: uninstall_unsafe_state")
	}
	if unitPresent {
		if err = rt.Run(ctx, "systemctl", "disable", "--now", "tinkercloud.service"); err != nil {
			return errors.New("tinkercloud: uninstall_service_failed")
		}
	}
	// Delete only canonical Tinkercloud state. In particular, do not remove
	// /var/lib/tinkercloud-acme: retaining its account and certificates avoids
	// unnecessary ACME issuance on a later reinstall.
	if err = removeIfPresent(rt, defaultDataDirectory, true); err != nil {
		return errors.New("tinkercloud: uninstall_state_failed")
	}
	if err = removeIfPresent(rt, "/etc/tinkercloud", true); err != nil {
		return errors.New("tinkercloud: uninstall_config_failed")
	}
	if err = removeIfPresent(rt, defaultUnitPath, false); err != nil {
		return errors.New("tinkercloud: uninstall_service_failed")
	}
	if err = removeIfPresent(rt, defaultBinaryPath, false); err != nil {
		return errors.New("tinkercloud: uninstall_binary_failed")
	}
	if unitPresent {
		if err = rt.Run(ctx, "systemctl", "daemon-reload"); err != nil {
			return errors.New("tinkercloud: uninstall_service_failed")
		}
		if err = rt.Run(ctx, "systemctl", "reset-failed", "tinkercloud.service"); err != nil {
			return errors.New("tinkercloud: uninstall_service_failed")
		}
	}
	if _, err = rt.LookupUser("tinkercloud"); err == nil {
		if err = rt.Run(ctx, "userdel", "tinkercloud"); err != nil {
			return errors.New("tinkercloud: uninstall_service_identity_failed")
		}
	} else if !isUnknownUser(err) {
		return errors.New("tinkercloud: uninstall_service_identity_failed")
	}
	fmt.Fprintln(out, "Tinkercloud uninstalled. The ACME cache was preserved.")
	return nil
}

func validateUninstallPaths(rt uninstallRuntime) error {
	if rt.Lstat == nil || rt.RemoveAll == nil || rt.Remove == nil || rt.Run == nil || rt.LookupUser == nil || rt.LoadConfig == nil || rt.MountInfo == nil {
		return errors.New("uninstall runtime unavailable")
	}
	// A custom installation is deliberately not guessed at or recursively
	// removed. The fixed installer owns exactly these paths; ambiguity denies
	// before the service is stopped.
	configInfo, err := rt.Lstat(defaultConfigPath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err == nil && (configInfo.Mode()&os.ModeSymlink != 0 || !configInfo.Mode().IsRegular()) {
		return errors.New("unsafe config")
	}
	if configInfo != nil {
		cfg, err := rt.LoadConfig(defaultConfigPath)
		if err != nil || cfg.DataDirectory != defaultDataDirectory || cfg.ACMECachedir != defaultACMECacheDirectory {
			return errors.New("noncanonical installation")
		}
	}
	for _, path := range []struct {
		path string
		dir  bool
	}{
		{defaultDataDirectory, true},
		{"/etc/tinkercloud", true},
		{defaultUnitPath, false},
		{defaultBinaryPath, false},
		{defaultACMECacheDirectory, true},
	} {
		info, err := rt.Lstat(path.path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil || info.Mode()&os.ModeSymlink != 0 || (path.dir && !info.IsDir()) || (!path.dir && !info.Mode().IsRegular()) {
			return errors.New("unsafe installation artifact")
		}
	}
	mountInfo, err := rt.MountInfo()
	if err != nil || uninstallCrossesMountBoundary(mountInfo) {
		return errors.New("unsafe mount boundary")
	}
	return nil
}

// uninstallCrossesMountBoundary parses the Linux mountinfo grammar just far
// enough to reject an owned directory that is itself a mount or contains one.
// RemoveAll must never be trusted to discover this after systemd is stopped.
func uninstallCrossesMountBoundary(raw []byte) bool {
	if len(raw) == 0 {
		return true
	}
	for _, line := range strings.Split(strings.TrimSuffix(string(raw), "\n"), "\n") {
		parts := strings.SplitN(line, " - ", 2)
		if len(parts) != 2 {
			return true
		}
		fields := strings.Fields(parts[0])
		if len(fields) < 6 {
			return true
		}
		mountPoint, ok := unescapeMountInfoPath(fields[4])
		if !ok || !filepath.IsAbs(mountPoint) || filepath.Clean(mountPoint) != mountPoint {
			return true
		}
		for _, root := range []string{"/etc/tinkercloud", defaultDataDirectory} {
			if mountPoint == root || strings.HasPrefix(mountPoint, root+"/") {
				return true
			}
		}
	}
	return false
}

func unescapeMountInfoPath(raw string) (string, bool) {
	var b strings.Builder
	for i := 0; i < len(raw); i++ {
		if raw[i] != '\\' {
			b.WriteByte(raw[i])
			continue
		}
		if i+3 >= len(raw) || raw[i+1] < '0' || raw[i+1] > '7' || raw[i+2] < '0' || raw[i+2] > '7' || raw[i+3] < '0' || raw[i+3] > '7' {
			return "", false
		}
		b.WriteByte((raw[i+1]-'0')*64 + (raw[i+2]-'0')*8 + raw[i+3] - '0')
		i += 3
	}
	value := b.String()
	return value, value != "" && !strings.ContainsRune(value, '\x00')
}

func pathPresent(rt uninstallRuntime, path string) (bool, error) {
	_, err := rt.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return err == nil, err
}

func removeIfPresent(rt uninstallRuntime, path string, recursive bool) error {
	present, err := pathPresent(rt, path)
	if err != nil || !present {
		return err
	}
	if recursive {
		return rt.RemoveAll(path)
	}
	return rt.Remove(path)
}

func isUnknownUser(err error) bool {
	var unknown user.UnknownUserError
	return errors.As(err, &unknown)
}
