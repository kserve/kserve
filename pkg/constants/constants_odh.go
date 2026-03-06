/*
Copyright 2021 The KServe Authors.

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

// InferenceGraph midstream constants
const (
	InferenceGraphAuthCRBName   = "kserve-inferencegraph-auth-verifiers"
	InferenceGraphFinalizerName = "inferencegraph.finalizers"
)

// Midstream annotation keys
var (
	OVMSAutoVersioningAnnotationKey = "storage.kserve.io/ovms-auto-versioning"
)

// Midstream networking constants
const (
	ODHKserveRawAuth               = "security.opendatahub.io/enable-auth"
	ODHRouteEnabled                = "exposed"
	ServingCertSecretSuffix        = "-serving-cert"
	OpenshiftServingCertAnnotation = "service.beta.openshift.io/serving-cert-secret-name"
)

// Midstream container names
const (
	OVMSVersioningContainerName = "ovms-auto-versioning"
)

// Midstream auth proxy constants
const (
	OauthProxyPort                  = 8443
	OauthProxyProbePort             = 8643
	OauthProxyResourceMemoryLimit   = "128Mi"
	OauthProxyResourceCPULimit      = "200m"
	OauthProxyResourceMemoryRequest = "64Mi"
	OauthProxyResourceCPURequest    = "100m"
	OauthProxySARCMName             = "kube-rbac-proxy-sar-config"
	// Used for test purposes
	OauthProxyImage       = "quay.io/opendatahub/odh-kube-auth-proxy@sha256:dcb09fbabd8811f0956ef612a0c9ddd5236804b9bd6548a0647d2b531c9d01b3"
	DefaultServiceAccount = "default"
	KubeRbacContainerName = "kube-rbac-proxy"
)

// OpenShift constants
const (
	OpenShiftServiceCaConfigMapName = "openshift-service-ca.crt"
)

type ResourceType string

const (
	InferenceServiceResource ResourceType = "InferenceService"
	InferenceGraphResource   ResourceType = "InferenceGraph"
)

func init() {
	ServiceAnnotationDisallowedList = append(ServiceAnnotationDisallowedList, ODHKserveRawAuth)
}
