package kservemodule

import "k8s.io/apimachinery/pkg/runtime/schema"

var unownedGroupKinds = map[schema.GroupKind]struct{}{
	{Group: llmISVCConfigGroup, Kind: llmISVCConfigKind}: {},
}

const (
	// Component names
	KserveComponentName             = "kserve"
	OdhModelControllerComponentName = "modelcontroller"
	WVAComponentName                = "wva"
	ModelCacheComponentName         = "modelcache"
	ObservabilityComponentName      = "observability"
	ConsoleDashboardsComponentName  = "console-dashboards"

	// Manifest source paths
	KserveManifestSourcePath        = "overlays/odh"
	KserveManifestSourcePathXKS     = "overlays/odh-xks"
	KserveCRDManifestSourcePath     = "overlays/odh-crds"
	ModelCacheManifestSourcePath    = "overlays/odh-modelcache"
	ModelControllerSourcePath       = "base"
	WVAManifestSourcePathOCP        = "overlays/namespace-scoped/openshift"
	ObservabilityManifestSourcePath      = "monitoring/llmisvc/dashboards"
	ConsoleDashboardsManifestSourcePath = "monitoring/llmisvc/dashboards-odc"

	// Deployment names
	kserveControllerDeployment     = "kserve-controller-manager"
	llmISVCControllerDeployment    = "llmisvc-controller-manager"
	localmodelControllerDeployment = "kserve-localmodel-controller-manager"
	odhModelControllerDeployment   = "odh-model-controller"
	wvaControllerDeployment        = "workload-variant-autoscaler-controller-manager"

	// Console dashboards target namespace
	consoleDashboardsNamespace = "openshift-config-managed"

	// SSA field manager
	fieldOwner = "kserve-module-controller"

	// Platform version ConfigMap
	platformVersionConfigMap    = "odh-kserve-config"
	platformVersionConfigMapKey = "platformVersion"

	// ConfigMap keys
	kserveConfigMapName     = "inferenceservice-config"
	ingressConfigKeyName    = "ingress"
	serviceConfigKeyName    = "service"
	configHashAnnotationKey = "kserve-module/config-hash"
	oauthProxyConfigKeyName = "oauthProxy"
	openshiftConfigKeyName  = "openshiftConfig"

	// LLMInferenceServiceConfig versioning
	wellKnownAnnotationKey   = "serving.kserve.io/well-known-config"
	wellKnownAnnotationValue = "true"
	llmISVCConfigPrefixEnv   = "LLM_INFERENCE_SERVICE_CONFIG_PREFIX"
	llmISVCConfigGroup       = "serving.kserve.io"
	llmISVCConfigKind        = "LLMInferenceServiceConfig"

	// Template (ServingRuntime) resource type
	templateGroup = "template.openshift.io"
	templateKind  = "Template"

	// ModuleFinalizerName is managed by the module operator on the Kserve CR;
	// added during reconcile, removed after cleanup on deletion.
	ModuleFinalizerName = "kserve-module.opendatahub.io/finalizer"

	// PlatformFinalizerName is set and removed by the platform operator.
	PlatformFinalizerName = "platform.opendatahub.io/finalizer"

	// cert-manager defaults
	defaultCAIssuerName  = "opendatahub-ca-issuer"
	defaultIssuerRefKind = "ClusterIssuer"
	defaultCertName      = "opendatahub-ca"
	// defaultCertManagerNS is the fallback when dynamic detection fails.
	// Prefer CA_SECRET_NAMESPACE env var or runtime discovery.
	defaultCertManagerNS   = "cert-manager"
	defaultIstioCACertPath = "/var/run/secrets/opendatahub/ca.crt"

	// Well-known cert-manager namespace candidates for discovery.
	certManagerNSCandidate        = "cert-manager"
	certManagerOperatorNS         = "cert-manager-operator"
	certManagerWebhookDeployment  = "cert-manager-webhook"
)
