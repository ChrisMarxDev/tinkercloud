package compatibility

import "testing"

func TestStrictVersionsAndRanges(t *testing.T) {
	valid := []string{"0.1.0", "1.0.0", "18446744073709551615.2.3"}
	for _, raw := range valid {
		if _, err := Parse(raw); err != nil {
			t.Fatalf("Parse(%q): %v", raw, err)
		}
	}
	invalid := []string{"", "1", "1.2", "01.2.3", "1.02.3", "1.2.03", "1.2.3-beta", "+1.2.3", "1.2.18446744073709551616"}
	for _, raw := range invalid {
		if _, err := Parse(raw); err == nil {
			t.Fatalf("Parse(%q) accepted", raw)
		}
	}
	r := Range{MinInclusive: "0.1.0", MaxExclusive: "1.0.0"}
	for _, raw := range []string{"0.1.0", "0.999.12"} {
		if !r.Contains(raw) {
			t.Fatalf("range rejected %s", raw)
		}
	}
	for _, raw := range []string{"0.0.9", "1.0.0", "invalid"} {
		if r.Contains(raw) {
			t.Fatalf("range accepted %s", raw)
		}
	}
}

func TestCurrentMatrixAndArtifactCompatibility(t *testing.T) {
	if !Current("0.1.0").Valid() {
		t.Fatal("current matrix invalid")
	}
	if Current("dev").Valid() {
		t.Fatal("development label accepted as release")
	}
	if !Runtime("dev").Valid() || Runtime("dev").ServerVersion != "0.0.0" {
		t.Fatal("development runtime matrix is not strict")
	}
	if !CompatibleArtifact("0.1.0", "1", "1") {
		t.Fatal("current artifact rejected")
	}
	for _, candidate := range [][3]string{{"dev", "1", "1"}, {"0.1.0", "2", "1"}, {"0.1.0", "1", "2"}} {
		if CompatibleArtifact(candidate[0], candidate[1], candidate[2]) {
			t.Fatalf("incompatible artifact accepted: %#v", candidate)
		}
	}
}
