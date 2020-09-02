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
	"github.com/go-logr/logr"
	"github.com/kubeflow/kfserving/pkg/constants"
	"github.com/kubeflow/kfserving/pkg/controller/v1beta1/inferenceservice/reconcilers/knative"
	"github.com/kubeflow/kfserving/pkg/credentials"
	"github.com/kubeflow/kfserving/pkg/utils"
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

func NewPredictor(client client.Client, scheme *runtime.Scheme, inferenceServiceConfig *v1beta1.InferenceServicesConfig) Component {
	return &Predictor{
		client:                 client,
		scheme:                 scheme,
		inferenceServiceConfig: inferenceServiceConfig,
		Log:                    ctrl.Log.WithName("PredictorReconciler"),
	}
}

// Reconcile observes the world and attempts to drive the status towards the desired state.
func (p *Predictor) Reconcile(isvc *v1beta1.InferenceService) error {
	p.Log.Info("Reconciling Predictor", "PredictorSpec", isvc.Spec)
	predictor := isvc.Spec.Predictor.GetImplementation()
	annotations := utils.Filter(isvc.Annotations, func(key string) bool {
		return !utils.Includes(constants.ServiceAnnotationDisallowedList, key)
	})
	// KNative does not support INIT containers or mounting, so we add annotations that trigger the
	// StorageInitializer injector to mutate the underlying deployment to provision model data
	if sourceURI := predictor.GetStorageUri(); sourceURI != nil {
		annotations[constants.StorageInitializerSourceUriInternalAnnotationKey] = *sourceURI
	}
	objectMeta := metav1.ObjectMeta{
		Name:      isvc.Name + "-" + string(v1beta1.PredictorComponent),
		Namespace: isvc.Namespace,
		Labels: utils.Union(isvc.Labels, map[string]string{
			constants.InferenceServicePodLabelKey: isvc.Name,
			constants.KServiceComponentLabel:      string(v1beta1.PredictorComponent),
		}),
		Annotations: annotations,
	}
	if isvc.Spec.Predictor.CustomPredictor == nil {
		container := predictor.GetContainer(isvc.ObjectMeta, isvc.Spec.Predictor.GetExtensions(), p.inferenceServiceConfig)
		isvc.Spec.Predictor.CustomPredictor = &v1beta1.CustomPredictor{
			PodTemplateSpec: v1.PodTemplateSpec{
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						*container,
					},
				},
			},
		}
	} else {
		container := predictor.GetContainer(isvc.ObjectMeta, isvc.Spec.Transformer.GetExtensions(), p.inferenceServiceConfig)
		isvc.Spec.Predictor.CustomPredictor.Spec.Containers[0] = *container
	}
	// Here we allow switch between knative and vanilla deployment
	r := knative.NewKsvcReconciler(p.client, p.scheme, objectMeta, &isvc.Spec.Predictor.ComponentExtensionSpec,
		&isvc.Spec.Predictor.CustomPredictor.Spec)

	if err := controllerutil.SetControllerReference(isvc, r.Service, p.scheme); err != nil {
		return err
	}
	if status, err := r.Reconcile(); err != nil {
		return err
	} else {
		isvc.Status.PropagateStatus(v1beta1.PredictorComponent, status)
		return nil
	}
}
