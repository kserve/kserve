/*
Copyright 2024 The KServe Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package ingress

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"

	"knative.dev/pkg/apis"

	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
)

func makeScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(s); err != nil {
		t.Fatalf("adding core scheme: %v", err)
	}
	if err := v1beta1.AddToScheme(s); err != nil {
		t.Fatalf("adding kserve v1beta1 scheme: %v", err)
	}
	return s
}

func readyISVC(name, ns string) *v1beta1.InferenceService {
	isvc := &v1beta1.InferenceService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
		},
	}

	isvc.Status.SetCondition(v1beta1.PredictorReady, &apis.Condition{
		Type:   v1beta1.PredictorReady,
		Status: corev1.ConditionTrue,
	})
	return isvc
}

func baseIngressConfig() *v1beta1.IngressConfig {
	class := "nginx"
	return &v1beta1.IngressConfig{
		KserveIngressGateway:       "kserve/kserve-ingress-gateway",
		IngressGateway:             "knative-serving/knative-ingress-gateway",
		KnativeLocalGatewayService: "knative-local-gateway.istio-system.svc.cluster.local",
		LocalGateway:               "knative-serving/knative-local-gateway",
		LocalGatewayServiceName:    "knative-local-gateway",
		IngressDomain:              "example.com",
		UrlScheme:                  "https",
		IngressClassName:           &class,
		DomainTemplate:             "{{ .Name }}.{{ .Namespace }}.{{ .IngressDomain }}",
	}
}

func minimalIsvcConfig() *v1beta1.InferenceServicesConfig {
	return &v1beta1.InferenceServicesConfig{}
}

func TestCreateRawIngress_Templates_Rendered(t *testing.T) {
	scheme := makeScheme(t)
	isvc := readyISVC("my-model", "ml")

	ingCfg := baseIngressConfig()
	ingCfg.AnnotationsTemplate = map[string]string{
		"external-dns.alpha.kubernetes.io/hostname": "{{ .Name }}.{{ .Namespace }}.{{ .IngressDomain }}",
	}
	ingCfg.TLSTemplate = []v1beta1.IngressTLSItem{
		{
			Hosts:      []string{"{{ .Name }}.{{ .Namespace }}.{{ .IngressDomain }}"},
			SecretName: "tls-{{ .Namespace }}",
		},
	}

	ing, err := createRawIngress(scheme, isvc, ingCfg, minimalIsvcConfig())
	if err != nil {
		t.Fatalf("createRawIngress returned error: %v", err)
	}
	if ing == nil {
		t.Fatalf("createRawIngress returned nil ingress")
	}

	gotAnno := ing.Annotations["external-dns.alpha.kubernetes.io/hostname"]
	wantAnno := "my-model.ml.example.com"
	if gotAnno != wantAnno {
		t.Fatalf("annotation mismatch: got %q want %q", gotAnno, wantAnno)
	}

	if len(ing.Spec.TLS) != 1 {
		t.Fatalf("expected 1 TLS entry, got %d", len(ing.Spec.TLS))
	}
	tls := ing.Spec.TLS[0]
	if len(tls.Hosts) != 1 || tls.Hosts[0] != "my-model.ml.example.com" {
		t.Fatalf("TLS hosts mismatch: got %#v", tls.Hosts)
	}
	if tls.SecretName != "tls-ml" {
		t.Fatalf("TLS secretName mismatch: got %q want %q", tls.SecretName, "tls-ml")
	}
}

func TestCreateRawIngress_Templates_Malformed_Skips(t *testing.T) {
	scheme := makeScheme(t)
	isvc := readyISVC("bad", "ns")

	ingCfg := baseIngressConfig()
	ingCfg.AnnotationsTemplate = map[string]string{
		"external-dns.alpha.kubernetes.io/hostname": "{{ .Name ",
	}
	ingCfg.TLSTemplate = []v1beta1.IngressTLSItem{
		{
			Hosts:      []string{"{{ .Name ", "{{ .Namespace }}.{{ .IngressDomain }}"},
			SecretName: "tls-{{ .Namespace }}",
		},
	}

	ing, err := createRawIngress(scheme, isvc, ingCfg, minimalIsvcConfig())
	if err != nil {
		t.Fatalf("createRawIngress returned error: %v", err)
	}
	if ing == nil {
		t.Fatalf("createRawIngress returned nil ingress")
	}

	if _, ok := ing.Annotations["external-dns.alpha.kubernetes.io/hostname"]; ok {
		t.Fatalf("expected malformed annotation to be skipped")
	}

	if len(ing.Spec.TLS) != 1 {
		t.Fatalf("expected 1 TLS entry, got %d", len(ing.Spec.TLS))
	}
	tls := ing.Spec.TLS[0]

	foundGood := false
	for _, h := range tls.Hosts {
		if h == "ns.example.com" {
			foundGood = true
		}
	}
	if !foundGood {
		t.Fatalf("expected at least one rendered TLS host, got %#v", tls.Hosts)
	}
	if tls.SecretName != "tls-ns" {
		t.Fatalf("TLS secretName mismatch: got %q want %q", tls.SecretName, "tls-ns")
	}
}

func TestCreateRawIngress_NoTemplates_NoTLS_NoExtraAnno(t *testing.T) {
	scheme := makeScheme(t)
	isvc := readyISVC("plain", "default")
	isvc.Annotations = map[string]string{"keep": "me"}

	ing, err := createRawIngress(scheme, isvc, baseIngressConfig(), minimalIsvcConfig())
	if err != nil {
		t.Fatalf("createRawIngress returned error: %v", err)
	}
	if ing == nil {
		t.Fatalf("createRawIngress returned nil ingress")
	}

	if len(ing.Spec.TLS) != 0 {
		t.Fatalf("expected no TLS entries, got %d", len(ing.Spec.TLS))
	}
	if len(ing.Annotations) != 1 || ing.Annotations["keep"] != "me" {
		t.Fatalf("unexpected annotations: %#v", ing.Annotations)
	}
}

func TestCreateRawIngress_DoesNotChangeClassName(t *testing.T) {
	scheme := makeScheme(t)
	isvc := readyISVC("classy", "ns")

	class := "nginx"
	ingCfg := baseIngressConfig()
	ingCfg.IngressClassName = &class
	ingCfg.AnnotationsTemplate = map[string]string{"foo/bar": "{{ .Name }}"}

	ing, err := createRawIngress(scheme, isvc, ingCfg, minimalIsvcConfig())
	if err != nil {
		t.Fatalf("createRawIngress returned error: %v", err)
	}
	if ing == nil {
		t.Fatalf("createRawIngress returned nil ingress")
	}
	if ing.Spec.IngressClassName == nil || *ing.Spec.IngressClassName != "nginx" {
		t.Fatalf("IngressClassName changed or missing; got %#v", ing.Spec.IngressClassName)
	}
	_ = ing.Spec.TLS
}
