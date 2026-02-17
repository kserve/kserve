/*
Copyright 2025 The KServe Authors.

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

package llmisvc_test

import (
	"testing"

	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/controller/v1alpha2/llmisvc"

	. "github.com/kserve/kserve/pkg/controller/v1alpha2/llmisvc/fixture"
)

func TestReconcileSingleNodeMainServiceAccount_UseExisting_SkipsManagedSA(t *testing.T) {
	g := NewGomegaWithT(t)
	ctx := t.Context()

	scheme := runtime.NewScheme()
	g.Expect(appsv1.AddToScheme(scheme)).To(Succeed())
	g.Expect(corev1.AddToScheme(scheme)).To(Succeed())
	g.Expect(rbacv1.AddToScheme(scheme)).To(Succeed())
	g.Expect(v1alpha2.AddToScheme(scheme)).To(Succeed())

	ns := "test-ns"
	llmName := "test-llm"
	existingSAName := "existing-sa"

	existingSA := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      existingSAName,
			Namespace: ns,
		},
	}

	// Build LLMInferenceService using fixtures
	// - serviceAccountName is explicitly set to use an existing SA
	llmSvc := LLMInferenceService(
		llmName,
		InNamespace[*v1alpha2.LLMInferenceService](ns),
		WithModelURI("pvc://test-pvc"),
		WithTemplate(&corev1.PodSpec{ServiceAccountName: existingSAName}),
	)

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(existingSA, llmSvc).
		Build()

	reconciler := &llmisvc.LLMISVCReconciler{
		Client:        fakeClient,
		EventRecorder: record.NewFakeRecorder(10),
		Clientset:     k8sfake.NewSimpleClientset(),
	}

	cfg := &llmisvc.Config{}

	// Act: invoke the reconcile logic through the test wrapper
	err := llmisvc.ReconcileSingleNodeMainServiceAccountForTest(ctx, reconciler, llmSvc, cfg)
	g.Expect(err).ToNot(HaveOccurred())

	// Assert: the existing ServiceAccount must remain untouched
	gotExisting := &corev1.ServiceAccount{}
	g.Expect(
		fakeClient.Get(ctx, types.NamespacedName{
			Name:      existingSAName,
			Namespace: ns,
		}, gotExisting),
	).To(Succeed())

	// Assert: reconciler must not create a managed ServiceAccount if an existing one is explicitly provided
	managedSAName := llmName + "-kserve"
	gotManaged := &corev1.ServiceAccount{}
	err = fakeClient.Get(ctx, types.NamespacedName{
		Name:      managedSAName,
		Namespace: ns,
	}, gotManaged)

	g.Expect(client.IgnoreNotFound(err)).To(Succeed())
	g.Expect(err).To(HaveOccurred(),
		"managed ServiceAccount must not be created when serviceAccountName is explicitly specified")
}

func TestReconcileSingleNodeMainServiceAccount_Default_DeletesManagedSA(t *testing.T) {
	g := NewGomegaWithT(t)
	ctx := t.Context()

	scheme := runtime.NewScheme()
	g.Expect(appsv1.AddToScheme(scheme)).To(Succeed())
	g.Expect(corev1.AddToScheme(scheme)).To(Succeed())
	g.Expect(rbacv1.AddToScheme(scheme)).To(Succeed())
	g.Expect(v1alpha2.AddToScheme(scheme)).To(Succeed())

	ns := "test-ns"
	llmName := "test-llm"

	// Build LLMInferenceService using fixtures
	// - serviceAccountName is not specified (default behavior)
	llmSvc := LLMInferenceService(
		llmName,
		InNamespace[*v1alpha2.LLMInferenceService](ns),
		WithModelURI("pvc://test-pvc"),
	)

	managedSAName := llmName + "-kserve"
	managedSA := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      managedSAName,
			Namespace: ns,
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(llmSvc, managedSA).
		Build()

	reconciler := &llmisvc.LLMISVCReconciler{
		Client:        fakeClient,
		EventRecorder: record.NewFakeRecorder(10),
		Clientset:     k8sfake.NewSimpleClientset(),
	}

	cfg := &llmisvc.Config{}

	// Act: invoke the reconcile logic through the test wrapper
	err := llmisvc.ReconcileSingleNodeMainServiceAccountForTest(ctx, reconciler, llmSvc, cfg)
	g.Expect(err).ToNot(HaveOccurred())

	// Assert: the managed ServiceAccount should be deleted in the default path
	gotManaged := &corev1.ServiceAccount{}
	err = fakeClient.Get(ctx, types.NamespacedName{
		Name:      managedSAName,
		Namespace: ns,
	}, gotManaged)

	g.Expect(client.IgnoreNotFound(err)).To(Succeed())
	g.Expect(err).To(HaveOccurred(), "managed ServiceAccount must be deleted when serviceAccountName is not specified")
}

func TestReconcileSingleNodeMainServiceAccount_RoutingEnabled_UseExisting_SkipsManagedSA(t *testing.T) {
	g := NewGomegaWithT(t)
	ctx := t.Context()

	scheme := runtime.NewScheme()
	g.Expect(appsv1.AddToScheme(scheme)).To(Succeed())
	g.Expect(corev1.AddToScheme(scheme)).To(Succeed())
	g.Expect(rbacv1.AddToScheme(scheme)).To(Succeed())
	g.Expect(v1alpha2.AddToScheme(scheme)).To(Succeed())

	ns := "test-ns"
	llmName := "test-llm"
	existingSAName := "existing-sa"

	existingSA := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      existingSAName,
			Namespace: ns,
		},
	}

	// Build LLMInferenceService using fixtures
	// - serviceAccountName is explicitly set to use an existing SA
	// - routing is enabled at the spec level via HTTPRouteSpec
	llmSvc := LLMInferenceService(
		llmName,
		InNamespace[*v1alpha2.LLMInferenceService](ns),
		WithModelURI("pvc://test-pvc"),
		WithTemplate(&corev1.PodSpec{ServiceAccountName: existingSAName}),
		WithHTTPRouteSpec(&gwapiv1.HTTPRouteSpec{}),
	)

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(existingSA, llmSvc).
		Build()

	reconciler := &llmisvc.LLMISVCReconciler{
		Client:        fakeClient,
		EventRecorder: record.NewFakeRecorder(10),
		Clientset:     k8sfake.NewSimpleClientset(),
	}

	cfg := &llmisvc.Config{}

	// Act: invoke the reconcile logic through the test wrapper
	err := llmisvc.ReconcileSingleNodeMainServiceAccountForTest(ctx, reconciler, llmSvc, cfg)
	g.Expect(err).ToNot(HaveOccurred())

	// Assert: existing SA still exists
	gotExisting := &corev1.ServiceAccount{}
	g.Expect(fakeClient.Get(ctx, types.NamespacedName{
		Name:      existingSAName,
		Namespace: ns,
	}, gotExisting)).To(Succeed())

	// Assert: managed SA is not created
	managedSAName := llmName + "-kserve"
	gotManaged := &corev1.ServiceAccount{}
	err = fakeClient.Get(ctx, types.NamespacedName{
		Name:      managedSAName,
		Namespace: ns,
	}, gotManaged)

	g.Expect(apierrors.IsNotFound(err)).To(BeTrue(),
		"managed ServiceAccount must not be created when an existing ServiceAccount is explicitly specified, even if routing is enabled")
}

func TestReconcileSingleNodeMainServiceAccount_RoutingEnabled_Default_DeletesManagedSA(t *testing.T) {
	g := NewGomegaWithT(t)
	ctx := t.Context()

	scheme := runtime.NewScheme()
	g.Expect(appsv1.AddToScheme(scheme)).To(Succeed())
	g.Expect(corev1.AddToScheme(scheme)).To(Succeed())
	g.Expect(rbacv1.AddToScheme(scheme)).To(Succeed())
	g.Expect(v1alpha2.AddToScheme(scheme)).To(Succeed())

	ns := "test-ns"
	llmName := "test-llm"

	// Build LLMInferenceService using fixtures
	// - serviceAccountName is not specified (default behavior)
	// - routing is enabled at the spec level via HTTPRouteSpec
	llmSvc := LLMInferenceService(
		llmName,
		InNamespace[*v1alpha2.LLMInferenceService](ns),
		WithModelURI("pvc://test-pvc"),
		WithHTTPRouteSpec(&gwapiv1.HTTPRouteSpec{}),
	)

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(llmSvc).
		Build()

	reconciler := &llmisvc.LLMISVCReconciler{
		Client:        fakeClient,
		EventRecorder: record.NewFakeRecorder(10),
		Clientset:     k8sfake.NewSimpleClientset(),
	}

	cfg := &llmisvc.Config{}

	// Act: invoke the reconcile logic through the test wrapper
	err := llmisvc.ReconcileSingleNodeMainServiceAccountForTest(ctx, reconciler, llmSvc, cfg)
	g.Expect(err).ToNot(HaveOccurred())

	// Assert: managed ServiceAccount is deleted
	managedSAName := llmName + "-kserve"
	gotManaged := &corev1.ServiceAccount{}
	err = fakeClient.Get(ctx, types.NamespacedName{
		Name:      managedSAName,
		Namespace: ns,
	}, gotManaged)

	g.Expect(apierrors.IsNotFound(err)).To(BeTrue(),
		"managed ServiceAccount must be deleted when routing is enabled but routing sidecar is not present")
}
