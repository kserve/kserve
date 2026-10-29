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

package llmisvc

import (
	"bytes"
	"crypto/x509"
	"encoding/pem"
	"slices"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/record"
	"knative.dev/pkg/kmeta"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/constants"
)

// A pod that has no IP yet and a headless Service report values that can never be
// certificate SANs. They must not force a new certificate on every reconcile, while
// an IP assigned later still must.
func TestReconcileSelfSignedCertsSecretIgnoresUnassignedIPs(t *testing.T) {
	ctx := t.Context()

	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add corev1 to scheme: %v", err)
	}
	if err := v1alpha2.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add v1alpha2 to scheme: %v", err)
	}
	mapper := meta.NewDefaultRESTMapper([]schema.GroupVersion{corev1.SchemeGroupVersion})
	mapper.Add(corev1.SchemeGroupVersion.WithKind("Secret"), meta.RESTScopeNamespace)

	llmSvc := &v1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{Name: "llm", Namespace: "test-ns", UID: "llm-uid"},
	}
	labels := map[string]string{
		constants.KubernetesAppNameLabelKey: llmSvc.Name,
		constants.KubernetesPartOfLabelKey:  constants.LLMInferenceServicePartOfValue,
	}
	runningPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "llm-running", Namespace: llmSvc.Namespace, Labels: labels},
		Status:     corev1.PodStatus{PodIP: "10.244.0.5", PodIPs: []corev1.PodIP{{IP: "10.244.0.5"}}},
	}
	pendingPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "llm-pending", Namespace: llmSvc.Namespace, Labels: labels},
		Status:     corev1.PodStatus{Phase: corev1.PodPending},
	}
	workloadSvc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "llm-kserve-workload-svc", Namespace: llmSvc.Namespace, Labels: labels},
		Spec:       corev1.ServiceSpec{ClusterIP: "10.96.0.10", ClusterIPs: []string{"10.96.0.10"}},
	}
	headlessSvc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "llm-headless", Namespace: llmSvc.Namespace, Labels: labels},
		Spec:       corev1.ServiceSpec{ClusterIP: corev1.ClusterIPNone, ClusterIPs: []string{corev1.ClusterIPNone}},
	}

	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithRESTMapper(mapper).
		WithObjects(llmSvc, runningPod, pendingPod, workloadSvc, headlessSvc).
		Build()
	r := &LLMISVCReconciler{Client: c, EventRecorder: record.NewFakeRecorder(10)}
	schedulerConfig := &SchedulerConfig{ExpirationAnnotations: DefaultExpirationAnnotations}

	reconcileCert := func() []byte {
		t.Helper()
		if err := r.reconcileSelfSignedCertsSecret(ctx, llmSvc, schedulerConfig); err != nil {
			t.Fatalf("reconcileSelfSignedCertsSecret() error = %v", err)
		}
		secret := &corev1.Secret{}
		key := client.ObjectKey{Namespace: llmSvc.Namespace, Name: kmeta.ChildName(llmSvc.Name, "-kserve-self-signed-certs")}
		if err := c.Get(ctx, key, secret); err != nil {
			t.Fatalf("failed to get certificate secret: %v", err)
		}
		return secret.Data["tls.crt"]
	}

	issued := reconcileCert()
	if !bytes.Equal(issued, reconcileCert()) {
		t.Fatal("certificate was reissued although no pod or service IP changed")
	}

	pod := &corev1.Pod{}
	if err := c.Get(ctx, client.ObjectKeyFromObject(pendingPod), pod); err != nil {
		t.Fatalf("failed to get pending pod: %v", err)
	}
	pod.Status.PodIP = "10.244.0.6"
	pod.Status.PodIPs = []corev1.PodIP{{IP: "10.244.0.6"}}
	if err := c.Status().Update(ctx, pod); err != nil {
		t.Fatalf("failed to assign pod IP: %v", err)
	}

	reissued := reconcileCert()
	if bytes.Equal(issued, reissued) {
		t.Fatal("certificate was not reissued after a pod received an IP")
	}
	if got := certificateIPs(t, reissued); !slices.Contains(got, "10.244.0.6") {
		t.Fatalf("reissued certificate IPs = %v, want them to include 10.244.0.6", got)
	}
	if !bytes.Equal(reissued, reconcileCert()) {
		t.Fatal("certificate was reissued although no pod or service IP changed")
	}
}

func certificateIPs(t *testing.T, certPEM []byte) []string {
	t.Helper()
	block, _ := pem.Decode(certPEM)
	if block == nil {
		t.Fatal("failed to decode certificate PEM")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatalf("failed to parse certificate: %v", err)
	}
	ips := make([]string, 0, len(cert.IPAddresses))
	for _, ip := range cert.IPAddresses {
		ips = append(ips, ip.String())
	}
	return ips
}
