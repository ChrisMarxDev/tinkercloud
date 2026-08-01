package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"unicode/utf8"

	"github.com/ChrisMarxDev/tinkercloud/internal/client"
	"github.com/ChrisMarxDev/tinkercloud/internal/collections"
	"github.com/ChrisMarxDev/tinkercloud/internal/kv"
	"github.com/ChrisMarxDev/tinkercloud/internal/releases"
)

const dataUsage = "usage: tinker data <kv|collections|documents> ..."
const maxDataInputBytes = 64 << 10

func validDataCommand(args []string) bool {
	// Exact parsing and all semantic validation happen in runData so malformed
	// commands still receive the normal saved-server/login path and a stable
	// usage response, never an accidental network request.
	return len(args) >= 1 && (args[0] == "kv" || args[0] == "collections" || args[0] == "documents")
}

func runData(args []string, server string, jsonOutput bool, stdout, stderr io.Writer, deps runnerDeps) int {
	c, code := authenticatedClient(server, jsonOutput, stdout, stderr, deps)
	if code != 0 {
		return code
	}
	if len(args) == 0 {
		return dataUsageError(jsonOutput, stdout, stderr)
	}
	switch args[0] {
	case "kv":
		return runDataKV(c, args[1:], jsonOutput, stdout, stderr, deps)
	case "collections":
		return runDataCollections(c, args[1:], jsonOutput, stdout, stderr)
	case "documents":
		return runDataDocuments(c, args[1:], jsonOutput, stdout, stderr, deps)
	default:
		return dataUsageError(jsonOutput, stdout, stderr)
	}
}

func runDataKV(c client.Client, args []string, jsonOutput bool, stdout, stderr io.Writer, deps runnerDeps) int {
	if len(args) < 2 || !releases.ValidSlug(args[1]) {
		return dataUsageError(jsonOutput, stdout, stderr)
	}
	verb, slug := args[0], args[1]
	switch verb {
	case "list":
		prefix, cursor, limit, ok := parseListFlags(args[2:])
		if !ok {
			return dataUsageError(jsonOutput, stdout, stderr)
		}
		out, err := c.ListDataKV(context.Background(), slug, prefix, cursor, limit)
		if err != nil {
			return dataRequestErrorFor(err, jsonOutput, stdout, stderr)
		}
		if out.Entries == nil {
			out.Entries = []client.DataEntry{}
		}
		if jsonOutput {
			writeJSON(stdout, out)
			return 0
		}
		for _, entry := range out.Entries {
			fmt.Fprintf(stdout, "%s\t%d\t%s\n", entry.Key, entry.Version, compactJSON(entry.Value))
		}
		if out.NextCursor != "" {
			fmt.Fprintf(stdout, "Next cursor: %s\n", out.NextCursor)
		}
		return 0
	case "get":
		if len(args) != 3 || !validDataKey(args[2]) {
			return dataUsageError(jsonOutput, stdout, stderr)
		}
		out, err := c.GetDataKV(context.Background(), slug, args[2])
		if err != nil {
			return dataRequestErrorFor(err, jsonOutput, stdout, stderr)
		}
		if jsonOutput {
			writeJSON(stdout, out)
		} else {
			fmt.Fprintln(stdout, compactJSON(out.Value))
		}
		return 0
	case "set":
		if len(args) < 4 || !validDataKey(args[2]) {
			return dataUsageError(jsonOutput, stdout, stderr)
		}
		value, expected, ok := readDataInput(args[3:], deps)
		if !ok {
			return dataInputError(jsonOutput, stdout, stderr)
		}
		key, err := client.IdempotencyKey()
		if err != nil {
			return dataRequestErrorFor(err, jsonOutput, stdout, stderr)
		}
		out, err := c.SetDataKV(context.Background(), slug, args[2], value, expected, key)
		if err != nil {
			return dataRequestErrorFor(err, jsonOutput, stdout, stderr)
		}
		return writeDataSuccess(out, jsonOutput, stdout)
	case "delete":
		if len(args) < 5 || !validDataKey(args[2]) || args[3] != "--expected-version" {
			return dataUsageError(jsonOutput, stdout, stderr)
		}
		expected, ok := positiveVersion(args[4])
		if !ok {
			return dataUsageError(jsonOutput, stdout, stderr)
		}
		if code := confirmDataDelete("delete:"+slug+":"+args[2], args[5:], jsonOutput, stdout, stderr, deps); code != 0 {
			return code
		}
		key, err := client.IdempotencyKey()
		if err != nil {
			return dataRequestError(jsonOutput, stdout, stderr)
		}
		if err = c.DeleteDataKV(context.Background(), slug, args[2], expected, key); err != nil {
			return dataRequestErrorFor(err, jsonOutput, stdout, stderr)
		}
		return writeDataSuccess(struct {
			Key     string `json:"key"`
			Deleted bool   `json:"deleted"`
		}{args[2], true}, jsonOutput, stdout)
	}
	return dataUsageError(jsonOutput, stdout, stderr)
}

func runDataCollections(c client.Client, args []string, jsonOutput bool, stdout, stderr io.Writer) int {
	if len(args) < 2 || args[0] != "list" || !releases.ValidSlug(args[1]) {
		return dataUsageError(jsonOutput, stdout, stderr)
	}
	prefix, cursor, limit, ok := parseListFlags(args[2:])
	if !ok || prefix != "" || (cursor != "" && !collections.ValidCollection(cursor)) {
		return dataUsageError(jsonOutput, stdout, stderr)
	}
	out, err := c.ListDataCollections(context.Background(), args[1], cursor, limit)
	if err != nil {
		return dataRequestErrorFor(err, jsonOutput, stdout, stderr)
	}
	if out.Collections == nil {
		out.Collections = []string{}
	}
	if jsonOutput {
		writeJSON(stdout, out)
		return 0
	}
	for _, name := range out.Collections {
		fmt.Fprintln(stdout, name)
	}
	if out.NextCursor != "" {
		fmt.Fprintf(stdout, "Next cursor: %s\n", out.NextCursor)
	}
	return 0
}

func runDataDocuments(c client.Client, args []string, jsonOutput bool, stdout, stderr io.Writer, deps runnerDeps) int {
	if len(args) < 3 || !releases.ValidSlug(args[1]) || !validDataCollection(args[2]) {
		return dataUsageError(jsonOutput, stdout, stderr)
	}
	verb, slug, collection := args[0], args[1], args[2]
	switch verb {
	case "list":
		if len(args) < 3 {
			return dataUsageError(jsonOutput, stdout, stderr)
		}
		prefix, cursor, limit, ok := parseListFlags(args[3:])
		if !ok || prefix != "" || (cursor != "" && !collections.ValidDocumentID(cursor)) {
			return dataUsageError(jsonOutput, stdout, stderr)
		}
		out, err := c.ListDataDocuments(context.Background(), slug, collection, cursor, limit)
		if err != nil {
			return dataRequestErrorFor(err, jsonOutput, stdout, stderr)
		}
		if out.Documents == nil {
			out.Documents = []client.DataDocument{}
		}
		if jsonOutput {
			writeJSON(stdout, out)
			return 0
		}
		for _, doc := range out.Documents {
			fmt.Fprintf(stdout, "%s\t%d\t%s\n", doc.ID, doc.Version, compactJSON(doc.Data))
		}
		if out.NextCursor != "" {
			fmt.Fprintf(stdout, "Next cursor: %s\n", out.NextCursor)
		}
		return 0
	case "get":
		if len(args) != 4 || !validDataDocumentID(args[3]) {
			return dataUsageError(jsonOutput, stdout, stderr)
		}
		out, err := c.GetDataDocument(context.Background(), slug, collection, args[3])
		if err != nil {
			return dataRequestErrorFor(err, jsonOutput, stdout, stderr)
		}
		if jsonOutput {
			writeJSON(stdout, out)
		} else {
			fmt.Fprintln(stdout, compactJSON(out.Data))
		}
		return 0
	case "create":
		value, expected, ok := readDataInput(args[3:], deps)
		if !ok || expected != nil {
			return dataInputError(jsonOutput, stdout, stderr)
		}
		key, err := client.IdempotencyKey()
		if err != nil {
			return dataRequestErrorFor(err, jsonOutput, stdout, stderr)
		}
		out, err := c.CreateDataDocument(context.Background(), slug, collection, value, key)
		if err != nil {
			return dataRequestErrorFor(err, jsonOutput, stdout, stderr)
		}
		return writeDataSuccess(out, jsonOutput, stdout)
	case "update":
		if len(args) < 6 || !validDataDocumentID(args[3]) {
			return dataUsageError(jsonOutput, stdout, stderr)
		}
		value, expected, ok := readDataInput(args[4:], deps)
		if !ok || expected == nil {
			return dataInputError(jsonOutput, stdout, stderr)
		}
		key, err := client.IdempotencyKey()
		if err != nil {
			return dataRequestErrorFor(err, jsonOutput, stdout, stderr)
		}
		out, err := c.UpdateDataDocument(context.Background(), slug, collection, args[3], value, *expected, key)
		if err != nil {
			return dataRequestErrorFor(err, jsonOutput, stdout, stderr)
		}
		return writeDataSuccess(out, jsonOutput, stdout)
	case "delete":
		if len(args) < 6 || !validDataDocumentID(args[3]) || args[4] != "--expected-version" {
			return dataUsageError(jsonOutput, stdout, stderr)
		}
		expected, ok := positiveVersion(args[5])
		if !ok {
			return dataUsageError(jsonOutput, stdout, stderr)
		}
		if code := confirmDataDelete("delete:"+slug+":"+collection+":"+args[3], args[6:], jsonOutput, stdout, stderr, deps); code != 0 {
			return code
		}
		key, err := client.IdempotencyKey()
		if err != nil {
			return dataRequestError(jsonOutput, stdout, stderr)
		}
		if err = c.DeleteDataDocument(context.Background(), slug, collection, args[3], expected, key); err != nil {
			return dataRequestErrorFor(err, jsonOutput, stdout, stderr)
		}
		return writeDataSuccess(struct {
			ID      string `json:"id"`
			Deleted bool   `json:"deleted"`
		}{args[3], true}, jsonOutput, stdout)
	}
	return dataUsageError(jsonOutput, stdout, stderr)
}

func parseListFlags(args []string) (prefix, cursor string, limit int, ok bool) {
	ok = true
	for len(args) > 0 {
		if len(args) < 2 {
			return "", "", 0, false
		}
		switch args[0] {
		case "--prefix":
			if prefix != "" {
				return "", "", 0, false
			}
			prefix = args[1]
		case "--cursor":
			if cursor != "" {
				return "", "", 0, false
			}
			cursor = args[1]
		case "--limit":
			if limit != 0 {
				return "", "", 0, false
			}
			var e error
			limit, e = strconv.Atoi(args[1])
			if e != nil || limit < 1 || limit > 100 {
				return "", "", 0, false
			}
		default:
			return "", "", 0, false
		}
		args = args[2:]
	}
	return
}

// readDataInput permits exactly one source and one JSON document. --file - is
// intentionally not magic: --stdin makes a potentially blocking action plain.
func readDataInput(args []string, deps runnerDeps) (json.RawMessage, *uint64, bool) {
	var raw []byte
	var expected *uint64
	source := false
	for len(args) > 0 {
		switch args[0] {
		case "--file":
			if source || len(args) < 2 {
				return nil, nil, false
			}
			f, err := os.Open(args[1])
			if err != nil {
				return nil, nil, false
			}
			raw, err = io.ReadAll(io.LimitReader(f, maxDataInputBytes+1))
			_ = f.Close()
			if err != nil {
				return nil, nil, false
			}
			source = true
			args = args[2:]
		case "--stdin":
			if source {
				return nil, nil, false
			}
			in := deps.input
			if in == nil {
				in = os.Stdin
			}
			var err error
			raw, err = io.ReadAll(io.LimitReader(in, maxDataInputBytes+1))
			if err != nil {
				return nil, nil, false
			}
			source = true
			args = args[1:]
		case "--expected-version":
			if expected != nil || len(args) < 2 {
				return nil, nil, false
			}
			v, ok := positiveVersion(args[1])
			if !ok {
				return nil, nil, false
			}
			expected = &v
			args = args[2:]
		default:
			return nil, nil, false
		}
	}
	if !source || len(raw) == 0 || len(raw) > maxDataInputBytes || !utf8.Valid(raw) {
		return nil, nil, false
	}
	var value json.RawMessage
	d := json.NewDecoder(bytes.NewReader(raw))
	if d.Decode(&value) != nil || d.Decode(&struct{}{}) != io.EOF || !json.Valid(value) {
		return nil, nil, false
	}
	return value, expected, true
}

func positiveVersion(raw string) (uint64, bool) {
	v, e := strconv.ParseUint(raw, 10, 64)
	return v, e == nil && v > 0
}
func validDataKey(raw string) bool        { return kv.ValidKey(raw) }
func validDataCollection(raw string) bool { return collections.ValidCollection(raw) }
func validDataDocumentID(raw string) bool { return collections.ValidDocumentID(raw) }

func confirmDataDelete(phrase string, args []string, jsonOutput bool, stdout, stderr io.Writer, deps runnerDeps) int {
	if len(args) == 2 && args[0] == "--confirm" {
		if args[1] == phrase {
			return 0
		}
		return dataUsageError(jsonOutput, stdout, stderr)
	}
	if len(args) != 0 {
		return dataUsageError(jsonOutput, stdout, stderr)
	}
	if jsonOutput {
		writeTo(stdout, stderr, true, result{Error: &cliError{"confirmation_required", "Destructive data deletion requires the exact target-bound --confirm value in JSON mode."}})
		return 2
	}
	if deps.prompt == nil {
		return dataConfirmationError(jsonOutput, stdout, stderr)
	}
	answer, err := deps.prompt.Ask("Type " + phrase + " to delete: ")
	if err != nil || answer != phrase {
		return dataConfirmationError(jsonOutput, stdout, stderr)
	}
	return 0
}
func compactJSON(raw json.RawMessage) string {
	var out bytes.Buffer
	if json.Compact(&out, raw) == nil {
		return out.String()
	}
	return ""
}
func dataUsageError(j bool, out, err io.Writer) int {
	writeTo(out, err, j, result{Error: &cliError{"usage", dataUsage}})
	return 2
}
func dataInputError(j bool, out, err io.Writer) int {
	writeTo(out, err, j, result{Error: &cliError{"invalid_json", "JSON input must be one valid value within the size limit."}})
	return 1
}
func dataRequestError(j bool, out, err io.Writer) int {
	writeTo(out, err, j, result{Error: &cliError{"data_request_failed", "Data request could not be completed."}})
	return 1
}
func dataRequestErrorFor(requestErr error, j bool, out, err io.Writer) int {
	code, message := "data_request_failed", "Data request could not be completed."
	switch {
	case errors.Is(requestErr, client.ErrUnauthorized):
		code, message = "not_authorized", "Data request is not authorized. Check app ownership; if this login predates data access, run tinker login --force."
	case errors.Is(requestErr, client.ErrRateLimited):
		code, message = "rate_limited", "Too many data requests. Wait and try again."
	case errors.Is(requestErr, client.ErrQuotaExceeded):
		code, message = "quota_exceeded", "The app data limit was reached."
	case errors.Is(requestErr, client.ErrConflict):
		code, message = "version_conflict", "Data changed before this request could be applied."
	case errors.Is(requestErr, client.ErrNotFound):
		code, message = "not_found", "The requested data was not found."
	case errors.Is(requestErr, client.ErrValidation):
		code, message = "validation_failed", "The data request is invalid."
	}
	writeTo(out, err, j, result{Error: &cliError{code, message}})
	return 1
}
func dataConfirmationError(j bool, out, err io.Writer) int {
	writeTo(out, err, j, result{Error: &cliError{"confirmation_required", "Deletion was not confirmed."}})
	return 1
}
func writeDataSuccess(value any, j bool, out io.Writer) int {
	if j {
		writeJSON(out, value)
	} else {
		b, _ := json.Marshal(value)
		fmt.Fprintln(out, string(b))
	}
	return 0
}
