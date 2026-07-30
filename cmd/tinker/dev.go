package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/ChrisMarxDev/tinkercloud/internal/emulator"
	"github.com/ChrisMarxDev/tinkercloud/internal/releases"
)

// devConfig is deliberately non-interactive: an app directory is the only
// ambiguous input and it defaults to the current directory.
func devConfig(args []string) (emulator.Config, error) {
	// Accept the natural `tinker dev ./app --listen ...` spelling as well as
	// flags-first form. The base flag package otherwise stops at ./app.
	var flags, positional []string
	for i := 0; i < len(args); i++ {
		if (args[i] == "--listen" || args[i] == "--viewer" || args[i] == "--email") && i+1 < len(args) {
			flags = append(flags, args[i], args[i+1])
			i++
			continue
		}
		if len(args[i]) > 2 && args[i][:2] == "--" {
			flags = append(flags, args[i])
			continue
		}
		positional = append(positional, args[i])
	}
	fs := flag.NewFlagSet("dev", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	listen := fs.String("listen", "127.0.0.1:8787", "loopback listener")
	viewer := fs.String("viewer", "local-viewer", "development viewer identity")
	email := fs.String("email", "local@example.test", "development viewer email")
	if err := fs.Parse(append(flags, positional...)); err != nil {
		return emulator.Config{}, err
	}
	if fs.NArg() > 1 {
		return emulator.Config{}, errors.New("usage: tinker dev [directory] [--listen 127.0.0.1:8787]")
	}
	project := "."
	if fs.NArg() == 1 {
		project = fs.Arg(0)
	}
	project, err := filepath.Abs(project)
	if err != nil {
		return emulator.Config{}, err
	}
	if stat, err := os.Stat(project); err != nil || !stat.IsDir() {
		return emulator.Config{}, errors.New("app directory is not available")
	}
	appDir, slug := project, "local-app"
	if raw, err := os.ReadFile(filepath.Join(project, "tinker.yaml")); err == nil {
		m, e := releases.ParseManifest(raw)
		if e != nil {
			return emulator.Config{}, errors.New("tinker.yaml is not valid")
		}
		if !safeOutput(project, m.BuildOutput) {
			return emulator.Config{}, errors.New("manifest build output is unsafe")
		}
		appDir = filepath.Join(project, filepath.FromSlash(m.BuildOutput))
		slug = m.Name
	}
	if stat, err := os.Stat(appDir); err != nil || !stat.IsDir() {
		return emulator.Config{}, errors.New("app build output directory is not available")
	}
	return emulator.Config{AppDir: appDir, StateDir: filepath.Join(project, ".tinker", "local"), Listen: *listen, Slug: slug, ViewerID: *viewer, ViewerEmail: *email}, nil
}
func runDev(args []string, jsonOutput bool, stdout, stderr io.Writer) int {
	if jsonOutput {
		writeTo(stdout, stderr, true, result{Error: &cliError{"usage", "tinker dev is a local server and does not support --json."}})
		return 2
	}
	cfg, err := devConfig(args)
	if err != nil {
		fmt.Fprintln(stderr, "tinker dev:", err)
		return 2
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	s, err := emulator.New(ctx, cfg)
	if err != nil {
		fmt.Fprintln(stderr, "tinker dev:", err)
		return 1
	}
	defer s.Close()
	l, err := net.Listen("tcp", cfg.Listen)
	if err != nil {
		fmt.Fprintf(stderr, "tinker dev: cannot listen on %s; choose an unused loopback port: %v\n", cfg.Listen, err)
		return 1
	}
	fmt.Fprintf(stdout, "Tinker development server: http://%s\nDevelopment identity: %s <%s>; app: %s\nState: %s\n", l.Addr(), cfg.ViewerID, cfg.ViewerEmail, cfg.Slug, cfg.StateDir)
	done := make(chan error, 1)
	go func() { done <- http.Serve(l, s.Handler()) }()
	select {
	case <-ctx.Done():
		_ = l.Close()
		if e := <-done; e != nil && !errors.Is(e, http.ErrServerClosed) {
			fmt.Fprintln(stderr, "tinker dev:", e)
		}
	case e := <-done:
		if e != nil && !errors.Is(e, http.ErrServerClosed) {
			fmt.Fprintln(stderr, "tinker dev:", e)
			return 1
		}
	}
	return 0
}
