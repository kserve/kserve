/*
Copyright 2023 The KServe Authors.

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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/constants"
	llmisvc "github.com/kserve/kserve/pkg/controller/v1alpha2/llmisvc"
	. "github.com/kserve/kserve/pkg/controller/v1alpha2/llmisvc/fixture"
)

func TestExpectedTokenizerDeployment_MissingServiceAccountFailsClosed(t *testing.T) {
	g := NewGomegaWithT(t)
	ctx := t.Context()

	scheme := runtime.NewScheme()
	g.Expect(appsv1.AddToScheme(scheme)).To(Succeed())
	g.Expect(corev1.AddToScheme(scheme)).To(Succeed())
	g.Expect(rbacv1.AddToScheme(scheme)).To(Succeed())
	g.Expect(v1alpha2.AddToScheme(scheme)).To(Succeed())

	// The main workload names a ServiceAccount that does not exist. The
	// tokenizer resolves that name, so the reconcile must fail instead of
	// building credentials from the namespace default ServiceAccount while
	// the pod still runs with the named identity.
	ns := "test-ns"
	llmSvc := LLMInferenceService(
		"test-llm",
		InNamespace[*v1alpha2.LLMInferenceService](ns),
		WithModelURI("pvc://test-pvc"),
		WithManagedRoute(),
		WithManagedGateway(),
		WithManagedScheduler(),
		WithManagedTokenizer(),
		WithTemplate(&corev1.PodSpec{
			ServiceAccountName: "missing-service-account",
		}),
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

	_, err := llmisvc.ExpectedTokenizerDeploymentForTest(ctx, reconciler, llmSvc)
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("failed to fetch tokenizer service account"))
}

func TestExpectedTokenizerDeployment_ExplicitServiceAccountFailsClosed(t *testing.T) {
	g := NewGomegaWithT(t)
	ctx := t.Context()

	scheme := runtime.NewScheme()
	g.Expect(appsv1.AddToScheme(scheme)).To(Succeed())
	g.Expect(corev1.AddToScheme(scheme)).To(Succeed())
	g.Expect(rbacv1.AddToScheme(scheme)).To(Succeed())
	g.Expect(v1alpha2.AddToScheme(scheme)).To(Succeed())

	// The tokenizer template itself names a ServiceAccount that does not
	// exist. An explicit name must be validated like an inherited one: the
	// reconcile fails instead of creating the Deployment with an identity
	// the API server would reject at pod scheduling.
	ns := "test-ns"
	llmSvc := LLMInferenceService(
		"test-llm",
		InNamespace[*v1alpha2.LLMInferenceService](ns),
		WithModelURI("pvc://test-pvc"),
		WithManagedRoute(),
		WithManagedGateway(),
		WithManagedScheduler(),
		WithManagedTokenizer(),
	)
	llmSvc.Spec.Router.Scheduler.Tokenizer.Template = &corev1.PodSpec{
		ServiceAccountName: "missing-tokenizer-service-account",
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(llmSvc).
		Build()

	reconciler := &llmisvc.LLMISVCReconciler{
		Client:        fakeClient,
		EventRecorder: record.NewFakeRecorder(10),
		Clientset:     k8sfake.NewSimpleClientset(),
	}

	_, err := llmisvc.ExpectedTokenizerDeploymentForTest(ctx, reconciler, llmSvc)
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("failed to fetch tokenizer service account test-ns/missing-tokenizer-service-account"))
}

func TestExpectedTokenizerDeployment_PreservesDeployedServiceAccount(t *testing.T) {
	g := NewGomegaWithT(t)
	ctx := t.Context()

	scheme := runtime.NewScheme()
	g.Expect(appsv1.AddToScheme(scheme)).To(Succeed())
	g.Expect(corev1.AddToScheme(scheme)).To(Succeed())
	g.Expect(rbacv1.AddToScheme(scheme)).To(Succeed())
	g.Expect(v1alpha2.AddToScheme(scheme)).To(Succeed())

	ns := "test-ns"
	llmName := "test-llm"

	// The main workload sets a ServiceAccount.
	llmSvc := LLMInferenceService(
		llmName,
		InNamespace[*v1alpha2.LLMInferenceService](ns),
		WithModelURI("pvc://test-pvc"),
		WithManagedRoute(),
		WithManagedGateway(),
		WithManagedScheduler(),
		WithManagedTokenizer(),
		WithTemplate(&corev1.PodSpec{
			ServiceAccountName: "main-sa",
		}),
	)

	// A tokenizer deployment is already running with the namespace default
	// ServiceAccount, as releases before the inheritance change deployed it.
	// The controller must keep the deployed identity instead of rewriting
	// the pod template and rolling the workload.
	deployedTokenizer := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      llmName + "-tokenizer",
			Namespace: ns,
		},
		Spec: appsv1.DeploymentSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{},
			},
		},
	}

	// The tokenizer builder loads the serving configuration ConfigMap.
	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(llmSvc, deployedTokenizer, InferenceServiceCfgMap(constants.KServeNamespace)).
		Build()

	reconciler := &llmisvc.LLMISVCReconciler{
		Client:        fakeClient,
		EventRecorder: record.NewFakeRecorder(10),
		Clientset:     k8sfake.NewSimpleClientset(),
	}

	d, err := llmisvc.ExpectedTokenizerDeploymentForTest(ctx, reconciler, llmSvc)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(d.Spec.Template.Spec.ServiceAccountName).To(Equal(""),
		"a deployed tokenizer keeps the ServiceAccount it runs with")
}

func TestExpectedTokenizerDeployment_ServiceAccountChangeFailsClosed(t *testing.T) {
	g := NewGomegaWithT(t)
	ctx := t.Context()

	scheme := runtime.NewScheme()
	g.Expect(appsv1.AddToScheme(scheme)).To(Succeed())
	g.Expect(corev1.AddToScheme(scheme)).To(Succeed())
	g.Expect(rbacv1.AddToScheme(scheme)).To(Succeed())
	g.Expect(v1alpha2.AddToScheme(scheme)).To(Succeed())

	ns := "test-ns"
	llmName := "test-llm"

	// The tokenizer template switches a running tokenizer to a
	// ServiceAccount that does not exist. The update must fail closed like
	// a creation does, instead of updating the Deployment to an identity
	// the API server would reject at pod scheduling.
	llmSvc := LLMInferenceService(
		llmName,
		InNamespace[*v1alpha2.LLMInferenceService](ns),
		WithModelURI("pvc://test-pvc"),
		WithManagedRoute(),
		WithManagedGateway(),
		WithManagedScheduler(),
		WithManagedTokenizer(),
	)
	llmSvc.Spec.Router.Scheduler.Tokenizer.Template = &corev1.PodSpec{
		ServiceAccountName: "missing-tokenizer-service-account",
	}

	deployedTokenizer := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      llmName + "-tokenizer",
			Namespace: ns,
		},
		Spec: appsv1.DeploymentSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					ServiceAccountName: "deployed-tokenizer-sa",
				},
			},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(llmSvc, deployedTokenizer).
		Build()

	reconciler := &llmisvc.LLMISVCReconciler{
		Client:        fakeClient,
		EventRecorder: record.NewFakeRecorder(10),
		Clientset:     k8sfake.NewSimpleClientset(),
	}

	_, err := llmisvc.ExpectedTokenizerDeploymentForTest(ctx, reconciler, llmSvc)
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("failed to fetch tokenizer service account test-ns/missing-tokenizer-service-account"))
}

func TestExpectedTokenizerDeployment_KeepsDeployedServiceAccountWithoutLookup(t *testing.T) {
	g := NewGomegaWithT(t)
	ctx := t.Context()

	scheme := runtime.NewScheme()
	g.Expect(appsv1.AddToScheme(scheme)).To(Succeed())
	g.Expect(corev1.AddToScheme(scheme)).To(Succeed())
	g.Expect(rbacv1.AddToScheme(scheme)).To(Succeed())
	g.Expect(v1alpha2.AddToScheme(scheme)).To(Succeed())

	ns := "test-ns"
	llmName := "test-llm"

	// The tokenizer template names the same ServiceAccount the deployed
	// tokenizer runs with. The identity an already-running tokenizer keeps
	// is not re-validated, so a ServiceAccount deleted in the meantime must
	// not block reconciling an otherwise unchanged deployment.
	llmSvc := LLMInferenceService(
		llmName,
		InNamespace[*v1alpha2.LLMInferenceService](ns),
		WithModelURI("pvc://test-pvc"),
		WithManagedRoute(),
		WithManagedGateway(),
		WithManagedScheduler(),
		WithManagedTokenizer(),
	)
	llmSvc.Spec.Router.Scheduler.Tokenizer.Template = &corev1.PodSpec{
		ServiceAccountName: "deleted-tokenizer-sa",
	}

	deployedTokenizer := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      llmName + "-tokenizer",
			Namespace: ns,
		},
		Spec: appsv1.DeploymentSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					ServiceAccountName: "deleted-tokenizer-sa",
				},
			},
		},
	}

	// No ServiceAccount object in the client: resolving the identity the
	// tokenizer already runs with must not look it up.
	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(llmSvc, deployedTokenizer, InferenceServiceCfgMap(constants.KServeNamespace)).
		Build()

	reconciler := &llmisvc.LLMISVCReconciler{
		Client:        fakeClient,
		EventRecorder: record.NewFakeRecorder(10),
		Clientset:     k8sfake.NewSimpleClientset(),
	}

	d, err := llmisvc.ExpectedTokenizerDeploymentForTest(ctx, reconciler, llmSvc)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(d.Spec.Template.Spec.ServiceAccountName).To(Equal("deleted-tokenizer-sa"),
		"the identity an already-running tokenizer keeps is not re-validated")
}
