package main

import (
	"context"
	"errors"
	"io"
	"os"
	"os/user"
	"strings"
	"testing"
	"time"

	"github.com/ChrisMarxDev/tinkercloud/internal/config"
)

type uninstallFileInfo struct {
	name string
	mode os.FileMode
}

func (i uninstallFileInfo) Name() string       { return i.name }
func (i uninstallFileInfo) Size() int64        { return 0 }
func (i uninstallFileInfo) Mode() os.FileMode  { return i.mode }
func (i uninstallFileInfo) ModTime() time.Time { return time.Time{} }
func (i uninstallFileInfo) IsDir() bool        { return i.mode.IsDir() }
func (i uninstallFileInfo) Sys() any           { return nil }

func safeUninstallRuntime(t *testing.T) (uninstallRuntime, *[]string, *[]string) {
	t.Helper()
	removed, runs := []string{}, []string{}
	entries := map[string]os.FileInfo{
		defaultConfigPath:         uninstallFileInfo{name: "config.yaml", mode: 0600},
		"/etc/tinkercloud":        uninstallFileInfo{name: "tinkercloud", mode: os.ModeDir | 0700},
		defaultDataDirectory:      uninstallFileInfo{name: "tinkercloud", mode: os.ModeDir | 0700},
		defaultACMECacheDirectory: uninstallFileInfo{name: "tinkercloud-acme", mode: os.ModeDir | 0700},
		defaultUnitPath:           uninstallFileInfo{name: "tinkercloud.service", mode: 0644},
		defaultBinaryPath:         uninstallFileInfo{name: "tinkercloud", mode: 0755},
	}
	rt := uninstallRuntime{
		Lstat: func(path string) (os.FileInfo, error) {
			if info, ok := entries[path]; ok {
				return info, nil
			}
			return nil, os.ErrNotExist
		},
		RemoveAll: func(path string) error { removed = append(removed, path); return nil },
		Remove:    func(path string) error { removed = append(removed, path); return nil },
		Run: func(_ context.Context, name string, args ...string) error {
			runs = append(runs, name+" "+strings.Join(args, " "))
			return nil
		},
		LookupUser: func(string) (*user.User, error) { return &user.User{Username: "tinkercloud"}, nil },
		LoadConfig: func(string) (config.Config, error) {
			return config.Config{DataDirectory: defaultDataDirectory, ACMECachedir: defaultACMECacheDirectory}, nil
		},
		MountInfo: func() ([]byte, error) { return []byte("36 25 0:32 / / rw - ext4 /dev/root rw\n"), nil },
	}
	return rt, &removed, &runs
}

func TestUninstallRemovesOnlyCanonicalStateAndPreservesACME(t *testing.T) {
	oldUID := effectiveUID
	effectiveUID = func() int { return 0 }
	t.Cleanup(func() { effectiveUID = oldUID })
	rt, removed, runs := safeUninstallRuntime(t)
	var out strings.Builder
	if err := runUninstall([]string{"--confirm-uninstall"}, &out, rt); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(*removed, ","); strings.Contains(got, defaultACMECacheDirectory) || !strings.Contains(got, defaultDataDirectory) || !strings.Contains(got, "/etc/tinkercloud") || !strings.Contains(got, defaultUnitPath) || !strings.Contains(got, defaultBinaryPath) {
		t.Fatalf("removed = %q", got)
	}
	wantRuns := "systemctl disable --now tinkercloud.service,systemctl daemon-reload,systemctl reset-failed tinkercloud.service,userdel tinkercloud"
	if got := strings.Join(*runs, ","); got != wantRuns {
		t.Fatalf("runs=%q want=%q", got, wantRuns)
	}
	if !strings.Contains(out.String(), "ACME cache was preserved") {
		t.Fatalf("output=%q", out.String())
	}
}

func TestUninstallDeniesBeforeMutation(t *testing.T) {
	oldUID := effectiveUID
	effectiveUID = func() int { return 0 }
	t.Cleanup(func() { effectiveUID = oldUID })
	for name, mutate := range map[string]func(*uninstallRuntime){
		"missing acknowledgement": func(rt *uninstallRuntime) {},
		"custom state": func(rt *uninstallRuntime) {
			rt.LoadConfig = func(string) (config.Config, error) {
				return config.Config{DataDirectory: "/srv/tinker", ACMECachedir: defaultACMECacheDirectory}, nil
			}
		},
		"symlink": func(rt *uninstallRuntime) {
			base := rt.Lstat
			rt.Lstat = func(path string) (os.FileInfo, error) {
				if path == defaultDataDirectory {
					return uninstallFileInfo{name: "tinkercloud", mode: os.ModeSymlink | 0777}, nil
				}
				return base(path)
			}
		},
		"service failure": func(rt *uninstallRuntime) {
			rt.Run = func(_ context.Context, name string, args ...string) error {
				if name == "systemctl" && len(args) > 0 && args[0] == "disable" {
					return errors.New("failed")
				}
				return nil
			}
		},
		"nested mount": func(rt *uninstallRuntime) {
			rt.MountInfo = func() ([]byte, error) {
				return []byte("36 25 0:32 / / rw - ext4 /dev/root rw\n37 36 0:33 / /var/lib/tinkercloud/releases rw - ext4 /dev/loop0 rw\n"), nil
			}
		},
	} {
		t.Run(name, func(t *testing.T) {
			rt, removed, runs := safeUninstallRuntime(t)
			mutate(&rt)
			args := []string{"--confirm-uninstall"}
			if name == "missing acknowledgement" {
				args = nil
			}
			err := runUninstall(args, io.Discard, rt)
			if err == nil {
				t.Fatal("accepted unsafe uninstall")
			}
			if name != "service failure" && (len(*removed) != 0 || len(*runs) != 0) {
				t.Fatalf("mutation before denial removed=%v runs=%v", *removed, *runs)
			}
		})
	}
}

func TestUninstallRequiresRoot(t *testing.T) {
	oldUID := effectiveUID
	effectiveUID = func() int { return 1 }
	t.Cleanup(func() { effectiveUID = oldUID })
	rt, removed, runs := safeUninstallRuntime(t)
	if err := runUninstall([]string{"--confirm-uninstall"}, io.Discard, rt); err == nil || err.Error() != "tinkercloud: root_required" {
		t.Fatalf("error=%v", err)
	}
	if len(*removed) != 0 || len(*runs) != 0 {
		t.Fatal("non-root uninstall mutated")
	}
}
