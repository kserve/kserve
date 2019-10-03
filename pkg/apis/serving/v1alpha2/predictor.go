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
	"regexp"
	"strings"

	"github.com/kubeflow/kfserving/pkg/constants"
	v1 "k8s.io/api/core/v1"
	resource "k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/klog"
)

type Predictor interface {
	GetStorageUri() string
	GetContainer(modelName string, config *PredictorsConfig) *v1.Container
	ApplyDefaults()
	Validate() error
}

const (
	// ExactlyOnePredictorViolatedError is a known error message
	ExactlyOnePredictorViolatedError = "Exactly one of [Custom, ONNX, Tensorflow, TensorRT, SKLearn, XGBoost] must be specified in PredictorSpec"
)

var (
	DefaultMemory = resource.MustParse("2Gi")
	DefaultCPU    = resource.MustParse("1")
)

// Returns a URI to the model. This URI is passed to the storage-initializer via the StorageInitializerSourceUriInternalAnnotationKey
func (p *PredictorSpec) GetStorageUri() string {
	return getPredictor(p).GetStorageUri()
}

func (p *PredictorSpec) GetContainer(modelName string, config *PredictorsConfig) *v1.Container {
	return getPredictor(p).GetContainer(modelName, config)
}

func (p *PredictorSpec) ApplyDefaults() {
	getPredictor(p).ApplyDefaults()
}

func (p *PredictorSpec) Validate() error {
	predictor, err := makePredictor(p)
	if err != nil {
		return err
	}
	if err := predictor.Validate(); err != nil {
		return err
	}
	if err := validateStorageURI(p.GetStorageUri()); err != nil {
		return err
	}
	if err := validateReplicas(p.MinReplicas, p.MaxReplicas); err != nil {
		return err
	}
	return nil
}

type PredictorConfig struct {
	ContainerImage string `json:"image"`

	//TODO add readiness/liveness probe config
}
type PredictorsConfig struct {
	Tensorflow PredictorConfig `json:"tensorflow,omitempty"`
	TensorRT   PredictorConfig `json:"tensorrt,omitempty"`
	Xgboost    PredictorConfig `json:"xgboost,omitempty"`
	SKlearn    PredictorConfig `json:"sklearn,omitempty"`
	PyTorch    PredictorConfig `json:"pytorch,omitempty"`
	ONNX       PredictorConfig `json:"onnx,omitempty"`
}

func validateStorageURI(storageURI string) error {
	if storageURI == "" {
		return nil
	}

	// local path (not some protocol?)
	if !regexp.MustCompile("\\w+?://").MatchString(storageURI) {
		return nil
	}

	// one of the prefixes we know?
	for _, prefix := range SupportedStorageURIPrefixList {
		if strings.HasPrefix(storageURI, prefix) {
			return nil
		}
	}

	azureURIMatcher := regexp.MustCompile(AzureBlobURIRegEx)
	if parts := azureURIMatcher.FindStringSubmatch(storageURI); parts != nil {
		return nil
	}

	return fmt.Errorf(UnsupportedStorageURIFormatError, strings.Join(SupportedStorageURIPrefixList, ", "), storageURI)
}

func validateReplicas(minReplicas int, maxReplicas int) error {
	if minReplicas < 0 {
		return fmt.Errorf(MinReplicasLowerBoundExceededError)
	}
	if maxReplicas < 0 {
		return fmt.Errorf(MaxReplicasLowerBoundExceededError)
	}
	if minReplicas > maxReplicas && maxReplicas != 0 {
		return fmt.Errorf(MinReplicasShouldBeLessThanMaxError)
	}
	return nil
}

func setResourceRequirementDefaults(requirements *v1.ResourceRequirements) {
	if requirements.Requests == nil {
		requirements.Requests = v1.ResourceList{}
	}

	if _, ok := requirements.Requests[v1.ResourceCPU]; !ok {
		requirements.Requests[v1.ResourceCPU] = DefaultCPU
	}
	if _, ok := requirements.Requests[v1.ResourceMemory]; !ok {
		requirements.Requests[v1.ResourceMemory] = DefaultMemory
	}

	if requirements.Limits == nil {
		requirements.Limits = v1.ResourceList{}
	}

	if _, ok := requirements.Limits[v1.ResourceCPU]; !ok {
		requirements.Limits[v1.ResourceCPU] = DefaultCPU
	}
	if _, ok := requirements.Limits[v1.ResourceMemory]; !ok {
		requirements.Limits[v1.ResourceMemory] = DefaultMemory
	}
}

func isGPUEnabled(requirements v1.ResourceRequirements) bool {
	_, ok := requirements.Limits[constants.NvidiaGPUResourceType]
	return ok
}

func getPredictor(modelSpec *PredictorSpec) Predictor {
	predictor, err := makePredictor(modelSpec)
	if err != nil {
		klog.Fatal(err)
	}

	return predictor
}

func makePredictor(predictorSpec *PredictorSpec) (Predictor, error) {
	handlers := []Predictor{}
	if predictorSpec.Custom != nil {
		handlers = append(handlers, predictorSpec.Custom)
	}
	if predictorSpec.XGBoost != nil {
		handlers = append(handlers, predictorSpec.XGBoost)
	}
	if predictorSpec.SKLearn != nil {
		handlers = append(handlers, predictorSpec.SKLearn)
	}
	if predictorSpec.Tensorflow != nil {
		handlers = append(handlers, predictorSpec.Tensorflow)
	}
	if predictorSpec.ONNX != nil {
		handlers = append(handlers, predictorSpec.ONNX)
	}
	if predictorSpec.PyTorch != nil {
		handlers = append(handlers, predictorSpec.PyTorch)
	}
	if predictorSpec.TensorRT != nil {
		handlers = append(handlers, predictorSpec.TensorRT)
	}
	if len(handlers) != 1 {
		return nil, fmt.Errorf(ExactlyOnePredictorViolatedError)
	}
	return handlers[0], nil
}
