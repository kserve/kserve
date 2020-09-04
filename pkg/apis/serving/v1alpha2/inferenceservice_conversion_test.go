package v1alpha2

import (
	"fmt"
	"github.com/kubeflow/kfserving/pkg/apis/serving/v1beta1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"testing"
)

func TestInferenceServiceConversion(t *testing.T) {
	isvc := InferenceService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "conversionTest",
			Namespace: "default",
		},
		Spec: InferenceServiceSpec{
			Default: EndpointSpec{
				Predictor: PredictorSpec{
					DeploymentSpec: DeploymentSpec{
						MinReplicas: GetIntReference(1),
						MaxReplicas: 3,
					},
					Tensorflow: &TensorflowSpec{
						StorageURI:     "s3://test/mnist/export",
						RuntimeVersion: "1.13.0",
					},
				},
				Transformer: &TransformerSpec{
					DeploymentSpec: DeploymentSpec{
						MinReplicas: GetIntReference(1),
						MaxReplicas: 3,
					},
					Custom: &CustomSpec{
						Container: v1.Container{
							Image: "transformer:v1",
						},
					},
				},
			},
		},
	}
	dst := &v1beta1.InferenceService{}
	isvc.ConvertTo(dst)
	fmt.Printf("%+v", dst.Spec.Predictor)
}
