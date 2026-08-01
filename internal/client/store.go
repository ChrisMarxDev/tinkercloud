package client

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
)

var ErrStore = errors.New("credential store unavailable")

// ErrCredentialNotFound is the only local state that is safe to treat as a
// logged-out CLI. Corrupt, unsafe, or unavailable storage remains ErrStore.
var ErrCredentialNotFound = errors.New("credential not found")

// ErrNoDefaultServer means this OS user has not selected a Tinkercloud platform
// yet. It is deliberately distinct from ErrStore so the CLI can offer an
// actionable first-run instruction without treating malformed local state as
// absent configuration.
var ErrNoDefaultServer = errors.New("default server not configured")

const (
	credentialVersion  = 1
	credentialDirName  = "tinker"
	maxCredentialFile  = 32 << 10
	maxCredentialToken = 16 << 10
	defaultServerFile  = "default-server.json"
	defaultServerLimit = 4 << 10
)

type MemoryStore map[string]string

const memoryDefaultServerKey = "\x00tinker-default-server"

func (m MemoryStore) Get(k string) (string, error) {
	v, ok := m[k]
	if !ok {
		return "", ErrCredentialNotFound
	}
	return v, nil
}
func (m MemoryStore) Put(k, v string) error { m[k] = v; return nil }
func (m MemoryStore) Delete(k string) error {
	delete(m, k)
	return nil
}
func (m MemoryStore) DefaultServer() (string, error) {
	v, ok := m[memoryDefaultServerKey]
	if !ok {
		return "", ErrNoDefaultServer
	}
	return v, nil
}
func (m MemoryStore) SetDefaultServer(server string) error {
	normalized, err := NormalizeServer(server, false)
	if err != nil {
		return ErrStore
	}
	m[memoryDefaultServerKey] = normalized
	return nil
}

// FileStore stores one deployer token per normalized Tinkercloud server.  Dir is
// provided for deterministic tests; when empty, the platform config directory
// is used with Tinker's private subdirectory.
type FileStore struct {
	Dir string
}

type credentialFile struct {
	Version int    `json:"version"`
	Server  string `json:"server"`
	Token   string `json:"token"`
}

type defaultServerFileRecord struct {
	Version int    `json:"version"`
	Server  string `json:"server"`
}

func (s FileStore) directory() (string, error) {
	if s.Dir != "" {
		return s.Dir, nil
	}
	dir, err := os.UserConfigDir()
	if err != nil || dir == "" {
		return "", ErrStore
	}
	return filepath.Join(dir, credentialDirName), nil
}

func normalizedCredentialService(service string) (string, error) {
	normalized, err := NormalizeServer(service, false)
	if err != nil {
		return "", ErrStore
	}
	return normalized, nil
}

func credentialFilename(server string) string {
	sum := sha256.Sum256([]byte(server))
	return hex.EncodeToString(sum[:]) + ".json"
}

func secureCredentialDir(dir string, create bool) error {
	info, err := os.Lstat(dir)
	if os.IsNotExist(err) && create {
		if err = os.MkdirAll(dir, 0700); err != nil {
			return ErrStore
		}
		// MkdirAll is subject to umask. The credential directory must have this
		// exact mode, rather than merely no group/world bits.
		if err = os.Chmod(dir, 0700); err != nil {
			return ErrStore
		}
		info, err = os.Lstat(dir)
	}
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.IsDir() || info.Mode()&(os.ModePerm|os.ModeSetuid|os.ModeSetgid|os.ModeSticky) != 0700 {
		return ErrStore
	}
	return nil
}

func secureCredentialFile(path string) (os.FileInfo, error) {
	info, err := os.Lstat(path)
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || info.Mode()&(os.ModePerm|os.ModeSetuid|os.ModeSetgid|os.ModeSticky) != 0600 || info.Size() < 1 || info.Size() > maxCredentialFile {
		return nil, ErrStore
	}
	return info, nil
}

func decodeCredential(r io.Reader) (credentialFile, error) {
	decoder := json.NewDecoder(r)
	first, err := decoder.Token()
	if err != nil {
		return credentialFile{}, ErrStore
	}
	if delim, ok := first.(json.Delim); !ok || delim != '{' {
		return credentialFile{}, ErrStore
	}
	var record credentialFile
	seen := map[string]bool{}
	for decoder.More() {
		key, err := decoder.Token()
		if err != nil {
			return credentialFile{}, ErrStore
		}
		name, ok := key.(string)
		if !ok || seen[name] {
			return credentialFile{}, ErrStore
		}
		seen[name] = true
		var value json.RawMessage
		if err = decoder.Decode(&value); err != nil {
			return credentialFile{}, ErrStore
		}
		switch name {
		case "version":
			err = json.Unmarshal(value, &record.Version)
		case "server":
			err = json.Unmarshal(value, &record.Server)
		case "token":
			err = json.Unmarshal(value, &record.Token)
		default:
			return credentialFile{}, ErrStore
		}
		if err != nil {
			return credentialFile{}, ErrStore
		}
	}
	last, err := decoder.Token()
	if err != nil {
		return credentialFile{}, ErrStore
	}
	if delim, ok := last.(json.Delim); !ok || delim != '}' {
		return credentialFile{}, ErrStore
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		return credentialFile{}, ErrStore
	}
	return record, nil
}

func (s FileStore) Get(service string) (string, error) {
	server, err := normalizedCredentialService(service)
	if err != nil {
		return "", ErrStore
	}
	dir, err := s.directory()
	if err != nil {
		return "", ErrStore
	}
	if _, err = os.Lstat(dir); os.IsNotExist(err) {
		return "", ErrCredentialNotFound
	} else if err != nil || secureCredentialDir(dir, false) != nil {
		return "", ErrStore
	}
	path := filepath.Join(dir, credentialFilename(server))
	if _, err = os.Lstat(path); os.IsNotExist(err) {
		return "", ErrCredentialNotFound
	} else if err != nil {
		return "", ErrStore
	}
	if _, err = secureCredentialFile(path); err != nil {
		return "", ErrStore
	}
	f, err := os.Open(path)
	if err != nil {
		return "", ErrStore
	}
	defer f.Close()
	// Check the opened object as well as its path, so a replacement between
	// Lstat and Open cannot turn a denied file into an accepted credential.
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Mode()&(os.ModePerm|os.ModeSetuid|os.ModeSetgid|os.ModeSticky) != 0600 || info.Size() < 1 || info.Size() > maxCredentialFile {
		return "", ErrStore
	}
	record, err := decodeCredential(io.LimitReader(f, maxCredentialFile+1))
	if err != nil || record.Version != credentialVersion || record.Server != server || record.Token == "" || len(record.Token) > maxCredentialToken {
		return "", ErrStore
	}
	return record.Token, nil
}

func (s FileStore) Put(service, token string) error {
	server, err := normalizedCredentialService(service)
	if err != nil || token == "" || len(token) > maxCredentialToken {
		return ErrStore
	}
	dir, err := s.directory()
	if err != nil || secureCredentialDir(dir, true) != nil {
		return ErrStore
	}
	b, err := json.Marshal(credentialFile{Version: credentialVersion, Server: server, Token: token})
	if err != nil || len(b) > maxCredentialFile {
		return ErrStore
	}
	target := filepath.Join(dir, credentialFilename(server))
	if _, err = os.Lstat(target); err == nil {
		if _, err = secureCredentialFile(target); err != nil {
			return ErrStore
		}
	} else if !os.IsNotExist(err) {
		return ErrStore
	}
	tmp, err := os.CreateTemp(dir, ".credential-")
	if err != nil {
		return ErrStore
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
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
		return ErrStore
	}
	if err = os.Rename(tmpName, target); err != nil {
		return ErrStore
	}
	// Persist the rename itself. This makes a successful login survive a power
	// loss as far as the host filesystem's fsync guarantees allow.
	d, err := os.Open(dir)
	if err != nil {
		return ErrStore
	}
	err = d.Sync()
	closeErr := d.Close()
	if err != nil || closeErr != nil {
		return ErrStore
	}
	return nil
}

// Delete removes only the credential selected by the normalized server. It is
// intentionally idempotent for a missing credential, but refuses to follow or
// remove an unsafe target.
func (s FileStore) Delete(service string) error {
	server, err := normalizedCredentialService(service)
	if err != nil {
		return ErrStore
	}
	dir, err := s.directory()
	if err != nil {
		return ErrStore
	}
	if _, err = os.Lstat(dir); os.IsNotExist(err) {
		return nil
	} else if err != nil || secureCredentialDir(dir, false) != nil {
		return ErrStore
	}
	target := filepath.Join(dir, credentialFilename(server))
	if _, err = os.Lstat(target); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return ErrStore
	}
	if _, err = secureCredentialFile(target); err != nil {
		return ErrStore
	}
	if err = os.Remove(target); err != nil {
		return ErrStore
	}
	return syncDirectory(dir)
}

// DefaultServer returns the normalized platform URL selected by the most
// recent successful interactive login. The URL is non-secret but is protected
// with the same narrow filesystem boundary as credentials to avoid silently
// redirecting deployer traffic to an attacker-selected host.
func (s FileStore) DefaultServer() (string, error) {
	dir, err := s.directory()
	if err != nil {
		return "", ErrStore
	}
	if _, err = os.Lstat(dir); os.IsNotExist(err) {
		return "", ErrNoDefaultServer
	} else if err != nil || secureCredentialDir(dir, false) != nil {
		return "", ErrStore
	}
	path := filepath.Join(dir, defaultServerFile)
	if _, err = os.Lstat(path); os.IsNotExist(err) {
		return "", ErrNoDefaultServer
	} else if err != nil {
		return "", ErrStore
	}
	info, err := secureCredentialFile(path)
	if err != nil || info.Size() > defaultServerLimit {
		return "", ErrStore
	}
	f, err := os.Open(path)
	if err != nil {
		return "", ErrStore
	}
	defer f.Close()
	info, err = f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Mode()&(os.ModePerm|os.ModeSetuid|os.ModeSetgid|os.ModeSticky) != 0600 || info.Size() < 1 || info.Size() > defaultServerLimit {
		return "", ErrStore
	}
	record, err := decodeDefaultServer(io.LimitReader(f, defaultServerLimit+1))
	if err != nil || record.Version != credentialVersion {
		return "", ErrStore
	}
	normalized, err := NormalizeServer(record.Server, false)
	if err != nil || normalized != record.Server {
		return "", ErrStore
	}
	return normalized, nil
}

// SetDefaultServer stores a normalized HTTPS platform URL only after a login
// flow has already verified and durably saved its bearer.
func (s FileStore) SetDefaultServer(service string) error {
	server, err := normalizedCredentialService(service)
	if err != nil {
		return ErrStore
	}
	dir, err := s.directory()
	if err != nil || secureCredentialDir(dir, true) != nil {
		return ErrStore
	}
	target := filepath.Join(dir, defaultServerFile)
	if _, err = os.Lstat(target); err == nil {
		info, secureErr := secureCredentialFile(target)
		if secureErr != nil || info.Size() > defaultServerLimit {
			return ErrStore
		}
	} else if !os.IsNotExist(err) {
		return ErrStore
	}
	b, err := json.Marshal(defaultServerFileRecord{Version: credentialVersion, Server: server})
	if err != nil || len(b) > defaultServerLimit {
		return ErrStore
	}
	return atomicPrivateWrite(dir, target, ".default-server-", b)
}

func decodeDefaultServer(r io.Reader) (defaultServerFileRecord, error) {
	decoder := json.NewDecoder(r)
	first, err := decoder.Token()
	if err != nil {
		return defaultServerFileRecord{}, ErrStore
	}
	if delim, ok := first.(json.Delim); !ok || delim != '{' {
		return defaultServerFileRecord{}, ErrStore
	}
	var record defaultServerFileRecord
	seen := map[string]bool{}
	for decoder.More() {
		key, err := decoder.Token()
		if err != nil {
			return defaultServerFileRecord{}, ErrStore
		}
		name, ok := key.(string)
		if !ok || seen[name] {
			return defaultServerFileRecord{}, ErrStore
		}
		seen[name] = true
		var value json.RawMessage
		if err = decoder.Decode(&value); err != nil {
			return defaultServerFileRecord{}, ErrStore
		}
		switch name {
		case "version":
			err = json.Unmarshal(value, &record.Version)
		case "server":
			err = json.Unmarshal(value, &record.Server)
		default:
			return defaultServerFileRecord{}, ErrStore
		}
		if err != nil {
			return defaultServerFileRecord{}, ErrStore
		}
	}
	last, err := decoder.Token()
	if err != nil {
		return defaultServerFileRecord{}, ErrStore
	}
	if delim, ok := last.(json.Delim); !ok || delim != '}' || decoder.Decode(&struct{}{}) != io.EOF {
		return defaultServerFileRecord{}, ErrStore
	}
	return record, nil
}

func atomicPrivateWrite(dir, target, prefix string, b []byte) error {
	tmp, err := os.CreateTemp(dir, prefix)
	if err != nil {
		return ErrStore
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err = tmp.Chmod(0600); err == nil {
		_, err = tmp.Write(b)
	}
	if err == nil {
		err = tmp.Sync()
	}
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err != nil || os.Rename(tmpName, target) != nil {
		return ErrStore
	}
	return syncDirectory(dir)
}

func syncDirectory(dir string) error {
	d, err := os.Open(dir)
	if err != nil {
		return ErrStore
	}
	err = d.Sync()
	closeErr := d.Close()
	if err != nil || closeErr != nil {
		return ErrStore
	}
	return nil
}
