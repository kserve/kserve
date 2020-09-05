package v1alpha2

import (
	"github.com/golang/protobuf/proto"
	"github.com/google/go-cmp/cmp"
	"github.com/kubeflow/kfserving/pkg/apis/serving/v1beta1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"testing"
)

func TestInferenceServiceConversion(t *testing.T) {
	scenarios := map[string]struct {
		v1alpha2spec *InferenceService
		v1beta1Spec  *v1beta1.InferenceService
	}{
		"transformer": {
			v1alpha2spec: &InferenceService{
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
								Parallelism: 1,
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
								Parallelism: 1,
							},
							Custom: &CustomSpec{
								Container: v1.Container{
									Name:  "kfserving-container",
									Image: "transformer:v1",
									Resources: v1.ResourceRequirements{
										Requests: v1.ResourceList{
											v1.ResourceCPU:    resource.MustParse("1"),
											v1.ResourceMemory: resource.MustParse("2Gi"),
										},
										Limits: v1.ResourceList{
											v1.ResourceCPU:    resource.MustParse("1"),
											v1.ResourceMemory: resource.MustParse("2Gi"),
										},
									},
								},
							},
						},
					},
				},
			},
			v1beta1Spec: &v1beta1.InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "conversionTest",
					Namespace: "default",
				},
				Spec: v1beta1.InferenceServiceSpec{
					Predictor: v1beta1.PredictorSpec{
						ComponentExtensionSpec: v1beta1.ComponentExtensionSpec{
							MinReplicas:          GetIntReference(1),
							MaxReplicas:          3,
							ContainerConcurrency: proto.Int64(1),
						},
						Tensorflow: &v1beta1.TFServingSpec{
							PredictorExtensionSpec: v1beta1.PredictorExtensionSpec{
								StorageURI:     proto.String("s3://test/mnist/export"),
								RuntimeVersion: proto.String("1.13.0"),
							},
						},
					},
					Transformer: &v1beta1.TransformerSpec{
						ComponentExtensionSpec: v1beta1.ComponentExtensionSpec{
							MinReplicas:          GetIntReference(1),
							MaxReplicas:          3,
							ContainerConcurrency: proto.Int64(1),
						},
						CustomTransformer: &v1beta1.CustomTransformer{
							PodTemplateSpec: v1.PodTemplateSpec{
								Spec: v1.PodSpec{
									Containers: []v1.Container{
										{
											Name:  "kfserving-container",
											Image: "transformer:v1",
											Resources: v1.ResourceRequirements{
												Requests: v1.ResourceList{
													v1.ResourceCPU:    resource.MustParse("1"),
													v1.ResourceMemory: resource.MustParse("2Gi"),
												},
												Limits: v1.ResourceList{
													v1.ResourceCPU:    resource.MustParse("1"),
													v1.ResourceMemory: resource.MustParse("2Gi"),
												},
											},
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
	for name, scenario := range scenarios {
		t.Run(name, func(t *testing.T) {
			dst := &v1beta1.InferenceService{}
			scenario.v1alpha2spec.ConvertTo(dst)
			if cmp.Diff(scenario.v1beta1Spec, dst) != "" {
				t.Errorf("diff: %s", cmp.Diff(scenario.v1beta1Spec, dst))
			}
			v1alpha2ExpectedSpec := &InferenceService{}
			v1alpha2ExpectedSpec.ConvertFrom(scenario.v1beta1Spec)
			if cmp.Diff(scenario.v1alpha2spec, v1alpha2ExpectedSpec) != "" {
				t.Errorf("diff: %s", cmp.Diff(scenario.v1alpha2spec, v1alpha2ExpectedSpec))
			}
		})
	}
}
