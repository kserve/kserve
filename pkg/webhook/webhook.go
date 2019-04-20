package webhook

import (
	"github.com/kubeflow/kfserving/pkg/apis/serving"
	"github.com/kubeflow/kfserving/pkg/apis/serving/v1alpha1"
	"github.com/kubeflow/kfserving/pkg/webhook/admission/kfservice"
	"k8s.io/api/admissionregistration/v1beta1"
	"k8s.io/apimachinery/pkg/types"

	"sigs.k8s.io/controller-runtime/pkg/manager"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
	webooktypes "sigs.k8s.io/controller-runtime/pkg/webhook/types"
)

var (
	// KFServingWebhookServerName is the name for the webhook server for KFServing resources
	KFServingWebhookServerName = "kfserving-webhook-server"

	// KFServingWebhookServerNamespace is the namespace for the webhook server for KFServing resources
	KFServingWebhookServerNamespace = "kubeflow-system"
)
var log = logf.Log.WithName(KFServingWebhookServerName)

// AddToManager adds all Controllers to the Manager
// +kubebuilder:rbac:groups=admissionregistration.k8s.io,resources=mutatingwebhookconfigurations;validatingwebhookconfigurations,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;create;update;patch;delete
func AddToManager(manager manager.Manager) error {
	server, err := webhook.NewServer("webhook-server", manager, webhook.ServerOptions{
		Port:    9876,
		CertDir: "/tmp/cert",
		BootstrapOptions: &webhook.BootstrapOptions{
			Secret: &types.NamespacedName{
				Namespace: KFServingWebhookServerNamespace,
				Name:      KFServingWebhookServerName + "-secret",
			},
			Service: &webhook.Service{
				Namespace: KFServingWebhookServerNamespace,
				Name:      KFServingWebhookServerName + "-service",
				Selectors: map[string]string{
					"control-plane": "controller-manager",
				},
			},
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

func register(manager manager.Manager, server *webhook.Server) error {
	return server.Register(&admission.Webhook{
		Name: KFServingWebhookServerName + serving.APIGroupName,
		Type: webooktypes.WebhookTypeValidating,
		Rules: []v1beta1.RuleWithOperations{{
			Operations: []v1beta1.OperationType{
				v1beta1.Create,
				v1beta1.Update,
			},
			Rule: v1beta1.Rule{
				APIGroups:   []string{serving.APIGroupName},
				APIVersions: []string{v1alpha1.APIVersion},
				Resources:   []string{v1alpha1.KFServiceAPIName},
			},
		}},
		Handlers: []admission.Handler{
			&kfservice.Validator{
				Client:  manager.GetClient(),
				Decoder: manager.GetAdmissionDecoder(),
			},
		},
	})
}
