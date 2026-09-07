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

package llmisvc

import (
	"context"
	"maps"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	"knative.dev/pkg/apis"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	igwapi "sigs.k8s.io/gateway-api-inference-extension/api/v1"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/constants"
	"github.com/kserve/kserve/pkg/utils"
)

func TestPropagateDeploymentMetadata(t *testing.T) {
	tests := []struct {
		name                          string
		objectMetaLabels              map[string]string
		objectMetaAnnotations         map[string]string
		workloadSpecLabels            map[string]string
		workloadSpecAnnotations       map[string]string
		expectedDeploymentLabels      map[string]string
		expectedDeploymentAnnotations map[string]string
		expectedPodLabels             map[string]string
		expectedPodAnnotations        map[string]string
		unexpectedLabels              []string
		unexpectedAnnotations         []string
	}{
		{
			name: "should only propagate approved-prefix annotations from top-level metadata",
			objectMetaAnnotations: map[string]string{
				"k8s.v1.cni.cncf.io/networks": "my-network",
				"kueue.x-k8s.io/queue-name":   "my-queue",
				"random.annotation/foo":       "bar",
			},
			expectedDeploymentAnnotations: map[string]string{
				"k8s.v1.cni.cncf.io/networks": "my-network",
				"kueue.x-k8s.io/queue-name":   "my-queue",
			},
			expectedPodAnnotations: map[string]string{
				"k8s.v1.cni.cncf.io/networks": "my-network",
				"kueue.x-k8s.io/queue-name":   "my-queue",
			},
			unexpectedAnnotations: []string{"random.annotation/foo"},
		},
		{
			name: "should only propagate approved-prefix labels from top-level metadata",
			objectMetaLabels: map[string]string{
				"kueue.x-k8s.io/queue-name": "my-queue",
				"random.label/foo":          "bar",
				"app.kubernetes.io/name":    "my-app",
			},
			expectedDeploymentLabels: map[string]string{
				"kueue.x-k8s.io/queue-name": "my-queue",
			},
			expectedPodLabels: map[string]string{
				"kueue.x-k8s.io/queue-name": "my-queue",
			},
			unexpectedLabels: []string{"random.label/foo", "app.kubernetes.io/name"},
		},
		{
			name: "should not propagate internal kserve annotations",
			objectMetaAnnotations: map[string]string{
				"internal.serving.kserve.io/something": "foo",
				"kueue.x-k8s.io/queue-name":            "my-queue",
			},
			expectedDeploymentAnnotations: map[string]string{
				"kueue.x-k8s.io/queue-name": "my-queue",
			},
			expectedPodAnnotations: map[string]string{
				"kueue.x-k8s.io/queue-name": "my-queue",
			},
			unexpectedAnnotations: []string{"internal.serving.kserve.io/something"},
		},
		{
			name: "should not propagate kubectl last-applied-configuration",
			objectMetaAnnotations: map[string]string{
				"kubectl.kubernetes.io/last-applied-configuration": "some-json",
				"k8s.v1.cni.cncf.io/networks":                      "my-network",
			},
			expectedDeploymentAnnotations: map[string]string{
				"k8s.v1.cni.cncf.io/networks": "my-network",
			},
			expectedPodAnnotations: map[string]string{
				"k8s.v1.cni.cncf.io/networks": "my-network",
			},
			unexpectedAnnotations: []string{"kubectl.kubernetes.io/last-applied-configuration"},
		},
		{
			name: "should propagate prometheus.io annotations from top-level metadata",
			objectMetaAnnotations: map[string]string{
				"prometheus.io/scrape":  "true",
				"prometheus.io/port":    "8080",
				"prometheus.io/path":    "/metrics",
				"prometheus.io/scheme":  "https",
				"random.annotation/foo": "bar",
			},
			expectedDeploymentAnnotations: map[string]string{
				"prometheus.io/scrape": "true",
				"prometheus.io/port":   "8080",
				"prometheus.io/path":   "/metrics",
				"prometheus.io/scheme": "https",
			},
			expectedPodAnnotations: map[string]string{
				"prometheus.io/scrape": "true",
				"prometheus.io/port":   "8080",
				"prometheus.io/path":   "/metrics",
				"prometheus.io/scheme": "https",
			},
			unexpectedAnnotations: []string{"random.annotation/foo"},
		},
		{
			name: "should always propagate WorkloadSpec labels and annotations to Pod template",
			objectMetaLabels: map[string]string{
				"kueue.x-k8s.io/queue-name": "meta-val",
			},
			objectMetaAnnotations: map[string]string{
				"kueue.x-k8s.io/queue-name": "meta-val",
			},
			workloadSpecLabels: map[string]string{
				"kueue.x-k8s.io/queue-name": "spec-val",
				"workload.label/extra":      "extra-val",
				"any.label/custom":          "custom-val",
			},
			workloadSpecAnnotations: map[string]string{
				"kueue.x-k8s.io/queue-name": "spec-val",
				"workload.annotation/extra": "extra-val",
				"any.annotation/custom":     "custom-val",
			},
			// Deployment only gets approved-prefix labels/annotations from top-level metadata
			expectedDeploymentLabels: map[string]string{
				"kueue.x-k8s.io/queue-name": "meta-val",
			},
			expectedDeploymentAnnotations: map[string]string{
				"kueue.x-k8s.io/queue-name": "meta-val",
			},
			// Pod gets WorkloadSpec values (which override top-level metadata) plus all WorkloadSpec entries
			expectedPodLabels: map[string]string{
				"kueue.x-k8s.io/queue-name": "spec-val",
				"workload.label/extra":      "extra-val",
				"any.label/custom":          "custom-val",
			},
			expectedPodAnnotations: map[string]string{
				"kueue.x-k8s.io/queue-name": "spec-val",
				"workload.annotation/extra": "extra-val",
				"any.annotation/custom":     "custom-val",
			},
		},
		{
			name: "should propagate localmodel labels and annotations from top-level metadata",
			objectMetaLabels: map[string]string{
				"internal.serving.kserve.io/localmodel":           "my-cache",
				"internal.serving.kserve.io/localmodel-namespace": "test-ns",
			},
			objectMetaAnnotations: map[string]string{
				"internal.serving.kserve.io/localmodel-sourceuri": "s3://bucket/model",
				"internal.serving.kserve.io/localmodel-pvc-name":  "my-cache-gpu1",
			},
			expectedDeploymentLabels: map[string]string{
				"internal.serving.kserve.io/localmodel":           "my-cache",
				"internal.serving.kserve.io/localmodel-namespace": "test-ns",
			},
			expectedDeploymentAnnotations: map[string]string{
				"internal.serving.kserve.io/localmodel-sourceuri": "s3://bucket/model",
				"internal.serving.kserve.io/localmodel-pvc-name":  "my-cache-gpu1",
			},
			expectedPodLabels: map[string]string{
				"internal.serving.kserve.io/localmodel":           "my-cache",
				"internal.serving.kserve.io/localmodel-namespace": "test-ns",
			},
			expectedPodAnnotations: map[string]string{
				"internal.serving.kserve.io/localmodel-sourceuri": "s3://bucket/model",
				"internal.serving.kserve.io/localmodel-pvc-name":  "my-cache-gpu1",
			},
		},
		{
			name:             "should propagate nothing when no matching prefixes and no WorkloadSpec",
			objectMetaLabels: map[string]string{"random.label/foo": "bar"},
			objectMetaAnnotations: map[string]string{
				"random.annotation/foo": "bar",
			},
			unexpectedLabels:      []string{"random.label/foo"},
			unexpectedAnnotations: []string{"random.annotation/foo"},
		},
		{
			name: "should propagate only WorkloadSpec entries when no top-level metadata matches approved prefixes",
			objectMetaLabels: map[string]string{
				"random.label/foo": "bar",
			},
			objectMetaAnnotations: map[string]string{
				"random.annotation/foo": "bar",
			},
			workloadSpecLabels: map[string]string{
				"workload.label/extra": "extra-val",
			},
			workloadSpecAnnotations: map[string]string{
				"workload.annotation/extra": "extra-val",
			},
			expectedPodLabels: map[string]string{
				"workload.label/extra": "extra-val",
			},
			expectedPodAnnotations: map[string]string{
				"workload.annotation/extra": "extra-val",
			},
			unexpectedLabels:      []string{"random.label/foo"},
			unexpectedAnnotations: []string{"random.annotation/foo"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &LLMISVCReconciler{}

			llmSvc := &v1alpha2.LLMInferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Labels:      tt.objectMetaLabels,
					Annotations: tt.objectMetaAnnotations,
				},
				Spec: v1alpha2.LLMInferenceServiceSpec{
					WorkloadSpec: v1alpha2.WorkloadSpec{
						Labels:      tt.workloadSpecLabels,
						Annotations: tt.workloadSpecAnnotations,
					},
				},
			}

			deployment := &appsv1.Deployment{}
			r.propagateDeploymentMetadata(llmSvc, deployment)
			utils.PropagateMap(llmSvc.Spec.Labels, &deployment.Spec.Template.Labels)
			utils.PropagateMap(llmSvc.Spec.Annotations, &deployment.Spec.Template.Annotations, AnnotationModelBasedRoutingEnabled)

			// Verify Deployment labels
			for k, v := range tt.expectedDeploymentLabels {
				assert.Equal(t, v, deployment.Labels[k], "Deployment Label %s mismatch", k)
			}
			// Verify Pod Template labels
			for k, v := range tt.expectedPodLabels {
				assert.Equal(t, v, deployment.Spec.Template.Labels[k], "Template Label %s mismatch", k)
			}

			// Verify unexpected labels
			for _, k := range tt.unexpectedLabels {
				assert.NotContains(t, deployment.Labels, k, "Deployment should not contain label %s", k)
				assert.NotContains(t, deployment.Spec.Template.Labels, k, "Template should not contain label %s", k)
			}

			// Verify Deployment annotations
			for k, v := range tt.expectedDeploymentAnnotations {
				assert.Equal(t, v, deployment.Annotations[k], "Deployment Annotation %s mismatch", k)
			}
			// Verify Pod Template annotations
			for k, v := range tt.expectedPodAnnotations {
				assert.Equal(t, v, deployment.Spec.Template.Annotations[k], "Template Annotation %s mismatch", k)
			}

			// Verify unexpected annotations
			for _, k := range tt.unexpectedAnnotations {
				assert.NotContains(t, deployment.Annotations, k, "Deployment should not contain annotation %s", k)
				assert.NotContains(t, deployment.Spec.Template.Annotations, k, "Template should not contain annotation %s", k)
			}
		})
	}
}

func TestPropagateSchedulerMetadata(t *testing.T) {
	tests := []struct {
		name                   string
		schedulerLabels        map[string]string
		schedulerAnnotations   map[string]string
		expectedPodLabels      map[string]string
		expectedPodAnnotations map[string]string
	}{
		{
			name: "should propagate scheduler labels and annotations to pod template",
			schedulerLabels: map[string]string{
				"custom.label/key":  "value",
				"another.label/key": "another-value",
			},
			schedulerAnnotations: map[string]string{
				"custom.annotation/key":  "value",
				"another.annotation/key": "another-value",
			},
			expectedPodLabels: map[string]string{
				"custom.label/key":  "value",
				"another.label/key": "another-value",
			},
			expectedPodAnnotations: map[string]string{
				"custom.annotation/key":  "value",
				"another.annotation/key": "another-value",
			},
		},
		{
			name: "should handle nil scheduler labels and annotations",
		},
		{
			name: "should propagate only labels when no annotations are specified",
			schedulerLabels: map[string]string{
				"custom.label/key": "value",
			},
			expectedPodLabels: map[string]string{
				"custom.label/key": "value",
			},
		},
		{
			name: "should propagate only annotations when no labels are specified",
			schedulerAnnotations: map[string]string{
				"custom.annotation/key": "value",
			},
			expectedPodAnnotations: map[string]string{
				"custom.annotation/key": "value",
			},
		},
		{
			name: "should propagate arbitrary scheduler labels and annotations without filtering",
			schedulerLabels: map[string]string{
				"kueue.x-k8s.io/queue-name": "my-queue",
				"any.domain/label":          "any-value",
				"no-domain-label":           "simple-value",
			},
			schedulerAnnotations: map[string]string{
				"prometheus.io/scrape": "true",
				"any.domain/ann":       "any-value",
				"no-domain-ann":        "simple-value",
			},
			expectedPodLabels: map[string]string{
				"kueue.x-k8s.io/queue-name": "my-queue",
				"any.domain/label":          "any-value",
				"no-domain-label":           "simple-value",
			},
			expectedPodAnnotations: map[string]string{
				"prometheus.io/scrape": "true",
				"any.domain/ann":       "any-value",
				"no-domain-ann":        "simple-value",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &LLMISVCReconciler{}

			llmSvc := &v1alpha2.LLMInferenceService{
				Spec: v1alpha2.LLMInferenceServiceSpec{
					Router: &v1alpha2.RouterSpec{
						Scheduler: &v1alpha2.SchedulerSpec{
							Labels:      tt.schedulerLabels,
							Annotations: tt.schedulerAnnotations,
						},
					},
				},
			}

			deployment := &appsv1.Deployment{}
			r.propagateSchedulerMetadata(llmSvc, deployment)

			for k, v := range tt.expectedPodLabels {
				assert.Equal(t, v, deployment.Spec.Template.Labels[k], "Template Label %s mismatch", k)
			}
			for k, v := range tt.expectedPodAnnotations {
				assert.Equal(t, v, deployment.Spec.Template.Annotations[k], "Template Annotation %s mismatch", k)
			}

			assert.Empty(t, deployment.Labels, "Scheduler labels should not be set on the Deployment itself")
			assert.Empty(t, deployment.Annotations, "Scheduler annotations should not be set on the Deployment itself")
		})
	}
}

func TestPropagateWorkloadServiceMetadata(t *testing.T) {
	llmSvc := &v1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-llm",
		},
		Spec: v1alpha2.LLMInferenceServiceSpec{
			WorkloadSpec: v1alpha2.WorkloadSpec{
				Labels: map[string]string{
					"model":    "llama-3.1-8b",
					"endpoint": "my-endpoint",
				},
				Annotations: map[string]string{
					"prometheus.io/scrape": "true",
				},
			},
		},
	}

	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{
				constants.KubernetesComponentLabelKey: constants.LLMComponentWorkload,
				constants.KubernetesAppNameLabelKey:   llmSvc.GetName(),
				constants.KubernetesPartOfLabelKey:    constants.LLMInferenceServicePartOfValue,
			},
		},
	}

	utils.PropagateMap(llmSvc.Spec.Labels, &svc.Labels)
	utils.PropagateMap(llmSvc.Spec.Annotations, &svc.Annotations)

	expectedLabels := map[string]string{
		constants.KubernetesComponentLabelKey: constants.LLMComponentWorkload,
		constants.KubernetesAppNameLabelKey:   "test-llm",
		constants.KubernetesPartOfLabelKey:    constants.LLMInferenceServicePartOfValue,
		"model":                               "llama-3.1-8b",
		"endpoint":                            "my-endpoint",
	}
	assert.Equal(t, expectedLabels, svc.Labels)
	assert.Equal(t, map[string]string{"prometheus.io/scrape": "true"}, svc.Annotations)
}

// TestDeploymentSelectorExcludesMetadataLabels verifies how the Deployment builders split
// labels between spec.selector and the pod template.
//
// spec.selector is immutable after create, so it carries the component's identity labels
// with the workload's own spec.labels applied on top - the override that lets one
// LLMInferenceService's pods join another's InferencePool. Labels propagated from
// top-level metadata, such as kueue.x-k8s.io/*, reach the pod template only.
func TestDeploymentSelectorExcludesMetadataLabels(t *testing.T) {
	const (
		nameLabel  = "app.kubernetes.io/name"
		queueLabel = "kueue.x-k8s.io/queue-name"
		userLabel  = "team"
	)

	modelURI, err := apis.ParseURL("hf://facebook/opt-125m")
	require.NoError(t, err)

	newSvc := func() *v1alpha2.LLMInferenceService {
		return &v1alpha2.LLMInferenceService{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "selector-test",
				Namespace: "default",
				Labels:    map[string]string{queueLabel: "team-alpha-queue"},
			},
			Spec: v1alpha2.LLMInferenceServiceSpec{
				Model: v1alpha2.LLMModelSpec{URI: *modelURI},
				WorkloadSpec: v1alpha2.WorkloadSpec{
					Labels: map[string]string{
						userLabel: "alpha",
						nameLabel: "other-service",
					},
				},
			},
		}
	}

	tests := []struct {
		name string
		// Only the main and prefill builders call propagateDeploymentMetadata, so only
		// they receive allowlisted labels from the LLMInferenceService's own metadata.
		propagatesMetadataLabels bool
		// The tokenizer takes no workload labels, so its selector is identity alone.
		appliesWorkloadLabels bool
		build                 func(t *testing.T, r *LLMISVCReconciler) *appsv1.Deployment
	}{
		{
			name:                     "single-node main deployment",
			propagatesMetadataLabels: true,
			appliesWorkloadLabels:    true,
			build: func(t *testing.T, r *LLMISVCReconciler) *appsv1.Deployment {
				d, err := r.expectedSingleNodeMainDeployment(context.Background(), newSvc(), &Config{})
				require.NoError(t, err)
				return d
			},
		},
		{
			name:                     "prefill deployment",
			propagatesMetadataLabels: true,
			appliesWorkloadLabels:    true,
			build: func(t *testing.T, r *LLMISVCReconciler) *appsv1.Deployment {
				svc := newSvc()
				svc.Spec.Prefill = &v1alpha2.WorkloadSpec{Labels: svc.Spec.Labels}
				d, err := r.expectedPrefillMainDeployment(context.Background(), svc, &Config{})
				require.NoError(t, err)
				return d
			},
		},
		{
			name:                  "scheduler deployment",
			appliesWorkloadLabels: true,
			build: func(t *testing.T, r *LLMISVCReconciler) *appsv1.Deployment {
				svc := newSvc()
				svc.Spec.Router = &v1alpha2.RouterSpec{
					Scheduler: &v1alpha2.SchedulerSpec{Labels: svc.Spec.Labels},
				}
				d, err := r.expectedSchedulerDeployment(context.Background(), svc)
				require.NoError(t, err)
				return d
			},
		},
		{
			name: "tokenizer deployment",
			build: func(t *testing.T, r *LLMISVCReconciler) *appsv1.Deployment {
				d, err := r.expectedTokenizerDeployment(context.Background(), newSvc())
				require.NoError(t, err)
				return d
			},
		},
		{
			// Labels copied from a referenced InferencePool identify the pods that pool
			// routes to, so they belong in the selector alongside the identity labels.
			name:                     "single-node main deployment with an InferencePool ref",
			propagatesMetadataLabels: true,
			appliesWorkloadLabels:    true,
			build: func(t *testing.T, r *LLMISVCReconciler) *appsv1.Deployment {
				svc := newSvc()
				svc.Spec.Router = &v1alpha2.RouterSpec{
					Scheduler: &v1alpha2.SchedulerSpec{
						Pool: &v1alpha2.InferencePoolSpec{
							Ref: &corev1.LocalObjectReference{Name: "shared-pool"},
						},
					},
				}
				d, err := r.expectedSingleNodeMainDeployment(context.Background(), svc, &Config{})
				require.NoError(t, err)
				assert.Equal(t, "shared", d.Spec.Selector.MatchLabels["pool"],
					"labels from a referenced InferencePool must reach the selector")
				return d
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &LLMISVCReconciler{Client: selectorTestClient(t), Clientset: k8sfake.NewSimpleClientset()}
			d := tt.build(t, r)

			selector := d.Spec.Selector.MatchLabels
			podLabels := d.Spec.Template.Labels

			// Propagated from metadata: pod template only, never the selector.
			if tt.propagatesMetadataLabels {
				assert.Contains(t, podLabels, queueLabel)
			}
			assert.NotContains(t, selector, queueLabel,
				"spec.selector is immutable and must not carry labels propagated from metadata")

			// From spec.labels: both, so the override reaches the selector.
			if tt.appliesWorkloadLabels {
				assert.Equal(t, "alpha", selector[userLabel])
				assert.Equal(t, "other-service", selector[nameLabel],
					"spec.labels must override the identity label in the selector")
			} else {
				assert.NotContains(t, selector, userLabel)
			}

			// Kubernetes requires the pod template to satisfy the selector.
			for k, v := range selector {
				assert.Equal(t, v, podLabels[k],
					"selector key %s must be satisfied by the pod template", k)
			}

			// A write to the pod template labels must not reach the selector.
			podLabels["mutation-probe"] = "x"
			assert.NotContains(t, selector, "mutation-probe",
				"spec.selector must not share its backing map with the pod template")
		})
	}
}

// selectorTestClient returns a client with the schemes the Deployment builders read
// through (existing Deployment, ServiceAccount, InferencePool) so they can run without
// an API server, seeded with the InferencePool the ref case looks up.
func selectorTestClient(t *testing.T) client.Client {
	t.Helper()
	scheme := runtime.NewScheme()
	require.NoError(t, v1alpha2.AddToScheme(scheme))
	require.NoError(t, appsv1.AddToScheme(scheme))
	require.NoError(t, corev1.AddToScheme(scheme))
	require.NoError(t, igwapi.Install(scheme))

	pool := &igwapi.InferencePool{
		ObjectMeta: metav1.ObjectMeta{Name: "shared-pool", Namespace: "default"},
		Spec: igwapi.InferencePoolSpec{
			Selector: igwapi.LabelSelector{
				MatchLabels: map[igwapi.LabelKey]igwapi.LabelValue{"pool": "shared"},
			},
		},
	}

	return fake.NewClientBuilder().WithScheme(scheme).WithObjects(pool).Build()
}

func TestDeploymentSelectorLabels(t *testing.T) {
	tests := []struct {
		name           string
		identity       map[string]string
		workloadLabels map[string]string
		want           map[string]string
	}{
		{
			name:     "nil workload labels leave the identity labels alone",
			identity: map[string]string{"app.kubernetes.io/name": "svc"},
			want:     map[string]string{"app.kubernetes.io/name": "svc"},
		},
		{
			name:           "nil identity labels",
			workloadLabels: map[string]string{"team": "alpha"},
			want:           map[string]string{"team": "alpha"},
		},
		{
			name: "both nil",
			want: map[string]string{},
		},
		{
			name:           "workload labels override identity labels",
			identity:       map[string]string{"app.kubernetes.io/name": "svc", "kserve.io/component": "workload"},
			workloadLabels: map[string]string{"app.kubernetes.io/name": "other", "team": "alpha"},
			want: map[string]string{
				"app.kubernetes.io/name": "other",
				"kserve.io/component":    "workload",
				"team":                   "alpha",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			identity := maps.Clone(tt.identity)

			got := deploymentSelectorLabels(identity, tt.workloadLabels)

			assert.Equal(t, tt.want, got)
			assert.Equal(t, tt.identity, identity, "the identity map must not be modified")
		})
	}
}
