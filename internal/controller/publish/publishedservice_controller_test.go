package publish

import (
	"context"
	"strings"
	"testing"

	certificatev1alpha1 "github.com/jerkytreats/dns-operator/api/certificate/v1alpha1"
	"github.com/jerkytreats/dns-operator/api/common"
	publishv1alpha1 "github.com/jerkytreats/dns-operator/api/publish/v1alpha1"
	publishdomain "github.com/jerkytreats/dns-operator/internal/publish"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestPublishedServiceReconcileRendersRuntimeArtifacts(t *testing.T) {
	t.Parallel()

	scheme := newPublishScheme(t)
	service := &publishv1alpha1.PublishedService{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "app",
			Namespace:  "dns-operator-system",
			Generation: 1,
			Labels: map[string]string{
				"publish.jerkytreats.dev/certificate-bundle": "internal-shared",
			},
		},
		Spec: publishv1alpha1.PublishedServiceSpec{
			Hostname:    "app.internal.example.test",
			PublishMode: publishv1alpha1.PublishModeHTTPSProxy,
			Backend: &publishv1alpha1.PublishBackend{
				Address:  "192.0.2.10",
				Port:     8080,
				Protocol: "http",
			},
			TLS: &publishv1alpha1.PublishTLS{Mode: publishv1alpha1.TLSModeSharedSAN},
		},
		Status: publishv1alpha1.PublishedServiceStatus{
			Conditions: []metav1.Condition{{
				Type:               common.ConditionDNSReady,
				Status:             metav1.ConditionTrue,
				Reason:             "Rendered",
				Message:            "service hostname rendered into authoritative zone output",
				ObservedGeneration: 1,
			}},
		},
	}
	bundle := &certificatev1alpha1.CertificateBundle{
		ObjectMeta: metav1.ObjectMeta{Name: "internal-shared", Namespace: "dns-operator-system"},
		Spec: certificatev1alpha1.CertificateBundleSpec{
			PublishedServiceSelector: &common.ServiceSelector{
				MatchLabels: map[string]string{"publish.jerkytreats.dev/certificate-bundle": "internal-shared"},
			},
		},
		Status: certificatev1alpha1.CertificateBundleStatus{
			State:                "Ready",
			EffectiveDomains:     []string{"app.internal.example.test"},
			CertificateSecretRef: &common.ObjectReference{Name: "internal-example-test-shared-tls", Namespace: "dns-operator-system"},
		},
	}
	bundleSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "internal-example-test-shared-tls", Namespace: "dns-operator-system"},
		Data: map[string][]byte{
			corev1.TLSCertKey:       []byte("cert"),
			corev1.TLSPrivateKeyKey: []byte("key"),
		},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&publishv1alpha1.PublishedService{}).
		WithObjects(service, bundle, bundleSecret).
		Build()

	reconciler := &PublishedServiceReconciler{Client: client, Scheme: scheme}
	if _, err := reconciler.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: service.Name, Namespace: service.Namespace},
	}); err != nil {
		t.Fatalf("reconcile returned error: %v", err)
	}

	var configMap corev1.ConfigMap
	if err := client.Get(context.Background(), types.NamespacedName{Name: publishdomain.RuntimeConfigMapName, Namespace: service.Namespace}, &configMap); err != nil {
		t.Fatalf("get runtime configmap: %v", err)
	}
	content := configMap.Data[publishdomain.RuntimeConfigMapKey]
	if !strings.Contains(content, "app.internal.example.test") {
		t.Fatalf("expected published hostname in Caddy config:\n%s", content)
	}

	var runtimeSecret corev1.Secret
	if err := client.Get(context.Background(), types.NamespacedName{Name: publishdomain.RuntimeCertificatesSecretName, Namespace: service.Namespace}, &runtimeSecret); err != nil {
		t.Fatalf("get runtime certificate secret: %v", err)
	}
	if len(runtimeSecret.Data["internal-example-test-shared-tls.crt"]) == 0 {
		t.Fatalf("expected copied cert data in runtime secret, got %#v", runtimeSecret.Data)
	}

	var updated publishv1alpha1.PublishedService
	if err := client.Get(context.Background(), types.NamespacedName{Name: service.Name, Namespace: service.Namespace}, &updated); err != nil {
		t.Fatalf("get updated service: %v", err)
	}
	if updated.Status.RenderedConfigMapName != publishdomain.RuntimeConfigMapName {
		t.Fatalf("expected rendered configmap name %s, got %s", publishdomain.RuntimeConfigMapName, updated.Status.RenderedConfigMapName)
	}
	if !metaConditionTrue(updated.Status.Conditions, common.ConditionRuntimeReady) {
		t.Fatalf("expected RuntimeReady condition true, got %#v", updated.Status.Conditions)
	}
	if !metaConditionTrue(updated.Status.Conditions, common.ConditionReady) {
		t.Fatalf("expected Ready condition true, got %#v", updated.Status.Conditions)
	}
}

func TestPublishedServiceReconcileReportsMissingBundleCoverage(t *testing.T) {
	t.Parallel()

	scheme := newPublishScheme(t)
	service := &publishv1alpha1.PublishedService{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "app",
			Namespace:  "dns-operator-system",
			Generation: 1,
			Labels: map[string]string{
				"publish.jerkytreats.dev/certificate-bundle": "internal-shared",
			},
		},
		Spec: publishv1alpha1.PublishedServiceSpec{
			Hostname:    "app.internal.example.test",
			PublishMode: publishv1alpha1.PublishModeHTTPSProxy,
			Backend: &publishv1alpha1.PublishBackend{
				Address:  "192.0.2.10",
				Port:     8080,
				Protocol: "http",
			},
			TLS: &publishv1alpha1.PublishTLS{Mode: publishv1alpha1.TLSModeSharedSAN},
		},
		Status: publishv1alpha1.PublishedServiceStatus{
			Conditions: []metav1.Condition{{
				Type:               common.ConditionDNSReady,
				Status:             metav1.ConditionTrue,
				Reason:             "Rendered",
				Message:            "service hostname rendered into authoritative zone output",
				ObservedGeneration: 1,
			}},
		},
	}
	bundle := &certificatev1alpha1.CertificateBundle{
		ObjectMeta: metav1.ObjectMeta{Name: "internal-shared", Namespace: "dns-operator-system"},
		Spec: certificatev1alpha1.CertificateBundleSpec{
			PublishedServiceSelector: &common.ServiceSelector{
				MatchLabels: map[string]string{"publish.jerkytreats.dev/certificate-bundle": "internal-shared"},
			},
		},
		Status: certificatev1alpha1.CertificateBundleStatus{
			State:                "Ready",
			EffectiveDomains:     []string{"other.internal.example.test"},
			CertificateSecretRef: &common.ObjectReference{Name: "internal-example-test-shared-tls", Namespace: "dns-operator-system"},
		},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&publishv1alpha1.PublishedService{}).
		WithObjects(service, bundle).
		Build()

	reconciler := &PublishedServiceReconciler{Client: client, Scheme: scheme}
	if _, err := reconciler.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: service.Name, Namespace: service.Namespace},
	}); err != nil {
		t.Fatalf("reconcile returned error: %v", err)
	}

	var updated publishv1alpha1.PublishedService
	if err := client.Get(context.Background(), types.NamespacedName{Name: service.Name, Namespace: service.Namespace}, &updated); err != nil {
		t.Fatalf("get updated service: %v", err)
	}
	if metaConditionTrue(updated.Status.Conditions, common.ConditionReady) {
		t.Fatalf("expected Ready condition false, got %#v", updated.Status.Conditions)
	}
	if !metaConditionFalse(updated.Status.Conditions, common.ConditionReferencesResolved) {
		t.Fatalf("expected ReferencesResolved condition false, got %#v", updated.Status.Conditions)
	}
}

func newPublishScheme(t *testing.T) *runtime.Scheme {
	t.Helper()

	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("add corev1 scheme: %v", err)
	}
	if err := publishv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("add publish scheme: %v", err)
	}
	if err := certificatev1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("add certificate scheme: %v", err)
	}
	return scheme
}

func metaConditionTrue(conditions []metav1.Condition, conditionType string) bool {
	for _, condition := range conditions {
		if condition.Type == conditionType {
			return condition.Status == metav1.ConditionTrue
		}
	}
	return false
}

func metaConditionFalse(conditions []metav1.Condition, conditionType string) bool {
	for _, condition := range conditions {
		if condition.Type == conditionType {
			return condition.Status == metav1.ConditionFalse
		}
	}
	return false
}
