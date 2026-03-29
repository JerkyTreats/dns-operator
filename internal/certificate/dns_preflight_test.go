package certificate

import "testing"

func TestPreflightFQDNUsesDedicatedName(t *testing.T) {
	t.Parallel()

	got := preflightFQDN("otel.internal.example.test")
	want := "_dns-operator-preflight.otel.internal.example.test"
	if got != want {
		t.Fatalf("preflightFQDN returned %q, want %q", got, want)
	}
}
