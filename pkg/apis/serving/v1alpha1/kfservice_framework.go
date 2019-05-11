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

package v1alpha1

import (
	"fmt"
	"log"

	v1 "k8s.io/api/core/v1"
)

type FrameworkHandler interface {
	CreateModelServingContainer(modelName string) *v1.Container
	Validate() error
}

const (
	// ExactlyOneModelSpecViolatedError is a known error message
	ExactlyOneModelSpecViolatedError = "Exactly one of [Custom, Tensorflow, ScikitLearn, XGBoost] must be specified in ModelSpec"
	// AtLeastOneModelSpecViolatedError is a known error message
	AtLeastOneModelSpecViolatedError = "At least one of [Custom, Tensorflow, ScikitLearn, XGBoost] must be specified in ModelSpec"
)

func (m *ModelSpec) CreateModelServingContainer(modelName string) *v1.Container {
	handler, err := makeHandler(m)
	if err != nil {
		log.Fatal(err)
	}

	return handler.CreateModelServingContainer(modelName)
}

func (m *ModelSpec) Validate() error {
	_, err := makeHandler(m)
	return err
}

func makeHandler(modelSpec *ModelSpec) (interface{ FrameworkHandler }, error) {
	handlers := []interface{ FrameworkHandler }{}
	if modelSpec.Custom != nil {
		handlers = append(handlers, modelSpec.Custom)
	}
	if modelSpec.XGBoost != nil {
		// TODO: add fwk for xgboost
	}
	if modelSpec.ScikitLearn != nil {
		// TODO: add fwk for sklearn
	}
	if modelSpec.Tensorflow != nil {
		handlers = append(handlers, modelSpec.Tensorflow)
	}
	if len(handlers) == 0 {
		return nil, fmt.Errorf(AtLeastOneModelSpecViolatedError)
	}
	if len(handlers) != 1 {
		return nil, fmt.Errorf(ExactlyOneModelSpecViolatedError)
	}
	return handlers[0], nil
}

func init() {
	SchemeBuilder.Register(&KFService{}, &KFServiceList{})
}
