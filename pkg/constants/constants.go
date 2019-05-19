package constants

import (
	"os"
	"strings"

	"k8s.io/api/admissionregistration/v1beta1"
)

// KFServing Constants
var (
	KFServingName         = "kfserving"
	KFServingAPIGroupName = "serving.kubeflow.org"
	KFServingNamespace    = getEnvOrDefault("POD_NAMESPACE", "kfserving-system")
)

// KFService Constants
var (
	KFServiceName        = "kfservice"
	KFServiceAPIName     = "kfservices"
	KFServicePodLabelKey = KFServingAPIGroupName + "/" + KFServiceName
)

// Controller Constants
var (
	ControllerLabelName        = KFServingName + "-controller-manager"
	DefaultTimeout       int64 = 300
	DefaultScalingTarget       = "1"
)

// Webhook Constants
var (
	WebhookServerName              = KFServingName + "-webhook-server"
	WebhookServerServiceName       = WebhookServerName + "-service"
	WebhookServerSecretName        = WebhookServerName + "-secret"
	KFServiceValidatingWebhookName = strings.Join([]string{KFServiceName, WebhookServerName, "validator"}, ".")
	KFServiceDefaultingWebhookName = strings.Join([]string{KFServiceName, WebhookServerName, "defaulter"}, ".")
	WebhookFailurePolicy           = v1beta1.Fail
)

func getEnvOrDefault(key string, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

func DefaultConfigurationName(name string) string {
	return name + "-default"
}

func CanaryConfigurationName(name string) string {
	return name + "-canary"
}
