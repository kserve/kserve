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
	"testing"

	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"
	"knative.dev/pkg/apis"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/constants"
)

// A CacheRequired workload that was created before its matching cache existed
// has no binding from defaulting: the reconciler must re-run the selection and
// persist it, without writing back the rendered spec it carries mid-reconcile.
func TestEnsureLocalModelCacheBindingBindsWaitingWorkload(t *testing.T) {
	g := NewGomegaWithT(t)

	uri, err := apis.ParseURL("hf://model-a")
	require.NoError(t, err)
	llmSvc := &v1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "waiting",
			Namespace: "ns-a",
			Annotations: map[string]string{
				constants.LocalModelCachePolicyAnnotationKey: constants.LocalModelCachePolicyCacheRequired,
			},
		},
		// The merged spec the reconciler works with once base refs resolved.
		Spec: v1alpha2.LLMInferenceServiceSpec{
			Model: v1alpha2.LLMModelSpec{URI: *uri},
		},
	}
	sharedCache := &v1alpha1.LocalModelNamespaceCache{
		ObjectMeta: metav1.ObjectMeta{Name: "late-cache", Namespace: "ns-a"},
		Spec: v1alpha1.LocalModelNamespaceCacheSpec{
			SourceModelUri: "hf://model-a",
			PVCRef:         ptr.To("shared-pvc"),
		},
	}

	scheme := runtime.NewScheme()
	g.Expect(v1alpha1.AddToScheme(scheme)).To(Succeed())
	g.Expect(v1alpha2.AddToScheme(scheme)).To(Succeed())
	r := &LLMISVCReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(llmSvc, sharedCache).Build(),
	}

	// The reconciler calls this with the merged (rendered) spec already in
	// place; simulate that by adding a field only the render would produce.
	rendered := llmSvc.DeepCopy()
	rendered.Spec.Runtime = ptr.To("rendered-runtime")

	g.Expect(r.ensureLocalModelCacheBinding(t.Context(), rendered)).To(Succeed())

	bound := &v1alpha2.LLMInferenceService{}
	g.Expect(r.Get(t.Context(), client.ObjectKeyFromObject(llmSvc), bound)).To(Succeed())
	g.Expect(bound.Labels).To(HaveKeyWithValue(constants.LocalModelLabel, sharedCache.Name))
	g.Expect(bound.Labels).To(HaveKeyWithValue(constants.LocalModelNamespaceLabel, "ns-a"))
	g.Expect(bound.Annotations).To(HaveKeyWithValue(constants.LocalModelPVCNameAnnotationKey, "shared-pvc"))
	g.Expect(bound.Annotations).To(HaveKeyWithValue(constants.LocalModelCachePolicyInternalAnnotationKey, constants.LocalModelCachePolicyCacheRequired))
	// The rendered spec was never persisted: only the metadata changed.
	g.Expect(bound.Spec.Runtime).To(BeNil())
}

func TestEnsureLocalModelCacheBindingSkipsBoundWorkload(t *testing.T) {
	g := NewGomegaWithT(t)

	uri, err := apis.ParseURL("hf://model-a")
	require.NoError(t, err)
	llmSvc := &v1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "bound",
			Namespace: "ns-a",
			Labels: map[string]string{
				constants.LocalModelLabel:          "late-cache",
				constants.LocalModelNamespaceLabel: "ns-a",
			},
			Annotations: map[string]string{
				constants.LocalModelCachePolicyAnnotationKey: constants.LocalModelCachePolicyCacheRequired,
			},
		},
		Spec: v1alpha2.LLMInferenceServiceSpec{
			Model: v1alpha2.LLMModelSpec{URI: *uri},
		},
	}

	scheme := runtime.NewScheme()
	g.Expect(v1alpha1.AddToScheme(scheme)).To(Succeed())
	g.Expect(v1alpha2.AddToScheme(scheme)).To(Succeed())
	// No caches at all: a fully bound workload must not even read them.
	r := &LLMISVCReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(llmSvc).Build(),
	}

	g.Expect(r.ensureLocalModelCacheBinding(t.Context(), llmSvc)).To(Succeed())
	g.Expect(llmSvc.Labels).To(HaveKeyWithValue(constants.LocalModelLabel, "late-cache"))
}
