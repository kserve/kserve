/*
Copyright 2026 The KServe Authors.

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
	"fmt"
	"testing"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	"github.com/kserve/kserve/pkg/constants"
)

const (
	testIsvcName  = "test-isvc"
	testNamespace = "default"
)

func testScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	_ = v1beta1.AddToScheme(s)
	_ = corev1.AddToScheme(s)
	_ = netv1.AddToScheme(s)
	return s
}

func makeService(name string, headless bool) *corev1.Service {
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: testNamespace},
		Spec:       corev1.ServiceSpec{ClusterIP: "10.0.0.1"},
	}
	if headless {
		svc.Spec.ClusterIP = corev1.ClusterIPNone
	}
	return svc
}

func predictorHost() string {
	return fmt.Sprintf("%s-predictor.%s.svc.cluster.local", testIsvcName, testNamespace)
}

func transformerHost() string {
	return fmt.Sprintf("%s-transformer.%s.svc.cluster.local", testIsvcName, testNamespace)
}

// ---------------------------------------------------------------------------
// TestCreateAddress -- unit tests for createAddress
//
// The internal address must always use "http" regardless of urlScheme config.
// ---------------------------------------------------------------------------

func TestCreateAddress(t *testing.T) {
	tests := []struct {
		name       string
		headless   bool
		wantScheme string
		wantHost   string
	}{
		{
			name:       "ClusterIP service",
			headless:   false,
			wantScheme: "http",
			wantHost:   predictorHost(),
		},
		{
			name:       "headless service appends port 8080",
			headless:   true,
			wantScheme: "http",
			wantHost:   predictorHost() + ":" + constants.InferenceServiceDefaultHttpPort,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			g := NewGomegaWithT(t)
			s := testScheme()

			svcName := constants.PredictorServiceName(testIsvcName)
			svc := makeService(svcName, tc.headless)
			cl := fake.NewClientBuilder().WithScheme(s).WithObjects(svc).Build()

			isvc := &v1beta1.InferenceService{
				ObjectMeta: metav1.ObjectMeta{Name: testIsvcName, Namespace: testNamespace},
				Spec:       v1beta1.InferenceServiceSpec{Predictor: v1beta1.PredictorSpec{}},
			}

			addr, err := createAddress(t.Context(), cl, isvc)
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(addr).ToNot(BeNil())
			g.Expect(addr.URL.Scheme).To(Equal(tc.wantScheme), "address scheme must be http for in-cluster endpoint")
			g.Expect(addr.URL.Host).To(Equal(tc.wantHost))
		})
	}
}

func TestCreateAddress_Transformer(t *testing.T) {
	g := NewGomegaWithT(t)
	s := testScheme()

	svcName := constants.TransformerServiceName(testIsvcName)
	svc := makeService(svcName, false)
	cl := fake.NewClientBuilder().WithScheme(s).WithObjects(svc).Build()

	isvc := &v1beta1.InferenceService{
		ObjectMeta: metav1.ObjectMeta{Name: testIsvcName, Namespace: testNamespace},
		Spec: v1beta1.InferenceServiceSpec{
			Predictor:   v1beta1.PredictorSpec{},
			Transformer: &v1beta1.TransformerSpec{},
		},
	}

	addr, err := createAddress(t.Context(), cl, isvc)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(addr.URL.Host).To(Equal(transformerHost()))
	g.Expect(addr.URL.Scheme).To(Equal("http"))
}

// ---------------------------------------------------------------------------
// TestRawIngressReconciler_URLAndAddress
//
// Integration test exercising the full Reconcile path.
// Uses DisableIngressCreation: true to skip ingress object management and
// focus on Status.URL / Status.Address logic.
//
// The urlScheme=https rows must produce http addresses, proving the internal
// address is decoupled from the ingress URL scheme.
// ---------------------------------------------------------------------------

func TestRawIngressReconciler_URLAndAddress(t *testing.T) {
	pHost := predictorHost()

	tests := []struct {
		name           string
		headless       bool
		urlScheme      string
		wantAddrScheme string
		wantAddrHost   string
	}{
		{
			name:           "ClusterIP, urlScheme=http",
			headless:       false,
			urlScheme:      "http",
			wantAddrScheme: "http",
			wantAddrHost:   pHost,
		},
		{
			name:           "ClusterIP, urlScheme=https -- address must still be http",
			headless:       false,
			urlScheme:      "https",
			wantAddrScheme: "http",
			wantAddrHost:   pHost,
		},
		{
			name:           "headless, urlScheme=http",
			headless:       true,
			urlScheme:      "http",
			wantAddrScheme: "http",
			wantAddrHost:   pHost + ":" + constants.InferenceServiceDefaultHttpPort,
		},
		{
			name:           "headless, urlScheme=https -- address must still be http",
			headless:       true,
			urlScheme:      "https",
			wantAddrScheme: "http",
			wantAddrHost:   pHost + ":" + constants.InferenceServiceDefaultHttpPort,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			g := NewGomegaWithT(t)
			s := testScheme()

			svcName := constants.PredictorServiceName(testIsvcName)
			svc := makeService(svcName, tc.headless)
			cl := fake.NewClientBuilder().WithScheme(s).WithObjects(svc).Build()

			isvc := &v1beta1.InferenceService{
				ObjectMeta: metav1.ObjectMeta{Name: testIsvcName, Namespace: testNamespace},
				Spec:       v1beta1.InferenceServiceSpec{Predictor: v1beta1.PredictorSpec{}},
				Status:     v1beta1.InferenceServiceStatus{},
			}

			ingressConfig := &v1beta1.IngressConfig{
				DisableIngressCreation: true,
				UrlScheme:              tc.urlScheme,
				IngressDomain:          "example.com",
				DomainTemplate:         "{{.Name}}-{{.Namespace}}.{{.IngressDomain}}",
			}
			isvcConfig := &v1beta1.InferenceServicesConfig{}

			reconciler, err := NewRawIngressReconciler(cl, s, ingressConfig, isvcConfig)
			g.Expect(err).ToNot(HaveOccurred())

			result, err := reconciler.Reconcile(t.Context(), isvc)
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(result).To(Equal(ctrl.Result{}))

			g.Expect(isvc.Status.URL).ToNot(BeNil(), "Status.URL should be set")

			g.Expect(isvc.Status.Address).ToNot(BeNil(), "Status.Address should be set")
			g.Expect(isvc.Status.Address.URL.Scheme).To(Equal(tc.wantAddrScheme), "address scheme must be http for in-cluster endpoint")
			g.Expect(isvc.Status.Address.URL.Host).To(Equal(tc.wantAddrHost))

			cond := isvc.Status.GetCondition(v1beta1.IngressReady)
			g.Expect(cond).ToNot(BeNil())
			g.Expect(cond.Status).To(Equal(corev1.ConditionTrue))
		})
	}
}

func TestRawIngressReconciler_URLAndAddress_Transformer(t *testing.T) {
	g := NewGomegaWithT(t)
	s := testScheme()

	tHost := transformerHost()
	svcName := constants.TransformerServiceName(testIsvcName)
	svc := makeService(svcName, false)
	cl := fake.NewClientBuilder().WithScheme(s).WithObjects(svc).Build()

	isvc := &v1beta1.InferenceService{
		ObjectMeta: metav1.ObjectMeta{Name: testIsvcName, Namespace: testNamespace},
		Spec: v1beta1.InferenceServiceSpec{
			Predictor:   v1beta1.PredictorSpec{},
			Transformer: &v1beta1.TransformerSpec{},
		},
		Status: v1beta1.InferenceServiceStatus{},
	}

	ingressConfig := &v1beta1.IngressConfig{
		DisableIngressCreation: true,
		UrlScheme:              "http",
		IngressDomain:          "example.com",
		DomainTemplate:         "{{.Name}}-{{.Namespace}}.{{.IngressDomain}}",
	}
	isvcConfig := &v1beta1.InferenceServicesConfig{}

	reconciler, err := NewRawIngressReconciler(cl, s, ingressConfig, isvcConfig)
	g.Expect(err).ToNot(HaveOccurred())

	result, err := reconciler.Reconcile(t.Context(), isvc)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result).To(Equal(ctrl.Result{}))

	g.Expect(isvc.Status.URL).ToNot(BeNil())
	g.Expect(isvc.Status.Address.URL.Host).To(Equal(tHost), "address should use transformer service host")
	g.Expect(isvc.Status.Address.URL.Scheme).To(Equal("http"))
}

func TestRawIngressReconciler_ClusterLocal(t *testing.T) {
	g := NewGomegaWithT(t)
	s := testScheme()

	pHost := predictorHost()
	svcName := constants.PredictorServiceName(testIsvcName)
	svc := makeService(svcName, false)
	cl := fake.NewClientBuilder().WithScheme(s).WithObjects(svc).Build()

	isvc := &v1beta1.InferenceService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testIsvcName,
			Namespace: testNamespace,
			Labels: map[string]string{
				constants.NetworkVisibility: constants.ClusterLocalVisibility,
			},
		},
		Spec:   v1beta1.InferenceServiceSpec{Predictor: v1beta1.PredictorSpec{}},
		Status: v1beta1.InferenceServiceStatus{},
	}

	ingressConfig := &v1beta1.IngressConfig{
		UrlScheme:      "http",
		IngressDomain:  "example.com",
		DomainTemplate: "{{.Name}}-{{.Namespace}}.{{.IngressDomain}}",
	}
	isvcConfig := &v1beta1.InferenceServicesConfig{}

	reconciler, err := NewRawIngressReconciler(cl, s, ingressConfig, isvcConfig)
	g.Expect(err).ToNot(HaveOccurred())

	result, err := reconciler.Reconcile(t.Context(), isvc)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result).To(Equal(ctrl.Result{}))

	g.Expect(isvc.Status.URL).ToNot(BeNil(), "URL should still be set for cluster-local")
	g.Expect(isvc.Status.Address).ToNot(BeNil(), "Address should still be set for cluster-local")
	g.Expect(isvc.Status.Address.URL.Host).To(Equal(pHost))
	g.Expect(isvc.Status.Address.URL.Scheme).To(Equal("http"))

	cond := isvc.Status.GetCondition(v1beta1.IngressReady)
	g.Expect(cond).ToNot(BeNil())
	g.Expect(cond.Status).To(Equal(corev1.ConditionTrue))
}
