// tiny is the intentionally small deployer-facing composition root. Network
// transport is attached by the control-plane adapter; this command never owns a
// listener or imports server persistence/runtime packages.
package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
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
	Valid      bool               `json:"valid"`
	Name       string             `json:"name,omitempty"`
	Deployment *deploymentReceipt `json:"deployment,omitempty"`
	Error      *cliError          `json:"error,omitempty"`
}
type cliError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}
type deploymentReceipt struct {
	ID           string                          `json:"deployment_id,omitempty"`
	URL          string                          `json:"url,omitempty"`
	State        string                          `json:"state"`
	Verification client.DeploymentEvidenceReason `json:"verification,omitempty"`
	Reason       client.ActivationFailureReason  `json:"reason,omitempty"`
	RequestID    string                          `json:"request_id,omitempty"`
}

func main() {
	os.Exit(runWith(os.Args[1:], os.Stdout, os.Stderr, runnerDeps{store: client.FileStore{}, prompt: stdinPrompt{r: bufio.NewReader(os.Stdin), w: os.Stderr}, input: os.Stdin}))
}

type runnerDeps struct {
	store     client.Store
	prompt    client.Prompt
	newClient func(string, string) client.Client
	input     io.Reader
}

const maxSetupServerInput = 2048

var (
	errServerSetupInput  = errors.New("server setup input invalid")
	errServerSetupFailed = errors.New("server setup verification failed")
)

type boundedPrompt interface {
	AskBounded(string, int) (string, error)
}

func runWith(argv []string, stdout, stderr io.Writer, deps runnerDeps) int {
	// Accept global flags before or after the subcommand without treating them as
	// positional arguments. Command-specific --file remains in argv.
	var globals, rest []string
	for i := 0; i < len(argv); i++ {
		if argv[i] == "--json" || argv[i] == "--force" {
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
	forceLogin := flag.Bool("force", false, "ignore a saved login and authenticate again")
	server := flag.String("server", "", "platform server")
	if err := flag.CommandLine.Parse(argv); err != nil {
		return 2
	}
	args := flag.Args()
	if *forceLogin && !(len(args) == 1 && args[0] == "login") {
		writeTo(stdout, stderr, *jsonOutput, result{Error: &cliError{"usage", "--force is only valid with login."}})
		return 2
	}
	if len(args) == 1 && args[0] == "login" {
		base, e := resolveServerForCommand(*server, *jsonOutput, deps)
		if e != nil {
			code, message := serverFailure(e)
			writeTo(stdout, stderr, *jsonOutput, result{Error: &cliError{code, message}})
			return 2
		}
		out, fresh, e := login(context.Background(), base, *forceLogin, deps)
		if e != nil {
			code, message := loginFailure(e)
			writeTo(stdout, stderr, *jsonOutput, result{Error: &cliError{code, message}})
			return 1
		}
		if deps.store == nil {
			writeTo(stdout, stderr, *jsonOutput, result{Error: &cliError{"credential_store", "Token could not be stored."}})
			return 1
		}
		if fresh {
			if e = deps.store.Put(base, out.Token); e != nil {
				writeTo(stdout, stderr, *jsonOutput, result{Error: &cliError{"credential_store", "Token could not be stored."}})
				return 1
			}
		}
		defaults, ok := deps.store.(client.DefaultServerStore)
		if !ok || defaults.SetDefaultServer(base) != nil {
			writeTo(stdout, stderr, *jsonOutput, result{Error: &cliError{"configuration_store", "Server could not be saved."}})
			return 1
		}
		if *jsonOutput {
			writeTo(stdout, stderr, true, result{Valid: true, Name: out.Email})
		} else {
			fmt.Fprintf(stdout, "Logged in as %s.\n", out.Email)
		}
		return 0
	}
	if len(args) == 1 && args[0] == "version" {
		fmt.Fprintln(stdout, "tiny "+client.BuildVersion)
		return 0
	}
	if len(args) >= 1 && args[0] == "dev" {
		return runDev(args[1:], *jsonOutput, stdout, stderr)
	}
	if len(args) >= 1 && args[0] == "host" {
		return runHost(args[1:], *jsonOutput, stdout, stderr)
	}
	if (len(args) == 1 || len(args) == 2) && args[0] == "init" {
		project := "."
		if len(args) == 2 {
			project = args[1]
		}
		return runInit(project, *jsonOutput, stdout, stderr, deps)
	}
	if len(args) == 2 && args[0] == "inspect-manifest" {
		return runInspectManifest(args[1], *jsonOutput, stdout, stderr)
	}
	if !requiresServer(args) {
		writeTo(stdout, stderr, *jsonOutput, result{Error: &cliError{"usage", "usage: tiny [--json] [--server URL] <command>"}})
		return 2
	}
	resolvedServer, resolveErr := resolveServerForCommand(*server, *jsonOutput, deps)
	if resolveErr != nil {
		code, message := serverFailure(resolveErr)
		writeTo(stdout, stderr, *jsonOutput, result{Error: &cliError{code, message}})
		return 2
	}
	if len(args) >= 1 && args[0] == "data" {
		return runData(args[1:], resolvedServer, *jsonOutput, stdout, stderr, deps)
	}
	if len(args) == 1 && args[0] == "logout" {
		if deps.store == nil {
			writeTo(stdout, stderr, *jsonOutput, result{Error: &cliError{"credential_store", "Token unavailable."}})
			return 1
		}
		deleter, ok := deps.store.(client.CredentialDeleter)
		if !ok {
			writeTo(stdout, stderr, *jsonOutput, result{Error: &cliError{"credential_store", "Token unavailable."}})
			return 1
		}
		token, err := deps.store.Get(resolvedServer)
		if errors.Is(err, client.ErrCredentialNotFound) || token == "" && err == nil {
			writeLogoutSuccess(stdout, stderr, *jsonOutput, "Already logged out.")
			return 0
		}
		if err != nil {
			writeTo(stdout, stderr, *jsonOutput, result{Error: &cliError{"credential_store", "Token could not be read."}})
			return 1
		}
		newClient := deps.newClient
		if newClient == nil {
			newClient = client.New
		}
		err = newClient(resolvedServer, token).Logout(context.Background())
		if err != nil && !errors.Is(err, client.ErrUnauthorized) {
			writeTo(stdout, stderr, *jsonOutput, result{Error: &cliError{"logout_failed", "Logout could not be completed."}})
			return 1
		}
		if err = deleter.Delete(resolvedServer); err != nil {
			writeTo(stdout, stderr, *jsonOutput, result{Error: &cliError{"credential_store", "Token could not be removed."}})
			return 1
		}
		writeLogoutSuccess(stdout, stderr, *jsonOutput, "Logged out.")
		return 0
	}
	if len(args) == 1 && args[0] == "whoami" {
		base := resolvedServer
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
		deployer := client.New(base, token)
		if deps.newClient != nil {
			deployer = deps.newClient(base, token)
		}
		if e = deployer.Do(context.Background(), "GET", "/api/v1/whoami", "", nil, &me); e != nil {
			writeTo(stdout, stderr, *jsonOutput, result{Error: &cliError{"not_authenticated", "Login required."}})
			return 1
		}
		writeIdentitySuccess(stdout, stderr, *jsonOutput, me.Email)
		return 0
	}
	if len(args) == 3 && args[0] == "access" && args[1] == "get" {
		if deps.store == nil {
			writeTo(stdout, stderr, *jsonOutput, result{Error: &cliError{"usage", "access get requires login"}})
			return 2
		}
		base := resolvedServer
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
		return runTokens(args[1:], resolvedServer, *jsonOutput, stdout, stderr, deps)
	}
	if (len(args) == 2 || len(args) == 3) && args[0] == "releases" && releases.ValidSlug(args[len(args)-1]) && (len(args) == 2 || args[1] == "list") {
		return runReleases(args[len(args)-1], resolvedServer, *jsonOutput, stdout, stderr, deps)
	}
	if len(args) == 5 && args[0] == "apps" && args[1] == "delete" && releases.ValidSlug(args[2]) && args[3] == "--confirm" && args[4] == "delete:"+args[2] {
		return runAppDelete(args[2], resolvedServer, *jsonOutput, stdout, stderr, deps)
	}
	if len(args) >= 2 && args[0] == "releases" {
		writeTo(stdout, stderr, *jsonOutput, result{Error: &cliError{"usage", "usage: tiny [--json] [--server URL] releases [list] APP"}})
		return 2
	}
	if len(args) >= 2 && args[0] == "apps" && args[1] == "delete" {
		writeTo(stdout, stderr, *jsonOutput, result{Error: &cliError{"usage", "usage: tiny [--json] [--server URL] apps delete APP --confirm delete:APP"}})
		return 2
	}
	if len(args) == 5 && args[0] == "access" && args[1] == "set" && args[3] == "--file" {
		if deps.store == nil {
			writeTo(stdout, stderr, *jsonOutput, result{Error: &cliError{"usage", "access set requires login"}})
			return 2
		}
		b, e := os.ReadFile(args[4])
		if e != nil {
			writeTo(stdout, stderr, *jsonOutput, result{Error: &cliError{"read_failed", "Policy file could not be read."}})
			return 1
		}
		var policy accessPolicyFile
		de := json.NewDecoder(bytes.NewReader(b))
		de.DisallowUnknownFields()
		if e = de.Decode(&policy); e != nil || de.Decode(&struct{}{}) != io.EOF {
			writeTo(stdout, stderr, *jsonOutput, result{Error: &cliError{"invalid_policy", "Policy file must contain only writable access-policy fields."}})
			return 1
		}
		base := resolvedServer
		token, e := deps.store.Get(base)
		if e != nil {
			writeTo(stdout, stderr, *jsonOutput, result{Error: &cliError{"not_authenticated", "Login required."}})
			return 1
		}
		deployer := client.New(base, token)
		if deps.newClient != nil {
			deployer = deps.newClient(base, token)
		}
		var current accessPolicyView
		if e = deployer.Do(context.Background(), "GET", "/api/v1/apps/"+url.PathEscape(args[2])+"/access", "", nil, &current); e != nil || current.Revision == 0 {
			writeTo(stdout, stderr, *jsonOutput, result{Error: &cliError{"policy_read_failed", "Current policy could not be read."}})
			return 1
		}
		if policy.ExpectedRevision == nil {
			policy.ExpectedRevision = &current.Revision
		}
		// The server remains the authorization authority. This client-side read
		// exists solely to name a broadened requested policy before its mutation.
		if policyRevisionMatches(policy, current.Revision) && policyBroadensAccess(policy, current) && !policy.ConfirmBroadening {
			if *jsonOutput || deps.prompt == nil {
				writeTo(stdout, stderr, *jsonOutput, result{Error: &cliError{"confirmation_required", "Policy adds viewer access. Set confirm_broadening to true and retry."}})
				return 1
			}
			answer, promptErr := deps.prompt.Ask("This policy adds viewer access. Continue? [y/N]: ")
			if promptErr != nil || !affirmative(answer) {
				writeTo(stdout, stderr, *jsonOutput, result{Error: &cliError{"confirmation_required", "Access broadening was not confirmed."}})
				return 1
			}
			policy.ConfirmBroadening = true
		}
		key, e := client.IdempotencyKey()
		if e != nil {
			return 1
		}
		if e = deployer.Do(context.Background(), "PUT", "/api/v1/apps/"+url.PathEscape(args[2])+"/access", key, policy, nil); e != nil {
			if errors.Is(e, client.ErrConflict) {
				writeTo(stdout, stderr, *jsonOutput, result{Error: &cliError{"policy_conflict", "Policy changed. Read it again and retry."}})
				return 1
			}
			writeTo(stdout, stderr, *jsonOutput, result{Error: &cliError{"policy_update_failed", "Policy update failed."}})
			return 1
		}
		writeTo(stdout, stderr, *jsonOutput, result{Valid: true, Name: "access policy updated"})
		return 0
	}
	if len(args) == 2 && args[0] == "apps" && args[1] == "list" {
		if deps.store == nil {
			return 2
		}
		base := resolvedServer
		token, e := deps.store.Get(base)
		if e != nil {
			writeTo(stdout, stderr, *jsonOutput, result{Error: &cliError{"not_authenticated", "Login required."}})
			return 1
		}
		deployer := client.New(base, token)
		if deps.newClient != nil {
			deployer = deps.newClient(base, token)
		}
		var out any
		if e = deployer.Do(context.Background(), "GET", "/api/v1/apps", "", nil, &out); e != nil {
			writeTo(stdout, stderr, *jsonOutput, result{Error: &cliError{"apps_failed", "Apps request failed."}})
			return 1
		}
		b, _ := json.Marshal(out)
		fmt.Fprintln(stdout, string(b))
		return 0
	}
	if len(args) == 3 && args[0] == "apps" && args[1] == "create" {
		if deps.store == nil || !releases.ValidSlug(args[2]) {
			return 2
		}
		base := resolvedServer
		token, e := deps.store.Get(base)
		if e != nil {
			writeTo(stdout, stderr, *jsonOutput, result{Error: &cliError{"not_authenticated", "Login required."}})
			return 1
		}
		k, e := client.IdempotencyKey()
		if e != nil {
			return 1
		}
		deployer := client.New(base, token)
		if deps.newClient != nil {
			deployer = deps.newClient(base, token)
		}
		var out any
		if e = deployer.Do(context.Background(), "POST", "/api/v1/apps", k, map[string]string{"slug": args[2]}, &out); e != nil {
			writeTo(stdout, stderr, *jsonOutput, result{Error: &cliError{"app_create_failed", "App could not be created."}})
			return 1
		}
		b, _ := json.Marshal(out)
		fmt.Fprintln(stdout, string(b))
		return 0
	}
	if (len(args) == 1 || len(args) == 2) && args[0] == "deploy" {
		if deps.store == nil {
			writeTo(stdout, stderr, *jsonOutput, result{Error: &cliError{"usage", "deploy requires login"}})
			return 2
		}
		base := resolvedServer
		project := "."
		if len(args) == 2 {
			project = args[1]
		}
		manifest, created, e := ensureManifest(project, *jsonOutput, deps)
		if e != nil {
			code, message := manifestFailure(e)
			writeTo(stdout, stderr, *jsonOutput, result{Error: &cliError{code, message}})
			return 1
		}
		if created && !*jsonOutput {
			fmt.Fprintf(stdout, "Created %s.\n", filepath.Join(project, "tiny.yaml"))
		}
		m, e := releases.ParseManifest(manifest)
		if e != nil {
			writeTo(stdout, stderr, *jsonOutput, result{Error: &cliError{"invalid_manifest", "Manifest does not meet the V1 contract."}})
			return 1
		}
		token, e := ensureDeployCredential(context.Background(), base, *jsonOutput, deps)
		if e != nil {
			code, message := loginFailure(e)
			if errors.Is(e, client.ErrStore) {
				code, message = "credential_store", "Token could not be stored."
			}
			if errors.Is(e, client.ErrCredentialNotFound) {
				code, message = "not_authenticated", "Login required."
			}
			writeTo(stdout, stderr, *jsonOutput, result{Error: &cliError{code, message}})
			return 1
		}
		archive, cleanup, err := stagedArchive(project, manifest)
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
			var active *client.ActiveButUnverifiedError
			if errors.As(e, &active) {
				writeTo(stdout, stderr, *jsonOutput, result{
					Deployment: &deploymentReceipt{
						ID:           active.Deployment.DeploymentID,
						URL:          active.Deployment.URL,
						State:        "active",
						Verification: active.Reason,
					},
					Error: &cliError{
						"active_but_unverified",
						"Deployment is active, but public verification is incomplete.",
					},
				})
				return 1
			}
			var activation *client.ActivationFailedError
			if errors.As(e, &activation) {
				writeTo(stdout, stderr, *jsonOutput, result{
					Deployment: &deploymentReceipt{
						ID:        activation.DeploymentID,
						State:     "verified",
						Reason:    activation.Reason,
						RequestID: activation.RequestID,
					},
					Error: &cliError{
						"activation_failed",
						"Deployment activation failed.",
					},
				})
				return 1
			}
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

type accessPolicyView struct {
	Revision uint64 `json:"revision"`
	Allow    struct {
		Emails  []string `json:"emails"`
		Domains []string `json:"domains"`
	} `json:"allow"`
}

type accessPolicyFile struct {
	Mode              string  `json:"mode"`
	ExpectedRevision  *uint64 `json:"expected_revision,omitempty"`
	ConfirmBroadening bool    `json:"confirm_broadening"`
	Allow             struct {
		Emails  []string `json:"emails"`
		Domains []string `json:"domains"`
	} `json:"allow"`
}

func policyRevisionMatches(policy accessPolicyFile, current uint64) bool {
	return policy.ExpectedRevision != nil && *policy.ExpectedRevision == current
}

func policyBroadensAccess(policy accessPolicyFile, current accessPolicyView) bool {
	currentEmails, currentDomains := map[string]bool{}, map[string]bool{}
	for _, value := range current.Allow.Emails {
		currentEmails[value] = true
	}
	for _, value := range current.Allow.Domains {
		currentDomains[value] = true
	}
	for _, value := range policy.Allow.Emails {
		if !currentEmails[value] {
			return true
		}
	}
	for _, value := range policy.Allow.Domains {
		if !currentDomains[value] {
			return true
		}
	}
	return false
}

func affirmative(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "y", "yes":
		return true
	default:
		return false
	}
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

var errManifestSetup = errors.New("manifest setup failed")

func runInspectManifest(path string, jsonOutput bool, stdout, stderr io.Writer) int {
	b, err := os.ReadFile(path)
	if err != nil {
		writeTo(stdout, stderr, jsonOutput, result{Error: &cliError{"read_failed", "Could not read manifest."}})
		return 1
	}
	m, err := releases.ParseManifest(b)
	if err != nil {
		writeTo(stdout, stderr, jsonOutput, result{Error: &cliError{"invalid_manifest", "Manifest does not meet the V1 contract."}})
		return 1
	}
	writeTo(stdout, stderr, jsonOutput, result{Valid: true, Name: m.Name})
	return 0
}

func runInit(project string, jsonOutput bool, stdout, stderr io.Writer, deps runnerDeps) int {
	if jsonOutput {
		writeTo(stdout, stderr, true, result{Error: &cliError{"manifest_required", "Create tiny.yaml interactively without --json."}})
		return 1
	}
	if _, err := createManifest(project, deps.prompt); err != nil {
		writeTo(stdout, stderr, false, result{Error: &cliError{"manifest_setup_failed", "Manifest could not be created."}})
		return 1
	}
	fmt.Fprintf(stdout, "Created %s.\n", filepath.Join(project, "tiny.yaml"))
	return 0
}

func ensureManifest(project string, jsonOutput bool, deps runnerDeps) ([]byte, bool, error) {
	dir, err := safeProjectDir(project)
	if err != nil {
		return nil, false, errManifestSetup
	}
	path := filepath.Join(dir, "tiny.yaml")
	info, err := os.Lstat(path)
	if err == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return nil, false, errManifestSetup
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return nil, false, errManifestSetup
		}
		return b, false, nil
	}
	if !os.IsNotExist(err) || jsonOutput || deps.prompt == nil {
		return nil, false, errManifestSetup
	}
	b, err := createManifest(dir, deps.prompt)
	return b, err == nil, err
}

func createManifest(project string, prompt client.Prompt) ([]byte, error) {
	dir, err := safeProjectDir(project)
	if err != nil || prompt == nil {
		return nil, errManifestSetup
	}
	target := filepath.Join(dir, "tiny.yaml")
	if _, err = os.Lstat(target); err == nil || !os.IsNotExist(err) {
		return nil, errManifestSetup
	}
	suggested := strings.ToLower(filepath.Base(dir))
	if !releases.ValidSlug(suggested) {
		suggested = "my-app"
	}
	slug, err := askSetup(prompt, "App slug ("+suggested+", Enter to accept): ", true)
	if slug == "" {
		slug = suggested
	}
	if err != nil || !releases.ValidSlug(slug) {
		return nil, errManifestSetup
	}
	description, err := askSetup(prompt, "Description (optional): ", true)
	if err != nil {
		return nil, errManifestSetup
	}
	defaultOutput := "."
	if safeOutput(dir, "dist") {
		defaultOutput = "dist"
	}
	output, err := askSetup(prompt, "Build output ("+defaultOutput+", Enter to accept): ", true)
	if output == "" {
		output = defaultOutput
	}
	if err != nil || !safeOutput(dir, output) {
		return nil, errManifestSetup
	}
	allow, err := askSetup(prompt, "Allowed emails or domains, comma-separated (optional): ", true)
	if err != nil {
		return nil, errManifestSetup
	}
	features, err := askSetup(prompt, "Features (kv,blobs,realtime; optional): ", true)
	if err != nil {
		return nil, errManifestSetup
	}
	fallback, err := askSetup(prompt, "SPA fallback (optional): ", true)
	if err != nil || (fallback != "" && !safeFallback(dir, output, fallback)) {
		return nil, errManifestSetup
	}
	m := releases.Manifest{Version: 1, Name: slug, Description: description, BuildOutput: output, SPAFallback: fallback}
	for _, item := range splitSetupList(allow) {
		if strings.Contains(item, "@") {
			m.Emails = append(m.Emails, item)
		} else {
			m.Domains = append(m.Domains, item)
		}
	}
	for _, feature := range splitSetupList(features) {
		switch feature {
		case "kv":
			m.KV = true
		case "blobs":
			m.Blobs = true
		case "realtime":
			m.Realtime = true
		default:
			return nil, errManifestSetup
		}
	}
	b, err := releases.GenerateManifest(m)
	if err != nil {
		return nil, errManifestSetup
	}
	if writeNewManifest(dir, target, b) != nil {
		return nil, errManifestSetup
	}
	return b, nil
}

func askSetup(p client.Prompt, label string, optional bool) (string, error) {
	var v string
	var err error
	if bounded, ok := p.(interface {
		AskBoundedOptional(string, int) (string, error)
	}); ok {
		v, err = bounded.AskBoundedOptional(label, maxSetupServerInput)
	} else {
		v, err = p.Ask(label)
	}
	if err != nil || len(v) > maxSetupServerInput {
		return "", errManifestSetup
	}
	v = strings.TrimSpace(v)
	if !optional && v == "" {
		return "", errManifestSetup
	}
	return v, nil
}
func splitSetupList(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	return parts
}
func safeProjectDir(project string) (string, error) {
	p, err := filepath.Abs(project)
	if err != nil {
		return "", err
	}
	info, err := os.Lstat(p)
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return "", errManifestSetup
	}
	return p, nil
}
func safeOutput(project, output string) bool {
	if !safeRelative(output) {
		return false
	}
	info, err := lstatBeneath(project, output)
	return err == nil && info.Mode()&os.ModeSymlink == 0 && info.IsDir()
}
func safeFallback(project, output, fallback string) bool {
	if !safeOutput(project, output) || !safeRelative(fallback) {
		return false
	}
	info, err := lstatBeneath(project, filepath.ToSlash(output)+"/"+fallback)
	return err == nil && info.Mode()&os.ModeSymlink == 0 && info.Mode().IsRegular()
}
func safeRelative(value string) bool {
	return value != "" && !filepath.IsAbs(value) && filepath.Clean(value) == value && !strings.Contains(value, "\\") && !strings.HasPrefix(value, "../") && value != ".."
}
func lstatBeneath(root, rel string) (os.FileInfo, error) {
	current := root
	for _, part := range strings.Split(filepath.ToSlash(rel), "/") {
		current = filepath.Join(current, filepath.FromSlash(part))
		info, err := os.Lstat(current)
		if err != nil || info.Mode()&os.ModeSymlink != 0 {
			return nil, errManifestSetup
		}
	}
	return os.Lstat(current)
}
func writeNewManifest(dir, target string, b []byte) error {
	tmp, err := os.CreateTemp(dir, ".tiny-manifest-")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if err = tmp.Chmod(0600); err == nil {
		_, err = tmp.Write(b)
	}
	if err == nil {
		err = tmp.Sync()
	}
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if err = os.Link(name, target); err != nil {
		return err
	}
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	err = d.Sync()
	closeErr := d.Close()
	if err != nil {
		return err
	}
	return closeErr
}

func ensureDeployCredential(ctx context.Context, base string, jsonOutput bool, deps runnerDeps) (string, error) {
	if deps.store == nil {
		return "", client.ErrCredentialNotFound
	}
	if jsonOutput {
		return deps.store.Get(base)
	}
	out, fresh, err := login(ctx, base, false, deps)
	if err != nil {
		return "", err
	}
	if fresh && (out.Token == "" || deps.store.Put(base, out.Token) != nil) {
		return "", client.ErrStore
	}
	if out.Token == "" {
		return "", client.ErrCredentialNotFound
	}
	return out.Token, nil
}

func manifestFailure(error) (string, string) {
	return "manifest_required", "Create a valid tiny.yaml with tiny init."
}

func archiveSize(f *os.File) int64 {
	info, err := f.Stat()
	if err != nil {
		return -1
	}
	return info.Size()
}

const tokenUsage = "usage: tiny [--json] [--server URL] tokens <list APP|create APP --scope SCOPE [--scope SCOPE] --expires-in SECONDS|revoke APP TOKEN_ID>"

// runTokens keeps the token lifecycle intentionally separate from generic
// command output: a newly-created secret is written once, while list/revoke
// output can never contain a raw credential.
func requiresServer(args []string) bool {
	if len(args) == 1 {
		return args[0] == "login" || args[0] == "logout" || args[0] == "whoami"
	}
	if len(args) == 3 && args[0] == "access" && args[1] == "get" {
		return true
	}
	if len(args) == 5 && args[0] == "access" && args[1] == "set" && args[3] == "--file" {
		return true
	}
	if (len(args) == 2 || len(args) == 3) && args[0] == "releases" && releases.ValidSlug(args[len(args)-1]) && (len(args) == 2 || args[1] == "list") {
		return true
	}
	if (len(args) == 1 || len(args) == 2) && args[0] == "deploy" {
		return true
	}
	if len(args) == 2 && args[0] == "apps" && args[1] == "list" {
		return true
	}
	if len(args) == 3 && args[0] == "apps" && args[1] == "create" && releases.ValidSlug(args[2]) {
		return true
	}
	if len(args) == 5 && args[0] == "apps" && args[1] == "delete" && releases.ValidSlug(args[2]) && args[3] == "--confirm" && args[4] == "delete:"+args[2] {
		return true
	}
	if len(args) >= 2 && args[0] == "tokens" {
		return validTokenCommand(args[1:])
	}
	if len(args) >= 2 && args[0] == "data" {
		return validDataCommand(args[1:])
	}
	return false
}

func validTokenCommand(args []string) bool {
	if len(args) == 2 && args[0] == "list" {
		return releases.ValidSlug(args[1])
	}
	if len(args) >= 2 && args[0] == "create" && releases.ValidSlug(args[1]) {
		_, ok := parseTokenInput(args[2:])
		return ok
	}
	return len(args) == 3 && args[0] == "revoke" && releases.ValidSlug(args[1]) && validTokenID(args[2])
}

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
		writeTo(stdout, stderr, jsonOutput, result{Error: &cliError{"usage", "Command requires login."}})
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

// login first proves a saved credential is still valid. Only an explicit
// authorization denial falls back to OTP; transport, compatibility, and rate
// limit failures remain failures so a dependency outage cannot silently drive a
// deployer into a different authentication path. --force deliberately skips
// reuse to support switching accounts, while storage happens only after the
// new token and server-derived identity have both been verified.
func login(ctx context.Context, base string, force bool, deps runnerDeps) (client.LoginResult, bool, error) {
	newClient := deps.newClient
	if newClient == nil {
		newClient = client.New
	}
	if !force && deps.store != nil {
		if token, err := deps.store.Get(base); err == nil && token != "" {
			out, err := client.VerifyLoginSession(ctx, newClient(base, token))
			if err == nil {
				return out, false, nil
			}
			if !errors.Is(err, client.ErrUnauthorized) {
				return client.LoginResult{}, false, err
			}
		} else if err != nil && !errors.Is(err, client.ErrCredentialNotFound) {
			return client.LoginResult{}, false, err
		}
	}
	out, err := client.LoginWithClient(ctx, newClient(base, ""), deps.prompt)
	return out, true, err
}

func loginFailure(err error) (string, string) {
	if errors.Is(err, client.ErrRateLimited) {
		return "rate_limited", "Too many sign-in attempts. Wait and try again."
	}
	return "login_failed", "Login could not be completed."
}

func resolveServer(explicit string, deps runnerDeps) (string, error) {
	if explicit != "" {
		return client.NormalizeServer(explicit, false)
	}
	defaults, ok := deps.store.(client.DefaultServerStore)
	if !ok || defaults == nil {
		return "", client.ErrNoDefaultServer
	}
	server, err := defaults.DefaultServer()
	if err != nil {
		return "", err
	}
	return client.NormalizeServer(server, false)
}

// resolveServerForCommand adds a deliberately narrow human-only first-run
// setup path around ordinary resolution. Explicit flags, JSON automation, and
// anything other than the exact missing-default sentinel never enter it.
func resolveServerForCommand(explicit string, jsonOutput bool, deps runnerDeps) (string, error) {
	server, err := resolveServer(explicit, deps)
	if err == nil || explicit != "" || jsonOutput || err != client.ErrNoDefaultServer {
		return server, err
	}
	defaults, ok := deps.store.(client.DefaultServerStore)
	if !ok || defaults == nil || deps.prompt == nil {
		return "", errServerSetupInput
	}
	raw, err := askServer(deps.prompt)
	if err != nil {
		return "", errServerSetupInput
	}
	server, err = client.NormalizeServer(raw, false)
	if err != nil {
		return "", errServerSetupInput
	}
	newClient := deps.newClient
	if newClient == nil {
		newClient = client.New
	}
	if err = client.VerifyServerCompatibility(context.Background(), newClient(server, "")); err != nil {
		return "", errServerSetupFailed
	}
	if err = defaults.SetDefaultServer(server); err != nil {
		return "", errServerSetupFailed
	}
	return server, nil
}

func askServer(p client.Prompt) (string, error) {
	if bounded, ok := p.(boundedPrompt); ok {
		return bounded.AskBounded("Server (https://...): ", maxSetupServerInput)
	}
	value, err := p.Ask("Server (https://...): ")
	if err != nil || len(value) == 0 || len(value) > maxSetupServerInput {
		return "", errServerSetupInput
	}
	return value, nil
}

func serverFailure(err error) (string, string) {
	if errors.Is(err, client.ErrNoDefaultServer) {
		return "usage", "No saved server. Run tiny login --server https://your-tinyhost.example."
	}
	if errors.Is(err, errServerSetupInput) {
		return "server_setup_failed", "Server setup requires a valid HTTPS URL."
	}
	if errors.Is(err, errServerSetupFailed) {
		return "server_setup_failed", "Server could not be verified or saved."
	}
	if errors.Is(err, client.ErrStore) {
		return "configuration_store", "Saved server could not be read."
	}
	return "invalid_server", "Server must use HTTPS."
}

func writeLogoutSuccess(stdout, stderr io.Writer, jsonOutput bool, message string) {
	if jsonOutput {
		writeTo(stdout, stderr, true, result{Valid: true, Name: "logged out"})
		return
	}
	fmt.Fprintln(stdout, message)
}

func writeIdentitySuccess(stdout, stderr io.Writer, jsonOutput bool, email string) {
	if jsonOutput {
		writeTo(stdout, stderr, true, result{Valid: true, Name: email})
		return
	}
	fmt.Fprintf(stdout, "Logged in as %s.\n", email)
}

func (p stdinPrompt) Ask(label string) (string, error) {
	fmt.Fprint(p.w, label)
	s, e := p.r.ReadString('\n')
	return strings.TrimSpace(s), e
}
func (p stdinPrompt) AskBounded(label string, limit int) (string, error) {
	if limit <= 0 {
		return "", errServerSetupInput
	}
	fmt.Fprint(p.w, label)
	b, err := p.r.ReadSlice('\n')
	if err != nil && err != io.EOF {
		return "", err
	}
	if err == io.EOF {
		err = nil
	}
	value := strings.TrimSpace(string(b))
	if len(value) == 0 || len(value) > limit {
		return "", errServerSetupInput
	}
	return value, err
}

// AskBoundedOptional is deliberately separate from AskBounded: setup fields
// may accept Enter, but they must never retain an arbitrarily large line in
// memory while a human is preparing a local manifest.
func (p stdinPrompt) AskBoundedOptional(label string, limit int) (string, error) {
	if limit <= 0 {
		return "", errServerSetupInput
	}
	fmt.Fprint(p.w, label)
	b, err := p.r.ReadSlice('\n')
	if err != nil && err != io.EOF {
		return "", err
	}
	if err == io.EOF {
		err = nil
	}
	value := strings.TrimSpace(string(b))
	if len(value) > limit {
		return "", errServerSetupInput
	}
	return value, err
}
func writeTo(stdout, stderr io.Writer, j bool, r result) {
	if j {
		b, _ := json.Marshal(r)
		fmt.Fprintln(stdout, string(b))
		return
	}
	if r.Error != nil {
		fmt.Fprintln(stderr, r.Error.Message)
		if r.Deployment != nil {
			if r.Deployment.ID != "" {
				fmt.Fprintf(stderr, "Deployment: %s\n", r.Deployment.ID)
			}
			fmt.Fprintf(stderr, "State: %s\n", r.Deployment.State)
			if r.Deployment.URL != "" {
				fmt.Fprintf(stderr, "URL: %s\n", r.Deployment.URL)
			}
			if r.Deployment.Verification != "" {
				fmt.Fprintf(stderr, "Verification: %s\n", r.Deployment.Verification)
			}
			if r.Deployment.Reason != "" {
				fmt.Fprintf(stderr, "Reason: %s\n", r.Deployment.Reason)
			}
			if r.Deployment.RequestID != "" {
				fmt.Fprintf(stderr, "Request ID: %s\n", r.Deployment.RequestID)
			}
		}
		return
	}
	fmt.Fprintf(stdout, "Manifest valid: %s\n", r.Name)
}
