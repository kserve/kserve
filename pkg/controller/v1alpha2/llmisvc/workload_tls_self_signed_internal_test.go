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
	"context"
	"crypto/rand"
	"crypto/rsa"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
)

// blockingKeyGenerator simulates a slow RSA key generation: every call blocks
// until release is closed (or fails immediately once released).
type blockingKeyGenerator struct {
	release   chan struct{}
	started   chan struct{}
	completed atomic.Int32
}

func (b *blockingKeyGenerator) generate() (*rsa.PrivateKey, error) {
	b.started <- struct{}{}
	<-b.release
	b.completed.Add(1)
	// A small key is insecure but instant; the crypto parameters are not under test.
	return rsa.GenerateKey(rand.Reader, 2048)
}

// TestReconcileSelfSignedCertsSecretDoesNotBlockOnKeyGeneration verifies that
// dispatching a new certificate does not wait for the (potentially minutes-long)
// RSA key generation: the reconcile returns errCertGenerationInProgress right
// away, the secret appears once the generation completes, and subsequent
// reconciliations reuse the certificate on the fast path.
func TestReconcileSelfSignedCertsSecretDoesNotBlockOnKeyGeneration(t *testing.T) {
	blocking := &blockingKeyGenerator{
		release: make(chan struct{}),
		started: make(chan struct{}, 1),
	}

	sch := runtime.NewScheme()
	if err := v1alpha2.AddToScheme(sch); err != nil {
		t.Fatalf("failed to register scheme: %v", err)
	}
	if err := clientgoscheme.AddToScheme(sch); err != nil {
		t.Fatalf("failed to register client-go scheme: %v", err)
	}
	// The Update path of the generic Reconcile helper resolves object scope
	// through the RESTMapper; the fake client does not provide one.
	restMapper := meta.NewDefaultRESTMapper([]schema.GroupVersion{corev1.SchemeGroupVersion})
	restMapper.Add(corev1.SchemeGroupVersion.WithKind("Secret"), meta.RESTScopeNamespace)
	llmSvc := &v1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{Name: "slow-cert", Namespace: "default"},
	}
	c := fake.NewClientBuilder().WithScheme(sch).WithRESTMapper(restMapper).WithObjects(llmSvc).Build()
	r := &LLMISVCReconciler{
		Client:                 c,
		EventRecorder:          record.NewFakeRecorder(128),
		GenerateCertificateKey: blocking.generate,
	}
	schedulerConfig := &SchedulerConfig{ExpirationAnnotations: DefaultExpirationAnnotations}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- r.reconcileSelfSignedCertsSecret(ctx, llmSvc, schedulerConfig)
	}()

	// The reconcile must return while key generation is still blocked.
	select {
	case err := <-done:
		if !errors.Is(err, errCertGenerationInProgress) {
			t.Fatalf("expected errCertGenerationInProgress, got %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("reconcileSelfSignedCertsSecret blocked on certificate generation")
	}

	select {
	case <-blocking.started:
	case <-time.After(5 * time.Second):
		t.Fatal("background certificate generation was never started")
	}

	secretKey := types.NamespacedName{Namespace: "default", Name: "slow-cert-kserve-self-signed-certs"}
	secret := &corev1.Secret{}
	if err := c.Get(ctx, secretKey, secret); err == nil {
		t.Fatal("secret must not exist while generation is in flight")
	}

	// A second reconcile while generation is in flight also returns immediately.
	go func() {
		done <- r.reconcileSelfSignedCertsSecret(ctx, llmSvc, schedulerConfig)
	}()
	select {
	case err := <-done:
		if !errors.Is(err, errCertGenerationInProgress) {
			t.Fatalf("expected errCertGenerationInProgress while generation is in flight, got %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("reconcileSelfSignedCertsSecret blocked on in-flight generation")
	}

	// Let the generation finish; the secret must appear.
	close(blocking.release)
	deadline := time.Now().Add(10 * time.Second)
	for {
		if err := c.Get(ctx, secretKey, secret); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("certificate secret was not created after generation completed")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if len(secret.Data["tls.key"]) == 0 || len(secret.Data["tls.crt"]) == 0 {
		t.Fatal("certificate secret is missing key or certificate material")
	}

	// With a valid certificate in place, the fast path succeeds without any
	// further key generation.
	if err := r.reconcileSelfSignedCertsSecret(ctx, llmSvc, schedulerConfig); err != nil {
		t.Fatalf("fast-path reconcile failed: %v", err)
	}
	if got := blocking.completed.Load(); got != 1 {
		t.Fatalf("expected exactly one key generation, got %d", got)
	}
}
