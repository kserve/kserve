package ingress

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
)

// Before fix 4807: FAIL (current code validates FQDN, label > 63 triggers error)
// After fix 4807: PASS (should skip validation when disableIngressCreation=true)
func Test_4807_DisableIngressCreation_ShouldNotBlockOnDNSLabelTooLong(t *testing.T) {
	isvc := &v1beta1.InferenceService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "t-44e43bb2",
			Namespace: "this-is-a-long-project-name-with-40-characters",
		},
	}

	ingressConfig := &v1beta1.IngressConfig{
		IngressDomain:          "example.com",
		UrlScheme:              "http",
		DomainTemplate:         "{{.Name}}-predictor-{{.Namespace}}.{{.IngressDomain}}",
		DisableIngressCreation: true,
	}

	_, err := createRawURL(isvc, ingressConfig)
	if err != nil {
		t.Fatalf("BUG: disableIngressCreation=true should not block reconciliation, got err: %v", err)
	}
}

// When disableIngressCreation=false, maintain existing behavior (long label should still be rejected)
func Test_4807_EnableIngressCreation_ShouldStillValidateDNS(t *testing.T) {
	isvc := &v1beta1.InferenceService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "t-44e43bb2",
			Namespace: "this-is-a-long-project-name-with-40-characters",
		},
	}

	ingressConfig := &v1beta1.IngressConfig{
		IngressDomain:          "example.com",
		UrlScheme:              "http",
		DomainTemplate:         "{{.Name}}-predictor-{{.Namespace}}.{{.IngressDomain}}",
		DisableIngressCreation: false,
	}

	_, err := createRawURL(isvc, ingressConfig)
	if err == nil {
		t.Fatalf("expected error due to DNS label length > 63, got nil")
	}
}
