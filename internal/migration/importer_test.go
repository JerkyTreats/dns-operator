package migration

import (
	"strings"
	"testing"

	certificatev1alpha1 "github.com/jerkytreats/dns-operator/api/certificate/v1alpha1"
	dnsv1alpha1 "github.com/jerkytreats/dns-operator/api/dns/v1alpha1"
	publishv1alpha1 "github.com/jerkytreats/dns-operator/api/publish/v1alpha1"
	tailscalev1alpha1 "github.com/jerkytreats/dns-operator/api/tailscale/v1alpha1"
	corev1 "k8s.io/api/core/v1"
)

func TestImportBuildsExpectedResources(t *testing.T) {
	t.Parallel()

	result, err := Import(ImportInput{
		Namespace:         "dns-operator-system",
		BundleName:        "internal-shared",
		NameserverAddress: "192.0.2.53",
		ConfigYAML: []byte(`
tailscale:
  api_key: tskey-api-123
  tailnet: example.ts.net
  dns:
    zone: internal.example.test
certificate:
  email: admin@example.com
  domain: internal.example.test
  cloudflare_api_token: cf-token
  use_production_certs: true
  renewal:
    renew_before: 720h
`),
		ZoneFile: []byte(`
$ORIGIN internal.example.test.
$TTL 300
@ IN SOA ns1.internal.example.test. hostmaster.internal.example.test. (
  2026000000 ; serial
  120 ; refresh
  60 ; retry
  604800 ; expire
  300 ; minimum
)
@ IN NS ns1.internal.example.test.
App 300 IN A 192.0.2.10
api.portal 300 IN A 192.0.2.20
`),
		ProxyRulesJSON: []byte(`{
  "app.internal.example.test": {
    "hostname": "App.Internal.Example.Test",
    "target_ip": "192.0.2.10",
    "target_port": 8443,
    "protocol": "https",
    "enabled": true,
    "created_at": "2026-03-16T10:00:00Z"
  },
  "disabled.internal.example.test": {
    "hostname": "disabled.internal.example.test",
    "target_ip": "192.0.2.99",
    "target_port": 8080,
    "protocol": "http",
    "enabled": false,
    "created_at": "2026-03-16T10:00:00Z"
  }
}`),
		CertificateDomainsJSON: []byte(`{
  "base_domain": "internal.example.test",
  "san_domains": ["App.Internal.Example.Test", "api.portal.internal.example.test"],
  "updated_at": "2026-03-16T10:00:00Z"
}`),
		Caddyfile: []byte(`
app.internal.example.test {
    route /* {
        reverse_proxy https://192.0.2.10:8443 {
            transport http {
                tls_insecure_skip_verify
            }
        }
    }
}
`),
	})
	if err != nil {
		t.Fatalf("import returned error: %v", err)
	}

	if result.Report.ImportedObjectCounts["Secret"] != 2 {
		t.Fatalf("expected 2 imported secrets, got %#v", result.Report.ImportedObjectCounts)
	}
	if got := result.Report.CaseCollisions["app.internal.example.test"]; len(got) == 0 {
		t.Fatalf("expected case collision report for app hostname, got %#v", result.Report.CaseCollisions)
	}
	if len(result.Report.SkippedDisabledProxyRules) != 1 || result.Report.SkippedDisabledProxyRules[0] != "disabled.internal.example.test" {
		t.Fatalf("unexpected skipped disabled rules: %#v", result.Report.SkippedDisabledProxyRules)
	}

	var dnsRecordCount int
	var importedService *publishv1alpha1.PublishedService
	var bundle *certificatev1alpha1.CertificateBundle
	var tailnetConfig *tailscalev1alpha1.TailnetDNSConfig
	for _, object := range result.Objects {
		switch typed := object.(type) {
		case *corev1.Secret:
			if typed.Name == DefaultCloudflareSecretName && typed.StringData["api-token"] != "cf-token" {
				t.Fatalf("unexpected cloudflare secret contents: %#v", typed.StringData)
			}
		case *dnsv1alpha1.DNSRecord:
			dnsRecordCount++
		case *publishv1alpha1.PublishedService:
			if typed.Name == "app-internal-example-test" {
				importedService = typed
			}
		case *certificatev1alpha1.CertificateBundle:
			bundle = typed
		case *tailscalev1alpha1.TailnetDNSConfig:
			tailnetConfig = typed
		}
	}

	if dnsRecordCount != 2 {
		t.Fatalf("expected 2 dns records, got %d", dnsRecordCount)
	}
	if importedService == nil {
		t.Fatal("expected imported published service")
	}
	if importedService.Spec.Backend.Transport == nil || !importedService.Spec.Backend.Transport.InsecureSkipVerify {
		t.Fatalf("expected insecureSkipVerify transport to be imported, got %#v", importedService.Spec.Backend.Transport)
	}
	if bundle == nil {
		t.Fatal("expected certificate bundle to be imported")
	}
	if bundle.Spec.Issuer.Provider != certificatev1alpha1.CertificateIssuerLetsEncrypt {
		t.Fatalf("expected production provider, got %s", bundle.Spec.Issuer.Provider)
	}
	if len(bundle.Spec.AdditionalDomains) != 3 {
		t.Fatalf("expected deduped additional domains including base domain, got %#v", bundle.Spec.AdditionalDomains)
	}
	if tailnetConfig == nil || tailnetConfig.Spec.Nameserver.Address != "192.0.2.53" {
		t.Fatalf("expected imported tailnet dns config, got %#v", tailnetConfig)
	}
}

func TestRenderYAMLProducesMultiDocumentOutput(t *testing.T) {
	t.Parallel()

	result, err := Import(ImportInput{
		Namespace: "dns-operator-system",
		ZoneFile: []byte(`
$ORIGIN internal.example.test.
app 300 IN A 192.0.2.10
`),
	})
	if err != nil {
		t.Fatalf("import returned error: %v", err)
	}

	data, err := RenderYAML(result.Objects)
	if err != nil {
		t.Fatalf("render yaml: %v", err)
	}
	if !strings.Contains(string(data), "kind: DNSRecord") {
		t.Fatalf("expected DNSRecord yaml, got:\n%s", string(data))
	}
}

func TestParseZoneFileHandlesSingleLineSOAWithoutOriginDirective(t *testing.T) {
	t.Parallel()

	records, origin, warnings, collisions, err := parseZoneFile([]byte(`
internal.example.test. 3600 IN SOA ns.internal.example.test. admin.internal.example.test. 2026031601 7200 3600 1209600 3600
internal.example.test. 3600 IN NS ns.internal.example.test.
App IN A 192.0.2.10
api.portal IN A 192.0.2.20
Anton IN A 192.0.2.30
anton IN A 192.0.2.31
`))
	if err != nil {
		t.Fatalf("parseZoneFile returned error: %v", err)
	}
	if origin != "internal.example.test" {
		t.Fatalf("expected inferred origin, got %q", origin)
	}
	if len(warnings) != 0 {
		t.Fatalf("expected no warnings, got %#v", warnings)
	}
	if len(records) != 3 {
		t.Fatalf("expected 3 deduped records, got %#v", records)
	}
	if records[0].Hostname != "anton.internal.example.test" {
		t.Fatalf("expected canonicalized hostname ordering, got %#v", records)
	}
	if len(records[0].Values) != 2 {
		t.Fatalf("expected duplicate-case A records to aggregate values, got %#v", records[0])
	}
	if got := collisions["anton.internal.example.test"]; len(got) == 0 {
		t.Fatalf("expected collision note for mixed-case label, got %#v", collisions)
	}
}
