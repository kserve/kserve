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
	"github.com/kserve/kserve/pkg/constants"
	"net/http"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"

	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

var log = logf.Log.WithName(constants.ServingRuntimeValidatorWebhookName)

const (
	InvalidPriorityError                      = "Same priority assigned for the model format %s"
	InvalidPriorityServingRuntimeError        = "%s in the servingruntimes %s and %s in namespace %s"
	InvalidPriorityClusterServingRuntimeError = "%s in the clusterservingruntimes %s and %s"
)

// +kubebuilder:webhook:verbs=create;update,path=/validate-serving-kserve-io-v1alpha1-clusterservingruntime,mutating=false,failurePolicy=fail,groups=serving.kserve.io,resources=clusterservingruntimes,versions=v1alpha1,name=clusterservingruntime.kserve-webhook-server.validator

type ClusterServingRuntimeValidator struct {
	Client  client.Client
	Decoder *admission.Decoder
}

// +kubebuilder:webhook:verbs=create;update,path=/validate-serving-kserve-io-v1alpha1-servingruntime,mutating=false,failurePolicy=fail,groups=serving.kserve.io,resources=servingruntimes,versions=v1alpha1,name=servingruntime.kserve-webhook-server.validator

type ServingRuntimeValidator struct {
	Client  client.Client
	Decoder *admission.Decoder
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

	for _, runtime := range ExistingRuntimes.Items {
		if err := validateServingRuntimePriority(&servingRuntime.Spec, &runtime.Spec, servingRuntime.Name, runtime.Name); err != nil {
			return admission.Denied(fmt.Sprintf(InvalidPriorityServingRuntimeError, err.Error(), runtime.Name, servingRuntime.Name, servingRuntime.Namespace))
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

	for _, runtime := range ExistingRuntimes.Items {
		if err := validateServingRuntimePriority(&clusterServingRuntime.Spec, &runtime.Spec, clusterServingRuntime.Name, runtime.Name); err != nil {
			return admission.Denied(fmt.Sprintf(InvalidPriorityClusterServingRuntimeError, err.Error(), runtime.Name, clusterServingRuntime.Name))
		}
	}
	return admission.Allowed("")
}

func validateServingRuntimePriority(newSpec *v1alpha1.ServingRuntimeSpec, existingSpec *v1alpha1.ServingRuntimeSpec, existingRuntimeName string, newRuntimeName string) error {
	// Skip the runtime if it is disabled or both are not multi model runtime or the stale runtime in the api server
	if (newSpec.IsMultiModelRuntime() != existingSpec.IsMultiModelRuntime()) || (existingSpec.IsDisabled()) || (existingRuntimeName == newRuntimeName) {
		return nil
	}
	// Only validate for priority if both servingruntimes supports the same protocol version
	isTheProtocolSame := false
	for _, protocolVersion := range existingSpec.ProtocolVersions {
		if contains(newSpec.ProtocolVersions, protocolVersion) {
			isTheProtocolSame = true
			break
		}
	}
	if isTheProtocolSame {
		for _, existingModelFormat := range existingSpec.SupportedModelFormats {
			for _, newModelFormat := range newSpec.SupportedModelFormats {
				// Only validate priority if autoselect is ture
				if (existingModelFormat.Name == newModelFormat.Name && existingModelFormat.IsAutoSelectEnabled() && newModelFormat.IsAutoSelectEnabled()) &&
					((existingModelFormat.Version == nil && newModelFormat.Version == nil) ||
						(existingModelFormat.Version != nil && newModelFormat.Version != nil && *existingModelFormat.Version == *newModelFormat.Version)) {
					if existingModelFormat.Priority != nil && newModelFormat.Priority != nil && *existingModelFormat.Priority == *newModelFormat.Priority {
						return errors.New(fmt.Sprintf(InvalidPriorityError, newModelFormat.Name))
					}
				}
			}
		}
	}
	return nil
}

func contains[T comparable](slice []T, element T) bool {
	for _, v := range slice {
		if v == element {
			return true
		}
	}
	return false
}

// InjectClient injects the client.
func (csr *ClusterServingRuntimeValidator) InjectClient(c client.Client) error {
	csr.Client = c
	return nil
}

// ClusterServingRuntimeValidator implements admission.DecoderInjector.
// A decoder will be automatically injected.

// InjectDecoder injects the decoder.
func (csr *ClusterServingRuntimeValidator) InjectDecoder(d *admission.Decoder) error {
	csr.Decoder = d
	return nil
}

// InjectClient injects the client.
func (sr *ServingRuntimeValidator) InjectClient(c client.Client) error {
	sr.Client = c
	return nil
}

// ServingRuntimeValidator implements admission.DecoderInjector.
// A decoder will be automatically injected.

// InjectDecoder injects the decoder.
func (sr *ServingRuntimeValidator) InjectDecoder(d *admission.Decoder) error {
	sr.Decoder = d
	return nil
}
