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
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	clientfake "sigs.k8s.io/controller-runtime/pkg/client/fake"
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	duckv1 "knative.dev/pkg/apis/duck/v1"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/constants"
)

// Covers a preset pinned by an earlier release. Once LLM_INFERENCE_SERVICE_CONFIG_PREFIX
// changes, its name is no longer well-known, and while the service fails to resolve,
// status.appliedConfigs is empty, so the pin alone holds it. A running controller only pins
// names under its own prefix, so envtest can't reach this without writing status.
func TestReconcileDeletionOfConfigPinnedByEarlierRelease(t *testing.T) {
	const (
		oldPreset  = "old-prefix-config-llm-template"
		svcName    = "pinned-svc"
		svcNs      = "user-ns"
		finalizer  = constants.KServeAPIGroupName + "/llmisvcconfig-finalizer"
		appliedRef = "some-other-config"
	)
	pins := map[string]string{constants.WellKnownConfigPinAnnotationPrefix + "llm-template": oldPreset}

	tests := []struct {
		name        string
		annotations map[string]string
		applied     []v1alpha2.AppliedConfigRef
		wantBlocked bool
	}{
		{
			name:        "the pin blocks deletion while appliedConfigs is empty",
			annotations: pins,
			wantBlocked: true,
		},
		{
			name: "deletion completes without the pin",
		},
		{
			name:        "appliedConfigs wins over the pin",
			annotations: pins,
			applied: []v1alpha2.AppliedConfigRef{{
				Name:      appliedRef,
				Namespace: svcNs,
				Source:    v1alpha2.AppliedConfigSourceUserRef,
			}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewGomegaWithT(t)
			g.Expect(WellKnownDefaultConfigs.Has(oldPreset)).To(BeFalse(),
				"a well-known name is held by referencingServices without reading the pin")

			scheme := runtime.NewScheme()
			g.Expect(v1alpha2.AddToScheme(scheme)).To(Succeed())

			now := metav1.Now()
			config := &v1alpha2.LLMInferenceServiceConfig{ObjectMeta: metav1.ObjectMeta{
				Name:              oldPreset,
				Namespace:         constants.KServeNamespace,
				Finalizers:        []string{finalizer},
				DeletionTimestamp: &now,
			}}
			svc := &v1alpha2.LLMInferenceService{
				ObjectMeta: metav1.ObjectMeta{Name: svcName, Namespace: svcNs},
				Status: v1alpha2.LLMInferenceServiceStatus{
					Status:            duckv1.Status{Annotations: tt.annotations},
					AppliedConfigRefs: tt.applied,
				},
			}

			cl := clientfake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(config, svc).
				WithStatusSubresource(config).
				Build()
			r := &LLMISVCConfigReconciler{Client: cl, EventRecorder: record.NewFakeRecorder(10)}

			_, err := r.Reconcile(t.Context(), ctrl.Request{NamespacedName: client.ObjectKeyFromObject(config)})
			g.Expect(err).ToNot(HaveOccurred())

			current := &v1alpha2.LLMInferenceServiceConfig{}
			err = cl.Get(t.Context(), client.ObjectKeyFromObject(config), current)
			if !tt.wantBlocked {
				g.Expect(apierrors.IsNotFound(err)).To(BeTrue(), "config should be deleted, got: %v", err)
				return
			}

			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(current.Finalizers).To(ContainElement(finalizer))
			g.Expect(current.Status.ReferencedBy).To(ConsistOf(v1alpha2.UntypedObjectReference{
				Name:      gwapiv1.ObjectName(svcName),
				Namespace: gwapiv1.Namespace(svcNs),
			}))
			inUse := current.GetStatus().GetCondition(v1alpha2.ConfigInUseConditionType)
			g.Expect(inUse).ToNot(BeNil())
			g.Expect(inUse.IsTrue()).To(BeTrue())
			g.Expect(inUse.Reason).To(Equal("DeletionBlocked"))
		})
	}
}
