/*
Copyright 2020 kubeflow.org.

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
	"fmt"
	"github.com/kubeflow/kfserving/pkg/apis/serving/v1alpha2"
	"github.com/kubeflow/kfserving/pkg/apis/serving/v1beta1"
	"github.com/kubeflow/kfserving/pkg/constants"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	knservingv1 "knative.dev/serving/pkg/apis/serving/v1"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"time"
)

func createTestInferenceService(serviceKey types.NamespacedName, hasStorageUri bool) *v1beta1.InferenceService {

	predictor := v1beta1.PredictorExtensionSpec{
		Container: v1.Container{
			Name: "kfs",
		},
	}
	if hasStorageUri {
		storageUri := "s3://test/mnist/export"
		predictor.StorageURI = &storageUri
	}
	instance := &v1beta1.InferenceService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceKey.Name,
			Namespace: serviceKey.Namespace,
		},
		Spec: v1beta1.InferenceServiceSpec{
			Predictor: v1beta1.PredictorSpec{
				ComponentExtensionSpec: v1beta1.ComponentExtensionSpec{
					MinReplicas: v1alpha2.GetIntReference(1),
					MaxReplicas: 3,
				},
				Tensorflow: &v1beta1.TensorflowSpec{
					PredictorExtensionSpec: predictor,
				},
			},
		},
	}
	return instance
}

var _ = Describe("v1beta1 inference service controller", func() {
	// Define utility constants for object names and testing timeouts/durations and intervals.
	const (
		timeout  = time.Second * 10
		duration = time.Second * 10
		interval = time.Millisecond * 250
	)
	var (
		configs = map[string]string{
			"predictors": `{
               "tensorflow": {
                  "image": "tensorflow/serving"
               },
               "sklearn": {
                  "image": "kfserving/sklearnserver"
               },
               "xgboost": {
                  "image": "kfserving/xgbserver"
               }
	         }`,
			"explainers": `{
               "alibi": {
                  "image": "kfserving/alibi-explainer",
			      "defaultImageVersion": "latest"
               }
            }`,
			"ingress": `{
               "ingressGateway": "knative-serving/knative-ingress-gateway",
               "ingressService": "test-destination"
            }`,
		}
	)

	Context("When creating inference service", func() {
		It("Should have knative service created", func() {
			By("By creating a new InferenceService")
			// Create configmap
			var configMap = &v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      constants.InferenceServiceConfigMapName,
					Namespace: constants.KFServingNamespace,
				},
				Data: configs,
			}
			Expect(k8sClient.Create(context.TODO(), configMap)).NotTo(HaveOccurred())
			defer k8sClient.Delete(context.TODO(), configMap)

			serviceName := "foo"
			var expectedRequest = reconcile.Request{NamespacedName: types.NamespacedName{Name: serviceName, Namespace: "default"}}
			var serviceKey = expectedRequest.NamespacedName
			var predictorService = types.NamespacedName{Name: constants.DefaultPredictorServiceName(serviceKey.Name),
				Namespace: serviceKey.Namespace}
			ctx := context.Background()
			instance := createTestInferenceService(serviceKey, true)

			Expect(k8sClient.Create(ctx, instance)).Should(Succeed())
			inferenceService := &v1beta1.InferenceService{}

			Eventually(func() bool {
				//Check if InferenceService is created
				err := k8sClient.Get(ctx, serviceKey, inferenceService)
				if err != nil {
					return false
				}
				return true
			}, timeout, interval).Should(BeTrue())

			defaultService := &knservingv1.Service{}
			Eventually(func() error { return k8sClient.Get(context.TODO(), predictorService, defaultService) }, timeout).
				Should(Succeed())
			fmt.Printf("knative service %+v\n", defaultService)
		})
	})

	Context("When creating and deleting inference service without storageUri", func() {
		// Create configmap
		var configMap = &v1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      constants.InferenceServiceConfigMapName,
				Namespace: constants.KFServingNamespace,
			},
			Data: configs,
		}

		serviceName := "bar"
		var expectedRequest = reconcile.Request{NamespacedName: types.NamespacedName{Name: serviceName, Namespace: "default"}}
		var serviceKey = expectedRequest.NamespacedName
		var multiModelConfigMapKey = types.NamespacedName{Name: constants.DefaultMultiModelConfigMapName(serviceName, 0),
			Namespace: serviceKey.Namespace}
		var ksvcKey = types.NamespacedName{Name: constants.DefaultServiceName(serviceKey.Name, constants.Predictor),
			Namespace: expectedRequest.Namespace}
		ctx := context.Background()
		instance := createTestInferenceService(serviceKey, false)

		It("Should have multi-model configmap created and mounted", func() {
			Expect(k8sClient.Create(context.TODO(), configMap)).NotTo(HaveOccurred())
			defer k8sClient.Delete(context.TODO(), configMap)
			By("By creating a new InferenceService")
			Expect(k8sClient.Create(ctx, instance)).Should(Succeed())

			inferenceService := &v1beta1.InferenceService{}
			Eventually(func() bool {
				//Check if InferenceService is created
				err := k8sClient.Get(ctx, serviceKey, inferenceService)
				if err != nil {
					return false
				}
				return true
			}, timeout, interval).Should(BeTrue())

			multiModelConfigMap := &corev1.ConfigMap{}
			Eventually(func() bool {
				//Check if multiModelConfigMap is created
				err := k8sClient.Get(ctx, multiModelConfigMapKey, multiModelConfigMap)
				if err != nil {
					return false
				}

				//Verify that this configmap's ownerreference is it's parent InferenceService
				Expect(multiModelConfigMap.OwnerReferences[0].Name).To(Equal(serviceKey.Name))

				return true
			}, timeout, interval).Should(BeTrue())

			ksvc := &knservingv1.Service{}
			Eventually(func() bool {
				//Check if ksvc is created
				err := k8sClient.Get(ctx, ksvcKey, ksvc)
				if err != nil {
					return false
				}
				//Check if the multi-model configmap is mounted
				isMounted := false
				for _, volume := range ksvc.Spec.Template.Spec.Volumes {
					if volume.Name == constants.MultiModelConfigVolumeName {
						isMounted = true
					}
				}
				if isMounted == false {
					return false
				}

				return true
			}, timeout, interval).Should(BeTrue())
		})

	})
})
