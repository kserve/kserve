/*
Copyright 2019 The Knative Authors

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

package resources

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/knative/pkg/ptr"
	pav1a1 "github.com/knative/serving/pkg/apis/autoscaling/v1alpha1"
	"github.com/knative/serving/pkg/apis/networking"
	nv1a1 "github.com/knative/serving/pkg/apis/networking/v1alpha1"
	"github.com/knative/serving/pkg/apis/serving"
	autoscalingv1 "k8s.io/api/autoscaling/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// MakeSKS makes an SKS resource from the PA, selector and operation mode.
func TestMakeSKS(t *testing.T) {
	pa := &pav1a1.PodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "here",
			Name:      "with-you",
			UID:       "2006",
			// Those labels are propagated from the Revision->KPA.
			Labels: map[string]string{
				serving.RevisionLabelKey: "with-you",
				serving.RevisionUID:      "2009",
			},
			Annotations: map[string]string{
				"a": "b",
			},
		},
		Spec: pav1a1.PodAutoscalerSpec{
			ProtocolType: networking.ProtocolHTTP1,
			ScaleTargetRef: autoscalingv1.CrossVersionObjectReference{
				APIVersion: "apps/v1",
				Kind:       "Deployment",
				Name:       "blah",
			},
		},
	}

	const mode = nv1a1.SKSOperationModeServe

	want := &nv1a1.ServerlessService{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "here",
			Name:      "with-you",
			// Those labels are propagated from the Revision->PA.
			Labels: map[string]string{
				serving.RevisionLabelKey: "with-you",
				serving.RevisionUID:      "2009",
			},
			Annotations: map[string]string{
				"a": "b",
			},
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion:         pav1a1.SchemeGroupVersion.String(),
				Kind:               "PodAutoscaler",
				Name:               "with-you",
				UID:                "2006",
				Controller:         ptr.Bool(true),
				BlockOwnerDeletion: ptr.Bool(true),
			}},
		},
		Spec: nv1a1.ServerlessServiceSpec{
			ProtocolType: networking.ProtocolHTTP1,
			Mode:         nv1a1.SKSOperationModeServe,
			ObjectRef: autoscalingv1.CrossVersionObjectReference{
				APIVersion: "apps/v1",
				Kind:       "Deployment",
				Name:       "blah",
			},
		},
	}
	if got, want := MakeSKS(pa, mode), want; !cmp.Equal(got, want) {
		t.Errorf("MakeSKS = %#v, want: %#v, diff: %s", got, want, cmp.Diff(got, want))
	}
}
