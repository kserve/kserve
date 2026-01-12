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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	knativeapis "knative.dev/pkg/apis"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	"github.com/kserve/kserve/pkg/constants"
)

var _ = Describe("ServingRuntime Watch", func() {
	var reconciler *InferenceServiceReconciler
	var testNamespace string

	BeforeEach(func() {
		testNamespace = "runtime-watch-test"
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: testNamespace,
			},
		}
		err := k8sClient.Create(context.Background(), ns)
		if err != nil {
			// Namespace might already exist, ignore error
			_ = k8sClient.Get(context.Background(), types.NamespacedName{Name: testNamespace}, ns)
		}

		reconciler = &InferenceServiceReconciler{
			Client: k8sClient,
		}
	})

	AfterEach(func() {
		// Clean up ISVCs created during tests
		var isvcList v1beta1.InferenceServiceList
		_ = k8sClient.List(context.Background(), &isvcList, client.InNamespace(testNamespace))
		for _, isvc := range isvcList.Items {
			_ = k8sClient.Delete(context.Background(), &isvc)
		}
	})

	Describe("clusterServingRuntimeFunc", func() {
		It("should only reconcile ISVCs that use the specific ClusterServingRuntime", func() {
			// Create ISVC using clusterRuntime1
			isvc1 := &v1beta1.InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "isvc-cluster-runtime-1",
					Namespace: testNamespace,
				},
				Spec: v1beta1.InferenceServiceSpec{
					Predictor: v1beta1.PredictorSpec{
						SKLearn: &v1beta1.SKLearnSpec{},
					},
				},
			}
			Expect(k8sClient.Create(context.Background(), isvc1)).To(Succeed())

			// Set the ClusterServingRuntimeName in status
			isvc1.Status.ClusterServingRuntimeName = "cluster-runtime-1"
			Expect(k8sClient.Status().Update(context.Background(), isvc1)).To(Succeed())

			// Create ISVC using clusterRuntime2
			isvc2 := &v1beta1.InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "isvc-cluster-runtime-2",
					Namespace: testNamespace,
				},
				Spec: v1beta1.InferenceServiceSpec{
					Predictor: v1beta1.PredictorSpec{
						SKLearn: &v1beta1.SKLearnSpec{},
					},
				},
			}
			Expect(k8sClient.Create(context.Background(), isvc2)).To(Succeed())

			// Set the ClusterServingRuntimeName in status
			isvc2.Status.ClusterServingRuntimeName = "cluster-runtime-2"
			Expect(k8sClient.Status().Update(context.Background(), isvc2)).To(Succeed())

			// Create a ClusterServingRuntime object (only need metadata for the mapper)
			csr := &v1alpha1.ClusterServingRuntime{
				ObjectMeta: metav1.ObjectMeta{
					Name: "cluster-runtime-1",
				},
			}

			// Call the mapper function
			requests := reconciler.clusterServingRuntimeFunc(context.Background(), csr)

			// Should only return request for isvc1, not isvc2
			Expect(requests).To(HaveLen(1))
			Expect(requests[0].Name).To(Equal("isvc-cluster-runtime-1"))
			Expect(requests[0].Namespace).To(Equal(testNamespace))
		})

		It("should not reconcile ISVCs that use a different ClusterServingRuntime", func() {
			// Create ISVC using clusterRuntime2
			isvc := &v1beta1.InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "isvc-different-runtime",
					Namespace: testNamespace,
				},
				Spec: v1beta1.InferenceServiceSpec{
					Predictor: v1beta1.PredictorSpec{
						SKLearn: &v1beta1.SKLearnSpec{},
					},
				},
			}
			Expect(k8sClient.Create(context.Background(), isvc)).To(Succeed())

			// Set the ClusterServingRuntimeName in status to a different runtime
			isvc.Status.ClusterServingRuntimeName = "cluster-runtime-other"
			Expect(k8sClient.Status().Update(context.Background(), isvc)).To(Succeed())

			// Create a ClusterServingRuntime object for "cluster-runtime-1"
			csr := &v1alpha1.ClusterServingRuntime{
				ObjectMeta: metav1.ObjectMeta{
					Name: "cluster-runtime-1",
				},
			}

			// Call the mapper function
			requests := reconciler.clusterServingRuntimeFunc(context.Background(), csr)

			// Should return empty since no ISVC uses cluster-runtime-1
			Expect(requests).To(BeEmpty())
		})

		It("should not reconcile ISVCs with auto-update disabled when ready", func() {
			// Create ISVC with auto-update disabled
			isvc := &v1beta1.InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "isvc-auto-update-disabled",
					Namespace: testNamespace,
					Annotations: map[string]string{
						constants.DisableAutoUpdateAnnotationKey: "true",
					},
				},
				Spec: v1beta1.InferenceServiceSpec{
					Predictor: v1beta1.PredictorSpec{
						SKLearn: &v1beta1.SKLearnSpec{},
					},
				},
			}
			Expect(k8sClient.Create(context.Background(), isvc)).To(Succeed())

			// Set the ClusterServingRuntimeName and make it ready
			isvc.Status.ClusterServingRuntimeName = "cluster-runtime-1"
			isvc.Status.SetCondition(v1beta1.PredictorReady, &knativeapis.Condition{
				Type:   v1beta1.PredictorReady,
				Status: corev1.ConditionTrue,
			})
			isvc.Status.SetCondition(v1beta1.IngressReady, &knativeapis.Condition{
				Type:   v1beta1.IngressReady,
				Status: corev1.ConditionTrue,
			})
			Expect(k8sClient.Status().Update(context.Background(), isvc)).To(Succeed())

			// Create a ClusterServingRuntime object
			csr := &v1alpha1.ClusterServingRuntime{
				ObjectMeta: metav1.ObjectMeta{
					Name: "cluster-runtime-1",
				},
			}

			// Call the mapper function
			requests := reconciler.clusterServingRuntimeFunc(context.Background(), csr)

			// Should not reconcile the ISVC because auto-update is disabled and it's ready
			Expect(requests).To(BeEmpty())
		})
	})

	Describe("servingRuntimeFunc", func() {
		It("should only reconcile ISVCs that use the specific ServingRuntime", func() {
			// Create ISVC using runtime1
			isvc1 := &v1beta1.InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "isvc-serving-runtime-1",
					Namespace: testNamespace,
				},
				Spec: v1beta1.InferenceServiceSpec{
					Predictor: v1beta1.PredictorSpec{
						SKLearn: &v1beta1.SKLearnSpec{},
					},
				},
			}
			Expect(k8sClient.Create(context.Background(), isvc1)).To(Succeed())

			// Set the ServingRuntimeName in status
			isvc1.Status.ServingRuntimeName = "serving-runtime-1"
			Expect(k8sClient.Status().Update(context.Background(), isvc1)).To(Succeed())

			// Create ISVC using runtime2
			isvc2 := &v1beta1.InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "isvc-serving-runtime-2",
					Namespace: testNamespace,
				},
				Spec: v1beta1.InferenceServiceSpec{
					Predictor: v1beta1.PredictorSpec{
						SKLearn: &v1beta1.SKLearnSpec{},
					},
				},
			}
			Expect(k8sClient.Create(context.Background(), isvc2)).To(Succeed())

			// Set the ServingRuntimeName in status
			isvc2.Status.ServingRuntimeName = "serving-runtime-2"
			Expect(k8sClient.Status().Update(context.Background(), isvc2)).To(Succeed())

			// Create a ServingRuntime object
			sr := &v1alpha1.ServingRuntime{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "serving-runtime-1",
					Namespace: testNamespace,
				},
			}

			// Call the mapper function
			requests := reconciler.servingRuntimeFunc(context.Background(), sr)

			// Should only return request for isvc1, not isvc2
			Expect(requests).To(HaveLen(1))
			Expect(requests[0].Name).To(Equal("isvc-serving-runtime-1"))
			Expect(requests[0].Namespace).To(Equal(testNamespace))
		})
	})
})
