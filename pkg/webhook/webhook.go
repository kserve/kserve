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

package webhook

import (
	"github.com/kubeflow/kfserving/pkg/apis/serving/v1alpha2"
	"github.com/kubeflow/kfserving/pkg/constants"
	"github.com/kubeflow/kfserving/pkg/webhook/admission/inferenceservice"
	"github.com/kubeflow/kfserving/pkg/webhook/admission/pod"
	"k8s.io/api/admissionregistration/v1beta1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"sigs.k8s.io/controller-runtime/pkg/manager"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
	webhooktypes "sigs.k8s.io/controller-runtime/pkg/webhook/types"
)

var log = logf.Log.WithName(constants.WebhookServerName)

// AddToManager adds all Controllers to the Manager
// +kubebuilder:rbac:groups=admissionregistration.k8s.io,resources=mutatingwebhookconfigurations;validatingwebhookconfigurations,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=namespaces,verbs=get;list;watch
func AddToManager(manager manager.Manager) error {
	server, err := webhook.NewServer(constants.WebhookServerName, manager, webhook.ServerOptions{
		Port:    9876,
		CertDir: "/tmp/cert",
		BootstrapOptions: &webhook.BootstrapOptions{
			Secret: &types.NamespacedName{
				Namespace: constants.KFServingNamespace,
				Name:      constants.WebhookServerSecretName,
			},
			Service: &webhook.Service{
				Namespace: constants.KFServingNamespace,
				Name:      constants.WebhookServerServiceName,
				Selectors: map[string]string{
					"control-plane": constants.ControllerLabelName,
				},
			},
			ValidatingWebhookConfigName: constants.InferenceServiceValidatingWebhookConfigName,
			MutatingWebhookConfigName:   constants.InferenceServiceMutatingWebhookConfigName,
		},
	})
	if err != nil {
		return err
	}

	if err := register(manager, server); err != nil {
		return err
	}

	return nil
}

// In 1.13, replace with the following webhook generators: ValidatingWebhookFor, DefaultingWebhookFor
// https://github.com/kubernetes-sigs/controller-runtime/blob/master/pkg/webhook/admission/validator.go#L35
// https://github.com/kubernetes-sigs/controller-runtime/blob/master/pkg/webhook/admission/defaulter.go#L34
func register(manager manager.Manager, server *webhook.Server) error {
	nsSelector := &metav1.LabelSelector{
		MatchLabels: map[string]string{
			constants.InferenceServicePodLabelKey: constants.EnableKFServingMutatingWebhook,
		},
	}
	return server.Register(&admission.Webhook{
		Name:          constants.InferenceServiceValidatingWebhookName,
		FailurePolicy: &constants.WebhookFailurePolicy,
		Type:          webhooktypes.WebhookTypeValidating,
		Rules: []v1beta1.RuleWithOperations{{
			Operations: []v1beta1.OperationType{
				v1beta1.Create,
				v1beta1.Update,
			},
			Rule: v1beta1.Rule{
				APIGroups:   []string{constants.KFServingAPIGroupName},
				APIVersions: []string{v1alpha2.APIVersion},
				Resources:   []string{constants.InferenceServiceAPIName},
			},
		}},
		Handlers: []admission.Handler{
			&inferenceservice.Validator{
				Client:  manager.GetClient(),
				Decoder: manager.GetAdmissionDecoder(),
			},
		},
	}, &admission.Webhook{
		Name:          constants.InferenceServiceDefaultingWebhookName,
		FailurePolicy: &constants.WebhookFailurePolicy,
		Type:          webhooktypes.WebhookTypeMutating,
		Rules: []v1beta1.RuleWithOperations{{
			Operations: []v1beta1.OperationType{
				v1beta1.Create,
				v1beta1.Update,
			},
			Rule: v1beta1.Rule{
				APIGroups:   []string{constants.KFServingAPIGroupName},
				APIVersions: []string{v1alpha2.APIVersion},
				Resources:   []string{constants.InferenceServiceAPIName},
			},
		}},
		Handlers: []admission.Handler{
			&inferenceservice.Defaulter{
				Client:  manager.GetClient(),
				Decoder: manager.GetAdmissionDecoder(),
			},
		},
	}, &admission.Webhook{
		Name:              constants.PodMutatorWebhookName,
		FailurePolicy:     &constants.WebhookFailurePolicy,
		Type:              webhooktypes.WebhookTypeMutating,
		NamespaceSelector: nsSelector,
		Rules: []v1beta1.RuleWithOperations{{
			Operations: []v1beta1.OperationType{
				v1beta1.Create,
				v1beta1.Update,
			},
			Rule: v1beta1.Rule{
				APIGroups:   []string{v1.GroupName},
				APIVersions: []string{v1.SchemeGroupVersion.Version},
				Resources:   []string{"pods"},
			},
		}},
		Handlers: []admission.Handler{
			&pod.Mutator{
				Client:  manager.GetClient(),
				Decoder: manager.GetAdmissionDecoder(),
			},
		},
	})
}
