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

func DefaultModelServiceName(name string) string {
	return name + "-default-model"
}

func CanaryModelServiceName(name string) string {
	return name + "-canary-model"
}
