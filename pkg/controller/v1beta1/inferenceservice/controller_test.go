package inferenceservice

import (
	"context"
	"fmt"
	"github.com/kubeflow/kfserving/pkg/apis/serving/v1alpha2"
	"github.com/kubeflow/kfserving/pkg/apis/serving/v1beta1"
	"github.com/kubeflow/kfserving/pkg/constants"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	knservingv1 "knative.dev/serving/pkg/apis/serving/v1"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"time"
)

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
        "tensorflow" : {
            "image" : "tensorflow/serving"
        },
        "sklearn" : {
            "image" : "kfserving/sklearnserver"
        },
        "xgboost" : {
            "image" : "kfserving/xgbserver"
        }
	}`,
			"explainers": `{
        "alibi": {
            "image" : "kfserving/alibi-explainer",
			"defaultImageVersion": "latest",
			"allowedImageVersions": [
				"latest"
			 ]
        }
	}`,
			"ingress": `{
        "ingressGateway" : "knative-serving/knative-ingress-gateway",
        "ingressService" : "test-destination"
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
			var storageUri = "s3://test/mnist/export"
			ctx := context.Background()
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
							PredictorExtensionSpec: v1beta1.PredictorExtensionSpec{
								StorageURI: &storageUri,
								Container: v1.Container{
									Name: "kfs",
								},
							},
						},
					},
				},
			}

			Expect(k8sClient.Create(ctx, instance)).Should(Succeed())
			inferenceService := &v1beta1.InferenceService{}

			// We'll need to retry getting this newly created CronJob, given that creation may not immediately happen.
			Eventually(func() bool {
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
})
