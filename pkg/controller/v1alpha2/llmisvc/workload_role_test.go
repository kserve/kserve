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
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"knative.dev/pkg/apis"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	lwsapi "sigs.k8s.io/lws/api/leaderworkerset/v1"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/constants"
	"github.com/kserve/kserve/pkg/credentials"
)

func TestNonDisaggregatedRole(t *testing.T) {
	tests := []struct {
		name     string
		version  string
		noRouter bool
		want     string
		wantErr  bool
	}{
		{name: "v0.11.0 gets prefill-decode", version: "0.11.0", want: constants.LLMDRolePrefillDecode},
		{name: "newer than v0.11.0 gets prefill-decode", version: "0.12.1", want: constants.LLMDRolePrefillDecode},
		{name: "v0.10.0 keeps both", version: "0.10.0", want: constants.LLMDRoleBoth},
		{name: "missing version keeps both", version: "", want: constants.LLMDRoleBoth},
		{name: "no router keeps both", noRouter: true, want: constants.LLMDRoleBoth},
		{name: "invalid version is an error", version: "latest", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewGomegaWithT(t)
			svc := roleTestSingleNodeSvc()
			if !tt.noRouter {
				withRouterVersion(svc, tt.version)
			}

			role, err := nonDisaggregatedRole(svc)

			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				return
			}
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(role).To(Equal(tt.want))
		})
	}
}

func roleTestClient(t *testing.T, objs ...client.Object) client.Client {
	t.Helper()
	scheme := runtime.NewScheme()
	for _, add := range []func(*runtime.Scheme) error{
		corev1.AddToScheme, appsv1.AddToScheme, v1alpha2.AddToScheme, lwsapi.AddToScheme,
	} {
		if err := add(scheme); err != nil {
			t.Fatalf("failed to build scheme: %v", err)
		}
	}
	return fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).Build()
}

func roleTestSingleNodeSvc() *v1alpha2.LLMInferenceService {
	modelURI, _ := apis.ParseURL("hf://facebook/opt-125m")
	return &v1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{Name: "role-test", Namespace: "default"},
		Spec:       v1alpha2.LLMInferenceServiceSpec{Model: v1alpha2.LLMModelSpec{URI: *modelURI}},
	}
}

// withRouterVersion sets the scheduler's app.kubernetes.io/version annotation,
// as the merged preset does. An empty version leaves the annotation out.
func withRouterVersion(svc *v1alpha2.LLMInferenceService, version string) {
	svc.Spec.Router = &v1alpha2.RouterSpec{Scheduler: &v1alpha2.SchedulerSpec{}}
	if version != "" {
		svc.Spec.Router.Scheduler.Annotations = map[string]string{"app.kubernetes.io/version": version}
	}
}

func TestExpectedSingleNodeMainDeploymentRole(t *testing.T) {
	existingWithRole := func(role string) *appsv1.Deployment {
		labels := map[string]string{constants.LLMDRoleLabelKey: role}
		return &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: mainDeploymentName(roleTestSingleNodeSvc()), Namespace: "default"},
			Spec: appsv1.DeploymentSpec{
				Selector: &metav1.LabelSelector{MatchLabels: labels},
				Template: corev1.PodTemplateSpec{ObjectMeta: metav1.ObjectMeta{Labels: labels}},
			},
		}
	}

	tests := []struct {
		name     string
		version  string
		existing []client.Object
		prefill  bool
		want     string
	}{
		{name: "non-P/D deployment with router v0.11.0 uses prefill-decode", version: "0.11.0", want: constants.LLMDRolePrefillDecode},
		{name: "non-P/D deployment with router v0.10.0 uses both", version: "0.10.0", want: constants.LLMDRoleBoth},
		{name: "existing both deployment switches to prefill-decode with router v0.11.0", version: "0.11.0", existing: []client.Object{existingWithRole(constants.LLMDRoleBoth)}, want: constants.LLMDRolePrefillDecode},
		{name: "existing prefill-decode deployment switches to both when the scheduler is removed", existing: []client.Object{existingWithRole(constants.LLMDRolePrefillDecode)}, want: constants.LLMDRoleBoth},
		{name: "P/D decode deployment uses decode", version: "0.11.0", prefill: true, want: constants.LLMDRoleDecode},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewGomegaWithT(t)
			r := &LLMISVCReconciler{Client: roleTestClient(t, tt.existing...)}
			svc := roleTestSingleNodeSvc()
			if tt.version != "" {
				withRouterVersion(svc, tt.version)
			}
			if tt.prefill {
				svc.Spec.Prefill = &v1alpha2.WorkloadSpec{}
			}

			d, err := r.expectedSingleNodeMainDeployment(t.Context(), svc, &Config{})

			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(d.Spec.Template.Labels).To(HaveKeyWithValue(constants.LLMDRoleLabelKey, tt.want))
			g.Expect(d.Spec.Selector.MatchLabels).To(HaveKeyWithValue(constants.LLMDRoleLabelKey, tt.want))
		})
	}
}

func TestExpectedMainMultiNodeLWSRole(t *testing.T) {
	svc := newLLMInferenceServiceWithMultiNode("role-test", "role-test-multi-node")
	existingBoth := &lwsapi.LeaderWorkerSet{
		ObjectMeta: metav1.ObjectMeta{Name: mainLWSName(svc), Namespace: svc.Namespace},
		Spec: lwsapi.LeaderWorkerSetSpec{
			LeaderWorkerTemplate: lwsapi.LeaderWorkerTemplate{
				LeaderTemplate: &corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{constants.LLMDRoleLabelKey: constants.LLMDRoleBoth}},
				},
			},
		},
	}

	tests := []struct {
		name     string
		version  string
		existing []client.Object
		prefill  bool
		want     string
	}{
		{name: "non-P/D LWS leader with router v0.11.0 uses prefill-decode", version: "0.11.0", want: constants.LLMDRolePrefillDecode},
		{name: "non-P/D LWS leader with router v0.10.0 uses both", version: "0.10.0", want: constants.LLMDRoleBoth},
		{name: "existing both LWS leader switches to prefill-decode with router v0.11.0", version: "0.11.0", existing: []client.Object{existingBoth}, want: constants.LLMDRolePrefillDecode},
		{name: "P/D decode LWS leader uses decode", version: "0.11.0", prefill: true, want: constants.LLMDRoleDecode},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewGomegaWithT(t)
			r := &LLMISVCReconciler{Client: roleTestClient(t, tt.existing...), EventRecorder: record.NewFakeRecorder(10)}
			config := &Config{CredentialConfig: &credentials.CredentialConfig{}}
			llmSvc := svc.DeepCopy()
			withRouterVersion(llmSvc, tt.version)
			if tt.prefill {
				llmSvc.Spec.Prefill = &v1alpha2.WorkloadSpec{}
			}

			lws, err := r.expectedMainMultiNodeLWS(t.Context(), llmSvc, config)

			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(lws.Spec.LeaderWorkerTemplate.LeaderTemplate).NotTo(BeNil())
			g.Expect(lws.Spec.LeaderWorkerTemplate.LeaderTemplate.Labels).To(HaveKeyWithValue(constants.LLMDRoleLabelKey, tt.want))
		})
	}
}
