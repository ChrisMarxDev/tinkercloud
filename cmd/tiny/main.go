// tiny is the intentionally small deployer-facing composition root. Network
// transport is attached by the control-plane adapter; this command never owns a
// listener or imports server persistence/runtime packages.
package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/tinyhost/tiny/internal/client"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/tinyhost/tiny/internal/releases"
)

type result struct {
	Valid bool      `json:"valid"`
	Name  string    `json:"name,omitempty"`
	Error *cliError `json:"error,omitempty"`
}
type cliError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func main() {
	os.Exit(runWith(os.Args[1:], os.Stdout, os.Stderr, runnerDeps{store: client.OSStore{}, prompt: stdinPrompt{r: bufio.NewReader(os.Stdin), w: os.Stderr}}))
}

type runnerDeps struct {
	store     client.Store
	prompt    client.Prompt
	newClient func(string, string) client.Client
}

func runWith(argv []string, stdout, stderr io.Writer, deps runnerDeps) int {
	// Accept global flags before or after the subcommand without treating them as
	// positional arguments. Command-specific --file remains in argv.
	var globals, rest []string
	for i := 0; i < len(argv); i++ {
		if argv[i] == "--json" {
			globals = append(globals, argv[i])
			continue
		}
		if argv[i] == "--server" && i+1 < len(argv) {
			globals = append(globals, argv[i], argv[i+1])
			i++
			continue
		}
		rest = append(rest, argv[i])
	}
	argv = append(globals, rest...)
	flag.CommandLine = flag.NewFlagSet("tiny", flag.ContinueOnError)
	flag.CommandLine.SetOutput(stderr)
	jsonOutput := flag.Bool("json", false, "write deterministic JSON")
	server := flag.String("server", "", "platform server")
	if err := flag.CommandLine.Parse(argv); err != nil {
		return 2
	}
	args := flag.Args()
	if len(args) == 1 && args[0] == "login" {
		if *server == "" {
			writeTo(stdout, stderr, *jsonOutput, result{Error: &cliError{"usage", "login requires --server"}})
			return 2
		}
		base, e := client.NormalizeServer(*server, false)
		if e != nil {
			writeTo(stdout, stderr, *jsonOutput, result{Error: &cliError{"invalid_server", "Server must use HTTPS."}})
			return 2
		}
		out, e := client.Login(context.Background(), base, deps.prompt)
		if e != nil {
			writeTo(stdout, stderr, *jsonOutput, result{Error: &cliError{"login_failed", "Login could not be completed."}})
			return 1
		}
		if deps.store == nil {
			writeTo(stdout, stderr, *jsonOutput, result{Error: &cliError{"credential_store", "Token could not be stored."}})
			return 1
		}
		if e = deps.store.Put(base, out.Token); e != nil {
			writeTo(stdout, stderr, *jsonOutput, result{Error: &cliError{"credential_store", "Token could not be stored."}})
			return 1
		}
		writeTo(stdout, stderr, *jsonOutput, result{Valid: true, Name: out.Email})
		return 0
	}
	if len(args) == 1 && args[0] == "version" {
		fmt.Fprintln(stdout, "tiny v1")
		return 0
	}
	if len(args) >= 1 && args[0] == "whoami" {
		if *server == "" {
			writeTo(stdout, stderr, *jsonOutput, result{Error: &cliError{"usage", "whoami requires --server"}})
			return 2
		}
		base, e := client.NormalizeServer(*server, false)
		if e != nil {
			writeTo(stdout, stderr, *jsonOutput, result{Error: &cliError{"invalid_server", "Server must use HTTPS."}})
			return 2
		}
		if deps.store == nil {
			writeTo(stdout, stderr, *jsonOutput, result{Error: &cliError{"credential_store", "Token unavailable."}})
			return 1
		}
		token, e := deps.store.Get(base)
		if e != nil {
			writeTo(stdout, stderr, *jsonOutput, result{Error: &cliError{"not_authenticated", "Login required."}})
			return 1
		}
		var me struct {
			Email string `json:"email"`
		}
		if e = client.New(base, token).Do(context.Background(), "GET", "/api/v1/whoami", "", nil, &me); e != nil {
			writeTo(stdout, stderr, *jsonOutput, result{Error: &cliError{"not_authenticated", "Login required."}})
			return 1
		}
		writeTo(stdout, stderr, *jsonOutput, result{Valid: true, Name: me.Email})
		return 0
	}
	if len(args) == 3 && args[0] == "access" && args[1] == "get" {
		if *server == "" || deps.store == nil {
			writeTo(stdout, stderr, *jsonOutput, result{Error: &cliError{"usage", "access get requires --server and login"}})
			return 2
		}
		base, e := client.NormalizeServer(*server, false)
		if e != nil {
			writeTo(stdout, stderr, *jsonOutput, result{Error: &cliError{"invalid_server", "Server must use HTTPS."}})
			return 2
		}
		token, e := deps.store.Get(base)
		if e != nil {
			writeTo(stdout, stderr, *jsonOutput, result{Error: &cliError{"not_authenticated", "Login required."}})
			return 1
		}
		var access any
		if e = client.New(base, token).Do(context.Background(), "GET", "/api/v1/apps/"+url.PathEscape(args[2])+"/access", "", nil, &access); e != nil {
			writeTo(stdout, stderr, *jsonOutput, result{Error: &cliError{"not_authorized", "Request not authorized."}})
			return 1
		}
		if *jsonOutput {
			b, _ := json.Marshal(access)
			fmt.Fprintln(stdout, string(b))
		} else {
			b, _ := json.Marshal(access)
			fmt.Fprintf(stdout, "Access policy: %s\n", string(b))
		}
		return 0
	}
	if len(args) >= 2 && args[0] == "tokens" {
		return runTokens(args[1:], *server, *jsonOutput, stdout, stderr, deps)
	}
	if (len(args) == 2 || len(args) == 3) && args[0] == "releases" && releases.ValidSlug(args[len(args)-1]) && (len(args) == 2 || args[1] == "list") {
		return runReleases(args[len(args)-1], *server, *jsonOutput, stdout, stderr, deps)
	}
	if len(args) == 5 && args[0] == "apps" && args[1] == "delete" && releases.ValidSlug(args[2]) && args[3] == "--confirm" && args[4] == "delete:"+args[2] {
		return runAppDelete(args[2], *server, *jsonOutput, stdout, stderr, deps)
	}
	if len(args) >= 2 && args[0] == "releases" {
		writeTo(stdout, stderr, *jsonOutput, result{Error: &cliError{"usage", "usage: tiny [--json] --server URL releases [list] APP"}})
		return 2
	}
	if len(args) >= 2 && args[0] == "apps" && args[1] == "delete" {
		writeTo(stdout, stderr, *jsonOutput, result{Error: &cliError{"usage", "usage: tiny [--json] --server URL apps delete APP --confirm delete:APP"}})
		return 2
	}
	if len(args) == 5 && args[0] == "access" && args[1] == "set" && args[3] == "--file" {
		if *server == "" || deps.store == nil {
			writeTo(stdout, stderr, *jsonOutput, result{Error: &cliError{"usage", "access set requires --server and login"}})
			return 2
		}
		b, e := os.ReadFile(args[4])
		if e != nil {
			writeTo(stdout, stderr, *jsonOutput, result{Error: &cliError{"read_failed", "Policy file could not be read."}})
			return 1
		}
		var policy map[string]any
		de := json.NewDecoder(bytes.NewReader(b))
		if e = de.Decode(&policy); e != nil || policy == nil || de.Decode(&struct{}{}) != io.EOF {
			writeTo(stdout, stderr, *jsonOutput, result{Error: &cliError{"invalid_policy", "Policy file must be valid JSON."}})
			return 1
		}
		base, e := client.NormalizeServer(*server, false)
		if e != nil {
			writeTo(stdout, stderr, *jsonOutput, result{Error: &cliError{"invalid_server", "Server must use HTTPS."}})
			return 2
		}
		token, e := deps.store.Get(base)
		if e != nil {
			writeTo(stdout, stderr, *jsonOutput, result{Error: &cliError{"not_authenticated", "Login required."}})
			return 1
		}
		deployer := client.New(base, token)
		if deps.newClient != nil {
			deployer = deps.newClient(base, token)
		}
		if _, supplied := policy["expected_revision"]; !supplied {
			var current struct {
				Revision uint64 `json:"revision"`
			}
			if e = deployer.Do(context.Background(), "GET", "/api/v1/apps/"+url.PathEscape(args[2])+"/access", "", nil, &current); e != nil || current.Revision == 0 {
				writeTo(stdout, stderr, *jsonOutput, result{Error: &cliError{"policy_read_failed", "Current policy could not be read."}})
				return 1
			}
			policy["expected_revision"] = current.Revision
		}
		key, e := client.IdempotencyKey()
		if e != nil {
			return 1
		}
		if e = deployer.Do(context.Background(), "PUT", "/api/v1/apps/"+url.PathEscape(args[2])+"/access", key, policy, nil); e != nil {
			writeTo(stdout, stderr, *jsonOutput, result{Error: &cliError{"not_authorized", "Policy update failed."}})
			return 1
		}
		writeTo(stdout, stderr, *jsonOutput, result{Valid: true, Name: "access policy updated"})
		return 0
	}
	if len(args) == 3 && args[0] == "rollback" {
		if *server == "" || deps.store == nil {
			writeTo(stdout, stderr, *jsonOutput, result{Error: &cliError{"usage", "rollback requires --server and login"}})
			return 2
		}
		base, e := client.NormalizeServer(*server, false)
		if e != nil {
			writeTo(stdout, stderr, *jsonOutput, result{Error: &cliError{"invalid_server", "Server must use HTTPS."}})
			return 2
		}
		token, e := deps.store.Get(base)
		if e != nil {
			writeTo(stdout, stderr, *jsonOutput, result{Error: &cliError{"not_authenticated", "Login required."}})
			return 1
		}
		key, e := client.IdempotencyKey()
		if e != nil {
			return 1
		}
		payload := map[string]string{"deployment": args[2]}
		if e = client.New(base, token).Do(context.Background(), "POST", "/api/v1/apps/"+url.PathEscape(args[1])+"/rollback", key, payload, nil); e != nil {
			writeTo(stdout, stderr, *jsonOutput, result{Error: &cliError{"rollback_failed", "Rollback could not be completed."}})
			return 1
		}
		writeTo(stdout, stderr, *jsonOutput, result{Valid: true, Name: "rollback requested"})
		return 0
	}
	if len(args) == 2 && args[0] == "apps" && args[1] == "list" {
		if *server == "" || deps.store == nil {
			return 2
		}
		base, e := client.NormalizeServer(*server, false)
		if e != nil {
			return 2
		}
		token, e := deps.store.Get(base)
		if e != nil {
			return 1
		}
		var out any
		if e = client.New(base, token).Do(context.Background(), "GET", "/api/v1/apps", "", nil, &out); e != nil {
			writeTo(stdout, stderr, *jsonOutput, result{Error: &cliError{"apps_failed", "Apps request failed."}})
			return 1
		}
		b, _ := json.Marshal(out)
		fmt.Fprintln(stdout, string(b))
		return 0
	}
	if len(args) == 3 && args[0] == "apps" && args[1] == "create" {
		if *server == "" || deps.store == nil || !releases.ValidSlug(args[2]) {
			return 2
		}
		base, e := client.NormalizeServer(*server, false)
		if e != nil {
			return 2
		}
		token, e := deps.store.Get(base)
		if e != nil {
			return 1
		}
		k, e := client.IdempotencyKey()
		if e != nil {
			return 1
		}
		var out any
		if e = client.New(base, token).Do(context.Background(), "POST", "/api/v1/apps", k, map[string]string{"slug": args[2]}, &out); e != nil {
			writeTo(stdout, stderr, *jsonOutput, result{Error: &cliError{"app_create_failed", "App could not be created."}})
			return 1
		}
		b, _ := json.Marshal(out)
		fmt.Fprintln(stdout, string(b))
		return 0
	}
	if len(args) == 2 && args[0] == "deploy" {
		if *server == "" || deps.store == nil {
			writeTo(stdout, stderr, *jsonOutput, result{Error: &cliError{"usage", "deploy requires --server and login"}})
			return 2
		}
		base, e := client.NormalizeServer(*server, false)
		if e != nil {
			writeTo(stdout, stderr, *jsonOutput, result{Error: &cliError{"invalid_server", "Server must use HTTPS."}})
			return 2
		}
		token, e := deps.store.Get(base)
		if e != nil {
			writeTo(stdout, stderr, *jsonOutput, result{Error: &cliError{"not_authenticated", "Login required."}})
			return 1
		}
		manifest, e := os.ReadFile(filepath.Join(args[1], "tiny.yaml"))
		if e != nil {
			writeTo(stdout, stderr, *jsonOutput, result{Error: &cliError{"invalid_manifest", "Could not read manifest."}})
			return 1
		}
		m, e := releases.ParseManifest(manifest)
		if e != nil {
			writeTo(stdout, stderr, *jsonOutput, result{Error: &cliError{"invalid_manifest", "Manifest does not meet the V1 contract."}})
			return 1
		}
		archive, cleanup, err := stagedArchive(args[1], manifest)
		if err != nil {
			writeTo(stdout, stderr, *jsonOutput, result{Error: &cliError{"invalid_directory", "Deployment directory is unsafe."}})
			return 1
		}
		defer cleanup()
		key, e := client.IdempotencyKey()
		if e != nil {
			writeTo(stdout, stderr, *jsonOutput, result{Error: &cliError{"deploy_failed", "Deployment could not be prepared."}})
			return 1
		}
		deployer := client.New(base, token)
		if deps.newClient != nil {
			deployer = deps.newClient(base, token)
		}
		// The manifest owns the stable slug and the candidate policy. Ensure the
		// caller owns that slug before streaming; a foreign-slug conflict is not
		// a successful deployment and is intentionally kept indistinguishable.
		if e = deployer.EnsureApp(context.Background(), m.Name); e != nil {
			writeTo(stdout, stderr, *jsonOutput, result{Error: &cliError{"deploy_failed", "Deployment could not be prepared."}})
			return 1
		}
		out, e := deployer.Deploy(context.Background(), m.Name, archive, archiveSize(archive), key)
		if e != nil {
			writeTo(stdout, stderr, *jsonOutput, result{Error: &cliError{"deploy_failed", "Deployment could not be verified."}})
			return 1
		}
		writeTo(stdout, stderr, *jsonOutput, result{Valid: true, Name: out.URL})
		return 0
	}
	if len(args) != 2 || args[0] != "inspect-manifest" {
		writeTo(stdout, stderr, *jsonOutput, result{Error: &cliError{"usage", "usage: tiny [--json] inspect-manifest PATH"}})
		return 2
	}
	b, err := os.ReadFile(args[1])
	if err != nil {
		writeTo(stdout, stderr, *jsonOutput, result{Error: &cliError{"read_failed", "Could not read manifest."}})
		return 1
	}
	m, err := releases.ParseManifest(b)
	if err != nil {
		writeTo(stdout, stderr, *jsonOutput, result{Error: &cliError{"invalid_manifest", "Manifest does not meet the V1 contract."}})
		return 1
	}
	writeTo(stdout, stderr, *jsonOutput, result{Valid: true, Name: m.Name})
	return 0
}

// stagedArchive keeps deploy bundles out of process memory and gives the
// network client a stable, seekable body for retries. CreateTemp is 0600 by
// default; chmod makes that invariant explicit on all supported filesystems.
func stagedArchive(project string, manifest []byte) (*os.File, func(), error) {
	f, err := os.CreateTemp("", "tiny-deploy-*.tar.gz")
	if err != nil {
		return nil, nil, err
	}
	cleanup := func() {
		_ = f.Close()
		_ = os.Remove(f.Name())
	}
	if err = f.Chmod(0600); err != nil {
		cleanup()
		return nil, nil, err
	}
	if err = client.ArchiveProject(project, manifest, f); err != nil {
		cleanup()
		return nil, nil, err
	}
	if err = f.Close(); err != nil {
		cleanup()
		return nil, nil, err
	}
	opened, err := os.Open(f.Name())
	if err != nil {
		cleanup()
		return nil, nil, err
	}
	return opened, func() {
		_ = opened.Close()
		_ = os.Remove(f.Name())
	}, nil
}

func archiveSize(f *os.File) int64 {
	info, err := f.Stat()
	if err != nil {
		return -1
	}
	return info.Size()
}

const tokenUsage = "usage: tiny [--json] --server URL tokens <list APP|create APP --scope SCOPE [--scope SCOPE] --expires-in SECONDS|revoke APP TOKEN_ID>"

// runTokens keeps the token lifecycle intentionally separate from generic
// command output: a newly-created secret is written once, while list/revoke
// output can never contain a raw credential.
func runTokens(args []string, server string, jsonOutput bool, stdout, stderr io.Writer, deps runnerDeps) int {
	if server == "" || deps.store == nil {
		writeTo(stdout, stderr, jsonOutput, result{Error: &cliError{"usage", tokenUsage}})
		return 2
	}
	base, err := client.NormalizeServer(server, false)
	if err != nil {
		writeTo(stdout, stderr, jsonOutput, result{Error: &cliError{"invalid_server", "Server must use HTTPS."}})
		return 2
	}
	credential, err := deps.store.Get(base)
	if err != nil {
		writeTo(stdout, stderr, jsonOutput, result{Error: &cliError{"not_authenticated", "Login required."}})
		return 1
	}
	newClient := deps.newClient
	if newClient == nil {
		newClient = client.New
	}
	c := newClient(base, credential)
	if len(args) == 2 && args[0] == "list" && releases.ValidSlug(args[1]) {
		items, err := c.ListTokens(context.Background(), args[1])
		if err != nil {
			writeTo(stdout, stderr, jsonOutput, result{Error: &cliError{"not_authorized", "Request not authorized."}})
			return 1
		}
		if jsonOutput {
			writeJSON(stdout, items)
			return 0
		}
		for _, item := range items {
			state := "active"
			if item.Revoked {
				state = "revoked"
			}
			fmt.Fprintf(stdout, "%s\t%s\t%s\t%s\n", item.ID, strings.Join(item.Scopes, ","), item.ExpiresAt, state)
		}
		return 0
	}
	if len(args) >= 2 && args[0] == "create" && releases.ValidSlug(args[1]) {
		input, ok := parseTokenInput(args[2:])
		if !ok {
			writeTo(stdout, stderr, jsonOutput, result{Error: &cliError{"usage", tokenUsage}})
			return 2
		}
		key, err := client.IdempotencyKey()
		if err != nil {
			writeTo(stdout, stderr, jsonOutput, result{Error: &cliError{"request_failed", "Token could not be created."}})
			return 1
		}
		created, err := c.CreateToken(context.Background(), args[1], input, key)
		if err != nil || created.Token == "" {
			writeTo(stdout, stderr, jsonOutput, result{Error: &cliError{"token_create_failed", "Token could not be created."}})
			return 1
		}
		if jsonOutput {
			writeJSON(stdout, created)
		} else {
			// The raw secret is purposefully the complete and only success output.
			fmt.Fprintln(stdout, created.Token)
		}
		return 0
	}
	if len(args) == 3 && args[0] == "revoke" && releases.ValidSlug(args[1]) && validTokenID(args[2]) {
		key, err := client.IdempotencyKey()
		if err != nil {
			writeTo(stdout, stderr, jsonOutput, result{Error: &cliError{"request_failed", "Token could not be revoked."}})
			return 1
		}
		if err = c.RevokeToken(context.Background(), args[1], args[2], key); err != nil {
			writeTo(stdout, stderr, jsonOutput, result{Error: &cliError{"token_revoke_failed", "Token could not be revoked."}})
			return 1
		}
		out := struct {
			ID      string `json:"id"`
			Revoked bool   `json:"revoked"`
		}{ID: args[2], Revoked: true}
		if jsonOutput {
			writeJSON(stdout, out)
		} else {
			fmt.Fprintf(stdout, "Token revoked: %s\n", args[2])
		}
		return 0
	}
	writeTo(stdout, stderr, jsonOutput, result{Error: &cliError{"usage", tokenUsage}})
	return 2
}

func parseTokenInput(args []string) (client.TokenInput, bool) {
	var out client.TokenInput
	seen := map[string]bool{}
	for len(args) > 0 {
		switch args[0] {
		case "--scope":
			if len(args) < 2 || !validScope(args[1]) || seen[args[1]] {
				return client.TokenInput{}, false
			}
			seen[args[1]] = true
			out.Scopes = append(out.Scopes, args[1])
			args = args[2:]
		case "--expires-in":
			if len(args) < 2 || out.ExpiresInSeconds != 0 {
				return client.TokenInput{}, false
			}
			seconds, err := strconv.ParseInt(args[1], 10, 64)
			if err != nil || seconds < 60 || seconds > 31536000 {
				return client.TokenInput{}, false
			}
			out.ExpiresInSeconds = seconds
			args = args[2:]
		default:
			return client.TokenInput{}, false
		}
	}
	return out, len(out.Scopes) > 0 && out.ExpiresInSeconds != 0
}

func validScope(scope string) bool {
	return map[string]bool{
		"app:read": true, "app:create": true, "deploy:create": true, "deploy:activate": true,
		"access:read": true, "access:write": true, "token:create": true, "token:revoke": true,
		"app:delete": true,
	}[scope]
}

// runReleases intentionally only exposes immutable release metadata. It does
// not surface archive paths, policy details, or credential-bearing transport
// errors to a deployer terminal.
func runReleases(slug, server string, jsonOutput bool, stdout, stderr io.Writer, deps runnerDeps) int {
	c, code := authenticatedClient(server, jsonOutput, stdout, stderr, deps)
	if code != 0 {
		return code
	}
	items, err := c.ListReleases(context.Background(), slug)
	if err != nil {
		writeTo(stdout, stderr, jsonOutput, result{Error: &cliError{"not_authorized", "Request not authorized."}})
		return 1
	}
	if jsonOutput {
		writeJSON(stdout, items)
		return 0
	}
	for _, item := range items {
		fmt.Fprintf(stdout, "%s\t%s\t%s\t%s\t%s\t%s\n", item.ID, item.ReleaseHash, item.State, item.CreatedAt, item.VerifiedAt, item.ActivatedAt)
	}
	return 0
}

// runAppDelete is deliberately reachable only after runWith has checked an
// exact, app-bound confirmation phrase. The request is idempotent but still
// carries a fresh idempotency key so the server can audit the destructive act.
func runAppDelete(slug, server string, jsonOutput bool, stdout, stderr io.Writer, deps runnerDeps) int {
	c, code := authenticatedClient(server, jsonOutput, stdout, stderr, deps)
	if code != 0 {
		return code
	}
	key, err := client.IdempotencyKey()
	if err != nil {
		writeTo(stdout, stderr, jsonOutput, result{Error: &cliError{"request_failed", "App could not be deleted."}})
		return 1
	}
	if err = c.DeleteApp(context.Background(), slug, key); err != nil {
		writeTo(stdout, stderr, jsonOutput, result{Error: &cliError{"not_authorized", "Request not authorized."}})
		return 1
	}
	out := struct {
		Slug    string `json:"slug"`
		Deleted bool   `json:"deleted"`
	}{Slug: slug, Deleted: true}
	if jsonOutput {
		writeJSON(stdout, out)
	} else {
		fmt.Fprintf(stdout, "App deleted: %s\n", slug)
	}
	return 0
}

// authenticatedClient centralizes the command-side credential boundary: an
// absent credential is never passed to a client and is never printed back.
func authenticatedClient(server string, jsonOutput bool, stdout, stderr io.Writer, deps runnerDeps) (client.Client, int) {
	if server == "" || deps.store == nil {
		writeTo(stdout, stderr, jsonOutput, result{Error: &cliError{"usage", "Command requires --server and login."}})
		return client.Client{}, 2
	}
	base, err := client.NormalizeServer(server, false)
	if err != nil {
		writeTo(stdout, stderr, jsonOutput, result{Error: &cliError{"invalid_server", "Server must use HTTPS."}})
		return client.Client{}, 2
	}
	credential, err := deps.store.Get(base)
	if err != nil {
		writeTo(stdout, stderr, jsonOutput, result{Error: &cliError{"not_authenticated", "Login required."}})
		return client.Client{}, 1
	}
	newClient := deps.newClient
	if newClient == nil {
		newClient = client.New
	}
	return newClient(base, credential), 0
}

func validTokenID(id string) bool {
	if id == "" || len(id) > 128 {
		return false
	}
	for _, c := range id {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_' || c == '-') {
			return false
		}
	}
	return true
}

func writeJSON(w io.Writer, value any) {
	b, _ := json.Marshal(value)
	fmt.Fprintln(w, string(b))
}

type stdinPrompt struct {
	r *bufio.Reader
	w io.Writer
}

func (p stdinPrompt) Ask(label string) (string, error) {
	fmt.Fprint(p.w, label)
	s, e := p.r.ReadString('\n')
	return strings.TrimSpace(s), e
}
func writeTo(stdout, stderr io.Writer, j bool, r result) {
	if j {
		b, _ := json.Marshal(r)
		fmt.Fprintln(stdout, string(b))
		return
	}
	if r.Error != nil {
		fmt.Fprintln(stderr, r.Error.Message)
		return
	}
	fmt.Fprintf(stdout, "Manifest valid: %s\n", r.Name)
}
