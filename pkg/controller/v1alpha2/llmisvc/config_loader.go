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
	"fmt"
	"strings"

	"k8s.io/client-go/kubernetes"

	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	"github.com/kserve/kserve/pkg/constants"
	"github.com/kserve/kserve/pkg/credentials"
	"github.com/kserve/kserve/pkg/types"
)

// Config holds configuration needed for LLM inference services
// It aggregates ingress, storage, and credential settings from the KServe configmap
type Config struct {
	SystemNamespace         string `json:"systemNamespace,omitempty"`
	IngressGatewayName      string `json:"ingressGatewayName,omitempty"`
	IngressGatewayNamespace string `json:"ingressGatewayNamespace,omitempty"`

	// Storage and credential configs are excluded from JSON serialization
	// as they contain sensitive information
	StorageConfig    *types.StorageInitializerConfig `json:"-"`
	CredentialConfig *credentials.CredentialConfig   `json:"-"`
}

// NewConfig creates an instance of llm-specific config based on predefined values
// in IngressConfig struct
func NewConfig(ingressConfig *v1beta1.IngressConfig, storageConfig *types.StorageInitializerConfig, credentialConfig *credentials.CredentialConfig) *Config {
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
		StorageConfig:           storageConfig,
		CredentialConfig:        credentialConfig,
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

	return NewConfig(ingressConfig, storageInitializerConfig, &credentialConfig), nil
}
