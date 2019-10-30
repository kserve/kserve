/*
Copyright 2019 kubeflow.org.
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

package v1alpha2

import (
	"fmt"

	"github.com/kubeflow/kfserving/pkg/constants"
	v1 "k8s.io/api/core/v1"
	"k8s.io/klog"
)

type Predictor interface {
	GetResourceRequirements() *v1.ResourceRequirements
	GetStorageUri() string
	GetContainer(modelName string, config *InferenceServicesConfig) *v1.Container
	ApplyDefaults(config *InferenceServicesConfig)
	Validate(config *InferenceServicesConfig) error
}

const (
	// ExactlyOnePredictorViolatedError is a known error message
	ExactlyOnePredictorViolatedError = "Exactly one of [Custom, ONNX, Tensorflow, TensorRT, SKLearn, XGBoost] must be specified in PredictorSpec"
)

// Returns a URI to the model. This URI is passed to the storage-initializer via the StorageInitializerSourceUriInternalAnnotationKey
func (p *PredictorSpec) GetStorageUri() string {
	predictor, err := getPredictor(p)
	if err != nil {
		return ""
	}
	return predictor.GetStorageUri()
}

func (p *PredictorSpec) GetContainer(modelName string, config *InferenceServicesConfig) *v1.Container {
	predictor, err := getPredictor(p)
	if err != nil {
		return nil
	}
	return predictor.GetContainer(modelName, config)
}

func (p *PredictorSpec) ApplyDefaults(config *InferenceServicesConfig) {
	predictor, err := getPredictor(p)
	if err == nil {
		predictor.ApplyDefaults(config)
	}
}

func (p *PredictorSpec) Validate(config *InferenceServicesConfig) error {
	predictor, err := getPredictor(p)
	if err != nil {
		return err
	}
	errs := []error{
		predictor.Validate(config),
		validateStorageURI(p.GetStorageUri()),
		validateReplicas(p.MinReplicas, p.MaxReplicas),
		validateResourceRequirements(predictor.GetResourceRequirements()),
	}
	for _, err := range errs {
		if err != nil {
			return err
		}
	}

	return nil
}

func isGPUEnabled(requirements v1.ResourceRequirements) bool {
	_, ok := requirements.Limits[constants.NvidiaGPUResourceType]
	return ok
}

func getPredictor(predictorSpec *PredictorSpec) (Predictor, error) {
	predictors := []Predictor{}
	if predictorSpec.Custom != nil {
		predictors = append(predictors, predictorSpec.Custom)
	}
	if predictorSpec.XGBoost != nil {
		predictors = append(predictors, predictorSpec.XGBoost)
	}
	if predictorSpec.SKLearn != nil {
		predictors = append(predictors, predictorSpec.SKLearn)
	}
	if predictorSpec.Tensorflow != nil {
		predictors = append(predictors, predictorSpec.Tensorflow)
	}
	if predictorSpec.ONNX != nil {
		predictors = append(predictors, predictorSpec.ONNX)
	}
	if predictorSpec.PyTorch != nil {
		predictors = append(predictors, predictorSpec.PyTorch)
	}
	if predictorSpec.TensorRT != nil {
		predictors = append(predictors, predictorSpec.TensorRT)
	}
	if len(predictors) != 1 {
		err := fmt.Errorf(ExactlyOnePredictorViolatedError)
		klog.Error(err)
		return nil, err
	}
	return predictors[0], nil
}
