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

package inferenceservice

import (
	"context"
	"strconv"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	"github.com/kserve/kserve/pkg/constants"
)

// predictorPod builds a pod that the predictor component will pick up via its
// "app: isvc.<predictor>" label selector, with the storage-initializer init
// container in the given state. InferenceServicePodLabelKey is required as
// well: the manager only caches pods carrying it (see NewCacheOptions), so
// without it the controller's List comes back empty.
func predictorPod(name, namespace, appLabel, isvcName string, initState corev1.ContainerState) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				constants.RawDeploymentAppLabel:       appLabel,
				constants.InferenceServicePodLabelKey: isvcName,
			},
		},
		Spec: corev1.PodSpec{
			InitContainers: []corev1.Container{
				{Name: constants.StorageInitializerContainerName, Image: "kserve/storage-initializer:latest"},
			},
			Containers: []corev1.Container{
				{Name: constants.InferenceServiceContainerName, Image: "tensorflow/serving:1.14.0"},
			},
		},
		Status: corev1.PodStatus{
			InitContainerStatuses: []corev1.ContainerStatus{
				{Name: constants.StorageInitializerContainerName, State: initState},
			},
		},
	}
}

var _ = Describe("InferenceService model status determinism", func() {
	Context("When several predictor pods report different storage-initializer states", func() {
		// PropagateModelStatus derives the whole model status from a single pod
		// (podList.Items[0]). Pods belonging to one ReplicaSet share a
		// CreationTimestamp - it is serialized as RFC3339, so it only has second
		// granularity - and the controller's cached client returns List results in
		// randomized map order. Unless the pod ordering is a total order, each
		// reconcile can land on a different pod and rewrite status.modelStatus with
		// a conflicting verdict, churning the resource forever without any user or
		// cluster change behind it.
		It("Should not flip status.modelStatus across repeated reconciles", func() {
			ctx := context.Background()

			configMap := createInferenceServiceConfigMap(getRawKubeTestConfigs())
			Expect(k8sClient.Create(ctx, configMap)).NotTo(HaveOccurred())
			defer k8sClient.Delete(ctx, configMap)

			servingRuntime := getServingRuntime("tf-serving-churn", "default")
			Expect(k8sClient.Create(ctx, &servingRuntime)).NotTo(HaveOccurred())
			defer k8sClient.Delete(ctx, &servingRuntime)

			serviceName := "model-status-churn"
			serviceKey := reconcile.Request{
				NamespacedName: types.NamespacedName{Name: serviceName, Namespace: "default"},
			}.NamespacedName
			predictorKey := types.NamespacedName{
				Name:      constants.PredictorServiceName(serviceKey.Name),
				Namespace: serviceKey.Namespace,
			}

			isvc := &v1beta1.InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:        serviceKey.Name,
					Namespace:   serviceKey.Namespace,
					Annotations: getDefaultAnnotations(constants.AutoscalerClassHPA),
				},
				Spec: v1beta1.InferenceServiceSpec{
					Predictor: v1beta1.PredictorSpec{
						ComponentExtensionSpec: v1beta1.ComponentExtensionSpec{
							MinReplicas: ptr.To(int32(2)),
							MaxReplicas: 3,
						},
						Tensorflow: &v1beta1.TFServingSpec{
							PredictorExtensionSpec: getCommonPredictorExtensionSpec(),
						},
					},
				},
			}
			isvc.DefaultInferenceService(nil, nil, &v1beta1.SecurityConfig{AutoMountServiceAccountToken: false}, nil, nil)
			Expect(k8sClient.Create(ctx, isvc)).Should(Succeed())
			defer k8sClient.Delete(ctx, isvc)

			By("Waiting for the predictor deployment to be created")
			Eventually(func() error {
				return k8sClient.Get(ctx, predictorKey, &appsv1.Deployment{})
			}, timeout, interval).Should(Succeed())

			By("Creating two predictor pods that disagree about the model state")
			appLabel := constants.GetRawServiceLabel(predictorKey.Name)
			loading := predictorPod("churn-pod-loading", serviceKey.Namespace, appLabel, serviceName,
				corev1.ContainerState{
					Running: &corev1.ContainerStateRunning{StartedAt: metav1.Now()},
				})
			failed := predictorPod("churn-pod-failed", serviceKey.Namespace, appLabel, serviceName,
				corev1.ContainerState{
					Terminated: &corev1.ContainerStateTerminated{
						Reason:   constants.StateReasonError,
						Message:  "failed to pull model",
						ExitCode: 1,
					},
				})

			for _, pod := range []*corev1.Pod{loading, failed} {
				status := pod.Status
				Expect(k8sClient.Create(ctx, pod)).To(Succeed())
				defer k8sClient.Delete(ctx, pod)
				pod.Status = status
				Expect(k8sClient.Status().Update(ctx, pod)).To(Succeed())
			}

			// The bug only manifests when the pods tie on CreationTimestamp, which
			// is what happens in a real ReplicaSet rollout. Creating them back to
			// back makes that overwhelmingly likely, but assert it rather than
			// silently passing if the two creates straddle a second boundary.
			Expect(loading.CreationTimestamp.Equal(&failed.CreationTimestamp)).To(BeTrue(),
				"pods must share a creation timestamp for this test to exercise the tie")

			By("Reconciling repeatedly and sampling the reported model state")
			settled := func() string {
				cur := &v1beta1.InferenceService{}
				Expect(k8sClient.Get(ctx, serviceKey, cur)).To(Succeed())
				if cur.Status.ModelStatus.ModelRevisionStates == nil {
					return ""
				}
				return string(cur.Status.ModelStatus.ModelRevisionStates.TargetModelState)
			}

			// Guard against a silent pass: if the pods never reach
			// PropagateModelStatus the state stays "Pending" and the ordering is
			// never exercised at all.
			var first string
			Eventually(func() string {
				first = settled()
				GinkgoWriter.Printf("observed target model state: %q\n", first)
				return first
			}, timeout, interval).Should(BeElementOf(
				string(v1beta1.Loading), string(v1beta1.FailedToLoad),
			), "predictor pods must drive status.modelStatus, otherwise this test proves nothing")

			// Each poll nudges the InferenceService to force a fresh reconcile and
			// then reports what that reconcile concluded. With a random List order
			// and two pods, every sample is an independent coin flip, so a
			// non-total ordering shows up within a handful of iterations.
			probe := 0
			Consistently(func(g Gomega) string {
				probe++
				g.Expect(retry.RetryOnConflict(retry.DefaultRetry, func() error {
					cur := &v1beta1.InferenceService{}
					if err := k8sClient.Get(ctx, serviceKey, cur); err != nil {
						return err
					}
					cur.Annotations["serving.kserve.io/churn-probe"] = strconv.Itoa(probe)
					return k8sClient.Update(ctx, cur)
				})).To(Succeed())
				return settled()
			}, 10*time.Second, 250*time.Millisecond).Should(Equal(first),
				"status.modelStatus flipped between reconciles without any pod change")
		})
	})
})
