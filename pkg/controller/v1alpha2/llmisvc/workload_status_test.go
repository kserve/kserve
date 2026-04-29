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

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"knative.dev/pkg/apis"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/constants"
)

func TestObserveWorkloadStatus(t *testing.T) {
	tests := []struct {
		name              string
		modify            func(svc *v1alpha2.LLMInferenceService)
		expectedWorkloads *v1alpha2.WorkloadStatus
	}{
		{
			name:   "single-node, no prefill, no scheduler",
			modify: func(_ *v1alpha2.LLMInferenceService) {},
			expectedWorkloads: &v1alpha2.WorkloadStatus{
				Primary: &corev1.TypedLocalObjectReference{
					APIGroup: ptr.To("apps"),
					Kind:     "Deployment",
					Name:     "test-svc-kserve",
				},
				Service: &corev1.TypedLocalObjectReference{
					Kind: "Service",
					Name: "test-svc-kserve-workload-svc",
				},
			},
		},
		{
			name: "single-node, with prefill",
			modify: func(svc *v1alpha2.LLMInferenceService) {
				svc.Spec.Prefill = &v1alpha2.WorkloadSpec{
					Template: &corev1.PodSpec{},
				}
			},
			expectedWorkloads: &v1alpha2.WorkloadStatus{
				Primary: &corev1.TypedLocalObjectReference{
					APIGroup: ptr.To("apps"),
					Kind:     "Deployment",
					Name:     "test-svc-kserve",
				},
				Prefill: &corev1.TypedLocalObjectReference{
					APIGroup: ptr.To("apps"),
					Kind:     "Deployment",
					Name:     "test-svc-kserve-prefill",
				},
				Service: &corev1.TypedLocalObjectReference{
					Kind: "Service",
					Name: "test-svc-kserve-workload-svc",
				},
			},
		},
		{
			name: "single-node, prefill with worker (prefill workload not reconciled)",
			modify: func(svc *v1alpha2.LLMInferenceService) {
				svc.Spec.Prefill = &v1alpha2.WorkloadSpec{
					Worker: &corev1.PodSpec{},
				}
			},
			expectedWorkloads: &v1alpha2.WorkloadStatus{
				Primary: &corev1.TypedLocalObjectReference{
					APIGroup: ptr.To("apps"),
					Kind:     "Deployment",
					Name:     "test-svc-kserve",
				},
				Service: &corev1.TypedLocalObjectReference{
					Kind: "Service",
					Name: "test-svc-kserve-workload-svc",
				},
			},
		},
		{
			name: "multi-node, no prefill",
			modify: func(svc *v1alpha2.LLMInferenceService) {
				svc.Spec.Worker = &corev1.PodSpec{}
			},
			expectedWorkloads: &v1alpha2.WorkloadStatus{
				Primary: &corev1.TypedLocalObjectReference{
					APIGroup: ptr.To("leaderworkerset.x-k8s.io"),
					Kind:     "LeaderWorkerSet",
					Name:     "test-svc-kserve-mn",
				},
				Service: &corev1.TypedLocalObjectReference{
					Kind: "Service",
					Name: "test-svc-kserve-workload-svc",
				},
			},
		},
		{
			name: "multi-node, prefill without worker (prefill workload not reconciled)",
			modify: func(svc *v1alpha2.LLMInferenceService) {
				svc.Spec.Worker = &corev1.PodSpec{}
				svc.Spec.Prefill = &v1alpha2.WorkloadSpec{
					Template: &corev1.PodSpec{},
				}
			},
			expectedWorkloads: &v1alpha2.WorkloadStatus{
				Primary: &corev1.TypedLocalObjectReference{
					APIGroup: ptr.To("leaderworkerset.x-k8s.io"),
					Kind:     "LeaderWorkerSet",
					Name:     "test-svc-kserve-mn",
				},
				Service: &corev1.TypedLocalObjectReference{
					Kind: "Service",
					Name: "test-svc-kserve-workload-svc",
				},
			},
		},
		{
			name: "multi-node, with prefill",
			modify: func(svc *v1alpha2.LLMInferenceService) {
				svc.Spec.Worker = &corev1.PodSpec{}
				svc.Spec.Prefill = &v1alpha2.WorkloadSpec{
					Worker: &corev1.PodSpec{},
				}
			},
			expectedWorkloads: &v1alpha2.WorkloadStatus{
				Primary: &corev1.TypedLocalObjectReference{
					APIGroup: ptr.To("leaderworkerset.x-k8s.io"),
					Kind:     "LeaderWorkerSet",
					Name:     "test-svc-kserve-mn",
				},
				Prefill: &corev1.TypedLocalObjectReference{
					APIGroup: ptr.To("leaderworkerset.x-k8s.io"),
					Kind:     "LeaderWorkerSet",
					Name:     "test-svc-kserve-mn-prefill",
				},
				Service: &corev1.TypedLocalObjectReference{
					Kind: "Service",
					Name: "test-svc-kserve-workload-svc",
				},
			},
		},
		{
			name: "with managed scheduler",
			modify: func(svc *v1alpha2.LLMInferenceService) {
				svc.Spec.Router = &v1alpha2.RouterSpec{
					Scheduler: &v1alpha2.SchedulerSpec{
						Template: &corev1.PodSpec{},
					},
				}
			},
			expectedWorkloads: &v1alpha2.WorkloadStatus{
				Primary: &corev1.TypedLocalObjectReference{
					APIGroup: ptr.To("apps"),
					Kind:     "Deployment",
					Name:     "test-svc-kserve",
				},
				Service: &corev1.TypedLocalObjectReference{
					Kind: "Service",
					Name: "test-svc-kserve-workload-svc",
				},
				Scheduler: &corev1.TypedLocalObjectReference{
					APIGroup: ptr.To("apps"),
					Kind:     "Deployment",
					Name:     "test-svc-kserve-router-scheduler",
				},
			},
		},
		{
			name: "scheduler with pool ref (external)",
			modify: func(svc *v1alpha2.LLMInferenceService) {
				svc.Spec.Router = &v1alpha2.RouterSpec{
					Scheduler: &v1alpha2.SchedulerSpec{
						Template: &corev1.PodSpec{},
						Pool: &v1alpha2.InferencePoolSpec{
							Ref: &corev1.LocalObjectReference{
								Name: "external-pool",
							},
						},
					},
				}
			},
			expectedWorkloads: &v1alpha2.WorkloadStatus{
				Primary: &corev1.TypedLocalObjectReference{
					APIGroup: ptr.To("apps"),
					Kind:     "Deployment",
					Name:     "test-svc-kserve",
				},
				Service: &corev1.TypedLocalObjectReference{
					Kind: "Service",
					Name: "test-svc-kserve-workload-svc",
				},
			},
		},
		{
			name: "force-stop clears workloads",
			modify: func(svc *v1alpha2.LLMInferenceService) {
				svc.Annotations = map[string]string{
					constants.StopAnnotationKey: "true",
				}
				// Pre-populate to verify it gets cleared.
				svc.Status.Workloads = &v1alpha2.WorkloadStatus{
					Primary: &corev1.TypedLocalObjectReference{
						APIGroup: ptr.To("apps"),
						Kind:     "Deployment",
						Name:     "stale",
					},
				}
			},
			expectedWorkloads: nil,
		},
		{
			name: "idempotency - calling twice produces identical results",
			modify: func(svc *v1alpha2.LLMInferenceService) {
				svc.Spec.Worker = &corev1.PodSpec{}
				svc.Spec.Prefill = &v1alpha2.WorkloadSpec{
					Worker: &corev1.PodSpec{},
				}
				svc.Spec.Router = &v1alpha2.RouterSpec{
					Scheduler: &v1alpha2.SchedulerSpec{
						Template: &corev1.PodSpec{},
					},
				}
				// Call once before the main test invocation to pre-populate.
				observeWorkloadStatus(svc)
			},
			expectedWorkloads: &v1alpha2.WorkloadStatus{
				Primary: &corev1.TypedLocalObjectReference{
					APIGroup: ptr.To("leaderworkerset.x-k8s.io"),
					Kind:     "LeaderWorkerSet",
					Name:     "test-svc-kserve-mn",
				},
				Prefill: &corev1.TypedLocalObjectReference{
					APIGroup: ptr.To("leaderworkerset.x-k8s.io"),
					Kind:     "LeaderWorkerSet",
					Name:     "test-svc-kserve-mn-prefill",
				},
				Service: &corev1.TypedLocalObjectReference{
					Kind: "Service",
					Name: "test-svc-kserve-workload-svc",
				},
				Scheduler: &corev1.TypedLocalObjectReference{
					APIGroup: ptr.To("apps"),
					Kind:     "Deployment",
					Name:     "test-svc-kserve-router-scheduler",
				},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc := &v1alpha2.LLMInferenceService{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "serving.kserve.io/v1alpha2",
					Kind:       "LLMInferenceService",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-svc",
					Namespace: "test-ns",
					UID:       "test-uid-1234",
				},
				Spec: v1alpha2.LLMInferenceServiceSpec{
					Model: v1alpha2.LLMModelSpec{
						URI:  apis.URL{Scheme: "hf", Host: "meta-llama/Llama-3.1-8B"},
						Name: ptr.To("meta-llama/Llama-3.1-8B"),
					},
				},
			}

			tc.modify(svc)

			observeWorkloadStatus(svc)

			assert.Equal(t, tc.expectedWorkloads, svc.Status.Workloads)
		})
	}
}
