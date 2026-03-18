package validation

import (
	"fmt"
	"net"
	"regexp"
	"strings"
)

const ManagedZone = "internal.example.test"

var hostnamePattern = regexp.MustCompile(`^([a-z0-9]([-a-z0-9]*[a-z0-9])?\.)+[a-z0-9]([-a-z0-9]*[a-z0-9])?$`)

func ValidateManagedHostname(hostname string) error {
	if hostname == "" {
		return fmt.Errorf("hostname cannot be empty")
	}

	if hostname != strings.ToLower(hostname) {
		return fmt.Errorf("hostname must be lowercase")
	}

	if strings.HasSuffix(hostname, ".") {
		return fmt.Errorf("hostname must not end with a dot")
	}

	if !strings.HasSuffix(hostname, "."+ManagedZone) {
		return fmt.Errorf("hostname must be within %s", ManagedZone)
	}

	if !hostnamePattern.MatchString(hostname) {
		return fmt.Errorf("hostname is not a valid DNS name")
	}

	return nil
}

func ValidateFQDN(hostname string) error {
	if hostname == "" {
		return fmt.Errorf("hostname cannot be empty")
	}

	if hostname != strings.ToLower(hostname) {
		return fmt.Errorf("hostname must be lowercase")
	}

	if strings.HasSuffix(hostname, ".") {
		return fmt.Errorf("hostname must not end with a dot")
	}

	if !hostnamePattern.MatchString(hostname) {
		return fmt.Errorf("hostname is not a valid DNS name")
	}

	return nil
}

func InferRecordFromAddress(address string) (string, string, error) {
	if address == "" {
		return "", "", fmt.Errorf("address cannot be empty")
	}

	if ip := net.ParseIP(address); ip != nil {
		if ip.To4() != nil {
			return "A", address, nil
		}
		return "AAAA", address, nil
	}

	if err := ValidateFQDN(address); err != nil {
		return "", "", fmt.Errorf("address must be an IP or FQDN: %w", err)
	}

	return "CNAME", address, nil
}

func RelativeName(hostname string) string {
	trimmedZone := "." + ManagedZone
	if hostname == ManagedZone {
		return "@"
	}

	return strings.TrimSuffix(hostname, trimmedZone)
}
