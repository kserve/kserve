/*
Copyright 2023 The KServe Authors.

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

package knative

import (
	"strconv"
	"testing"

	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	"github.com/kserve/kserve/pkg/constants"

	"knative.dev/serving/pkg/apis/autoscaling"
	knserving "knative.dev/serving/pkg/apis/serving"
	knservingv1 "knative.dev/serving/pkg/apis/serving/v1"
	rtesting "sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
)

func TestCreateKnativeService(t *testing.T) {
	componentMeta := metav1.ObjectMeta{
		Name:      "test-service",
		Namespace: "default",
		Labels: map[string]string{
			"app": "test-app",
		},
		Annotations: map[string]string{
			constants.RollOutDurationAnnotationKey:         "30s",
			constants.KnativeOpenshiftEnablePassthroughKey: "true",
			constants.EnableRoutingTagAnnotationKey:        "true",
		},
	}

	componentExt := &v1beta1.ComponentExtensionSpec{
		MinReplicas:          func() *int32 { i := int32(1); return &i }(),
		MaxReplicas:          3,
		ScaleTarget:          func() *int32 { i := int32(10); return &i }(),
		ScaleMetric:          (*v1beta1.ScaleMetric)(proto.String("concurrency")),
		CanaryTrafficPercent: proto.Int64(20),
		TimeoutSeconds:       proto.Int64(300),
		ContainerConcurrency: proto.Int64(100),
	}

	podSpec := &corev1.PodSpec{
		Containers: []corev1.Container{
			{
				Name:  "test-container",
				Image: "test-image",
			},
		},
	}

	componentStatus := v1beta1.ComponentStatusSpec{
		LatestRolledoutRevision:   "test-revision-1",
		LatestReadyRevision:       "test-revision-2",
		LatestCreatedRevision:     "test-revision-2",
		PreviousRolledoutRevision: "test-revision-0",
	}

	disallowedLabelList := []string{"serving.knative.dev/revision"}

	tests := []struct {
		name                string
		componentMeta       metav1.ObjectMeta
		componentExt        *v1beta1.ComponentExtensionSpec
		componentStatus     v1beta1.ComponentStatusSpec
		expectTrafficSplit  bool
		expectedTrafficLen  int
		expectedCanaryValue int64
	}{
		{
			name:                "With canary traffic split",
			componentMeta:       componentMeta,
			componentExt:        componentExt,
			componentStatus:     componentStatus,
			expectTrafficSplit:  true,
			expectedTrafficLen:  2,
			expectedCanaryValue: 20,
		},
		{
			name:                "Without canary traffic",
			componentMeta:       componentMeta,
			componentExt:        &v1beta1.ComponentExtensionSpec{},
			componentStatus:     componentStatus,
			expectTrafficSplit:  false,
			expectedTrafficLen:  1,
			expectedCanaryValue: 100,
		},
		{
			name:          "With canary 100 percent",
			componentMeta: componentMeta,
			componentExt: &v1beta1.ComponentExtensionSpec{
				CanaryTrafficPercent: proto.Int64(100),
			},
			componentStatus:     componentStatus,
			expectTrafficSplit:  true,
			expectedTrafficLen:  1,
			expectedCanaryValue: 100,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ksvc := createKnativeService(
				tt.componentMeta,
				tt.componentExt,
				podSpec,
				tt.componentStatus,
				disallowedLabelList,
			)
			require.NotNil(t, ksvc, "createKnativeService should not return nil ksvc")

			// Verify basic service properties
			assert.Equal(t, tt.componentMeta.Name, ksvc.Name)
			assert.Equal(t, tt.componentMeta.Namespace, ksvc.Namespace)

			// Verify traffic configuration
			assert.Len(t, ksvc.Spec.Traffic, tt.expectedTrafficLen)
			assert.Equal(t, tt.expectedCanaryValue, *ksvc.Spec.Traffic[0].Percent)

			// Check routing tag
			if tt.componentMeta.Annotations[constants.EnableRoutingTagAnnotationKey] == "true" {
				assert.Equal(t, "latest", ksvc.Spec.Traffic[0].Tag)
			}

			// Verify split traffic targets
			if tt.expectTrafficSplit && tt.expectedTrafficLen > 1 {
				assert.Equal(t, tt.componentStatus.LatestRolledoutRevision, ksvc.Spec.Traffic[1].RevisionName)
				assert.False(t, *ksvc.Spec.Traffic[1].LatestRevision)
				assert.Equal(t, 100-tt.expectedCanaryValue, *ksvc.Spec.Traffic[1].Percent)
				assert.Equal(t, "prev", ksvc.Spec.Traffic[1].Tag)
			}

			// Verify annotations
			if tt.componentExt.MinReplicas != nil {
				assert.Equal(t, strconv.Itoa(int(*tt.componentExt.MinReplicas)),
					ksvc.Spec.Template.Annotations[autoscaling.MinScaleAnnotationKey])
			} else {
				assert.Equal(t, strconv.Itoa(int(constants.DefaultMinReplicas)),
					ksvc.Spec.Template.Annotations[autoscaling.MinScaleAnnotationKey])
			}

			if tt.componentExt.MaxReplicas != 0 {
				assert.Equal(t, strconv.Itoa(int(tt.componentExt.MaxReplicas)),
					ksvc.Spec.Template.Annotations[autoscaling.MaxScaleAnnotationKey])
			}

			// Verify managed annotations at ksvc level
			for ksvcAnnotationKey := range managedKsvcAnnotations {
				if value, ok := tt.componentMeta.Annotations[ksvcAnnotationKey]; ok {
					assert.Equal(t, value, ksvc.Annotations[ksvcAnnotationKey])
					// Verify it's removed from template annotations
					_, exists := ksvc.Spec.Template.Annotations[ksvcAnnotationKey]
					assert.False(t, exists)
				}
			}

			// Verify container settings from componentExt
			assert.Equal(t, tt.componentExt.TimeoutSeconds, ksvc.Spec.Template.Spec.TimeoutSeconds)
			assert.Equal(t, tt.componentExt.TimeoutSeconds, ksvc.Spec.Template.Spec.ResponseStartTimeoutSeconds)
			assert.Equal(t, tt.componentExt.ContainerConcurrency, ksvc.Spec.Template.Spec.ContainerConcurrency)
		})
	}
}

func TestKsvcReconciler_Reconcile(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = knservingv1.AddToScheme(scheme)
	_ = v1beta1.AddToScheme(scheme)

	componentMeta := metav1.ObjectMeta{
		Name:      "test-service",
		Namespace: "default",
		Labels: map[string]string{
			"app": "test-app",
		},
		Annotations: map[string]string{
			constants.RollOutDurationAnnotationKey:  "30s",
			constants.EnableRoutingTagAnnotationKey: "true",
		},
	}

	componentExt := &v1beta1.ComponentExtensionSpec{
		MinReplicas:          func() *int32 { i := int32(1); return &i }(),
		MaxReplicas:          3,
		ScaleTarget:          func() *int32 { i := int32(10); return &i }(),
		ScaleMetric:          (*v1beta1.ScaleMetric)(proto.String("concurrency")),
		CanaryTrafficPercent: proto.Int64(20),
		TimeoutSeconds:       proto.Int64(300),
		ContainerConcurrency: proto.Int64(100),
	}

	podSpec := &corev1.PodSpec{
		Containers: []corev1.Container{
			{
				Name:  "test-container",
				Image: "test-image",
			},
		},
	}

	componentStatus := v1beta1.ComponentStatusSpec{
		LatestRolledoutRevision:   "test-revision-1",
		LatestReadyRevision:       "test-revision-2",
		LatestCreatedRevision:     "test-revision-2",
		PreviousRolledoutRevision: "test-revision-0",
	}

	disallowedLabelList := []string{"serving.knative.dev/revision"}

	tests := []struct {
		name           string
		existingKsvc   *knservingv1.Service
		expectedUpdate bool
		wantErr        bool
	}{
		{
			name:           "Create a new service when it doesn't exist",
			existingKsvc:   nil,
			expectedUpdate: false,
			wantErr:        false,
		},
		{
			name: "Update existing service with different configuration",
			existingKsvc: &knservingv1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-service",
					Namespace: "default",
					Labels: map[string]string{
						"app": "test-app-old",
					},
					Annotations: map[string]string{
						constants.RollOutDurationAnnotationKey: "60s",
						knserving.CreatorAnnotation:            "test-creator",
						knserving.UpdaterAnnotation:            "test-updater",
					},
				},
				Spec: knservingv1.ServiceSpec{
					ConfigurationSpec: knservingv1.ConfigurationSpec{
						Template: knservingv1.RevisionTemplateSpec{
							Spec: knservingv1.RevisionSpec{
								TimeoutSeconds:       proto.Int64(200),
								ContainerConcurrency: proto.Int64(50),
								PodSpec: corev1.PodSpec{
									Containers: []corev1.Container{
										{
											Name:  "test-container",
											Image: "test-image-old",
										},
									},
								},
							},
						},
					},
				},
			},
			expectedUpdate: true,
			wantErr:        false,
		},
		{
			name: "No update when service is identical",
			existingKsvc: &knservingv1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-service",
					Namespace: "default",
					Labels: map[string]string{
						"app": "test-app",
					},
					Annotations: map[string]string{
						constants.RollOutDurationAnnotationKey: "30s",
						knserving.CreatorAnnotation:            "test-creator",
						knserving.UpdaterAnnotation:            "test-updater",
					},
				},
				Spec: knservingv1.ServiceSpec{
					ConfigurationSpec: knservingv1.ConfigurationSpec{
						Template: knservingv1.RevisionTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{
								Annotations: map[string]string{
									autoscaling.MinScaleAnnotationKey:       "1",
									autoscaling.MaxScaleAnnotationKey:       "3",
									autoscaling.ClassAnnotationKey:          autoscaling.KPA,
									autoscaling.TargetAnnotationKey:         "10",
									autoscaling.MetricAnnotationKey:         "concurrency",
									constants.EnableRoutingTagAnnotationKey: "true",
								},
							},
							Spec: knservingv1.RevisionSpec{
								TimeoutSeconds:              proto.Int64(300),
								ResponseStartTimeoutSeconds: proto.Int64(300),
								ContainerConcurrency:        proto.Int64(100),
								PodSpec: corev1.PodSpec{
									Containers: []corev1.Container{
										{
											Name:  "test-container",
											Image: "test-image",
										},
									},
								},
							},
						},
					},
					RouteSpec: knservingv1.RouteSpec{
						Traffic: []knservingv1.TrafficTarget{
							{
								LatestRevision: proto.Bool(true),
								Percent:        proto.Int64(20),
								Tag:            "latest",
							},
							{
								RevisionName:   "test-revision-1",
								LatestRevision: proto.Bool(false),
								Percent:        proto.Int64(80),
								Tag:            "prev",
							},
						},
					},
				},
			},
			expectedUpdate: false,
			wantErr:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up fake client
			client := rtesting.NewClientBuilder().WithScheme(scheme).Build()
			// Create the existing KService if provided
			if tt.existingKsvc != nil {
				err := client.Create(t.Context(), tt.existingKsvc)
				require.NoError(t, err)
			}

			// Create reconciler
			reconciler := NewKsvcReconciler(
				client,
				scheme,
				componentMeta,
				componentExt,
				podSpec,
				componentStatus,
				disallowedLabelList,
			)

			// Call Reconcile
			status, err := reconciler.Reconcile(t.Context())
			// Verify expectations
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, status)
			}

			// Verify service was created/updated
			createdService := &knservingv1.Service{}
			err = client.Get(t.Context(),
				types.NamespacedName{Name: componentMeta.Name, Namespace: componentMeta.Namespace},
				createdService)
			require.NoError(t, err)
			assert.NoError(t, err)

			// Check for specific updates if needed
			if tt.existingKsvc != nil && tt.expectedUpdate {
				// Verify the service was updated with the correct values
				assert.Equal(t, componentMeta.Labels["app"], createdService.Labels["app"])
				assert.Equal(t, componentMeta.Annotations[constants.RollOutDurationAnnotationKey],
					createdService.Annotations[constants.RollOutDurationAnnotationKey])

				// Check that traffic was configured correctly
				assert.Len(t, createdService.Spec.Traffic, 2)
				assert.Equal(t, int64(20), *createdService.Spec.Traffic[0].Percent)
				assert.Equal(t, "latest", createdService.Spec.Traffic[0].Tag)
				assert.Equal(t, int64(80), *createdService.Spec.Traffic[1].Percent)
				assert.Equal(t, "prev", createdService.Spec.Traffic[1].Tag)

				// Check that container was updated
				assert.Equal(t, "test-image", createdService.Spec.Template.Spec.PodSpec.Containers[0].Image)
			}
		})
	}
}
