package operations

import (
	"errors"
	"path/filepath"
	"strings"
)

const TinkercloudServiceUnit = `[Unit]
Description=Tinkercloud gateway
After=network-online.target
Wants=network-online.target
[Service]
Type=simple
User=tinkercloud
Group=tinkercloud
EnvironmentFile=/etc/tinkercloud/credentials/tinkercloud.env
ExecStart=/usr/local/bin/tinkercloud serve --config /etc/tinkercloud/config.yaml
Restart=on-failure
RestartSec=5
NoNewPrivileges=yes
CapabilityBoundingSet=CAP_NET_BIND_SERVICE
AmbientCapabilities=CAP_NET_BIND_SERVICE
RestrictAddressFamilies=AF_UNIX AF_INET AF_INET6
SocketBindDeny=any
SocketBindAllow=tcp:80
SocketBindAllow=tcp:443
PrivateTmp=yes
ProtectSystem=strict
ProtectHome=yes
ReadWritePaths=/var/lib/tinkercloud
ReadWritePaths=/var/lib/tinkercloud-acme
UMask=0077
[Install]
WantedBy=multi-user.target
`

func ServiceUnit(configPath, credentialPath, dataPath, acmePath string) (string, error) {
	for _, p := range []string{configPath, credentialPath, dataPath, acmePath} {
		if !safeSystemdPath(p) {
			return "", errors.New("unsafe systemd path")
		}
	}
	if dataPath == acmePath {
		return "", errors.New("systemd writable paths must be distinct")
	}
	u := strings.Replace(TinkercloudServiceUnit, "/etc/tinkercloud/credentials/tinkercloud.env", credentialPath, 1)
	u = strings.Replace(u, "/etc/tinkercloud/config.yaml", configPath, 1)
	u = strings.Replace(u, "ReadWritePaths=/var/lib/tinkercloud\n", "ReadWritePaths="+dataPath+"\n", 1)
	u = strings.Replace(u, "ReadWritePaths=/var/lib/tinkercloud-acme\n", "ReadWritePaths="+acmePath+"\n", 1)
	if err := ValidateServiceUnit(u, dataPath, acmePath); err != nil {
		return "", err
	}
	return u, nil
}

// ValidateServiceUnit proves the security-relevant invariants of the unit
// without requiring a running systemd instance. The gateway is deliberately
// unprivileged; its only ambient capability is the ability to bind the two
// public gateway ports.
func ValidateServiceUnit(unit, dataPath, acmePath string) error {
	if dataPath == acmePath || !safeSystemdPath(dataPath) || !safeSystemdPath(acmePath) {
		return errors.New("unsafe systemd writable paths")
	}
	lines := strings.Split(strings.TrimSuffix(unit, "\n"), "\n")
	values := map[string][]string{}
	for _, line := range lines {
		key, value, ok := strings.Cut(line, "=")
		if ok {
			values[key] = append(values[key], value)
		}
	}
	want := map[string]string{
		"User":                    "tinkercloud",
		"Group":                   "tinkercloud",
		"NoNewPrivileges":         "yes",
		"CapabilityBoundingSet":   "CAP_NET_BIND_SERVICE",
		"AmbientCapabilities":     "CAP_NET_BIND_SERVICE",
		"RestrictAddressFamilies": "AF_UNIX AF_INET AF_INET6",
		"SocketBindDeny":          "any",
		"ProtectSystem":           "strict",
		"ProtectHome":             "yes",
		"PrivateTmp":              "yes",
		"UMask":                   "0077",
	}
	for key, expected := range want {
		got := values[key]
		if len(got) != 1 || got[0] != expected {
			return errors.New("unsafe systemd directive: " + key)
		}
	}
	bindAllow := values["SocketBindAllow"]
	if len(bindAllow) != 2 || bindAllow[0] != "tcp:80" || bindAllow[1] != "tcp:443" {
		return errors.New("unsafe systemd directive: SocketBindAllow")
	}
	writable := values["ReadWritePaths"]
	if len(writable) != 2 || writable[0] != dataPath || writable[1] != acmePath {
		return errors.New("unsafe systemd writable paths")
	}
	return nil
}

func safeSystemdPath(path string) bool {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path || path == string(filepath.Separator) {
		return false
	}
	for _, r := range path {
		if !(r == '/' || r == '.' || r == '_' || r == '-' ||
			r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' ||
			r >= '0' && r <= '9') {
			return false
		}
	}
	return true
}
