package releases

import (
	"bytes"
	"errors"
	"github.com/ChrisMarxDev/tinkercloud/internal/appnamespace"
	"github.com/ChrisMarxDev/tinkercloud/internal/identity"
	"io"
	"path"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"go.yaml.in/yaml/v3"
)

var ErrManifest = errors.New("invalid manifest")

type Manifest struct {
	Version     int
	Name        string
	Description string
	// Tags, AccessMode, and Indexing are immutable release metadata introduced
	// by manifest v2. Parsing them does not make an app publicly reachable: the
	// gateway and current policy installation remain private-only until the
	// separate public-static authorization slice exists.
	Tags                []string
	AccessMode          string
	Indexing            bool
	Emails, Domains     []string
	KV, Blobs, Realtime bool
	LLMChat             bool
	SPAFallback         string
	BuildOutput         string
}

// ValidSlug preserves the releases package's established validation seam while
// enforcing the shared public app-hostname namespace policy.
func ValidSlug(value string) bool { return appnamespace.Valid(value) }

type rawManifest struct {
	Version     int      `yaml:"version"`
	Name        string   `yaml:"name"`
	Description string   `yaml:"description"`
	Tags        []string `yaml:"tags"`
	Build       struct {
		Output string `yaml:"output"`
	} `yaml:"build"`
	Access struct {
		Mode     string `yaml:"mode"`
		Indexing *bool  `yaml:"indexing"`
		Allow    struct {
			Emails  []string `yaml:"emails"`
			Domains []string `yaml:"domains"`
		} `yaml:"allow"`
	} `yaml:"access"`
	Features struct {
		KV       bool `yaml:"kv"`
		Blobs    bool `yaml:"blobs"`
		Realtime bool `yaml:"realtime"`
	} `yaml:"features"`
	Capabilities struct {
		LLM struct {
			Chat bool `yaml:"chat"`
		} `yaml:"llm"`
	} `yaml:"capabilities"`
	SPA struct {
		Fallback string `yaml:"fallback"`
	} `yaml:"spa"`
}

func safe(n *yaml.Node) bool {
	if n.Alias != nil || n.Tag != "" && n.Tag != "!!map" && n.Tag != "!!seq" && n.Tag != "!!str" && n.Tag != "!!int" && n.Tag != "!!bool" && n.Tag != "!!null" {
		return false
	}
	if n.Kind == yaml.MappingNode {
		seen := map[string]bool{}
		for i := 0; i < len(n.Content); i += 2 {
			if seen[n.Content[i].Value] {
				return false
			}
			seen[n.Content[i].Value] = true
		}
	}
	for _, c := range n.Content {
		if !safe(c) {
			return false
		}
	}
	return true
}

// mappingField keeps versioned optional-field validation strict even when YAML
// decodes an explicit null into the same Go zero value as an omitted field.
func mappingField(n *yaml.Node, key string) (*yaml.Node, bool) {
	if n == nil {
		return nil, false
	}
	if n.Kind == yaml.DocumentNode && len(n.Content) == 1 {
		n = n.Content[0]
	}
	if n.Kind != yaml.MappingNode {
		return nil, false
	}
	for i := 0; i+1 < len(n.Content); i += 2 {
		if n.Content[i].Value == key {
			return n.Content[i+1], true
		}
	}
	return nil, false
}

// ParseManifest accepts exactly one strict v1 or v2 YAML document. Aliases,
// custom tags, duplicate keys and unknown fields are rejected before domain
// conversion. V1 deliberately retains its original private-only semantics.
func ParseManifest(data []byte) (Manifest, error) {
	d := yaml.NewDecoder(bytes.NewReader(data))
	var n yaml.Node
	if e := d.Decode(&n); e != nil || !safe(&n) {
		return Manifest{}, ErrManifest
	}
	tagsNode, tagsPresent := mappingField(&n, "tags")
	accessNode, _ := mappingField(&n, "access")
	indexingNode, indexingPresent := mappingField(accessNode, "indexing")
	var extra yaml.Node
	if e := d.Decode(&extra); e != io.EOF {
		return Manifest{}, ErrManifest
	}
	d = yaml.NewDecoder(bytes.NewReader(data))
	d.KnownFields(true)
	var raw rawManifest
	if e := d.Decode(&raw); e != nil {
		return Manifest{}, ErrManifest
	}
	description, err := NormalizeDescription(raw.Description)
	if err != nil {
		return Manifest{}, ErrManifest
	}
	m := Manifest{Version: raw.Version, Name: strings.ToLower(raw.Name), Description: description, Tags: append([]string(nil), raw.Tags...), KV: raw.Features.KV, Blobs: raw.Features.Blobs, Realtime: raw.Features.Realtime, LLMChat: raw.Capabilities.LLM.Chat, SPAFallback: raw.SPA.Fallback, BuildOutput: raw.Build.Output}
	if m.BuildOutput == "" {
		m.BuildOutput = "."
	}
	if path.IsAbs(m.BuildOutput) || path.Clean(m.BuildOutput) != m.BuildOutput || strings.Contains(m.BuildOutput, "\\") || strings.HasPrefix(m.BuildOutput, "../") {
		return Manifest{}, ErrManifest
	}
	if !ValidSlug(m.Name) || (m.Version != 1 && m.Version != 2) {
		return Manifest{}, ErrManifest
	}
	m.AccessMode = raw.Access.Mode
	if m.AccessMode == "" {
		m.AccessMode = "private"
	}
	if m.Version == 1 {
		// V1 receipts cannot silently acquire v2 reach semantics, including an
		// explicitly false indexing field or an empty tags list.
		if m.AccessMode != "private" || indexingPresent || tagsPresent {
			return Manifest{}, ErrManifest
		}
	} else {
		if tagsPresent && tagsNode.Kind != yaml.SequenceNode {
			return Manifest{}, ErrManifest
		}
		if indexingPresent && (indexingNode.Kind != yaml.ScalarNode || indexingNode.Tag != "!!bool") {
			return Manifest{}, ErrManifest
		}
		if m.AccessMode != "private" && m.AccessMode != "public" {
			return Manifest{}, ErrManifest
		}
		if raw.Access.Indexing != nil {
			m.Indexing = *raw.Access.Indexing
		}
		if m.AccessMode != "public" && m.Indexing {
			return Manifest{}, ErrManifest
		}
		if !canonicalTags(m.Tags) {
			return Manifest{}, ErrManifest
		}
		slices.Sort(m.Tags)
		if m.AccessMode == "public" && m.HasBrowserCapability() {
			return Manifest{}, ErrManifest
		}
	}
	if m.SPAFallback != "" && (path.IsAbs(m.SPAFallback) || path.Clean(m.SPAFallback) != m.SPAFallback || strings.Contains(m.SPAFallback, "\\")) {
		return Manifest{}, ErrManifest
	}
	seenEmails := map[string]bool{}
	for _, v := range raw.Access.Allow.Emails {
		v, err := identity.Normalize(v)
		if err != nil || seenEmails[v] {
			return Manifest{}, ErrManifest
		}
		seenEmails[v] = true
		m.Emails = append(m.Emails, v)
	}
	seenDomains := map[string]bool{}
	for _, v := range raw.Access.Allow.Domains {
		v = strings.ToLower(strings.TrimSpace(v))
		if !validDomain(v) || seenDomains[v] {
			return Manifest{}, ErrManifest
		}
		seenDomains[v] = true
		m.Domains = append(m.Domains, v)
	}
	// Canonical ordering makes a manifest's persisted policy stable regardless
	// of harmless YAML ordering differences.
	slices.Sort(m.Emails)
	slices.Sort(m.Domains)
	return m, nil
}

// HasBrowserCapability centralizes the capability-free public-candidate rule.
// New browser-exposed capabilities must join this check before public static
// serving is ever implemented.
func (m Manifest) HasBrowserCapability() bool {
	return m.KV || m.Blobs || m.Realtime || m.LLMChat
}

func canonicalTags(tags []string) bool {
	if len(tags) > 8 {
		return false
	}
	seen := make(map[string]struct{}, len(tags))
	for _, tag := range tags {
		if len(tag) < 1 || len(tag) > 24 {
			return false
		}
		for i := 0; i < len(tag); i++ {
			c := tag[i]
			if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-') {
				return false
			}
		}
		first, last := tag[0], tag[len(tag)-1]
		if !(first >= 'a' && first <= 'z' || first >= '0' && first <= '9') || !(last >= 'a' && last <= 'z' || last >= '0' && last <= '9') {
			return false
		}
		if _, ok := seen[tag]; ok {
			return false
		}
		seen[tag] = struct{}{}
	}
	return true
}

// ValidStoredManifest checks immutable manifest metadata after it has been
// decoded from persistence. It is intentionally stricter than a presentation
// helper: malformed active metadata makes a read model unavailable instead of
// leaking a partial or guessed catalog record.
func ValidStoredManifest(m Manifest) bool {
	if (m.Version != 1 && m.Version != 2) || !ValidSlug(m.Name) {
		return false
	}
	description, err := NormalizeDescription(m.Description)
	if err != nil || description != m.Description {
		return false
	}
	mode := m.AccessMode
	if mode == "" {
		mode = "private"
	}
	if m.Version == 1 {
		return mode == "private" && !m.Indexing && len(m.Tags) == 0
	}
	if (mode != "private" && mode != "public") || mode != "public" && m.Indexing || !canonicalTags(m.Tags) || !slices.IsSorted(m.Tags) {
		return false
	}
	return mode != "public" || !m.HasBrowserCapability()
}

// NormalizeDescription produces the only description form persisted in an
// immutable deployment manifest. Empty presentation text is permitted. A
// description is otherwise trimmed at its edges, remains a single line, and
// is bounded by Unicode code points rather than UTF-8 bytes.
func NormalizeDescription(value string) (string, error) {
	if !utf8.ValidString(value) {
		return "", ErrManifest
	}
	for _, r := range value {
		if unicode.IsControl(r) || r == '\u2028' || r == '\u2029' {
			return "", ErrManifest
		}
	}
	value = strings.TrimSpace(value)
	if utf8.RuneCountInString(value) > 280 {
		return "", ErrManifest
	}
	return value, nil
}

func validDomain(d string) bool {
	if d == "" || len(d) > 253 || strings.ContainsAny(d, "@/ \t\r\n") {
		return false
	}
	for _, label := range strings.Split(d, ".") {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, c := range label {
			if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-') {
				return false
			}
		}
	}
	return true
}

// GenerateManifest emits the deterministic strict V2 form used by `tinker init`.
// It round-trips ParseManifest before returning, so local generation and server
// deployment share the same contract rather than maintaining a second parser.
func GenerateManifest(m Manifest) ([]byte, error) {
	encode := func(value Manifest) ([]byte, error) {
		description, err := NormalizeDescription(value.Description)
		if err != nil || (value.Version != 0 && value.Version != 1 && value.Version != 2) || !ValidSlug(value.Name) {
			return nil, ErrManifest
		}
		mode := value.AccessMode
		if mode == "" {
			mode = "private"
		}
		if mode != "private" && mode != "public" || mode != "public" && value.Indexing || !canonicalTags(value.Tags) || mode == "public" && value.HasBrowserCapability() {
			return nil, ErrManifest
		}
		indexing := value.Indexing
		raw := rawManifest{Version: 2, Name: value.Name, Description: description, Tags: append([]string(nil), value.Tags...)}
		slices.Sort(raw.Tags)
		raw.Build.Output = value.BuildOutput
		if raw.Build.Output == "" {
			raw.Build.Output = "."
		}
		raw.Access.Mode = mode
		raw.Access.Indexing = &indexing
		raw.Access.Allow.Emails = append([]string(nil), value.Emails...)
		raw.Access.Allow.Domains = append([]string(nil), value.Domains...)
		raw.Features.KV, raw.Features.Blobs, raw.Features.Realtime = value.KV, value.Blobs, value.Realtime
		raw.Capabilities.LLM.Chat = value.LLMChat
		raw.SPA.Fallback = value.SPAFallback
		b, err := yaml.Marshal(raw)
		if err != nil {
			return nil, ErrManifest
		}
		return b, nil
	}
	b, err := encode(m)
	if err != nil {
		return nil, err
	}
	canonical, err := ParseManifest(b)
	if err != nil {
		return nil, ErrManifest
	}
	b, err = encode(canonical)
	if err != nil {
		return nil, ErrManifest
	}
	if _, err = ParseManifest(b); err != nil {
		return nil, ErrManifest
	}
	return b, nil
}
