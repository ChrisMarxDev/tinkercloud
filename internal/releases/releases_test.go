package releases

import "testing"

func TestActivationRequiresAllEvidence(t *testing.T) {
	d := Deployment{ID: "dep", AppID: "app", State: Verified}
	if err := d.CanActivate(ActivationRequirements{PolicyReady: true, CertificateReady: true}); err == nil {
		t.Fatal("accepted missing denial probe")
	}
	if _, err := PlanActivation(&Deployment{ID: "old", AppID: "app", State: Active}, d, ActivationRequirements{PolicyReady: true, CertificateReady: true, DenialProbePassed: true}); err != nil {
		t.Fatal(err)
	}
}

func TestGenerateManifestIsDeterministicAndRoundTrips(t *testing.T) {
	m := Manifest{Version: 1, Name: "demo", Description: "Useful dashboard", BuildOutput: "dist", Emails: []string{"Zebra@example.com", "alice@example.com"}, Domains: []string{"Example.com"}, KV: true, Realtime: true, LLMChat: true, SPAFallback: "index.html"}
	first, err := GenerateManifest(m)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseManifest(first)
	if err != nil || parsed.Name != "demo" || len(parsed.Emails) != 2 || parsed.Emails[0] != "Zebra@example.com" || !parsed.LLMChat {
		t.Fatalf("parsed=%#v err=%v", parsed, err)
	}
	second, err := GenerateManifest(parsed)
	if err != nil || string(first) != string(second) {
		t.Fatalf("not deterministic:\n%s\n%s\n%v", first, second, err)
	}
}
func TestManifestRejectsPublicAndUnknown(t *testing.T) {
	for _, in := range []string{
		"version: 1\nname: demo\naccess:\n  mode: public\n",
		"version: 1\nname: demo\nwat: true\n",
		"version: 1\nname: demo\ncapabilities:\n  llm:\n    chat: true\n    model: caller-selected\n",
		"version: 1\nname: demo\ncapabilities:\n  llm:\n    provider: attacker\n",
	} {
		if _, err := ParseManifest([]byte(in)); err == nil {
			t.Fatalf("accepted %q", in)
		}
	}
}
func TestManifestAcceptsLogicalLLMChatRequestOnly(t *testing.T) {
	m, err := ParseManifest([]byte("version: 1\nname: chat\ncapabilities:\n  llm:\n    chat: true\n"))
	if err != nil || !m.LLMChat {
		t.Fatalf("manifest=%+v err=%v", m, err)
	}
}
func TestManifestAcceptsCanonicalAccessRules(t *testing.T) {
	m, err := ParseManifest([]byte("version: 1\nname: invoice-review\naccess:\n  mode: private\n  allow:\n    emails:\n      - alice@example.com\n    domains:\n      - example.com\nfeatures:\n  kv: true\n  realtime: false\nspa:\n  fallback: index.html\n"))
	if err != nil || len(m.Emails) != 1 || len(m.Domains) != 1 {
		t.Fatalf("%+v %v", m, err)
	}
}

func TestManifestCanonicalizesAndRejectsInvalidAccessRules(t *testing.T) {
	m, err := ParseManifest([]byte("version: 1\nname: demo\naccess:\n  allow:\n    emails:\n      - Zebra@Example.com\n      - alice@example.com\n    domains:\n      - z.example.com\n      - internal.example\n      - train.example\n      - example.com\n"))
	if err != nil || len(m.Emails) != 2 || len(m.Domains) != 4 || m.Emails[0] != "Zebra@example.com" || m.Domains[0] != "example.com" {
		t.Fatalf("manifest=%+v err=%v", m, err)
	}
	for _, invalid := range []string{
		"version: 1\nname: demo\naccess:\n  allow:\n    emails: [not-an-email]\n",
		"version: 1\nname: demo\naccess:\n  allow:\n    domains: [bad_domain]\n",
		"version: 1\nname: demo\naccess:\n  allow:\n    domains: [bad\\t.example]\n",
		"version: 1\nname: demo\naccess:\n  allow:\n    domains: [bad\\r.example]\n",
	} {
		if _, err := ParseManifest([]byte(invalid)); err == nil {
			t.Fatalf("accepted invalid access: %q", invalid)
		}
	}
	for _, control := range []string{"\t", "\r", "\n"} {
		input := "version: 1\nname: demo\naccess:\n  allow:\n    domains:\n      - bad" + control + ".example\n"
		if _, err := ParseManifest([]byte(input)); err == nil {
			t.Fatalf("accepted control character %q in domain", control)
		}
	}
}
