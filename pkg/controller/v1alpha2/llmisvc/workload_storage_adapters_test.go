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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/tools/record"
	"knative.dev/pkg/apis"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/controller/v1alpha2/llmisvc"
)

func saScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme corev1: %v", err)
	}
	if err := v1alpha2.AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme v1alpha2: %v", err)
	}
	return scheme
}

func saReconciler(t *testing.T, objs ...client.Object) *llmisvc.LLMISVCReconciler {
	t.Helper()
	fakeClient := fake.NewClientBuilder().
		WithScheme(saScheme(t)).
		WithObjects(objs...).
		Build()
	return &llmisvc.LLMISVCReconciler{
		Client:        fakeClient,
		EventRecorder: record.NewFakeRecorder(20),
		Clientset:     k8sfake.NewSimpleClientset(),
	}
}

func baseLoRAPodSpec() corev1.PodSpec {
	return corev1.PodSpec{
		Containers: []corev1.Container{
			{Name: "main"},
		},
	}
}

func loRASvc(lora *v1alpha2.LoRASpec) *v1alpha2.LLMInferenceService {
	return &v1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{Name: "svc", Namespace: "ns"},
		Spec: v1alpha2.LLMInferenceServiceSpec{
			Model: v1alpha2.LLMModelSpec{LoRA: lora},
		},
	}
}

func adapterURI(t *testing.T, raw string) *apis.URL {
	t.Helper()
	u, err := apis.ParseURL(raw)
	if err != nil {
		t.Fatalf("ParseURL(%q): %v", raw, err)
	}
	return u
}

// TestStorageAdapters_NoAdapters: nil LoRA → no init containers, no lora-adapters volume.
func TestStorageAdapters_NoAdapters(t *testing.T) {
	g := NewGomegaWithT(t)
	ctx := t.Context()

	svc := loRASvc(nil)
	r := saReconciler(t, svc)
	podSpec := baseLoRAPodSpec()

	g.Expect(llmisvc.AttachLoRAAdapterArtifactsForTest(ctx, r, svc, &podSpec, nil, "main")).To(Succeed())

	g.Expect(podSpec.InitContainers).To(BeEmpty())
	g.Expect(podSpec.Volumes).To(BeEmpty())
	g.Expect(podSpec.Containers[0].VolumeMounts).To(BeEmpty())
}

// TestStorageAdapters_TwoAdaptersWithURI: 2 enabled adapters → 2 init containers,
// lora-adapters emptyDir, mount on "main".
func TestStorageAdapters_TwoAdaptersWithURI(t *testing.T) {
	g := NewGomegaWithT(t)
	ctx := t.Context()

	svc := loRASvc(&v1alpha2.LoRASpec{
		Adapters: []v1alpha2.LoRAAdapterSpec{
			{Name: "alpha", URI: adapterURI(t, "hf://adapters/alpha")},
			{Name: "beta", URI: adapterURI(t, "s3://bucket/beta")},
		},
	})
	r := saReconciler(t, svc)
	podSpec := baseLoRAPodSpec()

	g.Expect(llmisvc.AttachLoRAAdapterArtifactsForTest(ctx, r, svc, &podSpec, nil, "main")).To(Succeed())

	g.Expect(podSpec.InitContainers).To(HaveLen(2))

	names := map[string]corev1.Container{}
	for _, c := range podSpec.InitContainers {
		names[c.Name] = c
	}
	g.Expect(names).To(HaveKey("adapter-fetch-alpha"))
	g.Expect(names).To(HaveKey("adapter-fetch-beta"))

	alphaC := names["adapter-fetch-alpha"]
	g.Expect(alphaC.Args).To(Equal([]string{"hf://adapters/alpha", "/mnt/loras/alpha"}))
	g.Expect(alphaC.VolumeMounts).To(HaveLen(1))
	g.Expect(alphaC.VolumeMounts[0].Name).To(Equal("lora-adapters"))
	g.Expect(alphaC.VolumeMounts[0].MountPath).To(Equal("/mnt/loras"))

	// emptyDir volume added exactly once
	var loraVols []corev1.Volume
	for _, v := range podSpec.Volumes {
		if v.Name == "lora-adapters" {
			loraVols = append(loraVols, v)
		}
	}
	g.Expect(loraVols).To(HaveLen(1))
	g.Expect(loraVols[0].EmptyDir).ToNot(BeNil())

	// main container gets the mount
	g.Expect(podSpec.Containers[0].VolumeMounts).To(HaveLen(1))
	g.Expect(podSpec.Containers[0].VolumeMounts[0].MountPath).To(Equal("/mnt/loras"))
}

// TestStorageAdapters_RefConfigMap: adapter resolved from a ConfigMap key "uri".
func TestStorageAdapters_RefConfigMap(t *testing.T) {
	g := NewGomegaWithT(t)
	ctx := t.Context()

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "adapter-ref", Namespace: "ns"},
		Data:       map[string]string{"uri": "hf://adapters/gamma"},
	}
	svc := loRASvc(&v1alpha2.LoRASpec{
		Adapters: []v1alpha2.LoRAAdapterSpec{
			{Name: "gamma", Ref: &corev1.LocalObjectReference{Name: "adapter-ref"}},
		},
	})
	r := saReconciler(t, svc, cm)
	podSpec := baseLoRAPodSpec()

	g.Expect(llmisvc.AttachLoRAAdapterArtifactsForTest(ctx, r, svc, &podSpec, nil, "main")).To(Succeed())

	g.Expect(podSpec.InitContainers).To(HaveLen(1))
	g.Expect(podSpec.InitContainers[0].Name).To(Equal("adapter-fetch-gamma"))
	g.Expect(podSpec.InitContainers[0].Args[0]).To(Equal("hf://adapters/gamma"))
}

// TestStorageAdapters_DisabledSkipped: disabled adapter produces no init container.
func TestStorageAdapters_DisabledSkipped(t *testing.T) {
	g := NewGomegaWithT(t)
	ctx := t.Context()

	svc := loRASvc(&v1alpha2.LoRASpec{
		Adapters: []v1alpha2.LoRAAdapterSpec{
			{Name: "enabled", URI: adapterURI(t, "hf://adapters/enabled")},
			{Name: "off", URI: adapterURI(t, "hf://adapters/off"), Disabled: true},
		},
	})
	r := saReconciler(t, svc)
	podSpec := baseLoRAPodSpec()

	g.Expect(llmisvc.AttachLoRAAdapterArtifactsForTest(ctx, r, svc, &podSpec, nil, "main")).To(Succeed())

	g.Expect(podSpec.InitContainers).To(HaveLen(1))
	g.Expect(podSpec.InitContainers[0].Name).To(Equal("adapter-fetch-enabled"))
}

// TestStorageAdapters_MissingRef: non-existent Ref → error returned.
func TestStorageAdapters_MissingRef(t *testing.T) {
	g := NewGomegaWithT(t)
	ctx := t.Context()

	svc := loRASvc(&v1alpha2.LoRASpec{
		Adapters: []v1alpha2.LoRAAdapterSpec{
			{Name: "ghost", Ref: &corev1.LocalObjectReference{Name: "no-such-ref"}},
		},
	})
	r := saReconciler(t, svc)
	podSpec := baseLoRAPodSpec()

	err := llmisvc.AttachLoRAAdapterArtifactsForTest(ctx, r, svc, &podSpec, nil, "main")
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("ghost"))
}
