package inferencegraph

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"github.com/kserve/kserve/pkg/constants"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"knative.dev/pkg/kmp"
	"knative.dev/pkg/ptr"
	knservingdefaults "knative.dev/serving/pkg/apis/config"
	knservingv1 "knative.dev/serving/pkg/apis/serving/v1"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"time"
)

var _ = Describe("Inference Graph controller test", func() {

	// Define utility constants for object names and testing timeouts/durations and intervals.
	const (
		timeout  = time.Second * 10
		interval = time.Millisecond * 250
		domain   = "example.com"
	)

	var (
		configs = map[string]string{
			"router": `{
					  "image": "kserve/router:v0.10.0",
					  "memoryRequest": "100Mi",
					  "memoryLimit": "500Mi",
					  "cpuRequest": "100m",
					  "cpuLimit": "100m"
				}`,
		}
	)

	Context("When creating an inferencegraph with defaults", func() {
		It("Should create a knative service", func() {
			By("By creating a new InferenceGraph")
			var configMap = &v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      constants.InferenceServiceConfigMapName,
					Namespace: constants.KServeNamespace,
				},
				Data: configs,
			}
			Expect(k8sClient.Create(context.TODO(), configMap)).NotTo(HaveOccurred())
			defer k8sClient.Delete(context.TODO(), configMap)
			graphName := "singlenode"
			var expectedRequest = reconcile.Request{NamespacedName: types.NamespacedName{Name: graphName, Namespace: "default"}}
			var igkey = expectedRequest.NamespacedName
			ctx := context.Background()
			ig := &v1alpha1.InferenceGraph{
				ObjectMeta: metav1.ObjectMeta{
					Name:      igkey.Name,
					Namespace: igkey.Namespace,
					Annotations: map[string]string{
						"serving.kserve.io/deploymentMode": string(constants.Serverless),
					},
				},
				Spec: v1alpha1.InferenceGraphSpec{
					Nodes: map[string]v1alpha1.InferenceRouter{
						v1alpha1.GraphRootNodeName: {
							RouterType: v1alpha1.Sequence,
							Steps: []v1alpha1.InferenceStep{
								{
									InferenceTarget: v1alpha1.InferenceTarget{
										ServiceURL: "http://someservice.exmaple.com",
									},
								},
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, ig)).Should(Succeed())
			defer k8sClient.Delete(ctx, ig)
			inferenceGraphSubmitted := &v1alpha1.InferenceGraph{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, igkey, inferenceGraphSubmitted)
				if err != nil {
					return false
				}
				return true
			}, timeout, interval).Should(BeTrue())
			fmt.Println(inferenceGraphSubmitted)

			actualKnServiceCreated := &knservingv1.Service{}
			Eventually(func() error {
				return k8sClient.Get(context.TODO(), igkey, actualKnServiceCreated)
			}, timeout).
				Should(Succeed())
			fmt.Println(inferenceGraphSubmitted)

			printJsonAsString(actualKnServiceCreated)

			expectedKnService := &knservingv1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      igkey.Name,
					Namespace: igkey.Namespace,
				},
				Spec: knservingv1.ServiceSpec{
					ConfigurationSpec: knservingv1.ConfigurationSpec{
						Template: knservingv1.RevisionTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{
								Labels: map[string]string{
									"serving.kserve.io/inferencegraph": "singlenode",
								},
								Annotations: map[string]string{
									"autoscaling.knative.dev/min-scale": "1",
									"autoscaling.knative.dev/class":     "kpa.autoscaling.knative.dev",
									"serving.kserve.io/deploymentMode":  "Serverless",
								},
							},
							Spec: knservingv1.RevisionSpec{
								ContainerConcurrency: ptr.Int64(knservingdefaults.DefaultContainerConcurrency),
								TimeoutSeconds:       ptr.Int64(knservingdefaults.DefaultRevisionTimeoutSeconds),
								PodSpec: v1.PodSpec{
									Containers: []v1.Container{
										{
											Image: "kserve/router:v0.10.0",
											Name:  knservingdefaults.DefaultUserContainerName,
											Args: []string{
												"--graph-json",
												"{\"nodes\":{\"root\":{\"routerType\":\"Sequence\",\"steps\":[{\"serviceUrl\":\"http://someservice.exmaple.com\"}]}},\"resources\":{}}",
											},
											Resources: v1.ResourceRequirements{
												Limits: v1.ResourceList{
													v1.ResourceCPU:    resource.MustParse("100m"),
													v1.ResourceMemory: resource.MustParse("500Mi"),
												},
												Requests: v1.ResourceList{
													v1.ResourceCPU:    resource.MustParse("100m"),
													v1.ResourceMemory: resource.MustParse("100Mi"),
												},
											},
										},
									},
								},
							},
						},
					},
				},
			}
			expectedKnService.SetDefaults(context.TODO())
			printJsonAsString(expectedKnService)
			fmt.Println(inferenceGraphSubmitted)
			Expect(kmp.SafeDiff(actualKnServiceCreated.Spec, expectedKnService.Spec)).To(Equal(""))
		})
	})

	Context("When creating an inferencegraph with headers in global config", func() {
		It("Should create a knative service with headers as env var of podspec", func() {
			By("By creating a new InferenceGraph")
			configs["router"] = `{
					  "image": "kserve/router:v0.10.0",
					  "memoryRequest": "100Mi",
					  "memoryLimit": "500Mi",
					  "cpuRequest": "100m",
					  "cpuLimit": "100m",
					  "headers": {
						"propagate": [
						  "Authorization",
						  "Intuit_tid"
						]
					  }
				}`
			var configMap = &v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      constants.InferenceServiceConfigMapName,
					Namespace: constants.KServeNamespace,
				},
				Data: configs,
			}
			Expect(k8sClient.Create(context.TODO(), configMap)).NotTo(HaveOccurred())
			//defer k8sClient.Delete(context.TODO(), configMap)
			graphName := "singlenode"
			var expectedRequest = reconcile.Request{NamespacedName: types.NamespacedName{Name: graphName, Namespace: "default"}}
			var serviceKey = expectedRequest.NamespacedName
			ctx := context.Background()
			ig := &v1alpha1.InferenceGraph{
				ObjectMeta: metav1.ObjectMeta{
					Name:      serviceKey.Name,
					Namespace: serviceKey.Namespace,
					Annotations: map[string]string{
						"serving.kserve.io/deploymentMode": string(constants.Serverless),
					},
				},
				Spec: v1alpha1.InferenceGraphSpec{
					Nodes: map[string]v1alpha1.InferenceRouter{
						v1alpha1.GraphRootNodeName: {
							RouterType: v1alpha1.Sequence,
							Steps: []v1alpha1.InferenceStep{
								{
									InferenceTarget: v1alpha1.InferenceTarget{
										ServiceURL: "http://someservice.exmaple.com",
									},
								},
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, ig)).Should(Succeed())
			defer k8sClient.Delete(ctx, ig)
			inferenceGraphSubmitted := &v1alpha1.InferenceGraph{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, serviceKey, inferenceGraphSubmitted)
				if err != nil {
					return false
				}
				return true
			}, timeout, interval).Should(BeTrue())
			fmt.Println(inferenceGraphSubmitted)

			actualKnServiceCreated := &knservingv1.Service{}
			Eventually(func() error {
				return k8sClient.Get(context.TODO(), serviceKey, actualKnServiceCreated)
			}, timeout).
				Should(Succeed())
			fmt.Println(inferenceGraphSubmitted)

			printJsonAsString(actualKnServiceCreated)

			expectedKnService := &knservingv1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      serviceKey.Name,
					Namespace: serviceKey.Namespace,
				},
				Spec: knservingv1.ServiceSpec{
					ConfigurationSpec: knservingv1.ConfigurationSpec{
						Template: knservingv1.RevisionTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{
								Labels: map[string]string{
									"serving.kserve.io/inferencegraph": "singlenode",
								},
								Annotations: map[string]string{
									"autoscaling.knative.dev/min-scale": "1",
									"autoscaling.knative.dev/class":     "kpa.autoscaling.knative.dev",
									"serving.kserve.io/deploymentMode":  "Serverless",
								},
							},
							Spec: knservingv1.RevisionSpec{
								ContainerConcurrency: ptr.Int64(knservingdefaults.DefaultContainerConcurrency),
								TimeoutSeconds:       ptr.Int64(knservingdefaults.DefaultRevisionTimeoutSeconds),
								PodSpec: v1.PodSpec{
									Containers: []v1.Container{
										{
											Image: "kserve/router:v0.10.0",
											Name:  knservingdefaults.DefaultUserContainerName,
											Env: []v1.EnvVar{
												{
													Name:  "PROPAGATE_HEADERS",
													Value: "Authorization,Intuit_tid",
												},
											},
											Args: []string{
												"--graph-json",
												"{\"nodes\":{\"root\":{\"routerType\":\"Sequence\",\"steps\":[{\"serviceUrl\":\"http://someservice.exmaple.com\"}]}},\"resources\":{}}",
											},
											Resources: v1.ResourceRequirements{
												Limits: v1.ResourceList{
													v1.ResourceCPU:    resource.MustParse("100m"),
													v1.ResourceMemory: resource.MustParse("500Mi"),
												},
												Requests: v1.ResourceList{
													v1.ResourceCPU:    resource.MustParse("100m"),
													v1.ResourceMemory: resource.MustParse("100Mi"),
												},
											},
										},
									},
								},
							},
						},
					},
				},
			}
			expectedKnService.SetDefaults(context.TODO())
			printJsonAsString(expectedKnService)
			fmt.Println(inferenceGraphSubmitted)
			Expect(kmp.SafeDiff(actualKnServiceCreated.Spec, expectedKnService.Spec)).To(Equal(""))
		})
	})

	//Context("When creating an IG with Serverless annotation", func() {
	//	It("Should succeed")
	//})
	//
	//Context("When creating an IG with RawDeployment annotation", func() {
	//	It("Should not succeed")
	//
	//})

})

func printJsonAsString(x interface{}) {
	marshelledKNSvc, err := json.Marshal(x)
	if err == nil {
		fmt.Println("json output is: ", string(marshelledKNSvc))
	} else {
		fmt.Println("Error marshalling")
		fmt.Println(err)
	}
}
