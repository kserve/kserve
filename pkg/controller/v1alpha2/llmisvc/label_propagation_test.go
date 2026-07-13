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
	"testing"

	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
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
