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
	"context"
	"errors"
	"fmt"
	"regexp"

	utils "github.com/kserve/kserve/pkg/utils"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

const (
	// InvalidGraphNameFormatError defines the error message for invalid inference graph name
	InvalidGraphNameFormatError = "The InferenceGraph \"%s\" is invalid: a InferenceGraph name must consist of lower case alphanumeric characters or '-', and must start with alphabetical character. (e.g. \"my-name\" or \"abc-123\", regex used for validation is '%s')"
	// RootNodeNotFoundError defines the error message for root node not found
	RootNodeNotFoundError = "root node not found, InferenceGraph needs a node with name 'root' as the root node of the graph"
	// WeightNotProvidedError defines the error message for traffic weight is nil for inference step
	WeightNotProvidedError = "InferenceGraph[%s] Node[%s] Route[%s] missing the 'Weight'"
	// InvalidWeightError defines the error message for sum of traffic weight is not 100
	InvalidWeightError = "InferenceGraph[%s] Node[%s] splitter node: the sum of traffic weights for all routing targets should be 100"
	// DuplicateStepNameError defines the error message for more than one step contains same name
	DuplicateStepNameError = "Node \"%s\" of InferenceGraph \"%s\" contains more than one step with name \"%s\""
	// TargetNotProvidedError defines the error message for inference graph target not specified
	TargetNotProvidedError = "Step %d (\"%s\") in node \"%s\" of InferenceGraph \"%s\" does not specify an inference target"
	// InvalidTargetError defines the error message for inference graph target specifies more than one of nodeName, serviceName, serviceUrl
	InvalidTargetError = "Step %d (\"%s\") in node \"%s\" of InferenceGraph \"%s\" specifies more than one of nodeName, serviceName, serviceUrl"
)

const (
	// GraphNameFmt regular expressions for validation of isvc name
	GraphNameFmt string = "[a-z]([-a-z0-9]*[a-z0-9])?"
)

var (
	// logger for the validation webhook.
	validatorLogger = logf.Log.WithName("inferencegraph-v1alpha1-validation-webhook")
	// GraphRegexp regular expressions for validation of graph name
	GraphRegexp = regexp.MustCompile("^" + GraphNameFmt + "$")
)

// +kubebuilder:object:generate=false
// +k8s:openapi-gen=false
// InferenceGraphValidator is responsible for setting default values on the InferenceGraph resources
// when created or updated.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as it is used only for temporary operations and does not need to be deeply copied.
type InferenceGraphValidator struct{}

// +kubebuilder:webhook:verbs=create;update,path=/validate-inferencegraph,mutating=false,failurePolicy=fail,groups=serving.kserve.io,resources=pods,versions=v1alpha1,name=inferencegraph.kserve-webhook-server.validator

var _ webhook.CustomValidator = &InferenceGraphValidator{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (v *InferenceGraphValidator) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	ig, err := utils.Convert[*InferenceGraph](obj)
	if err != nil {
		validatorLogger.Error(err, "Unable to convert object to InferenceGraph")
		return nil, err
	}
	validatorLogger.Info("validate create", "name", ig.Name)
	return validateInferenceGraph(ig)
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (v *InferenceGraphValidator) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	ig, err := utils.Convert[*InferenceGraph](newObj)
	if err != nil {
		validatorLogger.Error(err, "Unable to convert object to InferenceGraph")
		return nil, err
	}
	validatorLogger.Info("validate update", "name", ig.Name)
	return validateInferenceGraph(ig)
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (v *InferenceGraphValidator) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	ig, err := utils.Convert[*InferenceGraph](obj)
	if err != nil {
		validatorLogger.Error(err, "Unable to convert object to InferenceGraph")
		return nil, err
	}
	validatorLogger.Info("validate delete", "name", ig.Name)
	return nil, nil
}

func validateInferenceGraph(ig *InferenceGraph) (admission.Warnings, error) {
	if err := validateInferenceGraphName(ig); err != nil {
		return nil, err
	}

	if err := validateInferenceGraphRouterRoot(ig); err != nil {
		return nil, err
	}

	if err := validateInferenceGraphStepNameUniqueness(ig); err != nil {
		return nil, err
	}

	if err := validateInferenceGraphSingleStepTargets(ig); err != nil {
		return nil, err
	}

	if err := validateInferenceGraphSplitterWeight(ig); err != nil {
		return nil, err
	}
	return nil, nil
}

// Validation of unique step names
func validateInferenceGraphStepNameUniqueness(ig *InferenceGraph) error {
	nodes := ig.Spec.Nodes
	for nodeName, node := range nodes {
		nameSet := sets.NewString()
		for _, route := range node.Steps {
			if route.StepName != "" {
				if nameSet.Has(route.StepName) {
					return fmt.Errorf(DuplicateStepNameError,
						nodeName, ig.Name, route.StepName)
				}
				nameSet.Insert(route.StepName)
			}
		}
	}
	return nil
}

// Validation of single step inference targets
func validateInferenceGraphSingleStepTargets(ig *InferenceGraph) error {
	nodes := ig.Spec.Nodes
	for nodeName, node := range nodes {
		for i, route := range node.Steps {
			target := route.InferenceTarget
			count := 0
			if target.NodeName != "" {
				count += 1
			}
			if target.ServiceName != "" {
				count += 1
			}
			if target.ServiceURL != "" {
				count += 1
			}
			if count == 0 {
				return fmt.Errorf(TargetNotProvidedError, i, route.StepName, nodeName, ig.Name)
			}
			if count != 1 {
				return fmt.Errorf(InvalidTargetError, i, route.StepName, nodeName, ig.Name)
			}
		}
	}
	return nil
}

// Validation of inference graph name
func validateInferenceGraphName(ig *InferenceGraph) error {
	if !GraphRegexp.MatchString(ig.Name) {
		return fmt.Errorf(InvalidGraphNameFormatError, ig.Name, GraphNameFmt)
	}
	return nil
}

// Validation of inference graph router root
func validateInferenceGraphRouterRoot(ig *InferenceGraph) error {
	nodes := ig.Spec.Nodes
	for name := range nodes {
		if name == GraphRootNodeName {
			return nil
		}
	}
	return errors.New(RootNodeNotFoundError)
}

// Validation of inference graph router type
func validateInferenceGraphSplitterWeight(ig *InferenceGraph) error {
	nodes := ig.Spec.Nodes
	for name, node := range nodes {
		weight := 0
		if node.RouterType == Splitter {
			for _, route := range node.Steps {
				if route.Weight == nil {
					return fmt.Errorf(WeightNotProvidedError, ig.Name, name, route.ServiceName)
				}
				weight += int(*route.Weight)
			}
			if weight != 100 {
				return fmt.Errorf(InvalidWeightError, ig.Name, name)
			}
		}
	}
	return nil
}
