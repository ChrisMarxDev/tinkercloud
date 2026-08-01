package controlapi

import (
	"testing"
)

func TestAccessInputValidation(t *testing.T) {
	bad := []AccessPolicyInput{{Mode: "other", ExpectedRevision: 1}, {Mode: ""}, {Mode: "private"}}
	for _, v := range bad {
		if validAccess(&v) {
			t.Fatal(v)
		}
	}
	v := AccessPolicyInput{Mode: "private", ExpectedRevision: 1}
	v.Allow.Emails = []string{"B@EXAMPLE.com", "a@example.com"}
	v.Allow.Domains = []string{"Z.example.com", "example.com"}
	if !validAccess(&v) || v.Allow.Emails[0] != "B@example.com" || v.Allow.Domains[0] != "example.com" {
		t.Fatal(v)
	}
	if !validAccess(&AccessPolicyInput{Mode: "public", ExpectedRevision: 1}) {
		t.Fatal("public mode is a valid access-rule maintenance shape")
	}
	for _, d := range []string{"bad domain", "a..b", "-a.com", "a@b.com"} {
		x := AccessPolicyInput{Mode: "private", ExpectedRevision: 1}
		x.Allow.Domains = []string{d}
		if validAccess(&x) {
			t.Fatal(d)
		}
	}
}
