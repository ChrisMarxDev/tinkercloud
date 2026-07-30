package main

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/user"
	"strconv"
	"strings"

	"github.com/ChrisMarxDev/tinkercloud/internal/config"
)

var (
	llmWritePrivate = writeAtomicPrivate
	llmLookupGroup  = user.LookupGroup
	llmChown        = os.Chown
	llmChmod        = os.Chmod
)

// runLLMEnable gives an existing host the same root-owned capability root
// boundary created by `tinkercloud init`. It deliberately has no flag for a
// caller-supplied key and never prints the generated value.
func runLLMEnable(args []string, out *os.File) error {
	if effectiveUID() != 0 {
		return errors.New("tinkercloud: root_required")
	}
	fs := flag.NewFlagSet("llm enable", flag.ContinueOnError)
	fs.SetOutput(nil)
	configPath := fs.String("config", defaultConfigPath, "")
	credentialPath := fs.String("credentials", defaultCredentialPath, "")
	if fs.Parse(args) != nil || len(fs.Args()) != 0 || !pathHasNoSymlink(*configPath, false) || !pathHasNoSymlink(*credentialPath, false) {
		return errors.New("tinkercloud: invalid_arguments")
	}
	cfg, err := config.LoadYAML(*configPath)
	if err != nil {
		return errors.New("tinkercloud: config_invalid")
	}
	hasRoot, rootState, credentialBytes := credentialRootState(*credentialPath)
	if rootState != nil {
		return errors.New("tinkercloud: llm_root_unavailable")
	}
	if cfg.LLMRootKeyRef != "" {
		if cfg.LLMRootKeyRef != "env:TINKERCLOUD_LLM_ROOT_KEY" || !hasRoot {
			return errors.New("tinkercloud: llm_root_unavailable")
		}
		fmt.Fprintln(out, "LLM capability root already configured.")
		return nil
	}
	info, err := os.Lstat(*credentialPath)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0077 != 0 {
		return errors.New("tinkercloud: unsafe_credential_path")
	}
	if !hasRoot {
		root := make([]byte, 32)
		if _, err := rand.Read(root); err != nil {
			return errors.New("tinkercloud: llm_root_generation_failed")
		}
		if len(credentialBytes) > 0 && credentialBytes[len(credentialBytes)-1] != '\n' {
			credentialBytes = append(credentialBytes, '\n')
		}
		credentialBytes = append(credentialBytes, []byte("TINKERCLOUD_LLM_ROOT_KEY="+hex.EncodeToString(root)+"\n")...)
		if llmWritePrivate(*credentialPath, credentialBytes) != nil || llmChown(*credentialPath, 0, 0) != nil || llmChmod(*credentialPath, 0600) != nil {
			return errors.New("tinkercloud: llm_root_write_failed")
		}
	}
	cfg.LLMRootKeyRef = "env:TINKERCLOUD_LLM_ROOT_KEY"
	rendered, err := cfg.RenderYAML()
	if err != nil || llmWritePrivate(*configPath, rendered) != nil {
		return errors.New("tinkercloud: llm_config_write_failed")
	}
	group, err := llmLookupGroup("tinkercloud")
	if err != nil {
		return errors.New("tinkercloud: service_identity_failed")
	}
	gid, err := strconv.Atoi(group.Gid)
	if err != nil || gid < 1 || llmChown(*configPath, 0, gid) != nil || llmChmod(*configPath, 0640) != nil {
		return errors.New("tinkercloud: config_ownership_failed")
	}
	fmt.Fprintln(out, "LLM capability root configured. Restart tinkercloud.service to enable operator connection controls.")
	return nil
}

// credentialRootState reads only the shape of the LLM root entry. It never
// returns the secret and treats a duplicate, malformed, or empty value as a
// failure rather than risking a different root on retry.
func credentialRootState(path string) (bool, error, []byte) {
	b, err := os.ReadFile(path)
	if err != nil {
		return false, err, nil
	}
	count := 0
	for _, line := range strings.Split(string(b), "\n") {
		key, value, ok := strings.Cut(line, "=")
		if ok && key == "TINKERCLOUD_LLM_ROOT_KEY" {
			count++
			decoded, err := hex.DecodeString(value)
			if err != nil || len(decoded) != 32 {
				return false, errors.New("invalid root"), nil
			}
		}
	}
	if count > 1 {
		return false, errors.New("duplicate root"), nil
	}
	return count == 1, nil, b
}
