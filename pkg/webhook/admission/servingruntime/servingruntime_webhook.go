/*
Copyright 2023 The KServe Authors.

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

package servingruntime

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"github.com/kserve/kserve/pkg/constants"
	"github.com/kserve/kserve/pkg/utils"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

var log = logf.Log.WithName(constants.ServingRuntimeValidatorWebhookName)

const (
	InvalidPriorityError                       = "same priority assigned for the model format %s"
	InvalidPriorityServingRuntimeError         = "%s in the servingruntimes %s and %s in namespace %s"
	InvalidPriorityClusterServingRuntimeError  = "%s in the clusterservingruntimes %s and %s"
	ProrityIsNotSameError                      = "different priorities assigned for the model format %s"
	ProrityIsNotSameServingRuntimeError        = "%s under the servingruntime %s"
	ProrityIsNotSameClusterServingRuntimeError = "%s under the clusterservingruntime %s"
	InvalidWorkerSpecSizeValueError            = "the WorkerSpec.Size cannot be less than 2(\"%d\")"
	InvalidPipelineParallelSizeValueError      = "the PIPELINE_PARALLE_SIZE cannot be less than 2(\"%s\")"
	InvalidTensorParallelSizeValueError        = "the TENSOR_PARALLE_SIZE cannot be less than 1(\"%s\")"
	InvalidParallelSizeValueError              = "the value of PIPELINE_PARALLEL_SIZE or TENSOR_PARALLEL_SIZE is incorrect"
	InvalidMultiNodeEnvVariablesError          = "the \"%s\" \"%s\" is invalid: \"%s\""
)

// +kubebuilder:webhook:verbs=create;update,path=/validate-serving-kserve-io-v1alpha1-clusterservingruntime,mutating=false,failurePolicy=fail,groups=serving.kserve.io,resources=clusterservingruntimes,versions=v1alpha1,name=clusterservingruntime.kserve-webhook-server.validator

type ClusterServingRuntimeValidator struct {
	Client  client.Client
	Decoder admission.Decoder
}

// +kubebuilder:webhook:verbs=create;update,path=/validate-serving-kserve-io-v1alpha1-servingruntime,mutating=false,failurePolicy=fail,groups=serving.kserve.io,resources=servingruntimes,versions=v1alpha1,name=servingruntime.kserve-webhook-server.validator

type ServingRuntimeValidator struct {
	Client  client.Client
	Decoder admission.Decoder
}

func (sr *ServingRuntimeValidator) Handle(ctx context.Context, req admission.Request) admission.Response {
	servingRuntime := &v1alpha1.ServingRuntime{}
	if err := sr.Decoder.Decode(req, servingRuntime); err != nil {
		log.Error(err, "Failed to decode serving runtime", "name", servingRuntime.Name, "namespace", servingRuntime.Namespace)
		return admission.Errored(http.StatusBadRequest, err)
	}

	ExistingRuntimes := &v1alpha1.ServingRuntimeList{}
	if err := sr.Client.List(context.TODO(), ExistingRuntimes, client.InNamespace(servingRuntime.Namespace)); err != nil {
		log.Error(err, "Failed to get serving runtime list", "namespace", servingRuntime.Namespace)
		return admission.Errored(http.StatusInternalServerError, err)
	}

	// Only validate for priority if the new serving runtime is not disabled
	if servingRuntime.Spec.IsDisabled() {
		return admission.Allowed("")
	}

	for i := range ExistingRuntimes.Items {
		if err := validateModelFormatPrioritySame(&servingRuntime.Spec); err != nil {
			return admission.Denied(fmt.Sprintf(ProrityIsNotSameServingRuntimeError, err.Error(), servingRuntime.Name))
		}

		if err := validateServingRuntimePriority(&servingRuntime.Spec, &ExistingRuntimes.Items[i].Spec, servingRuntime.Name, ExistingRuntimes.Items[i].Name); err != nil {
			return admission.Denied(fmt.Sprintf(InvalidPriorityServingRuntimeError, err.Error(), ExistingRuntimes.Items[i].Name, servingRuntime.Name, servingRuntime.Namespace))
		}
		if err := validateMultiNodeVariables(&servingRuntime.Spec); err != nil {
			return admission.Denied(fmt.Sprintf(InvalidMultiNodeEnvVariablesError, servingRuntime.Kind, servingRuntime.Name, err.Error()))
		}
	}

	return admission.Allowed("")
}

// Handle validates the incoming request
func (csr *ClusterServingRuntimeValidator) Handle(ctx context.Context, req admission.Request) admission.Response {
	clusterServingRuntime := &v1alpha1.ClusterServingRuntime{}
	if err := csr.Decoder.Decode(req, clusterServingRuntime); err != nil {
		log.Error(err, "Failed to decode cluster serving runtime", "name", clusterServingRuntime.Name)
		return admission.Errored(http.StatusBadRequest, err)
	}

	ExistingRuntimes := &v1alpha1.ClusterServingRuntimeList{}
	if err := csr.Client.List(context.TODO(), ExistingRuntimes); err != nil {
		log.Error(err, "Failed to get cluster serving runtime list")
		return admission.Errored(http.StatusInternalServerError, err)
	}

	// Only validate for priority if the new cluster serving runtime is not disabled
	if clusterServingRuntime.Spec.IsDisabled() {
		return admission.Allowed("")
	}

	for i := range ExistingRuntimes.Items {
		if err := validateModelFormatPrioritySame(&clusterServingRuntime.Spec); err != nil {
			return admission.Denied(fmt.Sprintf(ProrityIsNotSameClusterServingRuntimeError, err.Error(), clusterServingRuntime.Name))
		}
		if err := validateServingRuntimePriority(&clusterServingRuntime.Spec, &ExistingRuntimes.Items[i].Spec, clusterServingRuntime.Name, ExistingRuntimes.Items[i].Name); err != nil {
			return admission.Denied(fmt.Sprintf(InvalidPriorityClusterServingRuntimeError, err.Error(), ExistingRuntimes.Items[i].Name, clusterServingRuntime.Name))
		}
		if err := validateMultiNodeVariables(&clusterServingRuntime.Spec); err != nil {
			return admission.Denied(fmt.Sprintf(InvalidMultiNodeEnvVariablesError, clusterServingRuntime.Kind, clusterServingRuntime.Name, err.Error()))
		}
	}
	return admission.Allowed("")
}

func areSupportedModelFormatsEqual(m1 v1alpha1.SupportedModelFormat, m2 v1alpha1.SupportedModelFormat) bool {
	if strings.EqualFold(m1.Name, m2.Name) && ((m1.Version == nil && m2.Version == nil) ||
		(m1.Version != nil && m2.Version != nil && *m1.Version == *m2.Version)) {
		return true
	}
	return false
}

func validateModelFormatPrioritySame(newSpec *v1alpha1.ServingRuntimeSpec) error {
	nameToPriority := make(map[string]*int32)

	// Validate when same model format has same priority under same runtime.
	// If the same model format has different prority value then throws the error
	for _, newModelFormat := range newSpec.SupportedModelFormats {
		// Only validate priority if autoselect is ture
		if newModelFormat.IsAutoSelectEnabled() {
			if existingPriority, ok := nameToPriority[newModelFormat.Name]; ok {
				if existingPriority != nil && newModelFormat.Priority != nil && (*existingPriority != *newModelFormat.Priority) {
					return fmt.Errorf(ProrityIsNotSameError, newModelFormat.Name)
				}
			} else {
				nameToPriority[newModelFormat.Name] = newModelFormat.Priority
			}
		}
	}
	return nil
}

func validateServingRuntimePriority(newSpec *v1alpha1.ServingRuntimeSpec, existingSpec *v1alpha1.ServingRuntimeSpec, existingRuntimeName string, newRuntimeName string) error {
	// Skip the runtime if it is disabled or both are not multi model runtime and in update scenario skip the existing runtime if it is same as the new runtime
	if (newSpec.IsMultiModelRuntime() != existingSpec.IsMultiModelRuntime()) || (existingSpec.IsDisabled()) || (existingRuntimeName == newRuntimeName) {
		return nil
	}
	// Only validate for priority if both servingruntimes supports the same protocol version
	isTheProtocolSame := false
	for _, protocolVersion := range existingSpec.ProtocolVersions {
		if slices.Contains(newSpec.ProtocolVersions, protocolVersion) {
			isTheProtocolSame = true
			break
		}
	}
	if isTheProtocolSame {
		for _, existingModelFormat := range existingSpec.SupportedModelFormats {
			for _, newModelFormat := range newSpec.SupportedModelFormats {
				// Only validate priority if autoselect is ture
				if existingModelFormat.IsAutoSelectEnabled() && newModelFormat.IsAutoSelectEnabled() && areSupportedModelFormatsEqual(existingModelFormat, newModelFormat) {
					if existingModelFormat.Priority != nil && newModelFormat.Priority != nil && *existingModelFormat.Priority == *newModelFormat.Priority {
						return fmt.Errorf(InvalidPriorityError, newModelFormat.Name)
					}
				}
			}
		}
	}
	return nil
}

func validateMultiNodeVariables(newSpec *v1alpha1.ServingRuntimeSpec) error {
	if newSpec.WorkerSpec != nil {
		for i, container := range newSpec.Containers {
			if container.Name == constants.InferenceServiceContainerName {
				if envPipelineParallelSize, exists := utils.GetEnvVarValue(newSpec.Containers[i].Env, constants.PipelineParallelSizeEnvName); exists {
					// pipelineParallelSize should be bigger than 1 (head + worker)
					if intPipelineParallelSize, err := strconv.Atoi(envPipelineParallelSize); err == nil && intPipelineParallelSize < 2 {
						return fmt.Errorf(InvalidPipelineParallelSizeValueError, envPipelineParallelSize)
					} else {
						return errors.New(InvalidParallelSizeValueError)
					}
				}

				if envTensorParallelSize, exists := utils.GetEnvVarValue(newSpec.Containers[i].Env, constants.TensorParallelSizeEnvName); exists {
					// GPU resource should be bigger than 1.
					if intTensorParallelSize, err := strconv.Atoi(envTensorParallelSize); err == nil && intTensorParallelSize < 1 {
						return fmt.Errorf(InvalidTensorParallelSizeValueError, envTensorParallelSize)
					} else {
						return errors.New(InvalidParallelSizeValueError)
					}
				}
			}
		}

		// WorkerSpec.Size should be bigger than 0.
		WorkerSpecSize := newSpec.WorkerSpec.Size
		if WorkerSpecSize < 1 {
			return fmt.Errorf(InvalidWorkerSpecSizeValueError, WorkerSpecSize)
		}

	}
	return nil
}
