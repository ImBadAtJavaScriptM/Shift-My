package netpolicy

import "testing"

func TestControlledPolicyAllowsOnlyExactLabHosts(t *testing.T) {
	policy, err := NewControlled("lab.example.test")
	if err != nil { t.Fatal(err) }
	for _, host := range []string{
		"loc-a.lab.example.test",
		"loc-b.lab.example.test",
		"device-loc.lab.example.test",
		"LOC-A.LAB.EXAMPLE.TEST.",
	} {
		if !policy.AllowedHost(host) { t.Fatalf("expected %q allowed", host) }
	}
	for _, host := range []string{
		"lab.example.test",
		"example.test",
		"other.lab.example.test",
		"gs-loc.apple.com",
	} {
		if policy.AllowedHost(host) { t.Fatalf("unexpected host allowed: %q", host) }
	}
}

func TestControlledPolicyRejectsBlockedBaseDomain(t *testing.T) {
	if _, err := NewControlled("apple.com"); err == nil {
		t.Fatal("expected production base domain rejection")
	}
}
