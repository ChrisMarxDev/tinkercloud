package client

import (
	"context"
	"errors"
	"os/exec"
	"runtime"
	"strings"
)

var ErrStore = errors.New("credential store unavailable")

type MemoryStore map[string]string

func (m MemoryStore) Get(k string) (string, error) {
	v, ok := m[k]
	if !ok {
		return "", ErrStore
	}
	return v, nil
}
func (m MemoryStore) Put(k, v string) error { m[k] = v; return nil }

// OSStore intentionally has no filesystem fallback. Platform commands receive
// secret material through stdin when supported and their output is discarded.
type OSStore struct{ Context context.Context }

func (s OSStore) Get(k string) (string, error) {
	ctx := s.Context
	if ctx == nil {
		ctx = context.Background()
	}
	var c *exec.Cmd
	if runtime.GOOS == "darwin" {
		c = exec.CommandContext(ctx, "/usr/bin/security", "find-generic-password", "-s", k, "-w")
	} else if runtime.GOOS == "linux" {
		c = exec.CommandContext(ctx, "secret-tool", "lookup", "service", "tiny", "profile", k)
	} else {
		return "", ErrStore
	}
	b, e := c.Output()
	if e != nil {
		return "", ErrStore
	}
	return strings.TrimSpace(string(b)), nil
}
func (s OSStore) Put(k, v string) error {
	ctx := s.Context
	if ctx == nil {
		ctx = context.Background()
	}
	var c *exec.Cmd
	if runtime.GOOS == "darwin" {
		c = exec.CommandContext(ctx, "/usr/bin/security", "add-generic-password", "-U", "-s", k, "-w", v)
	} else if runtime.GOOS == "linux" {
		c = exec.CommandContext(ctx, "secret-tool", "store", "--label=Tiny deployer token", "service", "tiny", "profile", k)
		c.Stdin = strings.NewReader(v)
	} else {
		return ErrStore
	}
	if e := c.Run(); e != nil {
		return ErrStore
	}
	return nil
}
