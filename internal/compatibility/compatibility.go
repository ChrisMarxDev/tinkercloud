// Package compatibility owns the small, technology-neutral release and client
// version contract shared by build tooling, HTTP adapters, and the updater.
package compatibility

import (
	"errors"
	"strconv"
	"strings"
)

const (
	ControlAPIVersion     = "1"
	AppAPIVersion         = "1"
	SchemaVersion         = "1"
	ReleaseManifestSchema = "2"
	MinimumCLIVersion     = "0.1.0"
	MaximumCLIVersion     = "1.0.0"
	MinimumSDKVersion     = "0.1.0"
	MaximumSDKVersion     = "1.0.0"
)

var ErrInvalid = errors.New("invalid compatibility version")

type Version struct {
	Major uint64
	Minor uint64
	Patch uint64
}

func Parse(raw string) (Version, error) {
	parts := strings.Split(raw, ".")
	if len(parts) != 3 {
		return Version{}, ErrInvalid
	}
	values := [3]uint64{}
	for i, part := range parts {
		if part == "" || len(part) > 1 && part[0] == '0' {
			return Version{}, ErrInvalid
		}
		for _, r := range part {
			if r < '0' || r > '9' {
				return Version{}, ErrInvalid
			}
		}
		value, err := strconv.ParseUint(part, 10, 64)
		if err != nil {
			return Version{}, ErrInvalid
		}
		values[i] = value
	}
	return Version{Major: values[0], Minor: values[1], Patch: values[2]}, nil
}

func (v Version) Compare(other Version) int {
	if v.Major != other.Major {
		if v.Major < other.Major {
			return -1
		}
		return 1
	}
	if v.Minor != other.Minor {
		if v.Minor < other.Minor {
			return -1
		}
		return 1
	}
	if v.Patch < other.Patch {
		return -1
	}
	if v.Patch > other.Patch {
		return 1
	}
	return 0
}

type Range struct {
	MinInclusive string `json:"min_inclusive"`
	MaxExclusive string `json:"max_exclusive"`
}

func (r Range) Valid() bool {
	min, err := Parse(r.MinInclusive)
	if err != nil {
		return false
	}
	max, err := Parse(r.MaxExclusive)
	return err == nil && min.Compare(max) < 0
}

func (r Range) Contains(raw string) bool {
	if !r.Valid() {
		return false
	}
	value, err := Parse(raw)
	if err != nil {
		return false
	}
	min, _ := Parse(r.MinInclusive)
	max, _ := Parse(r.MaxExclusive)
	return min.Compare(value) <= 0 && value.Compare(max) < 0
}

type ClientAPI struct {
	Version string `json:"version"`
	Client  Range  `json:"client"`
}

// Runtime returns a truthful strict version document for development builds
// without making the non-release label "dev" part of the public version model.
func Runtime(serverVersion string) Matrix {
	if _, err := Parse(serverVersion); err != nil {
		serverVersion = "0.0.0"
	}
	return Current(serverVersion)
}

type Matrix struct {
	ServerVersion         string    `json:"server_version"`
	ControlAPI            ClientAPI `json:"control_api"`
	AppAPI                ClientAPI `json:"app_api"`
	SchemaVersion         string    `json:"schema_version"`
	ReleaseManifestSchema string    `json:"release_manifest_schema"`
}

func Current(serverVersion string) Matrix {
	return Matrix{
		ServerVersion: serverVersion,
		ControlAPI: ClientAPI{
			Version: ControlAPIVersion,
			Client:  Range{MinInclusive: MinimumCLIVersion, MaxExclusive: MaximumCLIVersion},
		},
		AppAPI: ClientAPI{
			Version: AppAPIVersion,
			Client:  Range{MinInclusive: MinimumSDKVersion, MaxExclusive: MaximumSDKVersion},
		},
		SchemaVersion:         SchemaVersion,
		ReleaseManifestSchema: ReleaseManifestSchema,
	}
}

func (m Matrix) Valid() bool {
	if _, err := Parse(m.ServerVersion); err != nil {
		return false
	}
	return m.ControlAPI.Version == ControlAPIVersion &&
		m.AppAPI.Version == AppAPIVersion &&
		m.SchemaVersion == SchemaVersion &&
		m.ReleaseManifestSchema == ReleaseManifestSchema &&
		m.ControlAPI.Client.Valid() &&
		m.AppAPI.Client.Valid()
}

func CompatibleArtifact(version, api, schema string) bool {
	_, err := Parse(version)
	return err == nil && api == ControlAPIVersion && schema == SchemaVersion
}
