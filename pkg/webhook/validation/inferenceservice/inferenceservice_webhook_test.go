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

package inferenceservice

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"google.golang.org/protobuf/proto"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	"github.com/kserve/kserve/pkg/constants"
)

var _ = Describe("InferenceService validator Webhook", func() {
	Context("When deleting InferenceService under Validating Webhook", func() {
		var validator v1beta1.InferenceServiceValidator
		var servingRuntime *v1alpha1.ServingRuntime
		var isvc1, isvc2 *v1beta1.InferenceService

		BeforeEach(func() {
			validator = v1beta1.InferenceServiceValidator{Client: k8sClient}

			// Create a serving runtime
			servingRuntime = &v1alpha1.ServingRuntime{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "tf-serving",
					Namespace: "default",
				},
				Spec: v1alpha1.ServingRuntimeSpec{
					SupportedModelFormats: []v1alpha1.SupportedModelFormat{
						{
							Name:       "tensorflow",
							Version:    proto.String("1"),
							AutoSelect: proto.Bool(true),
						},
					},
					ServingRuntimePodSpec: v1alpha1.ServingRuntimePodSpec{
						Containers: []corev1.Container{
							{
								Name:  constants.InferenceServiceContainerName,
								Image: "tensorflow/serving:1.14.0",
							},
						},
					},
					Disabled: proto.Bool(false),
				},
			}
			Expect(k8sClient.Create(ctx, servingRuntime)).To(Succeed())

			// Create two inference services to be referenced by an inference graph
			isvc1 = &v1beta1.InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "default",
				},
				Spec: v1beta1.InferenceServiceSpec{
					Predictor: v1beta1.PredictorSpec{
						Tensorflow: &v1beta1.TFServingSpec{
							PredictorExtensionSpec: v1beta1.PredictorExtensionSpec{
								StorageURI:     proto.String("s3://test/mnist/export"),
								RuntimeVersion: proto.String("1.14.0"),
							},
						},
					},
				},
			}
			isvc1.DefaultInferenceService(nil, nil, &v1beta1.SecurityConfig{AutoMountServiceAccountToken: false}, nil)
			isvc2 = isvc1.DeepCopy()
			isvc2.Name += "-second"

			Expect(k8sClient.Create(ctx, isvc1)).Should(Succeed())
			Expect(k8sClient.Create(ctx, isvc2)).Should(Succeed())
		})

		AfterEach(func() {
			Expect(k8sClient.Delete(ctx, servingRuntime)).To(WithTransform(client.IgnoreNotFound, Succeed()))
			Expect(k8sClient.Delete(ctx, isvc1)).To(WithTransform(client.IgnoreNotFound, Succeed()))
			Expect(k8sClient.Delete(ctx, isvc2)).To(WithTransform(client.IgnoreNotFound, Succeed()))
		})

		It("Should allow deleting an InferenceService that is not referenced by an InferenceGraph", func() {
			Expect(validator.ValidateDelete(ctx, isvc1)).Error().ToNot(HaveOccurred())
		})

		It("Should prevent deleting an InferenceService that is referenced by an InferenceGraph", func() {
			inferenceGraph := v1alpha1.InferenceGraph{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "inferencegraph-one",
					Namespace: "default",
				},
				Spec: v1alpha1.InferenceGraphSpec{
					Nodes: map[string]v1alpha1.InferenceRouter{
						"root": {
							RouterType: v1alpha1.Sequence,
							Steps: []v1alpha1.InferenceStep{
								{StepName: "first", InferenceTarget: v1alpha1.InferenceTarget{
									ServiceName: isvc1.GetName(),
								}},
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, &inferenceGraph)).To(Succeed())
			defer func(ctx context.Context, inferenceGraph *v1alpha1.InferenceGraph) {
				_ = k8sClient.Delete(ctx, inferenceGraph)
			}(ctx, &inferenceGraph)

			Eventually(func() error {
				var checkIg v1alpha1.InferenceGraph
				return k8sClient.Get(ctx, types.NamespacedName{
					Namespace: inferenceGraph.GetNamespace(),
					Name:      inferenceGraph.GetName(),
				}, &checkIg)
			}).ShouldNot(HaveOccurred())

			_, err := validator.ValidateDelete(ctx, isvc1)
			Expect(err).To(HaveOccurred())
		})
	})
})
