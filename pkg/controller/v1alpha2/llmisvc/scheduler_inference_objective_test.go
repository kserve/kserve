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
	apmeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/ptr"
	"knative.dev/pkg/kmeta"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	igwapiv1alpha2 "sigs.k8s.io/gateway-api-inference-extension/apix/v1alpha2"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/controller/v1alpha2/llmisvc"
)

// ioScheme returns a scheme with v1alpha2 and igwapiv1alpha2 registered.
func ioScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := v1alpha2.AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme v1alpha2: %v", err)
	}
	if err := igwapiv1alpha2.Install(scheme); err != nil {
		t.Fatalf("Install igwapiv1alpha2: %v", err)
	}
	return scheme
}

// ioRESTMapper returns a REST mapper that knows InferenceObjective is namespaced.
// igwapiv1alpha2.GroupVersion is metav1.GroupVersion, not schema.GroupVersion, so we use
// igwapiv1alpha2.SchemeGroupVersion (schema.GroupVersion) for the REST mapper.
func ioRESTMapper() apmeta.RESTMapper {
	gv := igwapiv1alpha2.SchemeGroupVersion
	rm := apmeta.NewDefaultRESTMapper([]schema.GroupVersion{gv})
	rm.Add(gv.WithKind("InferenceObjective"), apmeta.RESTScopeNamespace)
	rm.Add(gv.WithKind("InferencePool"), apmeta.RESTScopeNamespace)
	rm.Add(v1alpha2.SchemeGroupVersion.WithKind("LLMInferenceService"), apmeta.RESTScopeNamespace)
	return rm
}

func ioReconciler(t *testing.T, llmSvc *v1alpha2.LLMInferenceService) *llmisvc.LLMISVCReconciler {
	t.Helper()
	scheme := ioScheme(t)
	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithRESTMapper(ioRESTMapper()).
		WithObjects(llmSvc).
		Build()
	return &llmisvc.LLMISVCReconciler{
		Client:        fakeClient,
		EventRecorder: record.NewFakeRecorder(20),
		Clientset:     k8sfake.NewSimpleClientset(),
	}
}

func ioLLMSvc(lora *v1alpha2.LoRASpec) *v1alpha2.LLMInferenceService {
	const (
		name = "my-llm"
		ns   = "test-ns"
	)
	return &v1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
			UID:       types.UID("uid-" + name),
		},
		Spec: v1alpha2.LLMInferenceServiceSpec{
			Model: v1alpha2.LLMModelSpec{
				LoRA: lora,
			},
		},
	}
}

func TestReconcileSchedulerInferenceObjective_NoAdapters(t *testing.T) {
	g := NewGomegaWithT(t)
	ctx := t.Context()

	llmSvc := ioLLMSvc(nil)
	r := ioReconciler(t, llmSvc)

	err := llmisvc.ReconcileSchedulerInferenceObjectiveForTest(ctx, r, llmSvc)
	g.Expect(err).ToNot(HaveOccurred())

	list := &igwapiv1alpha2.InferenceObjectiveList{}
	g.Expect(r.Client.List(ctx, list)).To(Succeed())
	g.Expect(list.Items).To(BeEmpty())
}

func TestReconcileSchedulerInferenceObjective_TwoEnabledAdapters(t *testing.T) {
	g := NewGomegaWithT(t)
	ctx := t.Context()

	llmSvc := ioLLMSvc(&v1alpha2.LoRASpec{
		Adapters: []v1alpha2.LoRAAdapterSpec{
			{Name: "foo"},
			{Name: "bar"},
		},
	})
	r := ioReconciler(t, llmSvc)

	err := llmisvc.ReconcileSchedulerInferenceObjectiveForTest(ctx, r, llmSvc)
	g.Expect(err).ToNot(HaveOccurred())

	list := &igwapiv1alpha2.InferenceObjectiveList{}
	g.Expect(r.Client.List(ctx, list)).To(Succeed())
	g.Expect(list.Items).To(HaveLen(2))

	poolName := kmeta.ChildName("my-llm", "-inference-pool")
	names := map[string]bool{}
	for i := range list.Items {
		item := &list.Items[i]
		names[item.Name] = true
		g.Expect(metav1.IsControlledBy(item, llmSvc)).To(BeTrue(), "IO %s should be owned by llmSvc", item.Name)
		g.Expect(item.Spec.PoolRef.Name).To(Equal(igwapiv1alpha2.ObjectName(poolName)))
		g.Expect(item.Spec.Priority).ToNot(BeNil())
		g.Expect(*item.Spec.Priority).To(Equal(0))
	}
	g.Expect(names).To(HaveKey(kmeta.ChildName("my-llm", "-adapter-foo")))
	g.Expect(names).To(HaveKey(kmeta.ChildName("my-llm", "-adapter-bar")))
}

func TestReconcileSchedulerInferenceObjective_DisabledAdapterSkipped(t *testing.T) {
	g := NewGomegaWithT(t)
	ctx := t.Context()

	llmSvc := ioLLMSvc(&v1alpha2.LoRASpec{
		Adapters: []v1alpha2.LoRAAdapterSpec{
			{Name: "foo"},
			{Name: "bar", Disabled: true},
		},
	})
	r := ioReconciler(t, llmSvc)

	err := llmisvc.ReconcileSchedulerInferenceObjectiveForTest(ctx, r, llmSvc)
	g.Expect(err).ToNot(HaveOccurred())

	list := &igwapiv1alpha2.InferenceObjectiveList{}
	g.Expect(r.Client.List(ctx, list)).To(Succeed())
	g.Expect(list.Items).To(HaveLen(1))
	g.Expect(list.Items[0].Name).To(Equal(kmeta.ChildName("my-llm", "-adapter-foo")))
}

func TestReconcileSchedulerInferenceObjective_AdapterRemovedIsDeleted(t *testing.T) {
	g := NewGomegaWithT(t)
	ctx := t.Context()

	llmSvc := ioLLMSvc(&v1alpha2.LoRASpec{
		Adapters: []v1alpha2.LoRAAdapterSpec{
			{Name: "foo"},
			{Name: "bar"},
		},
	})
	r := ioReconciler(t, llmSvc)

	// First reconcile: create IOs for both foo and bar.
	err := llmisvc.ReconcileSchedulerInferenceObjectiveForTest(ctx, r, llmSvc)
	g.Expect(err).ToNot(HaveOccurred())

	list := &igwapiv1alpha2.InferenceObjectiveList{}
	g.Expect(r.Client.List(ctx, list)).To(Succeed())
	g.Expect(list.Items).To(HaveLen(2))

	// Second reconcile: bar removed from spec.
	llmSvc.Spec.Model.LoRA.Adapters = []v1alpha2.LoRAAdapterSpec{
		{Name: "foo"},
	}
	err = llmisvc.ReconcileSchedulerInferenceObjectiveForTest(ctx, r, llmSvc)
	g.Expect(err).ToNot(HaveOccurred())

	g.Expect(r.Client.List(ctx, list)).To(Succeed())
	g.Expect(list.Items).To(HaveLen(1))
	g.Expect(list.Items[0].Name).To(Equal(kmeta.ChildName("my-llm", "-adapter-foo")))
}

func TestReconcileSchedulerInferenceObjective_CustomPriority(t *testing.T) {
	g := NewGomegaWithT(t)
	ctx := t.Context()

	llmSvc := ioLLMSvc(&v1alpha2.LoRASpec{
		Adapters: []v1alpha2.LoRAAdapterSpec{
			{Name: "high", Priority: ptr.To[int32](10)},
			{Name: "low", Priority: ptr.To[int32](-5)},
			{Name: "default"},
		},
	})
	r := ioReconciler(t, llmSvc)

	err := llmisvc.ReconcileSchedulerInferenceObjectiveForTest(ctx, r, llmSvc)
	g.Expect(err).ToNot(HaveOccurred())

	list := &igwapiv1alpha2.InferenceObjectiveList{}
	g.Expect(r.Client.List(ctx, list)).To(Succeed())
	g.Expect(list.Items).To(HaveLen(3))

	priorities := map[string]int{}
	for _, item := range list.Items {
		g.Expect(item.Spec.Priority).ToNot(BeNil())
		priorities[item.Name] = *item.Spec.Priority
	}
	g.Expect(priorities[kmeta.ChildName("my-llm", "-adapter-high")]).To(Equal(10))
	g.Expect(priorities[kmeta.ChildName("my-llm", "-adapter-low")]).To(Equal(-5))
	g.Expect(priorities[kmeta.ChildName("my-llm", "-adapter-default")]).To(Equal(0))
}
