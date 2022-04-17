/*
Copyright 2022 The KServe Authors.

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

	"regexp"

	"k8s.io/apimachinery/pkg/runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

const (
	// InvalidGraphNameFormatError defines the error message for invalid inference graph name
	InvalidGraphNameFormatError = "The InferenceGraph \"%s\" is invalid: a InferenceGraph name must consist of lower case alphanumeric characters or '-', and must start with alphabetical character. (e.g. \"my-name\" or \"abc-123\", regex used for validation is '%s')"
)

const (
	// GraphNameFmt regular expressions for validation of isvc name
	GraphNameFmt string = "[a-z]([-a-z0-9]*[a-z0-9])?"
)

var (
	// logger for the validation webhook.
	validatorLogger = logf.Log.WithName("inferencegraph-v1alpha1-validation-webhook")
	//GraphRegexp regular expressions for validation of graph name
	GraphRegexp = regexp.MustCompile("^" + GraphNameFmt + "$")
)

// +kubebuilder:webhook:verbs=create;update,path=/validate-inferencegraph,mutating=false,failurePolicy=fail,groups=serving.kserve.io,resources=pods,versions=v1alpha1,name=inferencegraph.kserve-webhook-server.validator

var _ webhook.Validator = &InferenceGraph{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (ig *InferenceGraph) ValidateCreate() error {
	validatorLogger.Info("validate create", "name", ig.Name)

	if err := validateInferenceGraphName(ig); err != nil {
		return err
	}

	if err := validateInferenceGraphRouterType(ig); err != nil {
		return err
	}

	if err := validateInferenceGraphSplitterWeight(ig); err != nil {
		return err
	}
	return nil
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (ig *InferenceGraph) ValidateUpdate(old runtime.Object) error {
	validatorLogger.Info("validate update", "name:", ig.Name)

	return ig.ValidateCreate()
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (ig *InferenceGraph) ValidateDelete() error {
	validatorLogger.Info("validate delete", "name", ig.Name)
	return nil
}

// Validation of inference graph name
func validateInferenceGraphName(ig *InferenceGraph) error {
	if !GraphRegexp.MatchString(ig.Name) {
		return fmt.Errorf(InvalidGraphNameFormatError, ig.Name, GraphNameFmt)
	}
	return nil
}

//Validation of inference graph router type
func validateInferenceGraphRouterType(ig *InferenceGraph) error {
	nodes := ig.Spec.Nodes
	for name, node := range nodes {
		if node.RouterType != Single && node.RouterType != Splitter &&
			node.RouterType != Ensemble && node.RouterType != Switch {
			return fmt.Errorf("InferenceGraph[%s] Node[%s] RouterType[%s] is not supported, InferenceGraph supports RouterType List['Single', 'Splitter', 'Ensemble', 'Switch']",
				ig.Name, name, node.RouterType)
		}
	}
	return nil
}

//Validation of inference graph router type
func validateInferenceGraphSplitterWeight(ig *InferenceGraph) error {
	nodes := ig.Spec.Nodes
	for name, node := range nodes {
		weight := 0
		if node.RouterType == Splitter {
			for _, route := range node.Routes {
				if route.Weight == nil {
					return fmt.Errorf("InferenceGraph[%s] Node[%s] Route[%s] missing the 'Weight'.", ig.Name, name, route.Service)
				}
				weight += int(*route.Weight)
			}
			if weight != 100 {
				return fmt.Errorf("InferenceGraph[%s] Node[%s] splitter node: the sum of traffic weights for all routing targets should be 100", ig.Name, name)
			}
		}
	}
	return nil
}
