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
	"context"
	"github.com/go-logr/logr"
	"github.com/kubeflow/kfserving/pkg/constants"
	"github.com/kubeflow/kfserving/pkg/controller/v1beta1/inferenceservice/reconcilers/knative"
	"github.com/kubeflow/kfserving/pkg/credentials"
	"github.com/kubeflow/kfserving/pkg/utils"
	"github.com/pkg/errors"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	knservingv1 "knative.dev/serving/pkg/apis/serving/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/kubeflow/kfserving/pkg/apis/serving/v1beta1"
)

var _ Component = &Explainer{}

// Explainer reconciles resources for this component.
type Explainer struct {
	client                 client.Client
	scheme                 *runtime.Scheme
	inferenceServiceConfig *v1beta1.InferenceServicesConfig
	credentialBuilder      *credentials.CredentialBuilder
	Log                    logr.Logger
}

func NewExplainer(client client.Client, scheme *runtime.Scheme, inferenceServiceConfig *v1beta1.InferenceServicesConfig) Component {
	return &Explainer{
		client:                 client,
		scheme:                 scheme,
		inferenceServiceConfig: inferenceServiceConfig,
		Log:                    ctrl.Log.WithName("ExplainerReconciler"),
	}
}

// Reconcile observes the explainer and attempts to drive the status towards the desired state.
func (p *Explainer) Reconcile(isvc *v1beta1.InferenceService) error {
	p.Log.Info("Reconciling Explainer", "ExplainerSpec", isvc.Spec.Explainer)
	explainer := isvc.Spec.Explainer.GetImplementation()
	annotations := utils.Filter(isvc.Annotations, func(key string) bool {
		return !utils.Includes(constants.ServiceAnnotationDisallowedList, key)
	})
	// KNative does not support INIT containers or mounting, so we add annotations that trigger the
	// StorageInitializer injector to mutate the underlying deployment to provision model data
	if sourceURI := explainer.GetStorageUri(); sourceURI != nil {
		annotations[constants.StorageInitializerSourceUriInternalAnnotationKey] = *sourceURI
	}
	hasInferenceLogging := addLoggerAnnotations(isvc.Spec.Explainer.Logger, annotations)
	existing := &knservingv1.Service{}
	explainerName := constants.ExplainerServiceName(isvc.Name)
	predictorName := constants.PredictorServiceName(isvc.Name)
	err := p.client.Get(context.TODO(), types.NamespacedName{Name: constants.DefaultExplainerServiceName(isvc.Name), Namespace: isvc.Namespace}, existing)
	if err == nil {
		explainerName = constants.DefaultExplainerServiceName(isvc.Name)
		predictorName = constants.DefaultPredictorServiceName(isvc.Name)
	}
	objectMeta := metav1.ObjectMeta{
		Name:      explainerName,
		Namespace: isvc.Namespace,
		Labels: utils.Union(isvc.Labels, map[string]string{
			constants.InferenceServicePodLabelKey: isvc.Name,
			constants.KServiceComponentLabel:      string(v1beta1.ExplainerComponent),
		}),
		Annotations: annotations,
	}
	if len(isvc.Spec.Explainer.PodSpec.Containers) == 0 {
		container := explainer.GetContainer(isvc.ObjectMeta, isvc.Spec.Explainer.GetExtensions(), p.inferenceServiceConfig,
			predictorName)
		isvc.Spec.Explainer.PodSpec = v1beta1.PodSpec{
			Containers: []v1.Container{
				*container,
			},
		}
	} else {
		container := explainer.GetContainer(isvc.ObjectMeta, isvc.Spec.Explainer.GetExtensions(), p.inferenceServiceConfig,
			predictorName)
		isvc.Spec.Explainer.PodSpec.Containers[0] = *container
	}
	if hasInferenceLogging {
		addAgentContainerPort(&isvc.Spec.Explainer.PodSpec.Containers[0])
	}

	podSpec := v1.PodSpec(isvc.Spec.Explainer.PodSpec)
	r := knative.NewKsvcReconciler(p.client, p.scheme, objectMeta, &isvc.Spec.Explainer.ComponentExtensionSpec,
		&podSpec, isvc.Status.Components[v1beta1.ExplainerComponent])

	if err := controllerutil.SetControllerReference(isvc, r.Service, p.scheme); err != nil {
		return errors.Wrapf(err, "fails to set owner reference for explainer")
	}
	status, err := r.Reconcile()
	if err != nil {
		return errors.Wrapf(err, "fails to reconcile explainer")
	}
	isvc.Status.PropagateStatus(v1beta1.ExplainerComponent, status)
	return nil
}
