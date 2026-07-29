package main

import (
	"context"
	"io"

	"github.com/tinyhost/tiny/internal/hostops"
)

var hostRunner hostops.Runner = hostops.ExecRunner{}

func runHost(args []string, jsonOutput bool, stdout, stderr io.Writer) int {
	if jsonOutput || len(args) < 2 {
		writeTo(stdout, stderr, jsonOutput, result{Error: &cliError{"usage", "usage: tiny host install|status|doctor|update root@HOST [--release-base URL]"}})
		return 2
	}
	operation, target := args[0], args[1]
	releaseBase := ""
	if operation == "install" || operation == "update" {
		if len(args) != 4 || args[2] != "--release-base" {
			writeTo(stdout, stderr, false, result{Error: &cliError{"usage", "host install/update requires --release-base HTTPS_URL."}})
			return 2
		}
		releaseBase = args[3]
	} else if len(args) != 2 {
		writeTo(stdout, stderr, false, result{Error: &cliError{"usage", "host status/doctor requires exactly root@HOST."}})
		return 2
	}
	plan, err := hostops.Build(operation, target, releaseBase)
	if err != nil {
		writeTo(stdout, stderr, false, result{Error: &cliError{"invalid_host_operation", "Host target or release URL is invalid."}})
		return 2
	}
	if err = hostops.Execute(context.Background(), hostRunner, plan, stdout, stderr); err != nil {
		writeTo(stdout, stderr, false, result{Error: &cliError{"host_operation_failed", "Host operation failed."}})
		return 1
	}
	return 0
}
