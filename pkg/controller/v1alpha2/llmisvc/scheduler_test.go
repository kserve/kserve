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

	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
)

func TestPreserveSchedulerConfig(t *testing.T) {
	defaultSvc := &v1alpha2.LLMInferenceService{}
	inlineSvc := &v1alpha2.LLMInferenceService{
		Spec: v1alpha2.LLMInferenceServiceSpec{
			Router: &v1alpha2.RouterSpec{
				Scheduler: &v1alpha2.SchedulerSpec{
					Config: &v1alpha2.SchedulerConfigSpec{
						Inline: &runtime.RawExtension{Raw: []byte("updated-inline-config")},
					},
				},
			},
		},
	}

	tests := []struct {
		name     string
		llmSvc   *v1alpha2.LLMInferenceService
		curr     *appsv1.Deployment
		expected []string
	}{
		{
			name:     "no current deployment - generates fresh config",
			llmSvc:   defaultSvc,
			curr:     &appsv1.Deployment{},
			expected: []string{"--config-text", schedulerConfigText(defaultSvc)},
		},
		{
			name:   "current deployment with --config-text - preserves it",
			llmSvc: defaultSvc,
			curr: &appsv1.Deployment{
				Spec: appsv1.DeploymentSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name: "main",
									Args: []string{"--config-text", "existing-config-yaml"},
								},
							},
						},
					},
				},
			},
			expected: []string{"--config-text", "existing-config-yaml"},
		},
		{
			name:   "current deployment with -config-text - preserves it",
			llmSvc: defaultSvc,
			curr: &appsv1.Deployment{
				Spec: appsv1.DeploymentSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name: "main",
									Args: []string{"-config-text", "old-config"},
								},
							},
						},
					},
				},
			},
			expected: []string{"-config-text", "old-config"},
		},
		{
			name:   "current deployment with --config-file - preserves it",
			llmSvc: defaultSvc,
			curr: &appsv1.Deployment{
				Spec: appsv1.DeploymentSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name: "main",
									Args: []string{"--config-file", "/etc/scheduler/config.yaml"},
								},
							},
						},
					},
				},
			},
			expected: []string{"--config-file", "/etc/scheduler/config.yaml"},
		},
		{
			name:   "current deployment with non-main container - ignored",
			llmSvc: defaultSvc,
			curr: &appsv1.Deployment{
				Spec: appsv1.DeploymentSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name: "sidecar",
									Args: []string{"--config-text", "sidecar-config"},
								},
							},
						},
					},
				},
			},
			expected: []string{"--config-text", schedulerConfigText(defaultSvc)},
		},
		{
			name:   "inline config overrides existing deployment config",
			llmSvc: inlineSvc,
			curr: &appsv1.Deployment{
				Spec: appsv1.DeploymentSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name: "main",
									Args: []string{"--config-text", "stale-config"},
								},
							},
						},
					},
				},
			},
			expected: []string{"--config-text", "updated-inline-config"},
		},
		{
			name:     "inline config used when no existing deployment",
			llmSvc:   inlineSvc,
			curr:     &appsv1.Deployment{},
			expected: []string{"--config-text", "updated-inline-config"},
		},
		{
			name: "template already has --config-text - returns nil to avoid duplication",
			llmSvc: &v1alpha2.LLMInferenceService{
				Spec: v1alpha2.LLMInferenceServiceSpec{
					Router: &v1alpha2.RouterSpec{
						Scheduler: &v1alpha2.SchedulerSpec{
							Template: &corev1.PodSpec{
								Containers: []corev1.Container{
									{
										Name: "main",
										Args: []string{"--config-text", "template-config", "--poolName", "test"},
									},
								},
							},
						},
					},
				},
			},
			curr:     &appsv1.Deployment{},
			expected: nil,
		},
		{
			name: "inline config overrides template config args",
			llmSvc: &v1alpha2.LLMInferenceService{
				Spec: v1alpha2.LLMInferenceServiceSpec{
					Router: &v1alpha2.RouterSpec{
						Scheduler: &v1alpha2.SchedulerSpec{
							Config: &v1alpha2.SchedulerConfigSpec{
								Inline: &runtime.RawExtension{Raw: []byte("inline-override")},
							},
							Template: &corev1.PodSpec{
								Containers: []corev1.Container{
									{
										Name: "main",
										Args: []string{"--config-text", "template-config"},
									},
								},
							},
						},
					},
				},
			},
			curr:     &appsv1.Deployment{},
			expected: []string{"--config-text", "inline-override"},
		},
		{
			name:   "config flag as last arg without value - generates fresh config",
			llmSvc: defaultSvc,
			curr: &appsv1.Deployment{
				Spec: appsv1.DeploymentSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name: "main",
									Args: []string{"--config-text"},
								},
							},
						},
					},
				},
			},
			expected: []string{"--config-text", schedulerConfigText(defaultSvc)},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewGomegaWithT(t)
			result := preserveSchedulerConfig(tt.llmSvc, tt.curr)
			g.Expect(result).To(Equal(tt.expected))
		})
	}
}
