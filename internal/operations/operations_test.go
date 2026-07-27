package operations

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type diskSource struct {
	disk Disk
	err  error
}

func (s diskSource) Disk(context.Context) (Disk, error) { return s.disk, s.err }

func TestInitCannotSkipSteps(t *testing.T) {
	var s InitState
	if err := s.Complete(InitDatabase); err == nil {
		t.Fatal("skipped init accepted")
	}
	if err := s.Complete(InitPreflight); err != nil {
		t.Fatal(err)
	}
}
func TestCriticalDiskDeniesWrites(t *testing.T) {
	if (Watermarks{Warning: 80, Stop: 90}).AllowsWrite(Disk{Used: 90, Total: 100}) {
		t.Fatal("critical write allowed")
	}
}

func TestDiskWriteGateFailsClosedAtBoundaryAndSourceFailure(t *testing.T) {
	w := Watermarks{Warning: 80, Stop: 90}
	for _, tc := range []struct {
		source diskSource
		want   bool
	}{
		{diskSource{disk: Disk{Used: 89, Total: 100}}, true},
		{diskSource{disk: Disk{Used: 90, Total: 100}}, false},
		{diskSource{err: errors.New("statfs failed")}, false},
	} {
		g := DiskWriteGate{Source: tc.source, Watermarks: w}
		err := g.AllowWrite(context.Background(), WriteKV)
		if (err == nil) != tc.want {
			t.Fatalf("gate result %v, want %v", err, tc.want)
		}
	}
	if (Watermarks{Warning: 90, Stop: 80}).AllowsWrite(Disk{Used: 1, Total: 100}) {
		t.Fatal("invalid watermark allowed writes")
	}
}

func TestInitStateRejectsUnknownAndGappedSteps(t *testing.T) {
	for _, raw := range []string{
		`{"completed":{"unknown":true}}`,
		`{"completed":{"preflight":true,"database":true}}`,
	} {
		if _, err := ParseInitState([]byte(raw)); err == nil {
			t.Fatal("unsafe init state accepted", raw)
		}
	}
}

func TestServiceUnitOnlyAcceptsAbsoluteSingleLinePaths(t *testing.T) {
	u, err := ServiceUnit("/etc/tinyhost/config.yaml", "/etc/tinyhost/credentials/tinyhost.env", "/srv/tiny-data", "/srv/tiny-acme")
	if err != nil ||
		!strings.Contains(u, "EnvironmentFile=/etc/tinyhost/credentials/tinyhost.env") ||
		!strings.Contains(u, "ReadWritePaths=/srv/tiny-data\n") ||
		!strings.Contains(u, "ReadWritePaths=/srv/tiny-acme\n") {
		t.Fatal(u, err)
	}
	for _, paths := range [][4]string{
		{"relative", "/etc/creds", "/srv/data", "/srv/acme"},
		{"/etc/config\nExecStart=/bin/sh", "/etc/creds", "/srv/data", "/srv/acme"},
		{"/etc/config", "/etc/creds", "/srv/data", "/srv/data"},
		{"/etc/config", "/etc/creds", "/", "/srv/acme"},
		{"/etc/config", "/etc/creds", "/srv/data path", "/srv/acme"},
		{"/etc/config", "/etc/creds", "/srv/data/../escape", "/srv/acme"},
	} {
		if _, err := ServiceUnit(paths[0], paths[1], paths[2], paths[3]); err == nil {
			t.Fatal("unsafe systemd path accepted", paths)
		}
	}
}

func TestServiceUnitGrantsOnlyGatewayBindCapability(t *testing.T) {
	u, err := ServiceUnit("/etc/tinyhost/config.yaml", "/etc/tinyhost/credentials/tinyhost.env", "/srv/tiny-data", "/srv/tiny-acme")
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateServiceUnit(u, "/srv/tiny-data", "/srv/tiny-acme"); err != nil {
		t.Fatal(err)
	}
	for _, unsafe := range []string{
		strings.Replace(u, "AmbientCapabilities=CAP_NET_BIND_SERVICE", "AmbientCapabilities=CAP_NET_BIND_SERVICE CAP_SYS_ADMIN", 1),
		strings.Replace(u, "CapabilityBoundingSet=CAP_NET_BIND_SERVICE", "CapabilityBoundingSet=CAP_NET_BIND_SERVICE CAP_DAC_OVERRIDE", 1),
		strings.Replace(u, "RestrictAddressFamilies=AF_UNIX AF_INET AF_INET6", "RestrictAddressFamilies=AF_UNIX AF_INET AF_INET6 AF_PACKET", 1),
		strings.Replace(u, "SocketBindDeny=any", "SocketBindDeny=tcp:1-79", 1),
		strings.Replace(u, "SocketBindAllow=tcp:80", "SocketBindAllow=udp:80", 1),
		strings.Replace(u, "SocketBindAllow=tcp:443", "SocketBindAllow=tcp:443\nSocketBindAllow=tcp:8080", 1),
		strings.Replace(u, "User=tinyhost", "User=root", 1),
		strings.Replace(u, "NoNewPrivileges=yes", "NoNewPrivileges=no", 1),
		strings.Replace(u, "ReadWritePaths=/srv/tiny-acme", "ReadWritePaths=/srv/tiny-acme\nReadWritePaths=/etc", 1),
	} {
		if err := ValidateServiceUnit(unsafe, "/srv/tiny-data", "/srv/tiny-acme"); err == nil {
			t.Fatal("unsafe generated unit accepted")
		}
	}
}

func TestPackagedServiceUnitGrantsOnlyGatewayBindCapability(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("..", "..", "packaging", "systemd", "tinyhost.service"))
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateServiceUnit(string(body), "/var/lib/tinyhost", "/var/lib/tinyhost-acme"); err != nil {
		t.Fatal(err)
	}
}
