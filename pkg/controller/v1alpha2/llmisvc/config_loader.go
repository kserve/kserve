/*
Copyright 2025 The KServe Authors.

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

package llmisvc

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	"github.com/kserve/kserve/pkg/constants"
	"github.com/kserve/kserve/pkg/credentials"
	"github.com/kserve/kserve/pkg/types"
)

const (
	// schedulerConfigMapKey is the key in the inferenceservice-config ConfigMap
	// that holds scheduler-specific configuration (annotation keys, etc.).
	schedulerConfigMapKey = "scheduler"
)

// DefaultExpirationAnnotations is the default list of annotation keys
// checked for certificate secret expiration. The first entry is the write key.
var DefaultExpirationAnnotations = []string{"certificates.kserve.io/expiration"}

// SchedulerConfig holds configurable settings for the scheduler component,
// parsed from the "scheduler" key in the inferenceservice-config ConfigMap.
//
// Certificate annotation keys control the self-signed TLS certificate lifecycle:
//
//   - ExpirationAnnotations: first entry is the write key (set on new secrets),
//     all entries are read keys (checked for expiration, first match wins).
//
// Defaults (upstream - no ConfigMap override needed, can default to values shown below):
//
//	{"expirationAnnotations": ["certificates.kserve.io/expiration"]}
//
// Override example (reads both old and new keys during upgrade):
//
//	{"expirationAnnotations": ["certificates.kserve.io/expiration-v2", "certificates.kserve.io/expiration"]}
//
// During a rolling upgrade, existing secrets carrying the old annotation key are
// recognized via the read list; no unnecessary cert regeneration or secret updates.
// A one-time scheduler restart on upgrade is expected (Recreate strategy).
type SchedulerConfig struct {
	// ExpirationAnnotations is the ordered list of annotation keys checked when
	// determining whether a certificate secret has expired. The first entry is the
	// write key (set on newly created secrets); all entries are read keys.
	ExpirationAnnotations []string `json:"expirationAnnotations,omitempty"`
}

// NewSchedulerConfig parses the "scheduler" key from the inferenceservice-config
// ConfigMap, applying defaults for any missing fields.
func NewSchedulerConfig(isvcConfigMap *corev1.ConfigMap) (*SchedulerConfig, error) {
	cfg := &SchedulerConfig{}
	if raw, ok := isvcConfigMap.Data[schedulerConfigMapKey]; ok {
		if err := json.Unmarshal([]byte(raw), cfg); err != nil {
			return nil, fmt.Errorf("unable to parse scheduler config json: %w", err)
		}
	}

	if len(cfg.ExpirationAnnotations) == 0 {
		cfg.ExpirationAnnotations = DefaultExpirationAnnotations
	}

	return cfg, nil
}

// Config holds configuration needed for LLM inference services.
// It aggregates ingress, storage, credential, and autoscaling settings from the KServe configmap.
type Config struct {
	SystemNamespace         string `json:"systemNamespace,omitempty"`
	IngressGatewayName      string `json:"ingressGatewayName,omitempty"`
	IngressGatewayNamespace string `json:"ingressGatewayNamespace,omitempty"`
	UrlScheme               string `json:"urlScheme,omitempty"`
	EnableTLS               bool   `json:"enableTLS,omitempty"`

	// WVAAutoscalingConfig holds Prometheus and monitoring settings for WVA autoscaling.
	// nil when the "autoscaling-wva-controller-config" key is not present in inferenceservice-config.
	WVAAutoscalingConfig *WVAAutoscalingConfig `json:"-"`

	// Storage and credential configs are excluded from JSON serialization
	// as they contain sensitive information
	StorageConfig    *types.StorageInitializerConfig `json:"-"`
	CredentialConfig *credentials.CredentialConfig   `json:"-"`
	SchedulerConfig  *SchedulerConfig                `json:"-"`
}

// PrometheusConfig holds Prometheus connection and authentication settings used by KEDA
// to query the wva_desired_replicas metric.
type PrometheusConfig struct {
	// URL is the URL of the Prometheus server (used by KEDA to query wva_desired_replicas).
	URL string `json:"url"`
	// TLSInsecureSkipVerify disables TLS certificate verification for the Prometheus connection.
	TLSInsecureSkipVerify bool `json:"tlsInsecureSkipVerify"`
	// AuthModes is the KEDA authModes value for the Prometheus trigger
	// (e.g. "bearer", "basic", "tls"). Empty means no authentication.
	// See: https://keda.sh/docs/latest/scalers/prometheus/#authentication-parameters
	// +optional
	AuthModes string `json:"authModes,omitempty"`
	// TriggerAuthName is the name of a pre-existing TriggerAuthentication or
	// ClusterTriggerAuthentication CR that KEDA should use when querying Prometheus.
	// The CR must be created by the cluster admin before enabling KEDA autoscaling.
	// +optional
	TriggerAuthName string `json:"triggerAuthName,omitempty"`
	// TriggerAuthKind specifies the kind of the authentication CR referenced by
	// TriggerAuthName. Accepted values are "TriggerAuthentication" (namespaced)
	// and "ClusterTriggerAuthentication" (cluster-scoped). Defaults to "TriggerAuthentication"
	// when empty. ClusterTriggerAuthentication is recommended for multi-namespace deployments.
	// +optional
	TriggerAuthKind string `json:"triggerAuthKind,omitempty"`
}

// WVAAutoscalingConfig holds cluster-wide WVA autoscaling settings loaded from the
// "autoscaling-wva-controller-config" key in the inferenceservice-config ConfigMap.
// These are shared across all LLMISVC instances.
type WVAAutoscalingConfig struct {
	// Prometheus holds Prometheus connection and authentication settings.
	Prometheus PrometheusConfig `json:"prometheus"`
}

// autoscalingConfigName is the key in the inferenceservice-config ConfigMap
// that holds WVA-specific autoscaling controller configuration.
const autoscalingConfigName = "autoscaling-wva-controller-config"

// NewConfig creates an instance of llm-specific config based on predefined values
// in IngressConfig struct
func NewConfig(ingressConfig *v1beta1.IngressConfig, storageConfig *types.StorageInitializerConfig, credentialConfig *credentials.CredentialConfig, schedulerConfig *SchedulerConfig) *Config {
	igwNs := constants.KServeNamespace
	igwName := ingressConfig.KserveIngressGateway
	// Parse gateway name to extract namespace and name components
	// Format can be either "gateway-name" or "namespace/gateway-name"
	igw := strings.Split(igwName, "/")
	if len(igw) == 2 {
		igwNs = igw[0]
		igwName = igw[1]
	}

	return &Config{
		SystemNamespace:         constants.KServeNamespace,
		IngressGatewayNamespace: igwNs,
		IngressGatewayName:      igwName,
		UrlScheme:               ingressConfig.UrlScheme,
		EnableTLS:               ingressConfig.EnableLLMInferenceServiceTLS,
		StorageConfig:           storageConfig,
		CredentialConfig:        credentialConfig,
		SchedulerConfig:         schedulerConfig,
	}
}

// LoadConfig loads configuration from the KServe configmap in the cluster
// It fetches and converts the configmap into structured config objects needed by LLM services
func LoadConfig(ctx context.Context, clientset kubernetes.Interface) (*Config, error) {
	// Fetch the KServe configmap directly from the API server to get latest values
	isvcConfigMap, errCfgMap := v1beta1.GetInferenceServiceConfigMap(ctx, clientset)
	if errCfgMap != nil {
		return nil, fmt.Errorf("failed to load InferenceServiceConfigMap: %w", errCfgMap)
	}

	ingressConfig, errConvert := v1beta1.NewIngressConfig(isvcConfigMap)
	if errConvert != nil {
		return nil, fmt.Errorf("failed to convert InferenceServiceConfigMap to IngressConfig: %w", errConvert)
	}

	storageInitializerConfig, errConvert := v1beta1.GetStorageInitializerConfigs(isvcConfigMap)
	if errConvert != nil {
		return nil, fmt.Errorf("failed to convert InferenceServiceConfigMap to StorageInitializerConfig: %w", errConvert)
	}

	credentialConfig, errConvert := credentials.GetCredentialConfig(isvcConfigMap)
	if errConvert != nil {
		return nil, fmt.Errorf("failed to convert InferenceServiceConfigMap to CredentialConfig: %w", errConvert)
	}

	schedulerConfig, errConvert := NewSchedulerConfig(isvcConfigMap)
	if errConvert != nil {
		return nil, fmt.Errorf("failed to parse scheduler config: %w", errConvert)
	}

	config := NewConfig(ingressConfig, storageInitializerConfig, &credentialConfig, schedulerConfig)

	if autoscalingData, ok := isvcConfigMap.Data[autoscalingConfigName]; ok {
		asCfg := &WVAAutoscalingConfig{}
		if err := json.Unmarshal([]byte(autoscalingData), asCfg); err != nil {
			return nil, fmt.Errorf("failed to parse %s config json: %w", autoscalingConfigName, err)
		}
		config.WVAAutoscalingConfig = asCfg
	}

	return config, nil
}
