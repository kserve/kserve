/*
Copyright 2020 kubeflow.org.
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

package components

import (
	"fmt"
	"github.com/go-logr/logr"
	"github.com/kubeflow/kfserving/pkg/constants"
	"github.com/kubeflow/kfserving/pkg/controller/v1beta1/inferenceservice/reconcilers/knative"
	"github.com/kubeflow/kfserving/pkg/credentials"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/kubeflow/kfserving/pkg/apis/serving/v1beta1"
)

var _ Component = &Predictor{}

// Predictor reconciles resources for this component.
type Predictor struct {
	client                 client.Client
	scheme                 *runtime.Scheme
	inferenceServiceConfig *v1beta1.InferenceServicesConfig
	credentialBuilder      *credentials.CredentialBuilder
	Log                    logr.Logger
}

func NewPredictor(client client.Client, scheme *runtime.Scheme, config *v1.ConfigMap) Component {
	inferenceServiceConfig, err := v1beta1.NewInferenceServicesConfig()
	if err != nil {
		fmt.Printf("Failed to get inference service config %s", err)
		panic("Failed to get inference service config")
	}
	return &Predictor{
		client:                 client,
		scheme:                 scheme,
		inferenceServiceConfig: inferenceServiceConfig,
		Log:                    ctrl.Log.WithName("PredictorReconciler"),
	}
}

// Reconcile observes the world and attempts to drive the status towards the desired state.
func (p *Predictor) Reconcile(isvc *v1beta1.InferenceService) error {
	propagateStatusFn := isvc.Status.PropagateStatus
	p.Log.Info("Reconciling Predictor", "PredictorSpec", isvc.Spec)
	predictor := (&isvc.Spec.Predictor).GetImplementation()

	// KNative does not support INIT containers or mounting, so we add annotations that trigger the
	// StorageInitializer injector to mutate the underlying deployment to provision model data
	if sourceURI := predictor.GetStorageUri(); sourceURI != nil {
		isvc.Annotations[constants.StorageInitializerSourceUriInternalAnnotationKey] = *sourceURI
	}

	if isvc.Spec.Predictor.CustomPredictor == nil {
		container := predictor.GetContainer(isvc.ObjectMeta, isvc.Spec.Predictor.GetExtensions(), p.inferenceServiceConfig)
		isvc.Spec.Predictor.CustomPredictor = &v1beta1.CustomPredictor{
			PodTemplateSpec: v1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{},
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						*container,
					},
				},
			},
		}
	}
	r := knative.NewKsvcReconciler(p.client, p.scheme, &isvc.ObjectMeta, v1beta1.PredictorComponent, &isvc.Spec.Predictor.ComponentExtensionSpec,
		&isvc.Spec.Predictor.CustomPredictor.Spec)

	if err := controllerutil.SetControllerReference(isvc, r.Service, p.scheme); err != nil {
		return err
	}
	if status, err := r.Reconcile(); err != nil {
		return err
	} else {
		propagateStatusFn(v1beta1.PredictorComponent, status)
		return nil
	}
}
