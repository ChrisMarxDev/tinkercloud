package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/ChrisMarxDev/tinkercloud/internal/client"
	"github.com/ChrisMarxDev/tinkercloud/internal/compatibility"
	"github.com/ChrisMarxDev/tinkercloud/internal/hostops"
)

var hostRunner hostops.Runner = hostops.ExecRunner{}

func runHost(args []string, jsonOutput bool, stdout, stderr io.Writer, input io.Reader) int {
	if jsonOutput || len(args) < 2 {
		writeTo(stdout, stderr, jsonOutput, result{Error: &cliError{"usage", "usage: tinker host install|status|doctor|update|uninstall root@HOST [--release-base URL]"}})
		return 2
	}
	operation, target := args[0], args[1]
	releaseBase := ""
	confirmUninstall := false
	if operation == "install" {
		switch {
		case len(args) == 2:
			var ok bool
			releaseBase, ok = releaseBaseForBuild(client.BuildVersion)
			if !ok {
				writeTo(stdout, stderr, false, result{Error: &cliError{"release_required", "Development builds need --release-base HTTPS_URL."}})
				return 2
			}
		case len(args) == 4 && args[2] == "--release-base":
			releaseBase = args[3]
		default:
			writeTo(stdout, stderr, false, result{Error: &cliError{"usage", "host install accepts optional --release-base HTTPS_URL."}})
			return 2
		}
	} else if operation == "update" {
		if len(args) != 4 || args[2] != "--release-base" {
			writeTo(stdout, stderr, false, result{Error: &cliError{"usage", "host update requires --release-base HTTPS_URL."}})
			return 2
		}
		releaseBase = args[3]
	} else if operation == "uninstall" {
		yes := len(args) == 3 && args[2] == "--yes"
		if len(args) != 2 && !yes {
			writeTo(stdout, stderr, false, result{Error: &cliError{"usage", "host uninstall requires root@HOST and optional --yes."}})
			return 2
		}
		confirmUninstall = !yes
	} else if len(args) != 2 {
		writeTo(stdout, stderr, false, result{Error: &cliError{"usage", "host status/doctor requires exactly root@HOST."}})
		return 2
	}
	plan, err := hostops.Build(operation, target, releaseBase)
	if err != nil {
		writeTo(stdout, stderr, false, result{Error: &cliError{"invalid_host_operation", "Host target or release URL is invalid."}})
		return 2
	}
	if confirmUninstall && !confirmHostUninstall(input, target, stderr) {
		writeTo(stdout, stderr, false, result{Error: &cliError{"uninstall_cancelled", "Host uninstall cancelled."}})
		return 1
	}
	if err = hostops.Execute(context.Background(), hostRunner, plan, stdout, stderr); err != nil {
		writeTo(stdout, stderr, false, result{Error: &cliError{"host_operation_failed", "Host operation failed."}})
		return 1
	}
	return 0
}

func releaseBaseForBuild(version string) (string, bool) {
	if _, err := compatibility.Parse(version); err != nil {
		return "", false
	}
	return "https://github.com/ChrisMarxDev/tinkercloud/releases/download/v" + version + "/", true
}

const maxHostConfirmationBytes = 1024

func confirmHostUninstall(input io.Reader, target string, stderr io.Writer) bool {
	if input == nil {
		return false
	}
	want := "uninstall " + target
	fmt.Fprintf(stderr, "This permanently removes Tinkercloud application state from %s. ACME certificates are preserved. Type %q to continue: ", target, want)
	line, err := bufio.NewReader(io.LimitReader(input, maxHostConfirmationBytes)).ReadString('\n')
	return err == nil && strings.TrimSpace(line) == want
}
