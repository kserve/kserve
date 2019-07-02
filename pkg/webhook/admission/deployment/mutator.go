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
package deployment

import (
	"context"
	"encoding/json"
	"net/http"

	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"

	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	k8types "k8s.io/apimachinery/pkg/types"

	"github.com/kubeflow/kfserving/pkg/constants"
	"github.com/kubeflow/kfserving/pkg/controller/kfservice/resources/credentials"
	"github.com/kubeflow/kfserving/pkg/webhook/third_party"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission/types"
)

var log = logf.Log.WithName("kfserving-admission-mutator")

// Mutator is a webhook that injects incoming pods
type Mutator struct {
	Client  client.Client
	Decoder types.Decoder
}

// Handle decodes the incoming Pod and executes mutation logic.
func (mutator *Mutator) Handle(ctx context.Context, req types.Request) types.Response {
	deployment := &appsv1.Deployment{}

	if err := mutator.Decoder.Decode(req, deployment); err != nil {
		return admission.ErrorResponse(http.StatusBadRequest, err)
	}

	configMap := &v1.ConfigMap{}
	err := mutator.Client.Get(context.TODO(), k8types.NamespacedName{Name: constants.KFServiceConfigMapName, Namespace: constants.KFServingNamespace}, configMap)
	if err != nil {
		log.Error(err, "Failed to find config map", "name", constants.KFServiceConfigMapName)
		return admission.ErrorResponse(http.StatusBadRequest, err)
	}

	if err := mutator.mutate(deployment, configMap); err != nil {
		return admission.ErrorResponse(http.StatusInternalServerError, err)
	}

	patch, err := json.Marshal(deployment)
	if err != nil {
		return admission.ErrorResponse(http.StatusInternalServerError, err)
	}

	return third_party.PatchResponseFromRaw(req.AdmissionRequest.Object.Raw, patch)
}

func (mutator *Mutator) mutate(deployment *appsv1.Deployment, configMap *v1.ConfigMap) error {

	credentialBuilder := credentials.NewCredentialBulder(mutator.Client, configMap)
	modelInitializer := &ModelInitializerInjector{
		credentialBuilder: credentialBuilder,
	}

	mutators := []func(deployment *appsv1.Deployment) error{
		InjectGKEAcceleratorSelector,
		modelInitializer.InjectModelInitializer,
	}

	for _, mutator := range mutators {
		if err := mutator(deployment); err != nil {
			return err
		}
	}

	return nil
}
