package validation

import "testing"

func TestValidateManagedHostname(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		hostname string
		wantErr  bool
	}{
		{name: "valid nested hostname", hostname: "api.portal.internal.example.test"},
		{name: "valid simple hostname", hostname: "app.internal.example.test"},
		{name: "reject uppercase", hostname: "App.internal.example.test", wantErr: true},
		{name: "reject trailing dot", hostname: "app.internal.example.test.", wantErr: true},
		{name: "reject wrong zone", hostname: "app.example.com", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := ValidateManagedHostname(tt.hostname)
			if tt.wantErr && err == nil {
				t.Fatalf("expected error for %q", tt.hostname)
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected error for %q: %v", tt.hostname, err)
			}
		})
	}
}

func TestInferRecordFromAddress(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		address string
		want    string
		wantErr bool
	}{
		{name: "ipv4", address: "192.0.2.10", want: "A"},
		{name: "ipv6", address: "2001:db8::10", want: "AAAA"},
		{name: "fqdn", address: "backend.internal.example.test", want: "CNAME"},
		{name: "invalid", address: "bad host", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, _, err := InferRecordFromAddress(tt.address)
			if tt.wantErr && err == nil {
				t.Fatalf("expected error for %q", tt.address)
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected error for %q: %v", tt.address, err)
			}
			if got != tt.want {
				t.Fatalf("expected type %q, got %q", tt.want, got)
			}
		})
	}
}
