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

package inferenceservice

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"google.golang.org/protobuf/proto"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"k8s.io/utils/ptr"

	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	"github.com/kserve/kserve/pkg/constants"
)

var _ = Describe("Canary deployment controller", func() {
	configs := getRawKubeTestConfigs()

	makeCanaryISVC := func(name, namespace, stableURI string, minReplicas int32, canaries []v1beta1.CanarySpec) *v1beta1.InferenceService {
		return &v1beta1.InferenceService{
			ObjectMeta: metav1.ObjectMeta{
				Name:        name,
				Namespace:   namespace,
				Annotations: getDefaultAnnotations(constants.AutoscalerClassNone),
			},
			Spec: v1beta1.InferenceServiceSpec{
				Predictor: v1beta1.PredictorSpec{
					ComponentExtensionSpec: v1beta1.ComponentExtensionSpec{
						MinReplicas: ptr.To(minReplicas),
					},
					Model: &v1beta1.ModelSpec{
						ModelFormat: v1beta1.ModelFormat{Name: "tensorflow"},
						PredictorExtensionSpec: v1beta1.PredictorExtensionSpec{
							StorageURI:     proto.String(stableURI),
							RuntimeVersion: proto.String("0.14.0"),
						},
					},
				},
				Canary: canaries,
			},
		}
	}

	makeCanary := func(name string, traffic int32, uri string) v1beta1.CanarySpec {
		return v1beta1.CanarySpec{
			TrafficPercent: traffic,
			Predictor: v1beta1.PredictorSpec{
				Name: name,
				Model: &v1beta1.ModelSpec{
					ModelFormat: v1beta1.ModelFormat{Name: "tensorflow"},
					PredictorExtensionSpec: v1beta1.PredictorExtensionSpec{
						StorageURI:     proto.String(uri),
						RuntimeVersion: proto.String("0.14.0"),
					},
				},
			},
		}
	}

	Context("When creating an InferenceService with a canary", func() {
		It("Should create separate Deployments for stable and canary", func() {
			configMap := createInferenceServiceConfigMap(configs)
			Expect(k8sClient.Create(context.TODO(), configMap)).NotTo(HaveOccurred())
			defer k8sClient.Delete(context.TODO(), configMap)

			servingRuntime := getServingRuntime("tf-canary-create", "default")
			Expect(k8sClient.Create(context.TODO(), &servingRuntime)).NotTo(HaveOccurred())
			defer k8sClient.Delete(context.TODO(), &servingRuntime)

			serviceName := "canary-create-test"
			ctx := context.Background()

			isvc := makeCanaryISVC(serviceName, "default", "s3://test/model-v1", 4,
				[]v1beta1.CanarySpec{makeCanary("v2", 25, "s3://test/model-v2")})
			isvc.DefaultInferenceService(nil, nil, &v1beta1.SecurityConfig{AutoMountServiceAccountToken: false}, nil, nil)
			Expect(k8sClient.Create(ctx, isvc)).Should(Succeed())
			defer k8sClient.Delete(ctx, isvc)

			stableKey := types.NamespacedName{Name: constants.PredictorServiceName(serviceName), Namespace: "default"}
			canaryKey := types.NamespacedName{Name: constants.PredictorServiceName(serviceName, "v2"), Namespace: "default"}

			// Stable gets 3 replicas (4 - 1), canary gets 1 (25% of 4)
			Eventually(func() int32 {
				deploy := &appsv1.Deployment{}
				if err := k8sClient.Get(ctx, stableKey, deploy); err != nil {
					return -1
				}
				return *deploy.Spec.Replicas
			}, timeout, interval).Should(Equal(int32(3)))

			Eventually(func() int32 {
				deploy := &appsv1.Deployment{}
				if err := k8sClient.Get(ctx, canaryKey, deploy); err != nil {
					return -1
				}
				return *deploy.Spec.Replicas
			}, timeout, interval).Should(Equal(int32(1)))

			// Verify canary has correct storageUri annotation
			Eventually(func() string {
				deploy := &appsv1.Deployment{}
				if err := k8sClient.Get(ctx, canaryKey, deploy); err != nil {
					return ""
				}
				return deploy.Spec.Template.Annotations[constants.StorageInitializerSourceUriInternalAnnotationKey]
			}, timeout, interval).Should(Equal("s3://test/model-v2"))

			stableDeploy := &appsv1.Deployment{}
			Expect(k8sClient.Get(ctx, stableKey, stableDeploy)).Should(Succeed())
			Expect(stableDeploy.Spec.Template.Annotations[constants.StorageInitializerSourceUriInternalAnnotationKey]).To(Equal("s3://test/model-v1"))
		})
	})

	Context("When bumping canary traffic", func() {
		It("Should adjust replica counts", func() {
			configMap := createInferenceServiceConfigMap(configs)
			Expect(k8sClient.Create(context.TODO(), configMap)).NotTo(HaveOccurred())
			defer k8sClient.Delete(context.TODO(), configMap)

			servingRuntime := getServingRuntime("tf-canary-traffic", "default")
			Expect(k8sClient.Create(context.TODO(), &servingRuntime)).NotTo(HaveOccurred())
			defer k8sClient.Delete(context.TODO(), &servingRuntime)

			serviceName := "canary-traffic-test"
			serviceKey := types.NamespacedName{Name: serviceName, Namespace: "default"}
			ctx := context.Background()

			isvc := makeCanaryISVC(serviceName, "default", "s3://test/model-v1", 4,
				[]v1beta1.CanarySpec{makeCanary("v2", 25, "s3://test/model-v2")})
			isvc.DefaultInferenceService(nil, nil, &v1beta1.SecurityConfig{AutoMountServiceAccountToken: false}, nil, nil)
			Expect(k8sClient.Create(ctx, isvc)).Should(Succeed())
			defer k8sClient.Delete(ctx, isvc)

			stableKey := types.NamespacedName{Name: constants.PredictorServiceName(serviceName), Namespace: "default"}
			canaryKey := types.NamespacedName{Name: constants.PredictorServiceName(serviceName, "v2"), Namespace: "default"}

			// Wait for initial deployment
			Eventually(func() error {
				return k8sClient.Get(ctx, canaryKey, &appsv1.Deployment{})
			}, timeout, interval).Should(Succeed())

			// Bump traffic to 50%
			Expect(retry.RetryOnConflict(retry.DefaultRetry, func() error {
				updatedISVC := &v1beta1.InferenceService{}
				if err := k8sClient.Get(ctx, serviceKey, updatedISVC); err != nil {
					return err
				}
				updatedISVC.Spec.Canary[0].TrafficPercent = 50
				return k8sClient.Update(ctx, updatedISVC)
			})).Should(Succeed())

			// Verify replica adjustment: 50% of 4 = 2 canary, 2 stable
			Eventually(func() int32 {
				deploy := &appsv1.Deployment{}
				if err := k8sClient.Get(ctx, canaryKey, deploy); err != nil {
					return -1
				}
				return *deploy.Spec.Replicas
			}, timeout, interval).Should(Equal(int32(2)))

			Eventually(func() int32 {
				deploy := &appsv1.Deployment{}
				if err := k8sClient.Get(ctx, stableKey, deploy); err != nil {
					return -1
				}
				return *deploy.Spec.Replicas
			}, timeout, interval).Should(Equal(int32(2)))
		})
	})

	Context("When promoting a canary", func() {
		It("Should not trigger a redeployment of the canary pods", func() {
			configMap := createInferenceServiceConfigMap(configs)
			Expect(k8sClient.Create(context.TODO(), configMap)).NotTo(HaveOccurred())
			defer k8sClient.Delete(context.TODO(), configMap)

			servingRuntime := getServingRuntime("tf-canary-promote", "default")
			Expect(k8sClient.Create(context.TODO(), &servingRuntime)).NotTo(HaveOccurred())
			defer k8sClient.Delete(context.TODO(), &servingRuntime)

			serviceName := "canary-promote-test"
			serviceKey := types.NamespacedName{Name: serviceName, Namespace: "default"}
			ctx := context.Background()

			isvc := makeCanaryISVC(serviceName, "default", "s3://test/model-v1", 4,
				[]v1beta1.CanarySpec{makeCanary("v2", 25, "s3://test/model-v2")})
			isvc.DefaultInferenceService(nil, nil, &v1beta1.SecurityConfig{AutoMountServiceAccountToken: false}, nil, nil)
			Expect(k8sClient.Create(ctx, isvc)).Should(Succeed())
			defer k8sClient.Delete(ctx, isvc)

			canaryKey := types.NamespacedName{Name: constants.PredictorServiceName(serviceName, "v2"), Namespace: "default"}

			// Wait for canary deployment and record its pod template annotations
			var prePromotionAnnotations map[string]string
			Eventually(func() error {
				deploy := &appsv1.Deployment{}
				if err := k8sClient.Get(ctx, canaryKey, deploy); err != nil {
					return err
				}
				prePromotionAnnotations = deploy.Spec.Template.Annotations
				return nil
			}, timeout, interval).Should(Succeed())

			// Promote: set stable name to v2, update model, remove canary
			Expect(retry.RetryOnConflict(retry.DefaultRetry, func() error {
				updatedISVC := &v1beta1.InferenceService{}
				if err := k8sClient.Get(ctx, serviceKey, updatedISVC); err != nil {
					return err
				}
				updatedISVC.Spec.Predictor.Name = "v2"
				updatedISVC.Spec.Predictor.Model.StorageURI = proto.String("s3://test/model-v2")
				updatedISVC.Spec.Canary = nil
				return k8sClient.Update(ctx, updatedISVC)
			})).Should(Succeed())

			// The canary deployment (now stable) should keep the same pod template
			// — no annotation changes means no rollout triggered
			Consistently(func() map[string]string {
				deploy := &appsv1.Deployment{}
				if err := k8sClient.Get(ctx, canaryKey, deploy); err != nil {
					return nil
				}
				return deploy.Spec.Template.Annotations
			}, fastTimeout, interval).Should(Equal(prePromotionAnnotations))

			// Old stable deployment should be cleaned up
			oldStableKey := types.NamespacedName{Name: constants.PredictorServiceName(serviceName), Namespace: "default"}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, oldStableKey, &appsv1.Deployment{})
				return apierr.IsNotFound(err)
			}, timeout, interval).Should(BeTrue())
		})
	})

	Context("When removing a canary", func() {
		It("Should delete the canary Deployment and Service and restore stable replicas", func() {
			configMap := createInferenceServiceConfigMap(configs)
			Expect(k8sClient.Create(context.TODO(), configMap)).NotTo(HaveOccurred())
			defer k8sClient.Delete(context.TODO(), configMap)

			servingRuntime := getServingRuntime("tf-canary-remove", "default")
			Expect(k8sClient.Create(context.TODO(), &servingRuntime)).NotTo(HaveOccurred())
			defer k8sClient.Delete(context.TODO(), &servingRuntime)

			serviceName := "canary-remove-test"
			serviceKey := types.NamespacedName{Name: serviceName, Namespace: "default"}
			ctx := context.Background()

			isvc := makeCanaryISVC(serviceName, "default", "s3://test/model-v1", 4,
				[]v1beta1.CanarySpec{makeCanary("v2", 25, "s3://test/model-v2")})
			isvc.DefaultInferenceService(nil, nil, &v1beta1.SecurityConfig{AutoMountServiceAccountToken: false}, nil, nil)
			Expect(k8sClient.Create(ctx, isvc)).Should(Succeed())
			defer k8sClient.Delete(ctx, isvc)

			stableKey := types.NamespacedName{Name: constants.PredictorServiceName(serviceName), Namespace: "default"}
			canaryKey := types.NamespacedName{Name: constants.PredictorServiceName(serviceName, "v2"), Namespace: "default"}

			// Wait for canary deployment
			Eventually(func() error {
				return k8sClient.Get(ctx, canaryKey, &appsv1.Deployment{})
			}, timeout, interval).Should(Succeed())

			// Remove canary
			Expect(retry.RetryOnConflict(retry.DefaultRetry, func() error {
				updatedISVC := &v1beta1.InferenceService{}
				if err := k8sClient.Get(ctx, serviceKey, updatedISVC); err != nil {
					return err
				}
				updatedISVC.Spec.Canary = nil
				return k8sClient.Update(ctx, updatedISVC)
			})).Should(Succeed())

			// Canary deployment should be deleted
			Eventually(func() bool {
				err := k8sClient.Get(ctx, canaryKey, &appsv1.Deployment{})
				return apierr.IsNotFound(err)
			}, timeout, interval).Should(BeTrue())

			// Canary service should be deleted
			Eventually(func() bool {
				err := k8sClient.Get(ctx, canaryKey, &corev1.Service{})
				return apierr.IsNotFound(err)
			}, timeout, interval).Should(BeTrue())

			// Stable replicas should be restored to 4
			Eventually(func() int32 {
				deploy := &appsv1.Deployment{}
				if err := k8sClient.Get(ctx, stableKey, deploy); err != nil {
					return -1
				}
				return *deploy.Spec.Replicas
			}, timeout, interval).Should(Equal(int32(4)))
		})
	})

	Context("When creating multiple canaries", func() {
		It("Should create separate Deployments with correct replica split", func() {
			configMap := createInferenceServiceConfigMap(configs)
			Expect(k8sClient.Create(context.TODO(), configMap)).NotTo(HaveOccurred())
			defer k8sClient.Delete(context.TODO(), configMap)

			servingRuntime := getServingRuntime("tf-canary-multi", "default")
			Expect(k8sClient.Create(context.TODO(), &servingRuntime)).NotTo(HaveOccurred())
			defer k8sClient.Delete(context.TODO(), &servingRuntime)

			serviceName := "canary-multi-test"
			ctx := context.Background()

			isvc := makeCanaryISVC(serviceName, "default", "s3://test/model-v1", 4,
				[]v1beta1.CanarySpec{
					makeCanary("v2", 25, "s3://test/model-v2"),
					makeCanary("v3", 25, "s3://test/model-v3"),
				})
			isvc.DefaultInferenceService(nil, nil, &v1beta1.SecurityConfig{AutoMountServiceAccountToken: false}, nil, nil)
			Expect(k8sClient.Create(ctx, isvc)).Should(Succeed())
			defer k8sClient.Delete(ctx, isvc)

			stableKey := types.NamespacedName{Name: constants.PredictorServiceName(serviceName), Namespace: "default"}
			canaryV2Key := types.NamespacedName{Name: constants.PredictorServiceName(serviceName, "v2"), Namespace: "default"}
			canaryV3Key := types.NamespacedName{Name: constants.PredictorServiceName(serviceName, "v3"), Namespace: "default"}

			// 4 total: 2 stable, 1 v2 (25%), 1 v3 (25%)
			Eventually(func() int32 {
				deploy := &appsv1.Deployment{}
				if err := k8sClient.Get(ctx, stableKey, deploy); err != nil {
					return -1
				}
				return *deploy.Spec.Replicas
			}, timeout, interval).Should(Equal(int32(2)))

			Eventually(func() int32 {
				deploy := &appsv1.Deployment{}
				if err := k8sClient.Get(ctx, canaryV2Key, deploy); err != nil {
					return -1
				}
				return *deploy.Spec.Replicas
			}, timeout, interval).Should(Equal(int32(1)))

			Eventually(func() int32 {
				deploy := &appsv1.Deployment{}
				if err := k8sClient.Get(ctx, canaryV3Key, deploy); err != nil {
					return -1
				}
				return *deploy.Spec.Replicas
			}, timeout, interval).Should(Equal(int32(1)))
		})
	})

	Context("When force stopping an InferenceService with canary", func() {
		It("Should delete both stable and canary Deployments", func() {
			configMap := createInferenceServiceConfigMap(configs)
			Expect(k8sClient.Create(context.TODO(), configMap)).NotTo(HaveOccurred())
			defer k8sClient.Delete(context.TODO(), configMap)

			servingRuntime := getServingRuntime("tf-canary-stop", "default")
			Expect(k8sClient.Create(context.TODO(), &servingRuntime)).NotTo(HaveOccurred())
			defer k8sClient.Delete(context.TODO(), &servingRuntime)

			serviceName := "canary-stop-test"
			serviceKey := types.NamespacedName{Name: serviceName, Namespace: "default"}
			ctx := context.Background()

			isvc := makeCanaryISVC(serviceName, "default", "s3://test/model-v1", 4,
				[]v1beta1.CanarySpec{makeCanary("v2", 25, "s3://test/model-v2")})
			isvc.DefaultInferenceService(nil, nil, &v1beta1.SecurityConfig{AutoMountServiceAccountToken: false}, nil, nil)
			Expect(k8sClient.Create(ctx, isvc)).Should(Succeed())
			defer k8sClient.Delete(ctx, isvc)

			stableKey := types.NamespacedName{Name: constants.PredictorServiceName(serviceName), Namespace: "default"}
			canaryKey := types.NamespacedName{Name: constants.PredictorServiceName(serviceName, "v2"), Namespace: "default"}

			// Wait for both deployments
			Eventually(func() error {
				return k8sClient.Get(ctx, stableKey, &appsv1.Deployment{})
			}, timeout, interval).Should(Succeed())
			Eventually(func() error {
				return k8sClient.Get(ctx, canaryKey, &appsv1.Deployment{})
			}, timeout, interval).Should(Succeed())

			// Apply force stop
			Expect(retry.RetryOnConflict(retry.DefaultRetry, func() error {
				updatedISVC := &v1beta1.InferenceService{}
				if err := k8sClient.Get(ctx, serviceKey, updatedISVC); err != nil {
					return err
				}
				if updatedISVC.Annotations == nil {
					updatedISVC.Annotations = make(map[string]string)
				}
				updatedISVC.Annotations[constants.StopAnnotationKey] = "true"
				return k8sClient.Update(ctx, updatedISVC)
			})).Should(Succeed())

			// Both deployments should be deleted
			Eventually(func() bool {
				err := k8sClient.Get(ctx, stableKey, &appsv1.Deployment{})
				return apierr.IsNotFound(err)
			}, timeout, interval).Should(BeTrue())

			Eventually(func() bool {
				err := k8sClient.Get(ctx, canaryKey, &appsv1.Deployment{})
				return apierr.IsNotFound(err)
			}, timeout, interval).Should(BeTrue())
		})
	})
})
