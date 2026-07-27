package main

import (
	"errors"
	"flag"
	"github.com/tinyhost/tiny/internal/config"
	"github.com/tinyhost/tiny/internal/operations"
	"os"
	"os/exec"
	"path/filepath"
)

var systemctlRunner = func(args ...string) error { return exec.Command("systemctl", args...).Run() }

func runInstallService(args []string) error {
	if effectiveUID() != 0 {
		return errors.New("tinyhost: root_required")
	}
	fs := flag.NewFlagSet("install-service", flag.ContinueOnError)
	p := fs.String("unit-path", "/etc/systemd/system/tinyhost.service", "")
	cfgPath := fs.String("config", "/etc/tinyhost/config.yaml", "")
	credentialPath := fs.String("credentials", "/etc/tinyhost/credentials/tinyhost.env", "")
	if fs.Parse(args) != nil {
		return errors.New("tinyhost: invalid_arguments")
	}
	if len(fs.Args()) != 0 {
		return errors.New("tinyhost: invalid_arguments")
	}
	if !pathHasNoSymlink(*p, true) || !pathHasNoSymlink(*cfgPath, false) || !pathHasNoSymlink(*credentialPath, false) {
		return errors.New("tinyhost: unsafe_unit_path")
	}
	cfg, e := config.LoadYAML(*cfgPath)
	if e != nil {
		return errors.New("tinyhost: config_invalid")
	}
	if !pathHasNoSymlink(cfg.DataDirectory, false) || !pathHasNoSymlink(cfg.ACMECachedir, false) {
		return errors.New("tinyhost: unsafe_unit_path")
	}
	if e := os.MkdirAll(filepath.Dir(*p), 0755); e != nil {
		return e
	}
	unit, e := operations.ServiceUnit(*cfgPath, *credentialPath, cfg.DataDirectory, cfg.ACMECachedir)
	if e != nil {
		return errors.New("tinyhost: unsafe_unit_path")
	}
	if e := os.WriteFile(*p, []byte(unit), 0644); e != nil {
		return e
	}
	if e := systemctlRunner("daemon-reload"); e != nil {
		return errors.New("tinyhost: service_failed")
	}
	if e := systemctlRunner("enable", "--now", "tinyhost.service"); e != nil {
		return errors.New("tinyhost: service_failed")
	}
	return nil
}

// pathHasNoSymlink prevents a root-run installer from following a swapped
// config, credential, writable directory, unit, or parent directory.
func pathHasNoSymlink(path string, missingLeafOK bool) bool {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return false
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err == nil {
		return resolved == path
	}
	if !missingLeafOK || !os.IsNotExist(err) {
		return false
	}
	parent := filepath.Dir(path)
	resolved, err = filepath.EvalSymlinks(parent)
	return err == nil && resolved == parent
}
