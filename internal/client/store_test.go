package client

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const testServer = "https://tinker.example.test"

func TestFileStoreRoundTripAndLastLoginWins(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "credentials")
	store := FileStore{Dir: dir}
	if err := store.Put(testServer+"/dashboard?tab=1", "first"); err != nil {
		t.Fatalf("Put() first: %v", err)
	}
	if err := store.Put(testServer, "second"); err != nil {
		t.Fatalf("Put() second: %v", err)
	}
	got, err := store.Get(testServer)
	if err != nil || got != "second" {
		t.Fatalf("Get() = %q, %v; want second, nil", got, err)
	}
	info, err := os.Stat(dir)
	if err != nil || info.Mode().Perm() != 0700 {
		t.Fatalf("credential dir mode = %v, %v; want 0700", info.Mode(), err)
	}
	info, err = os.Stat(filepath.Join(dir, credentialFilename(testServer)))
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("credential file mode = %v, %v; want 0600", info.Mode(), err)
	}
}

func TestFileStoreMissingDirectoryMeansNoCredential(t *testing.T) {
	store := FileStore{Dir: filepath.Join(t.TempDir(), "not-created")}
	if _, err := store.Get(testServer); err != ErrCredentialNotFound {
		t.Fatalf("Get missing directory = %v", err)
	}
	if err := store.Delete(testServer); err != nil {
		t.Fatalf("Delete missing directory = %v", err)
	}
}

func TestFileStoreRejectsUnsafeAndTamperedState(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(t *testing.T, dir, path string)
	}{
		{"permissive directory", func(t *testing.T, dir, _ string) {
			if err := os.Chmod(dir, 0755); err != nil {
				t.Fatal(err)
			}
		}},
		{"nonregular file", func(t *testing.T, _ string, path string) {
			if err := os.Remove(path); err != nil {
				t.Fatal(err)
			}
			if err := os.Mkdir(path, 0700); err != nil {
				t.Fatal(err)
			}
		}},
		{"permissive file", func(t *testing.T, _ string, path string) {
			if err := os.Chmod(path, 0644); err != nil {
				t.Fatal(err)
			}
		}},
		{"malformed JSON", func(t *testing.T, _ string, path string) {
			if err := os.WriteFile(path, []byte("{"), 0600); err != nil {
				t.Fatal(err)
			}
		}},
		{"unknown JSON field", func(t *testing.T, _ string, path string) {
			if err := os.WriteFile(path, []byte(`{"version":1,"server":"https://tinker.example.test","token":"token","extra":true}`), 0600); err != nil {
				t.Fatal(err)
			}
		}},
		{"duplicate JSON field", func(t *testing.T, _ string, path string) {
			if err := os.WriteFile(path, []byte(`{"version":1,"server":"https://tinker.example.test","token":"token","token":"replacement"}`), 0600); err != nil {
				t.Fatal(err)
			}
		}},
		{"wrong server", func(t *testing.T, _ string, path string) {
			writeRecord(t, path, credentialFile{Version: credentialVersion, Server: "https://other.example", Token: "token"})
		}},
		{"wrong version", func(t *testing.T, _ string, path string) {
			writeRecord(t, path, credentialFile{Version: credentialVersion + 1, Server: testServer, Token: "token"})
		}},
		{"oversized token", func(t *testing.T, _ string, path string) {
			writeRecord(t, path, credentialFile{Version: credentialVersion, Server: testServer, Token: strings.Repeat("x", maxCredentialToken+1)})
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "credentials")
			store := FileStore{Dir: dir}
			if err := store.Put(testServer, "token"); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(dir, credentialFilename(testServer))
			tt.mutate(t, dir, path)
			if _, err := store.Get(testServer); err != ErrStore {
				t.Fatalf("Get() error = %v, want ErrStore", err)
			}
		})
	}
}

func TestFileStoreRejectsSymlink(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "credentials")
	store := FileStore{Dir: dir}
	if err := store.Put(testServer, "token"); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, credentialFilename(testServer))
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("/dev/null", path); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Get(testServer); err != ErrStore {
		t.Fatalf("Get() error = %v, want ErrStore", err)
	}
}

func TestFileStorePutRejectsUnsafeExistingTarget(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(t *testing.T, path string)
	}{
		{"permissive", func(t *testing.T, path string) {
			if err := os.Chmod(path, 0644); err != nil {
				t.Fatal(err)
			}
		}},
		{"nonregular", func(t *testing.T, path string) {
			if err := os.Remove(path); err != nil {
				t.Fatal(err)
			}
			if err := os.Mkdir(path, 0700); err != nil {
				t.Fatal(err)
			}
		}},
		{"symlink", func(t *testing.T, path string) {
			if err := os.Remove(path); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink("/dev/null", path); err != nil {
				t.Fatal(err)
			}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "credentials")
			store := FileStore{Dir: dir}
			if err := store.Put(testServer, "old"); err != nil {
				t.Fatal(err)
			}
			tt.mutate(t, filepath.Join(dir, credentialFilename(testServer)))
			if err := store.Put(testServer, "new"); err != ErrStore {
				t.Fatalf("Put() error = %v, want ErrStore", err)
			}
		})
	}
}

func TestFileStoreDefaultServerRoundTripAndStrictDecoding(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "credentials")
	store := FileStore{Dir: dir}
	if _, err := store.DefaultServer(); err != ErrNoDefaultServer {
		t.Fatalf("missing default = %v, want ErrNoDefaultServer", err)
	}
	if err := store.SetDefaultServer(testServer + "/dashboard?ignored=1"); err != nil {
		t.Fatal(err)
	}
	if got, err := store.DefaultServer(); err != nil || got != testServer {
		t.Fatalf("DefaultServer = %q, %v", got, err)
	}
	path := filepath.Join(dir, defaultServerFile)
	for _, body := range []string{
		`{"version":1,"server":"https://tinker.example.test","extra":true}`,
		`{"version":1,"server":"https://tinker.example.test","server":"https://other.example"}`,
		`{"version":1,"server":"http://tinker.example.test"}`,
		`{"version":2,"server":"https://tinker.example.test"}`,
		`{`,
	} {
		if err := os.WriteFile(path, []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := store.DefaultServer(); err != ErrStore {
			t.Fatalf("DefaultServer(%q) = %v, want ErrStore", body, err)
		}
	}
}

func TestFileStoreDefaultServerRejectsUnsafeFileAndDeleteIsScoped(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "credentials")
	store := FileStore{Dir: dir}
	other := "https://other.example"
	if err := store.Put(testServer, "primary"); err != nil {
		t.Fatal(err)
	}
	if err := store.Put(other, "other"); err != nil {
		t.Fatal(err)
	}
	if err := store.SetDefaultServer(testServer); err != nil {
		t.Fatal(err)
	}
	defaultPath := filepath.Join(dir, defaultServerFile)
	if err := os.Remove(defaultPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("/dev/null", defaultPath); err != nil {
		t.Fatal(err)
	}
	if _, err := store.DefaultServer(); err != ErrStore {
		t.Fatalf("symlink default = %v", err)
	}
	if err := os.Remove(defaultPath); err != nil {
		t.Fatal(err)
	}
	if err := store.SetDefaultServer(testServer); err != nil {
		t.Fatal(err)
	}
	if err := store.Delete(testServer); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Get(testServer); err != ErrCredentialNotFound {
		t.Fatalf("deleted credential = %v", err)
	}
	if got, err := store.Get(other); err != nil || got != "other" {
		t.Fatalf("other credential = %q, %v", got, err)
	}
	if got, err := store.DefaultServer(); err != nil || got != testServer {
		t.Fatalf("default changed by delete = %q, %v", got, err)
	}
	if err := store.Delete(testServer); err != nil {
		t.Fatalf("missing delete = %v", err)
	}
}

func TestFileStoreDeleteRejectsUnsafeTarget(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "credentials")
	store := FileStore{Dir: dir}
	if err := store.Put(testServer, "token"); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, credentialFilename(testServer))
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("/dev/null", path); err != nil {
		t.Fatal(err)
	}
	if err := store.Delete(testServer); err != ErrStore {
		t.Fatalf("Delete symlink = %v, want ErrStore", err)
	}
}

func writeRecord(t *testing.T, path string, record credentialFile) {
	t.Helper()
	b, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, b, 0600); err != nil {
		t.Fatal(err)
	}
	if err = os.Chmod(path, 0600); err != nil {
		t.Fatal(err)
	}
}
