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
		name        string
		currentRole string
		want        string
	}{
		{name: "new workload gets prefill-decode", currentRole: "", want: constants.LLMDRolePrefillDecode},
		{name: "existing workload labelled both keeps both", currentRole: constants.LLMDRoleBoth, want: constants.LLMDRoleBoth},
		{name: "existing workload labelled prefill-decode keeps prefill-decode", currentRole: constants.LLMDRolePrefillDecode, want: constants.LLMDRolePrefillDecode},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewGomegaWithT(t)
			g.Expect(nonDisaggregatedRole(tt.currentRole)).To(Equal(tt.want))
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
		existing []client.Object
		prefill  bool
		want     string
	}{
		{name: "new non-P/D deployment uses prefill-decode", want: constants.LLMDRolePrefillDecode},
		{name: "existing non-P/D deployment labelled both keeps both", existing: []client.Object{existingWithRole(constants.LLMDRoleBoth)}, want: constants.LLMDRoleBoth},
		{name: "P/D decode deployment uses decode", prefill: true, want: constants.LLMDRoleDecode},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewGomegaWithT(t)
			r := &LLMISVCReconciler{Client: roleTestClient(t, tt.existing...)}
			svc := roleTestSingleNodeSvc()
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
		existing []client.Object
		prefill  bool
		want     string
	}{
		{name: "new non-P/D LWS leader uses prefill-decode", want: constants.LLMDRolePrefillDecode},
		{name: "existing non-P/D LWS leader labelled both keeps both", existing: []client.Object{existingBoth}, want: constants.LLMDRoleBoth},
		{name: "P/D decode LWS leader uses decode", prefill: true, want: constants.LLMDRoleDecode},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewGomegaWithT(t)
			r := &LLMISVCReconciler{Client: roleTestClient(t, tt.existing...), EventRecorder: record.NewFakeRecorder(10)}
			config := &Config{CredentialConfig: &credentials.CredentialConfig{}}
			llmSvc := svc.DeepCopy()
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

func TestCurrentMainWorkloadRole(t *testing.T) {
	singleNode := roleTestSingleNodeSvc()
	multiNode := newLLMInferenceServiceWithMultiNode("role-test", "role-test-multi-node")
	bothLabels := map[string]string{constants.LLMDRoleLabelKey: constants.LLMDRoleBoth}

	deploymentBoth := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: mainDeploymentName(singleNode), Namespace: "default"},
		Spec: appsv1.DeploymentSpec{
			Template: corev1.PodTemplateSpec{ObjectMeta: metav1.ObjectMeta{Labels: bothLabels}},
		},
	}
	lwsBoth := &lwsapi.LeaderWorkerSet{
		ObjectMeta: metav1.ObjectMeta{Name: mainLWSName(multiNode), Namespace: multiNode.Namespace},
		Spec: lwsapi.LeaderWorkerSetSpec{
			LeaderWorkerTemplate: lwsapi.LeaderWorkerTemplate{
				LeaderTemplate: &corev1.PodTemplateSpec{ObjectMeta: metav1.ObjectMeta{Labels: bothLabels}},
			},
		},
	}
	lwsWithoutLeader := &lwsapi.LeaderWorkerSet{
		ObjectMeta: metav1.ObjectMeta{Name: mainLWSName(multiNode), Namespace: multiNode.Namespace},
	}

	tests := []struct {
		name   string
		svc    *v1alpha2.LLMInferenceService
		client func(t *testing.T) client.Client
		want   string
	}{
		{
			name:   "single-node without a deployment returns empty",
			svc:    singleNode,
			client: func(t *testing.T) client.Client { return roleTestClient(t) },
			want:   "",
		},
		{
			name:   "single-node returns the deployment pod label",
			svc:    singleNode,
			client: func(t *testing.T) client.Client { return roleTestClient(t, deploymentBoth) },
			want:   constants.LLMDRoleBoth,
		},
		{
			name:   "multi-node without an LWS returns empty",
			svc:    multiNode,
			client: func(t *testing.T) client.Client { return roleTestClient(t) },
			want:   "",
		},
		{
			name:   "multi-node returns the LWS leader pod label",
			svc:    multiNode,
			client: func(t *testing.T) client.Client { return roleTestClient(t, lwsBoth) },
			want:   constants.LLMDRoleBoth,
		},
		{
			name:   "multi-node LWS without a leader template returns empty",
			svc:    multiNode,
			client: func(t *testing.T) client.Client { return roleTestClient(t, lwsWithoutLeader) },
			want:   "",
		},
		{
			name:   "multi-node without the LWS CRD installed returns empty",
			svc:    multiNode,
			client: newFakeClientWithLWSNoMatch,
			want:   "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewGomegaWithT(t)
			r := &LLMISVCReconciler{Client: tt.client(t)}

			role, err := r.currentMainWorkloadRole(t.Context(), tt.svc)

			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(role).To(Equal(tt.want))
		})
	}
}

func TestCheckRoleFiltersOnLegacyBothWorkload(t *testing.T) {
	const roleFilterConfig = `apiVersion: llm-d.ai/v1alpha1
kind: EndpointPickerConfig
plugins:
- type: decode-filter
- type: queue-scorer
schedulingProfiles:
- name: default
  plugins:
  - pluginRef: decode-filter
  - pluginRef: queue-scorer
`
	const noRoleFilterConfig = `apiVersion: llm-d.ai/v1alpha1
kind: EndpointPickerConfig
plugins:
- type: queue-scorer
schedulingProfiles:
- name: default
  plugins:
  - pluginRef: queue-scorer
`
	// The router puts every declared filter into a default profile when the
	// config has no schedulingProfiles, so the filter is used.
	const roleFilterNoProfilesConfig = `apiVersion: llm-d.ai/v1alpha1
kind: EndpointPickerConfig
plugins:
- type: decode-filter
- type: queue-scorer
`
	const unusedRoleFilterConfig = `apiVersion: llm-d.ai/v1alpha1
kind: EndpointPickerConfig
plugins:
- type: decode-filter
- type: queue-scorer
schedulingProfiles:
- name: default
  plugins:
  - pluginRef: queue-scorer
`
	const namedRoleFilterConfig = `apiVersion: llm-d.ai/v1alpha1
kind: EndpointPickerConfig
plugins:
- name: my-filter
  type: prefill-filter
- type: queue-scorer
schedulingProfiles:
- name: default
  plugins:
  - pluginRef: my-filter
  - pluginRef: queue-scorer
`

	scheduler := func(version, config string) *appsv1.Deployment {
		return &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "role-test-epp", Namespace: "default"},
			Spec: appsv1.DeploymentSpec{
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{"app.kubernetes.io/version": version}},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{{Name: "main", Args: []string{"--config-text", config}}},
					},
				},
			},
		}
	}
	workload := func(role string) *appsv1.Deployment {
		return &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: mainDeploymentName(roleTestSingleNodeSvc()), Namespace: "default"},
			Spec: appsv1.DeploymentSpec{
				Template: corev1.PodTemplateSpec{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{constants.LLMDRoleLabelKey: role}}},
			},
		}
	}

	tests := []struct {
		name     string
		version  string
		config   string
		existing []client.Object
		prefill  bool
		stopped  bool
		// wantError is the filter the error must name; "" means no error.
		wantError string
	}{
		{name: "rejects role filter on an existing both workload with v0.11.0", version: "0.11.0", config: roleFilterConfig, existing: []client.Object{workload(constants.LLMDRoleBoth)}, wantError: "decode-filter"},
		{name: "allows role filter on an existing both workload with v0.10.0", version: "0.10.0", config: roleFilterConfig, existing: []client.Object{workload(constants.LLMDRoleBoth)}},
		{name: "allows role filter on a prefill-decode workload", version: "0.11.0", config: roleFilterConfig, existing: []client.Object{workload(constants.LLMDRolePrefillDecode)}},
		{name: "allows role filter when the workload does not exist yet", version: "0.11.0", config: roleFilterConfig},
		{name: "allows a config without role filters on an existing both workload", version: "0.11.0", config: noRoleFilterConfig, existing: []client.Object{workload(constants.LLMDRoleBoth)}},
		{name: "rejects a role filter used by the default profile when no profiles are set", version: "0.11.0", config: roleFilterNoProfilesConfig, existing: []client.Object{workload(constants.LLMDRoleBoth)}, wantError: "decode-filter"},
		{name: "allows a role filter that no profile uses", version: "0.11.0", config: unusedRoleFilterConfig, existing: []client.Object{workload(constants.LLMDRoleBoth)}},
		{name: "rejects a role filter used by a profile under a custom name", version: "0.11.0", config: namedRoleFilterConfig, existing: []client.Object{workload(constants.LLMDRoleBoth)}, wantError: "prefill-filter"},
		{name: "ignores P/D services", version: "0.11.0", config: roleFilterConfig, existing: []client.Object{workload(constants.LLMDRoleBoth)}, prefill: true},
		{name: "does not block stopping the service", version: "0.11.0", config: roleFilterConfig, existing: []client.Object{workload(constants.LLMDRoleBoth)}, stopped: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewGomegaWithT(t)
			recorder := record.NewFakeRecorder(10)
			r := &LLMISVCReconciler{Client: roleTestClient(t, tt.existing...), EventRecorder: recorder}
			svc := roleTestSingleNodeSvc()
			if tt.prefill {
				svc.Spec.Prefill = &v1alpha2.WorkloadSpec{}
			}
			if tt.stopped {
				svc.Annotations = map[string]string{constants.StopAnnotationKey: "true"}
			}

			err := r.checkRoleFiltersOnLegacyBothWorkload(t.Context(), scheduler(tt.version, tt.config), svc)

			if tt.wantError == "" {
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(recorder.Events).To(BeEmpty())
				return
			}
			g.Expect(err).To(MatchError(ContainSubstring(tt.wantError)))
			g.Expect(recorder.Events).To(Receive(ContainSubstring(tt.wantError)))
		})
	}
}

// TestReconcileSchedulerDeploymentStopsOnRoleFilterWithLegacyBoth checks the
// wiring: the scheduler reconcile runs the role filter check and, when it
// fails, leaves the existing scheduler Deployment as it is.
func TestReconcileSchedulerDeploymentStopsOnRoleFilterWithLegacyBoth(t *testing.T) {
	const roleFilterConfig = `apiVersion: llm-d.ai/v1alpha1
kind: EndpointPickerConfig
plugins:
- type: decode-filter
- type: queue-scorer
schedulingProfiles:
- name: default
  plugins:
  - pluginRef: decode-filter
  - pluginRef: queue-scorer
`
	const noRoleFilterConfig = `apiVersion: llm-d.ai/v1alpha1
kind: EndpointPickerConfig
plugins:
- type: queue-scorer
schedulingProfiles:
- name: default
  plugins:
  - pluginRef: queue-scorer
`

	newSvc := func(config string) *v1alpha2.LLMInferenceService {
		svc := roleTestSingleNodeSvc()
		svc.UID = "role-test-uid"
		svc.Spec.Router = &v1alpha2.RouterSpec{
			Scheduler: &v1alpha2.SchedulerSpec{
				Annotations: map[string]string{"app.kubernetes.io/version": "0.11.0"},
				Template: &corev1.PodSpec{
					Containers: []corev1.Container{{
						Name:  "main",
						Image: "ghcr.io/llm-d/llm-d-router-endpoint-picker:v0.11.0",
						Args:  []string{"--config-text", config},
					}},
				},
			},
		}
		return svc
	}
	workloadBoth := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: mainDeploymentName(roleTestSingleNodeSvc()), Namespace: "default"},
		Spec: appsv1.DeploymentSpec{
			Template: corev1.PodTemplateSpec{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{constants.LLMDRoleLabelKey: constants.LLMDRoleBoth}}},
		},
	}
	// The scheduler still running the old router image.
	oldScheduler := func(svc *v1alpha2.LLMInferenceService) *appsv1.Deployment {
		return &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: schedulerDeploymentName(svc), Namespace: "default"},
			Spec: appsv1.DeploymentSpec{
				Template: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "main", Image: "ghcr.io/llm-d/llm-d-router-endpoint-picker:v0.10.0"}}},
				},
			},
		}
	}

	t.Run("stops the rollout and keeps the old scheduler", func(t *testing.T) {
		g := NewGomegaWithT(t)
		svc := newSvc(roleFilterConfig)
		recorder := record.NewFakeRecorder(10)
		c := roleTestClient(t, roleTestInferenceServiceConfigMap(), workloadBoth, oldScheduler(svc))
		r := &LLMISVCReconciler{Client: c, EventRecorder: recorder}

		err := r.reconcileSchedulerDeployment(t.Context(), svc)

		g.Expect(err).To(MatchError(ContainSubstring("decode-filter")))
		g.Expect(recorder.Events).To(Receive(ContainSubstring("decode-filter")))
		curr := &appsv1.Deployment{}
		g.Expect(c.Get(t.Context(), client.ObjectKeyFromObject(oldScheduler(svc)), curr)).To(Succeed())
		g.Expect(curr.Spec.Template.Spec.Containers[0].Image).To(HaveSuffix(":v0.10.0"))
	})

	t.Run("builds the scheduler when the config has no role filter", func(t *testing.T) {
		g := NewGomegaWithT(t)
		svc := newSvc(noRoleFilterConfig)
		recorder := record.NewFakeRecorder(10)
		r := &LLMISVCReconciler{Client: roleTestClient(t, roleTestInferenceServiceConfigMap(), workloadBoth), EventRecorder: recorder}

		d, err := r.expectedSchedulerDeployment(t.Context(), svc)

		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(d.Spec.Template.Spec.Containers[0].Image).To(HaveSuffix(":v0.11.0"))
		g.Expect(recorder.Events).To(BeEmpty())
	})
}

// roleTestInferenceServiceConfigMap returns the smallest inferenceservice-config
// ConfigMap the scheduler reconcile can load.
func roleTestInferenceServiceConfigMap() *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: constants.InferenceServiceConfigMapName, Namespace: constants.KServeNamespace},
		Data: map[string]string{
			"ingress":            `{"ingressGateway": "knative-serving/knative-ingress-gateway"}`,
			"storageInitializer": `{"memoryRequest": "100Mi", "memoryLimit": "1Gi", "cpuRequest": "100m", "cpuLimit": "1"}`,
			"credentials":        `{}`,
		},
	}
}
