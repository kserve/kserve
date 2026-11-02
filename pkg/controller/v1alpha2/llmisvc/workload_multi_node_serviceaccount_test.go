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
	disaggregatedsetv1 "sigs.k8s.io/lws/api/disaggregatedset/v1"
	lwsapi "sigs.k8s.io/lws/api/leaderworkerset/v1"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/constants"
	"github.com/kserve/kserve/pkg/controller/v1alpha2/llmisvc"
	"github.com/kserve/kserve/pkg/credentials"

	. "github.com/kserve/kserve/pkg/controller/v1alpha2/llmisvc/fixture"
)

func TestReconcileMultiNodeMainServiceAccount_UseExisting_SkipsManagedSA(t *testing.T) {
	g := NewGomegaWithT(t)
	ctx := t.Context()

	scheme := runtime.NewScheme()
	g.Expect(corev1.AddToScheme(scheme)).To(Succeed())
	g.Expect(lwsapi.AddToScheme(scheme)).To(Succeed())
	g.Expect(disaggregatedsetv1.AddToScheme(scheme)).To(Succeed())
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
		WithTemplate(&corev1.PodSpec{
			ServiceAccountName: existingSAName,
		}),
		WithWorker(SimpleWorkerPodSpec()),
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
	err := llmisvc.ReconcileMultiNodeMainServiceAccountForTest(ctx, reconciler, llmSvc, cfg)
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
	managedSAName := llmName + "-kserve-mn"
	gotManaged := &corev1.ServiceAccount{}
	err = fakeClient.Get(ctx, types.NamespacedName{
		Name:      managedSAName,
		Namespace: ns,
	}, gotManaged)

	g.Expect(client.IgnoreNotFound(err)).To(Succeed())
	g.Expect(err).To(HaveOccurred(),
		"managed ServiceAccount must not be created when serviceAccountName is explicitly specified")
}

func TestReconcileMultiNodeMainServiceAccount_Default_CreatesManagedSA(t *testing.T) {
	g := NewGomegaWithT(t)
	ctx := t.Context()

	scheme := runtime.NewScheme()
	g.Expect(corev1.AddToScheme(scheme)).To(Succeed())
	g.Expect(lwsapi.AddToScheme(scheme)).To(Succeed())
	g.Expect(disaggregatedsetv1.AddToScheme(scheme)).To(Succeed())
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
		WithWorker(SimpleWorkerPodSpec()),
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
	err := llmisvc.ReconcileMultiNodeMainServiceAccountForTest(ctx, reconciler, llmSvc, cfg)
	g.Expect(err).ToNot(HaveOccurred())

	// Assert: managed ServiceAccount should be created
	managedSAName := llmName + "-kserve-mn"
	gotManaged := &corev1.ServiceAccount{}
	err = fakeClient.Get(ctx, types.NamespacedName{
		Name:      managedSAName,
		Namespace: ns,
	}, gotManaged)

	g.Expect(err).ToNot(HaveOccurred(), "managed ServiceAccount should be created when serviceAccountName is not specified")
}

func TestReconcileMultiNodeMainServiceAccount_Default_WhenWorkerNil_DeletesManagedSA(t *testing.T) {
	g := NewGomegaWithT(t)
	ctx := t.Context()

	scheme := runtime.NewScheme()
	g.Expect(corev1.AddToScheme(scheme)).To(Succeed())
	g.Expect(lwsapi.AddToScheme(scheme)).To(Succeed())
	g.Expect(disaggregatedsetv1.AddToScheme(scheme)).To(Succeed())
	g.Expect(rbacv1.AddToScheme(scheme)).To(Succeed())
	g.Expect(v1alpha2.AddToScheme(scheme)).To(Succeed())

	ns := "test-ns"
	llmName := "test-llm"

	// Build LLMInferenceService using fixtures
	// - serviceAccountName is not specified (default behavior)
	// - worker is intentionally left nil to trigger the delete path
	llmSvc := LLMInferenceService(
		llmName,
		InNamespace[*v1alpha2.LLMInferenceService](ns),
		WithModelURI("pvc://test-pvc"),
	)

	managedSAName := llmName + "-kserve-mn"
	managedSA := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      managedSAName,
			Namespace: ns,
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(llmSvc, v1alpha2.LLMInferenceServiceGVK),
			},
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
	err := llmisvc.ReconcileMultiNodeMainServiceAccountForTest(ctx, reconciler, llmSvc, cfg)
	g.Expect(err).ToNot(HaveOccurred())

	// Assert: the managed ServiceAccount should be deleted in the default path when Worker is nil
	gotManaged := &corev1.ServiceAccount{}
	err = fakeClient.Get(ctx, types.NamespacedName{
		Name:      managedSAName,
		Namespace: ns,
	}, gotManaged)

	g.Expect(client.IgnoreNotFound(err)).To(Succeed())
	g.Expect(err).To(HaveOccurred(), "managed ServiceAccount must be deleted when serviceAccountName is not specified and Worker is nil")
}

func TestReconcileMultiNodePrefillServiceAccount_UseExisting_SkipsManagedSA(t *testing.T) {
	g := NewGomegaWithT(t)
	ctx := t.Context()

	scheme := runtime.NewScheme()
	g.Expect(corev1.AddToScheme(scheme)).To(Succeed())
	g.Expect(lwsapi.AddToScheme(scheme)).To(Succeed())
	g.Expect(disaggregatedsetv1.AddToScheme(scheme)).To(Succeed())
	g.Expect(rbacv1.AddToScheme(scheme)).To(Succeed())
	g.Expect(v1alpha2.AddToScheme(scheme)).To(Succeed())

	ns := "test-ns"
	llmName := "test-llm-prefill"
	existingSAName := "existing-prefill-sa"

	existingSA := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      existingSAName,
			Namespace: ns,
		},
	}

	// Build LLMInferenceService using fixtures
	// - prefill serviceAccountName is explicitly set to use an existing SA
	llmSvc := LLMInferenceService(
		llmName,
		InNamespace[*v1alpha2.LLMInferenceService](ns),
		WithModelURI("pvc://test-pvc"),
		WithPrefill(&corev1.PodSpec{
			ServiceAccountName: existingSAName,
		}),
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

	// Act: invoke the reconcile logic through the test wrapper
	err := llmisvc.ReconcileMultiNodePrefillServiceAccountForTest(ctx, reconciler, llmSvc)
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
	managedSAName := llmName + "-kserve-mn-prefill"
	gotManaged := &corev1.ServiceAccount{}
	err = fakeClient.Get(ctx, types.NamespacedName{
		Name:      managedSAName,
		Namespace: ns,
	}, gotManaged)

	g.Expect(client.IgnoreNotFound(err)).To(Succeed())
	g.Expect(err).To(HaveOccurred(),
		"managed prefill ServiceAccount must not be created when serviceAccountName is explicitly specified")
}

func TestReconcileMultiNodePrefillServiceAccount_Default_CreatesManagedSA(t *testing.T) {
	g := NewGomegaWithT(t)
	ctx := t.Context()

	scheme := runtime.NewScheme()
	g.Expect(corev1.AddToScheme(scheme)).To(Succeed())
	g.Expect(lwsapi.AddToScheme(scheme)).To(Succeed())
	g.Expect(disaggregatedsetv1.AddToScheme(scheme)).To(Succeed())
	g.Expect(rbacv1.AddToScheme(scheme)).To(Succeed())
	g.Expect(v1alpha2.AddToScheme(scheme)).To(Succeed())

	ns := "test-ns"
	llmName := "test-llm-prefill"

	// Build LLMInferenceService using fixtures
	// - prefill serviceAccountName is not specified (default behavior)
	// - prefill worker is set so the reconciler should create the managed SA
	llmSvc := LLMInferenceService(
		llmName,
		InNamespace[*v1alpha2.LLMInferenceService](ns),
		WithModelURI("pvc://test-pvc"),
		WithPrefillWorker(SimpleWorkerPodSpec()),
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

	// Act: invoke the reconcile logic through the test wrapper
	err := llmisvc.ReconcileMultiNodePrefillServiceAccountForTest(ctx, reconciler, llmSvc)
	g.Expect(err).ToNot(HaveOccurred())

	// Assert: managed prefill ServiceAccount should be created
	managedSAName := llmName + "-kserve-mn-prefill"
	gotManaged := &corev1.ServiceAccount{}
	err = fakeClient.Get(ctx, types.NamespacedName{
		Name:      managedSAName,
		Namespace: ns,
	}, gotManaged)

	g.Expect(err).ToNot(HaveOccurred(),
		"managed prefill ServiceAccount should be created when serviceAccountName is not specified")
}

func TestReconcileMultiNodePrefillServiceAccount_PrefillRemoved_DeletesManagedSA(t *testing.T) {
	g := NewGomegaWithT(t)
	ctx := t.Context()

	scheme := runtime.NewScheme()
	g.Expect(corev1.AddToScheme(scheme)).To(Succeed())
	g.Expect(lwsapi.AddToScheme(scheme)).To(Succeed())
	g.Expect(disaggregatedsetv1.AddToScheme(scheme)).To(Succeed())
	g.Expect(rbacv1.AddToScheme(scheme)).To(Succeed())
	g.Expect(v1alpha2.AddToScheme(scheme)).To(Succeed())

	ns := "test-ns"
	llmName := "test-llm-prefill-cleanup"

	// Start with no ServiceAccount configured anywhere: the reconciler
	// creates the managed prefill ServiceAccount.
	llmSvc := LLMInferenceService(
		llmName,
		InNamespace[*v1alpha2.LLMInferenceService](ns),
		WithModelURI("pvc://test-pvc"),
		WithPrefillWorker(SimpleWorkerPodSpec()),
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

	g.Expect(llmisvc.ReconcileMultiNodePrefillServiceAccountForTest(ctx, reconciler, llmSvc)).To(Succeed())

	managedSAName := llmName + "-kserve-mn-prefill"
	gotManaged := &corev1.ServiceAccount{}
	g.Expect(fakeClient.Get(ctx, types.NamespacedName{
		Name:      managedSAName,
		Namespace: ns,
	}, gotManaged)).To(Succeed(), "managed prefill ServiceAccount should exist after the first reconcile")

	// Remove the prefill spec and set the main ServiceAccount on the worker.
	// The main-workload fallback must not mark the prefill workload as using
	// an existing ServiceAccount: without a prefill workload, the managed
	// ServiceAccount stays on the delete path.
	llmSvc.Spec.Prefill = nil
	llmSvc.Spec.Worker = &corev1.PodSpec{ServiceAccountName: llmName + "-worker-sa"}
	g.Expect(fakeClient.Update(ctx, llmSvc)).To(Succeed())

	g.Expect(llmisvc.ReconcileMultiNodePrefillServiceAccountForTest(ctx, reconciler, llmSvc)).To(Succeed())

	err := fakeClient.Get(ctx, types.NamespacedName{
		Name:      managedSAName,
		Namespace: ns,
	}, gotManaged)
	g.Expect(apierrors.IsNotFound(err)).To(BeTrue(),
		"managed prefill ServiceAccount must be deleted when the prefill spec is removed")
}

func TestReconcileMultiNodePrefillServiceAccount_PrefillRemovedWhileDeployed_KeepsManagedSA(t *testing.T) {
	g := NewGomegaWithT(t)
	ctx := t.Context()

	scheme := runtime.NewScheme()
	g.Expect(corev1.AddToScheme(scheme)).To(Succeed())
	g.Expect(lwsapi.AddToScheme(scheme)).To(Succeed())
	g.Expect(disaggregatedsetv1.AddToScheme(scheme)).To(Succeed())
	g.Expect(rbacv1.AddToScheme(scheme)).To(Succeed())
	g.Expect(v1alpha2.AddToScheme(scheme)).To(Succeed())

	ns := "test-ns"
	llmName := "test-llm-prefill-removed-deployed"
	managedSAName := llmName + "-kserve-mn-prefill"

	// Start with no ServiceAccount configured anywhere: the reconciler
	// creates the managed prefill ServiceAccount.
	llmSvc := LLMInferenceService(
		llmName,
		InNamespace[*v1alpha2.LLMInferenceService](ns),
		WithModelURI("pvc://test-pvc"),
		WithPrefillWorker(SimpleWorkerPodSpec()),
	)

	// The prefill LeaderWorkerSet deployed by an earlier reconcile, still
	// running with the managed ServiceAccount.
	deployedLWS := &lwsapi.LeaderWorkerSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      managedSAName,
			Namespace: ns,
		},
		Spec: lwsapi.LeaderWorkerSetSpec{
			LeaderWorkerTemplate: lwsapi.LeaderWorkerTemplate{
				WorkerTemplate: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{ServiceAccountName: managedSAName},
				},
			},
		},
	}

	// A pod of the prefill workload that is still shutting down.
	prefillPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      llmName + "-prefill-worker-0",
			Namespace: ns,
			Labels: map[string]string{
				constants.KubernetesAppNameLabelKey: llmName,
			},
		},
		Spec: corev1.PodSpec{
			ServiceAccountName: managedSAName,
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(llmSvc, deployedLWS, prefillPod).
		Build()

	reconciler := &llmisvc.LLMISVCReconciler{
		Client:        fakeClient,
		EventRecorder: record.NewFakeRecorder(10),
		Clientset:     k8sfake.NewSimpleClientset(),
	}

	g.Expect(llmisvc.ReconcileMultiNodePrefillServiceAccountForTest(ctx, reconciler, llmSvc)).To(Succeed())

	gotManaged := &corev1.ServiceAccount{}
	g.Expect(fakeClient.Get(ctx, types.NamespacedName{
		Name:      managedSAName,
		Namespace: ns,
	}, gotManaged)).To(Succeed(), "managed prefill ServiceAccount should exist after the first reconcile")

	// Remove the prefill spec. ServiceAccount reconciliation runs before
	// the prefill LeaderWorkerSet is deleted, and the workload and its pods
	// still reference the managed ServiceAccount: deleting it now breaks
	// the projected-token refresh of the pods that are still running.
	llmSvc.Spec.Prefill = nil
	llmSvc.Spec.Worker = &corev1.PodSpec{ServiceAccountName: llmName + "-worker-sa"}
	g.Expect(fakeClient.Update(ctx, llmSvc)).To(Succeed())

	g.Expect(llmisvc.ReconcileMultiNodePrefillServiceAccountForTest(ctx, reconciler, llmSvc)).To(Succeed())

	g.Expect(fakeClient.Get(ctx, types.NamespacedName{
		Name:      managedSAName,
		Namespace: ns,
	}, gotManaged)).To(Succeed(),
		"managed prefill ServiceAccount must be kept while the deployed prefill workload still references it")

	// The prefill LeaderWorkerSet is gone, but its pods may still be
	// terminating.
	g.Expect(fakeClient.Delete(ctx, deployedLWS)).To(Succeed())
	g.Expect(llmisvc.ReconcileMultiNodePrefillServiceAccountForTest(ctx, reconciler, llmSvc)).To(Succeed())

	g.Expect(fakeClient.Get(ctx, types.NamespacedName{
		Name:      managedSAName,
		Namespace: ns,
	}, gotManaged)).To(Succeed(),
		"managed prefill ServiceAccount must be kept while prefill pods still run with it")

	// Once the last prefill pod is gone, the managed ServiceAccount is
	// deleted.
	g.Expect(fakeClient.Delete(ctx, prefillPod)).To(Succeed())
	g.Expect(llmisvc.ReconcileMultiNodePrefillServiceAccountForTest(ctx, reconciler, llmSvc)).To(Succeed())

	err := fakeClient.Get(ctx, types.NamespacedName{
		Name:      managedSAName,
		Namespace: ns,
	}, gotManaged)
	g.Expect(apierrors.IsNotFound(err)).To(BeTrue(),
		"managed prefill ServiceAccount must be deleted once the prefill workload and its pods are gone")
}

func TestReconcileMultiNodePrefillServiceAccount_InheritsMain_DeletesManagedSA(t *testing.T) {
	g := NewGomegaWithT(t)
	ctx := t.Context()

	scheme := runtime.NewScheme()
	g.Expect(corev1.AddToScheme(scheme)).To(Succeed())
	g.Expect(lwsapi.AddToScheme(scheme)).To(Succeed())
	g.Expect(disaggregatedsetv1.AddToScheme(scheme)).To(Succeed())
	g.Expect(rbacv1.AddToScheme(scheme)).To(Succeed())
	g.Expect(v1alpha2.AddToScheme(scheme)).To(Succeed())

	ns := "test-ns"
	llmName := "test-llm-prefill-inherit"
	mainSAName := llmName + "-main-sa"

	mainSA := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      mainSAName,
			Namespace: ns,
		},
	}

	// The main workload sets a ServiceAccount and the prefill workload sets
	// none: the prefill workload inherits the main ServiceAccount.
	llmSvc := LLMInferenceService(
		llmName,
		InNamespace[*v1alpha2.LLMInferenceService](ns),
		WithModelURI("pvc://test-pvc"),
		WithTemplate(&corev1.PodSpec{ServiceAccountName: mainSAName}),
		WithPrefillWorker(SimpleWorkerPodSpec()),
	)

	// Simulate state from before the inheritance: the managed prefill
	// ServiceAccount already exists and is controlled by this service.
	managedSAName := llmName + "-kserve-mn-prefill"
	managedSA := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      managedSAName,
			Namespace: ns,
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(llmSvc, v1alpha2.LLMInferenceServiceGVK),
			},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(mainSA, llmSvc, managedSA).
		Build()

	reconciler := &llmisvc.LLMISVCReconciler{
		Client:        fakeClient,
		EventRecorder: record.NewFakeRecorder(10),
		Clientset:     k8sfake.NewSimpleClientset(),
	}

	g.Expect(llmisvc.ReconcileMultiNodePrefillServiceAccountForTest(ctx, reconciler, llmSvc)).To(Succeed())

	// The managed prefill ServiceAccount is no longer used and must be deleted.
	gotManaged := &corev1.ServiceAccount{}
	err := fakeClient.Get(ctx, types.NamespacedName{
		Name:      managedSAName,
		Namespace: ns,
	}, gotManaged)
	g.Expect(apierrors.IsNotFound(err)).To(BeTrue(),
		"managed prefill ServiceAccount must be deleted when the prefill workload inherits the main ServiceAccount")

	// The inherited ServiceAccount must remain untouched.
	gotMain := &corev1.ServiceAccount{}
	g.Expect(fakeClient.Get(ctx, types.NamespacedName{
		Name:      mainSAName,
		Namespace: ns,
	}, gotMain)).To(Succeed())
}

func TestReconcileMultiNodePrefillServiceAccount_PreservesDeployedManagedSA(t *testing.T) {
	g := NewGomegaWithT(t)
	ctx := t.Context()

	scheme := runtime.NewScheme()
	g.Expect(corev1.AddToScheme(scheme)).To(Succeed())
	g.Expect(lwsapi.AddToScheme(scheme)).To(Succeed())
	g.Expect(disaggregatedsetv1.AddToScheme(scheme)).To(Succeed())
	g.Expect(rbacv1.AddToScheme(scheme)).To(Succeed())
	g.Expect(v1alpha2.AddToScheme(scheme)).To(Succeed())

	ns := "test-ns"
	llmName := "test-llm-prefill-keep"
	managedSAName := llmName + "-kserve-mn-prefill"

	// A prefill LeaderWorkerSet runs with the managed ServiceAccount, as
	// releases before the inheritance change deployed it, and the main
	// workload sets its own ServiceAccount: the upgrade must keep the
	// deployed prefill identity instead of switching it.
	deployedLWS := &lwsapi.LeaderWorkerSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      managedSAName,
			Namespace: ns,
		},
		Spec: lwsapi.LeaderWorkerSetSpec{
			LeaderWorkerTemplate: lwsapi.LeaderWorkerTemplate{
				WorkerTemplate: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{ServiceAccountName: managedSAName},
				},
			},
		},
	}

	llmSvc := LLMInferenceService(
		llmName,
		InNamespace[*v1alpha2.LLMInferenceService](ns),
		WithModelURI("pvc://test-pvc"),
		WithTemplate(&corev1.PodSpec{
			ServiceAccountName: "main-sa",
		}),
		WithPrefill(&corev1.PodSpec{}),
		WithPrefillWorker(SimpleWorkerPodSpec()),
	)

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(llmSvc, deployedLWS).
		Build()

	reconciler := &llmisvc.LLMISVCReconciler{
		Client:        fakeClient,
		EventRecorder: record.NewFakeRecorder(10),
		Clientset:     k8sfake.NewSimpleClientset(),
	}

	g.Expect(llmisvc.ReconcileMultiNodePrefillServiceAccountForTest(ctx, reconciler, llmSvc)).To(Succeed())

	// The managed ServiceAccount stays under the controller's management.
	gotManaged := &corev1.ServiceAccount{}
	g.Expect(fakeClient.Get(ctx, types.NamespacedName{
		Name:      managedSAName,
		Namespace: ns,
	}, gotManaged)).To(Succeed(),
		"the managed prefill ServiceAccount must stay managed when the deployed workload runs with it")
}

func TestReconcileMultiNodePrefillServiceAccount_DeletesManagedSAAfterWorkloadSwitch(t *testing.T) {
	g := NewGomegaWithT(t)
	ctx := t.Context()

	scheme := runtime.NewScheme()
	g.Expect(corev1.AddToScheme(scheme)).To(Succeed())
	g.Expect(lwsapi.AddToScheme(scheme)).To(Succeed())
	g.Expect(disaggregatedsetv1.AddToScheme(scheme)).To(Succeed())
	g.Expect(rbacv1.AddToScheme(scheme)).To(Succeed())
	g.Expect(v1alpha2.AddToScheme(scheme)).To(Succeed())

	ns := "test-ns"
	llmName := "test-llm-prefill-defer"
	managedSAName := llmName + "-kserve-mn-prefill"
	newSAName := "new-prefill-sa"

	newSA := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      newSAName,
			Namespace: ns,
		},
	}

	// The prefill spec switches to an explicit ServiceAccount while the
	// deployed workload still runs with the managed one.
	llmSvc := LLMInferenceService(
		llmName,
		InNamespace[*v1alpha2.LLMInferenceService](ns),
		WithModelURI("pvc://test-pvc"),
		WithPrefill(&corev1.PodSpec{
			ServiceAccountName: newSAName,
		}),
		WithPrefillWorker(SimpleWorkerPodSpec()),
	)

	managedSA := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      managedSAName,
			Namespace: ns,
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(llmSvc, v1alpha2.LLMInferenceServiceGVK),
			},
		},
	}

	deployedLWS := &lwsapi.LeaderWorkerSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      managedSAName,
			Namespace: ns,
		},
		Spec: lwsapi.LeaderWorkerSetSpec{
			LeaderWorkerTemplate: lwsapi.LeaderWorkerTemplate{
				WorkerTemplate: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{ServiceAccountName: managedSAName},
				},
			},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(newSA, llmSvc, managedSA, deployedLWS).
		Build()

	reconciler := &llmisvc.LLMISVCReconciler{
		Client:        fakeClient,
		EventRecorder: record.NewFakeRecorder(10),
		Clientset:     k8sfake.NewSimpleClientset(),
	}

	// First reconcile: the deployed workload still references the managed
	// ServiceAccount, so its deletion is deferred.
	g.Expect(llmisvc.ReconcileMultiNodePrefillServiceAccountForTest(ctx, reconciler, llmSvc)).To(Succeed())
	gotManaged := &corev1.ServiceAccount{}
	g.Expect(fakeClient.Get(ctx, types.NamespacedName{
		Name:      managedSAName,
		Namespace: ns,
	}, gotManaged)).To(Succeed(),
		"the managed prefill ServiceAccount must survive while the deployed workload still references it")

	// The workload update lands: the deployed template no longer references
	// the managed ServiceAccount.
	deployedLWS.Spec.LeaderWorkerTemplate.WorkerTemplate.Spec.ServiceAccountName = newSAName
	g.Expect(fakeClient.Update(ctx, deployedLWS)).To(Succeed())

	g.Expect(llmisvc.ReconcileMultiNodePrefillServiceAccountForTest(ctx, reconciler, llmSvc)).To(Succeed())

	err := fakeClient.Get(ctx, types.NamespacedName{
		Name:      managedSAName,
		Namespace: ns,
	}, gotManaged)
	g.Expect(apierrors.IsNotFound(err)).To(BeTrue(),
		"the managed prefill ServiceAccount must be deleted once no deployed workload references it")
}

func TestReconcileMultiNodePrefillServiceAccount_Default_WhenWorkerNil_DeletesManagedSA(t *testing.T) {
	g := NewGomegaWithT(t)
	ctx := t.Context()

	scheme := runtime.NewScheme()
	g.Expect(corev1.AddToScheme(scheme)).To(Succeed())
	g.Expect(lwsapi.AddToScheme(scheme)).To(Succeed())
	g.Expect(disaggregatedsetv1.AddToScheme(scheme)).To(Succeed())
	g.Expect(rbacv1.AddToScheme(scheme)).To(Succeed())
	g.Expect(v1alpha2.AddToScheme(scheme)).To(Succeed())

	ns := "test-ns"
	llmName := "test-llm-prefill"

	// Build LLMInferenceService using fixtures
	// - prefill serviceAccountName is not specified (default behavior)
	// - prefill worker is intentionally left nil to trigger the delete path
	llmSvc := LLMInferenceService(
		llmName,
		InNamespace[*v1alpha2.LLMInferenceService](ns),
		WithModelURI("pvc://test-pvc"),
		WithPrefill(SimpleWorkerPodSpec()), // Intentionally not using WithPrefillWorker to leave Worker nil
	)

	managedSAName := llmName + "-kserve-mn-prefill"
	managedSA := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      managedSAName,
			Namespace: ns,
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(llmSvc, v1alpha2.LLMInferenceServiceGVK),
			},
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

	// Act: invoke the reconcile logic through the test wrapper
	err := llmisvc.ReconcileMultiNodePrefillServiceAccountForTest(ctx, reconciler, llmSvc)
	g.Expect(err).ToNot(HaveOccurred())

	// Assert: the managed prefill ServiceAccount should be deleted
	gotManaged := &corev1.ServiceAccount{}
	err = fakeClient.Get(ctx, types.NamespacedName{
		Name:      managedSAName,
		Namespace: ns,
	}, gotManaged)

	g.Expect(client.IgnoreNotFound(err)).To(Succeed())
	g.Expect(err).To(HaveOccurred(),
		"managed prefill ServiceAccount must be deleted when serviceAccountName is not specified and prefill worker is nil")
}

func TestExpectedMultiNodePrefillLeaderWorkerTemplate_CascadesMainServiceAccount(t *testing.T) {
	g := NewGomegaWithT(t)
	ctx := t.Context()

	scheme := runtime.NewScheme()
	g.Expect(corev1.AddToScheme(scheme)).To(Succeed())
	g.Expect(lwsapi.AddToScheme(scheme)).To(Succeed())
	g.Expect(disaggregatedsetv1.AddToScheme(scheme)).To(Succeed())
	g.Expect(rbacv1.AddToScheme(scheme)).To(Succeed())
	g.Expect(v1alpha2.AddToScheme(scheme)).To(Succeed())

	ns := "test-ns"
	llmName := "test-llm"
	mainSAName := llmName + "-main-sa"

	mainSA := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      mainSAName,
			Namespace: ns,
			Annotations: map[string]string{
				credentials.AwsIrsaAnnotationKey: "arn:aws:iam::123456789012:role/kserve-s3",
			},
		},
	}

	// Main template sets the ServiceAccount; the prefill spec sets none.
	llmSvc := LLMInferenceService(
		llmName,
		InNamespace[*v1alpha2.LLMInferenceService](ns),
		WithModelURI("pvc://test-pvc"),
		WithTemplate(&corev1.PodSpec{
			ServiceAccountName: mainSAName,
		}),
		WithPrefill(&corev1.PodSpec{}),
		WithPrefillWorker(SimpleWorkerPodSpec()),
	)

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(mainSA, llmSvc).
		Build()

	reconciler := &llmisvc.LLMISVCReconciler{
		Client:        fakeClient,
		EventRecorder: record.NewFakeRecorder(10),
		Clientset:     k8sfake.NewSimpleClientset(),
	}

	group, err := llmisvc.ExpectedMultiNodePrefillLeaderWorkerTemplateForTest(ctx, reconciler, llmSvc, &llmisvc.Config{}, nil, corev1.PodSpec{})
	g.Expect(err).ToNot(HaveOccurred())

	g.Expect(group.Template.LeaderTemplate.Spec.ServiceAccountName).To(Equal(mainSAName),
		"prefill leader pod must run with the main workload ServiceAccount when the prefill spec sets none")
	g.Expect(group.Template.WorkerTemplate.Spec.ServiceAccountName).To(Equal(mainSAName),
		"prefill worker pod must run with the main workload ServiceAccount when the prefill spec sets none")
}

func TestExpectedMultiNodePrefillLeaderWorkerTemplate_PreservesDeployedServiceAccount(t *testing.T) {
	g := NewGomegaWithT(t)
	ctx := t.Context()

	scheme := runtime.NewScheme()
	g.Expect(corev1.AddToScheme(scheme)).To(Succeed())
	g.Expect(lwsapi.AddToScheme(scheme)).To(Succeed())
	g.Expect(disaggregatedsetv1.AddToScheme(scheme)).To(Succeed())
	g.Expect(rbacv1.AddToScheme(scheme)).To(Succeed())
	g.Expect(v1alpha2.AddToScheme(scheme)).To(Succeed())

	ns := "test-ns"
	llmName := "test-llm"
	mainSAName := llmName + "-main-sa"
	managedSAName := llmName + "-kserve-mn-prefill"

	// A prefill LeaderWorkerSet is already running with the managed
	// ServiceAccount, as releases before the inheritance change deployed it.
	// The controller must keep the deployed identity instead of rewriting
	// the pod templates and rolling the workload.
	deployedLWS := &lwsapi.LeaderWorkerSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      managedSAName,
			Namespace: ns,
		},
		Spec: lwsapi.LeaderWorkerSetSpec{
			LeaderWorkerTemplate: lwsapi.LeaderWorkerTemplate{
				WorkerTemplate: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{ServiceAccountName: managedSAName},
				},
			},
		},
	}

	// Main template sets the ServiceAccount; the prefill spec sets none.
	llmSvc := LLMInferenceService(
		llmName,
		InNamespace[*v1alpha2.LLMInferenceService](ns),
		WithModelURI("pvc://test-pvc"),
		WithTemplate(&corev1.PodSpec{
			ServiceAccountName: mainSAName,
		}),
		WithPrefill(&corev1.PodSpec{}),
		WithPrefillWorker(SimpleWorkerPodSpec()),
	)

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(llmSvc, deployedLWS).
		Build()

	reconciler := &llmisvc.LLMISVCReconciler{
		Client:        fakeClient,
		EventRecorder: record.NewFakeRecorder(10),
		Clientset:     k8sfake.NewSimpleClientset(),
	}

	group, err := llmisvc.ExpectedMultiNodePrefillLeaderWorkerTemplateForTest(ctx, reconciler, llmSvc, &llmisvc.Config{}, nil, corev1.PodSpec{})
	g.Expect(err).ToNot(HaveOccurred())

	g.Expect(group.Template.LeaderTemplate.Spec.ServiceAccountName).To(Equal(managedSAName),
		"a deployed prefill workload keeps the ServiceAccount it runs with")
	g.Expect(group.Template.WorkerTemplate.Spec.ServiceAccountName).To(Equal(managedSAName),
		"a deployed prefill workload keeps the ServiceAccount it runs with")
}

func TestExpectedMultiNodePrefillLeaderWorkerTemplate_PreservesDisaggregatedSetRoleServiceAccount(t *testing.T) {
	g := NewGomegaWithT(t)
	ctx := t.Context()

	scheme := runtime.NewScheme()
	g.Expect(corev1.AddToScheme(scheme)).To(Succeed())
	g.Expect(lwsapi.AddToScheme(scheme)).To(Succeed())
	g.Expect(disaggregatedsetv1.AddToScheme(scheme)).To(Succeed())
	g.Expect(rbacv1.AddToScheme(scheme)).To(Succeed())
	g.Expect(v1alpha2.AddToScheme(scheme)).To(Succeed())

	ns := "test-ns"
	llmName := "test-llm"
	mainSAName := llmName + "-main-sa"
	roleSAName := "ds-prefill-sa"

	roleSA := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      roleSAName,
			Namespace: ns,
		},
	}

	// A prefill role of a DisaggregatedSet is already running with its own
	// ServiceAccount. Whichever backend deploys the prefill workload, the
	// controller must keep the deployed identity instead of rewriting the
	// pod templates and rolling the workload.
	deployedDS := &disaggregatedsetv1.DisaggregatedSet{
		ObjectMeta: metav1.ObjectMeta{
			// disaggregatedSetName of a short service name.
			Name:      llmName + "-kserve-pd",
			Namespace: ns,
		},
		Spec: disaggregatedsetv1.DisaggregatedSetSpec{Roles: []disaggregatedsetv1.DisaggregatedRoleSpec{
			{Name: constants.LLMDRolePrefill, LeaderWorkerSetTemplateSpec: lwsapi.LeaderWorkerSetTemplateSpec{
				Spec: lwsapi.LeaderWorkerSetSpec{
					LeaderWorkerTemplate: lwsapi.LeaderWorkerTemplate{
						WorkerTemplate: corev1.PodTemplateSpec{
							Spec: corev1.PodSpec{ServiceAccountName: roleSAName},
						},
					},
				},
			}},
		}},
	}

	// Main template sets a ServiceAccount that deliberately does not exist:
	// without reading the deployed DisaggregatedSet role, the resolver
	// would fall back to it and fail. The prefill spec sets none.
	llmSvc := LLMInferenceService(
		llmName,
		InNamespace[*v1alpha2.LLMInferenceService](ns),
		WithModelURI("pvc://test-pvc"),
		WithTemplate(&corev1.PodSpec{
			ServiceAccountName: mainSAName,
		}),
		WithPrefill(&corev1.PodSpec{}),
		WithPrefillWorker(SimpleWorkerPodSpec()),
	)

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(roleSA, llmSvc, deployedDS).
		Build()

	reconciler := &llmisvc.LLMISVCReconciler{
		Client:        fakeClient,
		EventRecorder: record.NewFakeRecorder(10),
		Clientset:     k8sfake.NewSimpleClientset(),
	}

	group, err := llmisvc.ExpectedMultiNodePrefillLeaderWorkerTemplateForTest(ctx, reconciler, llmSvc, &llmisvc.Config{}, nil, corev1.PodSpec{})
	g.Expect(err).ToNot(HaveOccurred())

	g.Expect(group.Template.LeaderTemplate.Spec.ServiceAccountName).To(Equal(roleSAName),
		"a deployed prefill role keeps the ServiceAccount it runs with")
	g.Expect(group.Template.WorkerTemplate.Spec.ServiceAccountName).To(Equal(roleSAName),
		"a deployed prefill role keeps the ServiceAccount it runs with")
}

func TestExpectedMultiNodePrefillLeaderWorkerTemplate_CascadesWorkerServiceAccount(t *testing.T) {
	g := NewGomegaWithT(t)
	ctx := t.Context()

	scheme := runtime.NewScheme()
	g.Expect(corev1.AddToScheme(scheme)).To(Succeed())
	g.Expect(lwsapi.AddToScheme(scheme)).To(Succeed())
	g.Expect(disaggregatedsetv1.AddToScheme(scheme)).To(Succeed())
	g.Expect(rbacv1.AddToScheme(scheme)).To(Succeed())
	g.Expect(v1alpha2.AddToScheme(scheme)).To(Succeed())

	ns := "test-ns"
	llmName := "test-llm"
	workerSAName := llmName + "-worker-sa"

	workerSA := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      workerSAName,
			Namespace: ns,
			Annotations: map[string]string{
				credentials.AwsIrsaAnnotationKey: "arn:aws:iam::123456789012:role/kserve-s3",
			},
		},
	}

	// Worker-only configuration: the main workload sets its ServiceAccount
	// through spec.worker; no template ServiceAccount and no prefill
	// ServiceAccount is set. The prefill fallback must use the same
	// resolution precedence as the main workload.
	llmSvc := LLMInferenceService(
		llmName,
		InNamespace[*v1alpha2.LLMInferenceService](ns),
		WithModelURI("pvc://test-pvc"),
		WithWorker(&corev1.PodSpec{
			ServiceAccountName: workerSAName,
		}),
		WithPrefill(&corev1.PodSpec{}),
		WithPrefillWorker(SimpleWorkerPodSpec()),
	)

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(workerSA, llmSvc).
		Build()

	reconciler := &llmisvc.LLMISVCReconciler{
		Client:        fakeClient,
		EventRecorder: record.NewFakeRecorder(10),
		Clientset:     k8sfake.NewSimpleClientset(),
	}

	group, err := llmisvc.ExpectedMultiNodePrefillLeaderWorkerTemplateForTest(ctx, reconciler, llmSvc, &llmisvc.Config{}, nil, corev1.PodSpec{})
	g.Expect(err).ToNot(HaveOccurred())

	g.Expect(group.Template.LeaderTemplate.Spec.ServiceAccountName).To(Equal(workerSAName),
		"prefill leader pod must run with the worker ServiceAccount in a worker-only configuration")
	g.Expect(group.Template.WorkerTemplate.Spec.ServiceAccountName).To(Equal(workerSAName),
		"prefill worker pod must run with the worker ServiceAccount in a worker-only configuration")
}

func TestReconcileMultiNodePrefillServiceAccount_RolloutInFlight_KeepsManagedSA(t *testing.T) {
	g := NewGomegaWithT(t)
	ctx := t.Context()

	scheme := runtime.NewScheme()
	g.Expect(corev1.AddToScheme(scheme)).To(Succeed())
	g.Expect(lwsapi.AddToScheme(scheme)).To(Succeed())
	g.Expect(disaggregatedsetv1.AddToScheme(scheme)).To(Succeed())
	g.Expect(rbacv1.AddToScheme(scheme)).To(Succeed())
	g.Expect(v1alpha2.AddToScheme(scheme)).To(Succeed())

	ns := "test-ns"
	llmName := "test-llm-prefill-rollout"
	existingSAName := llmName + "-prefill-sa"
	managedSAName := llmName + "-kserve-mn-prefill"

	existingSA := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      existingSAName,
			Namespace: ns,
		},
	}

	// The prefill workload is switched to an existing ServiceAccount after
	// the managed one was in use. The prefill spec keeps its worker so the
	// reconciler resolves the existing ServiceAccount for the multi-node
	// prefill workload.
	llmSvc := LLMInferenceService(
		llmName,
		InNamespace[*v1alpha2.LLMInferenceService](ns),
		WithModelURI("pvc://test-pvc"),
		WithPrefill(&corev1.PodSpec{
			ServiceAccountName: existingSAName,
		}),
		WithPrefillWorker(SimpleWorkerPodSpec()),
	)

	// The managed ServiceAccount an earlier reconcile created.
	managedSA := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      managedSAName,
			Namespace: ns,
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(llmSvc, v1alpha2.LLMInferenceServiceGVK),
			},
		},
	}

	// The deployed LeaderWorkerSet already runs with the existing
	// ServiceAccount (none is deployed in this test, so the deployed
	// workload references none), but a pod of the previous revision of the
	// in-flight rollout still runs with the managed ServiceAccount: pods
	// keep the ServiceAccount they were created with until they are
	// replaced.
	rolloutPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      llmName + "-prefill-worker-0",
			Namespace: ns,
			Labels: map[string]string{
				constants.KubernetesAppNameLabelKey: llmName,
			},
		},
		Spec: corev1.PodSpec{
			ServiceAccountName: managedSAName,
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(existingSA, managedSA, llmSvc, rolloutPod).
		Build()

	reconciler := &llmisvc.LLMISVCReconciler{
		Client:        fakeClient,
		EventRecorder: record.NewFakeRecorder(10),
		Clientset:     k8sfake.NewSimpleClientset(),
	}

	g.Expect(llmisvc.ReconcileMultiNodePrefillServiceAccountForTest(ctx, reconciler, llmSvc)).To(Succeed())

	gotManaged := &corev1.ServiceAccount{}
	g.Expect(fakeClient.Get(ctx, types.NamespacedName{
		Name:      managedSAName,
		Namespace: ns,
	}, gotManaged)).To(Succeed(),
		"managed prefill ServiceAccount must be kept while rollout pods still run with it")

	// Once the rollout finishes and the last pod using the managed
	// ServiceAccount is gone, the managed ServiceAccount is deleted.
	g.Expect(fakeClient.Delete(ctx, rolloutPod)).To(Succeed())
	g.Expect(llmisvc.ReconcileMultiNodePrefillServiceAccountForTest(ctx, reconciler, llmSvc)).To(Succeed())

	err := fakeClient.Get(ctx, types.NamespacedName{
		Name:      managedSAName,
		Namespace: ns,
	}, gotManaged)
	g.Expect(apierrors.IsNotFound(err)).To(BeTrue(),
		"managed prefill ServiceAccount must be deleted once no pod runs with it")
}

func TestReconcileMultiNodePrefillServiceAccount_DisaggregatedSetRoleInUse_KeepsManagedSA(t *testing.T) {
	g := NewGomegaWithT(t)
	ctx := t.Context()

	scheme := runtime.NewScheme()
	g.Expect(corev1.AddToScheme(scheme)).To(Succeed())
	g.Expect(lwsapi.AddToScheme(scheme)).To(Succeed())
	g.Expect(disaggregatedsetv1.AddToScheme(scheme)).To(Succeed())
	g.Expect(rbacv1.AddToScheme(scheme)).To(Succeed())
	g.Expect(v1alpha2.AddToScheme(scheme)).To(Succeed())

	ns := "test-ns"
	llmName := "test-llm-ds"
	managedSAName := llmName + "-kserve-mn-prefill"

	// The prefill spec is removed while a DisaggregatedSet prefill role
	// still runs with the managed ServiceAccount. The DisaggregatedSet is
	// deleted after this step, so its role must not lose the
	// ServiceAccount it still runs with.
	llmSvc := LLMInferenceService(
		llmName,
		InNamespace[*v1alpha2.LLMInferenceService](ns),
		WithModelURI("pvc://test-pvc"),
	)

	// The managed ServiceAccount an earlier reconcile created.
	managedSA := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      managedSAName,
			Namespace: ns,
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(llmSvc, v1alpha2.LLMInferenceServiceGVK),
			},
		},
	}

	// The deployed DisaggregatedSet prefill role still runs with the
	// managed ServiceAccount.
	deployedDS := &disaggregatedsetv1.DisaggregatedSet{
		ObjectMeta: metav1.ObjectMeta{
			// disaggregatedSetName of a short service name.
			Name:      llmName + "-kserve-pd",
			Namespace: ns,
		},
		Spec: disaggregatedsetv1.DisaggregatedSetSpec{Roles: []disaggregatedsetv1.DisaggregatedRoleSpec{
			{Name: constants.LLMDRolePrefill, LeaderWorkerSetTemplateSpec: lwsapi.LeaderWorkerSetTemplateSpec{
				Spec: lwsapi.LeaderWorkerSetSpec{
					LeaderWorkerTemplate: lwsapi.LeaderWorkerTemplate{
						WorkerTemplate: corev1.PodTemplateSpec{
							Spec: corev1.PodSpec{ServiceAccountName: managedSAName},
						},
					},
				},
			}},
		}},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(llmSvc, managedSA, deployedDS).
		Build()

	reconciler := &llmisvc.LLMISVCReconciler{
		Client:        fakeClient,
		EventRecorder: record.NewFakeRecorder(10),
		Clientset:     k8sfake.NewSimpleClientset(),
	}

	g.Expect(llmisvc.ReconcileMultiNodePrefillServiceAccountForTest(ctx, reconciler, llmSvc)).To(Succeed())

	gotManaged := &corev1.ServiceAccount{}
	g.Expect(fakeClient.Get(ctx, types.NamespacedName{
		Name:      managedSAName,
		Namespace: ns,
	}, gotManaged)).To(Succeed(),
		"managed prefill ServiceAccount must be kept while the deployed DisaggregatedSet role still runs with it")
}

func TestReconcileMultiNodePrefillServiceAccount_TerminatedPod_DoesNotBlockDeletion(t *testing.T) {
	g := NewGomegaWithT(t)
	ctx := t.Context()

	scheme := runtime.NewScheme()
	g.Expect(corev1.AddToScheme(scheme)).To(Succeed())
	g.Expect(lwsapi.AddToScheme(scheme)).To(Succeed())
	g.Expect(disaggregatedsetv1.AddToScheme(scheme)).To(Succeed())
	g.Expect(rbacv1.AddToScheme(scheme)).To(Succeed())
	g.Expect(v1alpha2.AddToScheme(scheme)).To(Succeed())

	ns := "test-ns"
	llmName := "test-llm-prefill-terminated"
	existingSAName := llmName + "-prefill-sa"
	managedSAName := llmName + "-kserve-mn-prefill"

	existingSA := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      existingSAName,
			Namespace: ns,
		},
	}

	// The prefill workload is switched to an existing ServiceAccount after
	// the managed one was in use. The prefill spec keeps its worker so the
	// reconciler resolves the existing ServiceAccount for the multi-node
	// prefill workload.
	llmSvc := LLMInferenceService(
		llmName,
		InNamespace[*v1alpha2.LLMInferenceService](ns),
		WithModelURI("pvc://test-pvc"),
		WithPrefill(&corev1.PodSpec{
			ServiceAccountName: existingSAName,
		}),
		WithPrefillWorker(SimpleWorkerPodSpec()),
	)

	// The managed ServiceAccount an earlier reconcile created.
	managedSA := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      managedSAName,
			Namespace: ns,
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(llmSvc, v1alpha2.LLMInferenceServiceGVK),
			},
		},
	}

	// A terminated pod no longer refreshes its projected tokens, so it does
	// not keep the managed ServiceAccount in use.
	terminatedPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      llmName + "-prefill-worker-0",
			Namespace: ns,
			Labels: map[string]string{
				constants.KubernetesAppNameLabelKey: llmName,
			},
		},
		Spec: corev1.PodSpec{
			ServiceAccountName: managedSAName,
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodSucceeded,
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(existingSA, managedSA, llmSvc, terminatedPod).
		Build()

	reconciler := &llmisvc.LLMISVCReconciler{
		Client:        fakeClient,
		EventRecorder: record.NewFakeRecorder(10),
		Clientset:     k8sfake.NewSimpleClientset(),
	}

	g.Expect(llmisvc.ReconcileMultiNodePrefillServiceAccountForTest(ctx, reconciler, llmSvc)).To(Succeed())

	gotManaged := &corev1.ServiceAccount{}
	err := fakeClient.Get(ctx, types.NamespacedName{
		Name:      managedSAName,
		Namespace: ns,
	}, gotManaged)
	g.Expect(apierrors.IsNotFound(err)).To(BeTrue(),
		"managed prefill ServiceAccount must be deleted when only terminated pods ran with it")
}
