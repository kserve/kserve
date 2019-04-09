package tensorflow

import (
	"github.com/kubeflow/kfserving/pkg/apis/serving/v1alpha1"
	"k8s.io/api/core/v1"
)

const (
	TensorflowEntrypointCommand = "/usr/bin/tensorflow_model_server"
	TensorflowServingPort       = "9000"
	TensorflowServingRestPort   = "8080"
)

func CreateTensorflowContainer(tfSpec *v1alpha1.TensorflowSpec, modelName string) *v1.Container {
	//TODO(@yuzisun) add configmap for default resources, readiness/liveness probe
	return &v1.Container{
		Image:   "tensorflow/serving:" + tfSpec.RuntimeVersion,
		Command: []string{TensorflowEntrypointCommand},
		Args: []string{
			"--port=" + TensorflowServingPort,
			"--rest_api_port=" + TensorflowServingRestPort,
			"--model_name=" + modelName,
			"--model_base_path=" + tfSpec.ModelUri,
		},
	}
}
