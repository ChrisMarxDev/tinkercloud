package main

// The human setup surface is intentionally only a collector for the trusted,
// deterministic init contract.  Keeping provisioning in runInit prevents the
// two entry points from acquiring subtly different security behaviour.

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ChrisMarxDev/tinkercloud/internal/config"
	"github.com/ChrisMarxDev/tinkercloud/internal/identity"
)

type setupRuntime struct {
	UID       func() int
	IsTTY     func(*os.File) bool
	ReadLine  func(*os.File, *os.File, string) (string, error)
	Preflight func(initRuntime) error
	RunInit   func([]string, *os.File, initRuntime) error
	Init      initRuntime
}

var productionSetupRuntime = setupRuntime{
	UID: effectiveUID,
	IsTTY: func(f *os.File) bool {
		info, err := f.Stat()
		return err == nil && info.Mode()&os.ModeCharDevice != 0
	},
	ReadLine: promptSetupLine,
	Preflight: func(rt initRuntime) error {
		return runPreflight(context.Background(), rt)
	},
	RunInit: runInit,
	Init:    productionInitRuntime,
}

func promptSetupLine(in, out *os.File, prompt string) (string, error) {
	if _, err := fmt.Fprint(out, prompt); err != nil {
		return "", err
	}
	s := bufio.NewScanner(in)
	if !s.Scan() {
		if err := s.Err(); err != nil {
			return "", err
		}
		return "", errors.New("input unavailable")
	}
	return strings.TrimSpace(s.Text()), nil
}

// runSetup never takes credentials on argv. A Resend source is a root-readable
// regular file; this keeps the secret outside terminal history and logs.
func runSetup(args []string, out, errout *os.File, rt setupRuntime) error {
	if rt.UID == nil || rt.UID() != 0 {
		return errors.New("tinkercloud: root_required")
	}
	if rt.IsTTY == nil || !rt.IsTTY(out) || !rt.IsTTY(os.Stdin) {
		return errors.New("tinkercloud: setup_tty_required")
	}
	fs := flag.NewFlagSet("setup", flag.ContinueOnError)
	fs.SetOutput(errout)
	cfgPath := fs.String("config", defaultConfigPath, "")
	credentialPath := fs.String("credentials", defaultCredentialPath, "")
	if fs.Parse(args) != nil || len(fs.Args()) != 0 || !filepath.IsAbs(*cfgPath) || !filepath.IsAbs(*credentialPath) {
		return errors.New("tinkercloud: invalid_arguments")
	}

	var cfg config.Config
	existing := false
	if info, err := os.Lstat(*cfgPath); err == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || info.Mode().Perm()&0037 != 0 {
			return errors.New("tinkercloud: unsafe_config_path")
		}
		var loadErr error
		cfg, loadErr = config.LoadYAML(*cfgPath)
		if loadErr != nil {
			return errors.New("tinkercloud: config_invalid")
		}
		existing = true
	} else if !os.IsNotExist(err) {
		return err
	}
	if !existing {
		if rt.Preflight == nil {
			return errors.New("tinkercloud: setup_preflight_failed")
		}
		if preflightErr := rt.Preflight(rt.Init); preflightErr != nil {
			return preflightErr
		}
	}

	domain, operator, sender := cfg.Domain, cfg.ACMEEmail, cfg.EmailFrom
	var err error
	if domain == "" {
		domain, err = rt.ReadLine(os.Stdin, out, "Base domain: ")
		if err != nil || domain == "" {
			return errors.New("tinkercloud: setup_input_required")
		}
	}
	if operator == "" {
		operator, err = rt.ReadLine(os.Stdin, out, "Operator email: ")
		if err != nil {
			return errors.New("tinkercloud: setup_input_required")
		}
		operator, err = identity.Normalize(operator)
		if err != nil {
			return errors.New("tinkercloud: config_invalid")
		}
	}
	if sender == "" {
		sender, err = rt.ReadLine(os.Stdin, out, "Verified Resend sender email: ")
		if err != nil {
			return errors.New("tinkercloud: setup_input_required")
		}
		sender, err = identity.Normalize(sender)
		if err != nil {
			return errors.New("tinkercloud: config_invalid")
		}
	}

	// Existing credentials are already the durable secret boundary. Otherwise
	// ask for only a source path, validate it before any host mutation, and
	// generate the HMAC material into a private temporary source file.
	haveCredentials := false
	if info, statErr := os.Lstat(*credentialPath); statErr == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
			return errors.New("tinkercloud: unsafe_credential_path")
		}
		haveCredentials = true
	} else if !os.IsNotExist(statErr) {
		return statErr
	}
	resendFile, hmacFile := "", ""
	if !haveCredentials {
		resendFile, err = rt.ReadLine(os.Stdin, out, "Root-readable Resend API key file: ")
		if err != nil || !filepath.IsAbs(resendFile) {
			return errors.New("tinkercloud: setup_secret_source_required")
		}
		if _, err = readSecretFile(resendFile); err != nil {
			return err
		}
		dir, mkErr := os.MkdirTemp("", "tinkercloud-setup-")
		if mkErr != nil {
			return mkErr
		}
		defer os.RemoveAll(dir)
		hmacFile = filepath.Join(dir, "hmac")
		key := make([]byte, 32)
		if _, err = rand.Read(key); err != nil {
			return errors.New("tinkercloud: hmac_generation_failed")
		}
		if err = os.WriteFile(hmacFile, []byte(hex.EncodeToString(key)), 0600); err != nil {
			return err
		}
	}

	initArgs := []string{"--non-interactive", "--config", *cfgPath, "--credentials", *credentialPath, "--operator-email", operator}
	if !existing {
		initArgs = append(initArgs, "--domain", domain, "--email-from", sender)
	}
	if !haveCredentials {
		initArgs = append(initArgs, "--resend-api-key-file", resendFile, "--hmac-key-file", hmacFile)
	}
	return rt.RunInit(initArgs, out, rt.Init)
}
