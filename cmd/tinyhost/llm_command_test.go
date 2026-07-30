package main

import (
	"errors"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tinyhost/tiny/internal/config"
)

func llmCommandConfig(root string) config.Config {
	return config.Config{PlatformHost: "tiny.example.test", AppSuffix: "apps.tiny.example.test", SessionCookie: "__Host-tiny_app", ListenHTTP: ":80", ListenHTTPS: ":443", DataDirectory: filepath.Join(root, "data"), ACMECachedir: filepath.Join(root, "acme"), EmailFrom: "operator@example.test", ACMEEmail: "operator@example.test", ResendAPIKeyRef: "env:RESEND_API_KEY", HMACKeyRef: "env:TINYHOST_HMAC_KEY", OTPExpiry: time.Minute, OTPMaxAttempts: 5, SessionExpiry: time.Hour}
}

func TestLLMEnableRetriesAfterConfigWriteFailureWithoutReplacingRoot(t *testing.T) {
	root, err := os.MkdirTemp("/private/tmp", "tinyhost-llm-test-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	if err := os.MkdirAll(filepath.Join(root, "data"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "acme"), 0700); err != nil {
		t.Fatal(err)
	}
	configPath, credentialPath := filepath.Join(root, "config.yaml"), filepath.Join(root, "credentials.env")
	cfg := llmCommandConfig(root)
	b, err := cfg.RenderYAML()
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(configPath, b, 0640); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(credentialPath, []byte("RESEND_API_KEY=value\nTINYHOST_HMAC_KEY=0123456789abcdef0123456789abcdef\n"), 0600); err != nil {
		t.Fatal(err)
	}
	oldUID, oldWrite, oldGroup, oldChown, oldChmod := effectiveUID, llmWritePrivate, llmLookupGroup, llmChown, llmChmod
	t.Cleanup(func() {
		effectiveUID, llmWritePrivate, llmLookupGroup, llmChown, llmChmod = oldUID, oldWrite, oldGroup, oldChown, oldChmod
	})
	effectiveUID = func() int { return 0 }
	llmLookupGroup = func(string) (*user.Group, error) { return &user.Group{Gid: "1"}, nil }
	llmChown = func(string, int, int) error { return nil }
	llmChmod = os.Chmod
	writes := 0
	llmWritePrivate = func(path string, data []byte) error {
		writes++
		if writes == 2 {
			return errors.New("injected")
		}
		return writeAtomicPrivate(path, data)
	}
	if err := runLLMEnable([]string{"--config", configPath, "--credentials", credentialPath}, os.Stdout); err == nil || !strings.Contains(err.Error(), "llm_config_write_failed") {
		t.Fatalf("first enable=%v", err)
	}
	has, state, first := credentialRootState(credentialPath)
	if !has || state != nil {
		t.Fatalf("first root state has=%v err=%v", has, state)
	}
	llmWritePrivate = writeAtomicPrivate
	if err := runLLMEnable([]string{"--config", configPath, "--credentials", credentialPath}, os.Stdout); err != nil {
		t.Fatalf("retry=%v", err)
	}
	has, state, second := credentialRootState(credentialPath)
	if !has || state != nil || string(first) != string(second) {
		t.Fatalf("root was not preserved has=%v err=%v", has, state)
	}
	loaded, err := config.LoadYAML(configPath)
	if err != nil || loaded.LLMRootKeyRef != "env:TINYHOST_LLM_ROOT_KEY" {
		t.Fatalf("config=%#v err=%v", loaded, err)
	}
}

func TestLLMEnableRejectsMalformedOrDuplicateExistingRoot(t *testing.T) {
	for _, credential := range []string{"TINYHOST_LLM_ROOT_KEY=bad\n", "TINYHOST_LLM_ROOT_KEY=0000000000000000000000000000000000000000000000000000000000000000\nTINYHOST_LLM_ROOT_KEY=0000000000000000000000000000000000000000000000000000000000000000\n"} {
		root := t.TempDir()
		path := filepath.Join(root, "credentials")
		if err := os.WriteFile(path, []byte(credential), 0600); err != nil {
			t.Fatal(err)
		}
		if has, err, _ := credentialRootState(path); has || err == nil {
			t.Fatalf("credential accepted has=%v err=%v", has, err)
		}
	}
}
