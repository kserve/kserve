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
		serviceAnnotationDisallowed   []string
		serviceLabelDisallowed        []string
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
			name:                        "should filter internal kserve annotations",
			serviceAnnotationDisallowed: []string{},
			serviceLabelDisallowed:      []string{},
			objectMetaAnnotations: map[string]string{
				"internal.serving.kserve.io/something": "foo",
				"allowed.annotation/bar":               "baz",
			},
			expectedDeploymentAnnotations: map[string]string{
				"allowed.annotation/bar": "baz",
			},
			expectedPodAnnotations: map[string]string{
				"allowed.annotation/bar": "baz",
			},
			unexpectedAnnotations: []string{"internal.serving.kserve.io/something"},
		},
		{
			name:                        "should filter disallowed labels and annotations from ObjectMeta",
			serviceAnnotationDisallowed: []string{"disallowed.annotation/foo", "kubectl.kubernetes.io/last-applied-configuration"},
			serviceLabelDisallowed:      []string{"disallowed.label/bar"},
			objectMetaLabels: map[string]string{
				"allowed.label/foo":    "bar",
				"disallowed.label/bar": "baz",
			},
			objectMetaAnnotations: map[string]string{
				"allowed.annotation/foo":                           "bar",
				"disallowed.annotation/foo":                        "baz",
				"kubectl.kubernetes.io/last-applied-configuration": "some-json",
			},
			expectedDeploymentLabels: map[string]string{
				"allowed.label/foo": "bar",
			},
			expectedDeploymentAnnotations: map[string]string{
				"allowed.annotation/foo": "bar",
			},
			expectedPodLabels: map[string]string{
				"allowed.label/foo": "bar",
			},
			expectedPodAnnotations: map[string]string{
				"allowed.annotation/foo": "bar",
			},
			unexpectedLabels:      []string{"disallowed.label/bar"},
			unexpectedAnnotations: []string{"disallowed.annotation/foo", "kubectl.kubernetes.io/last-applied-configuration"},
		},
		{
			name:                        "should always propagate WorkloadSpec labels and annotations (overriding ObjectMeta if conflict)",
			serviceAnnotationDisallowed: []string{"disallowed.annotation/foo"},
			serviceLabelDisallowed:      []string{"disallowed.label/bar"},
			objectMetaLabels: map[string]string{
				"allowed.label/foo": "meta-val",
			},
			objectMetaAnnotations: map[string]string{
				"allowed.annotation/foo": "meta-val",
			},
			workloadSpecLabels: map[string]string{
				"allowed.label/foo":    "spec-val",
				"workload.label/extra": "extra-val",
			},
			workloadSpecAnnotations: map[string]string{
				"allowed.annotation/foo":    "spec-val",
				"workload.annotation/extra": "extra-val",
			},
			// Deployment retains meta-val because WorkloadSpec is pod-only
			expectedDeploymentLabels: map[string]string{
				"allowed.label/foo": "meta-val",
			},
			expectedDeploymentAnnotations: map[string]string{
				"allowed.annotation/foo": "meta-val",
			},
			// Pod gets spec-val (override) and extra-val
			expectedPodLabels: map[string]string{
				"allowed.label/foo":    "spec-val",
				"workload.label/extra": "extra-val",
			},
			expectedPodAnnotations: map[string]string{
				"allowed.annotation/foo":    "spec-val",
				"workload.annotation/extra": "extra-val",
			},
		},
		{
			name:                        "should allow everything when lists are empty",
			serviceAnnotationDisallowed: []string{},
			serviceLabelDisallowed:      []string{},
			objectMetaLabels: map[string]string{
				"any.label/foo": "bar",
			},
			objectMetaAnnotations: map[string]string{
				"any.annotation/foo": "bar",
			},
			expectedDeploymentLabels: map[string]string{
				"any.label/foo": "bar",
			},
			expectedDeploymentAnnotations: map[string]string{
				"any.annotation/foo": "bar",
			},
			expectedPodLabels: map[string]string{
				"any.label/foo": "bar",
			},
			expectedPodAnnotations: map[string]string{
				"any.annotation/foo": "bar",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &LLMISVCReconciler{}
			config := &Config{
				ServiceAnnotationDisallowedList: tt.serviceAnnotationDisallowed,
				ServiceLabelDisallowedList:      tt.serviceLabelDisallowed,
			}

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
			r.propagateDeploymentMetadata(llmSvc, deployment, config)

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
