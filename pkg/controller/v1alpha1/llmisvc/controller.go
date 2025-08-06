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

	logf "sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	"github.com/kserve/kserve/pkg/constants"
	kserveTypes "github.com/kserve/kserve/pkg/types"

	"k8s.io/client-go/tools/record"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
)

type Config struct {
	SystemNamespace             string   `json:"systemNamespace,omitempty"`
	IngressGatewayName          string   `json:"ingressGatewayName,omitempty"`
	IngressGatewayNamespace     string   `json:"ingressGatewayNamespace,omitempty"`
	IstioGatewayControllerNames []string `json:"istioGatewayControllerNames,omitempty"`

	StorageConfig *kserveTypes.StorageInitializerConfig `json:"-"`
}

// +kubebuilder:rbac:groups=serving.kserve.io,resources=llminferenceservices;llminferenceservices/finalizers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=serving.kserve.io,resources=llminferenceservices/status,verbs=get;update;patch

// LLMISVCReconciler reconciles a LLMInferenceService object
// This controller is responsible for managing the lifecycle of LLMInferenceService resources.
type LLMISVCReconciler struct {
	client.Client
	record.EventRecorder
	Clientset kubernetes.Interface
}

// NewConfig creates an instance of llm-specific config based on predefined values
// in IngressConfig struct
func NewConfig(ingressConfig *v1beta1.IngressConfig, storageConfig *kserveTypes.StorageInitializerConfig) *Config {
	igwNs := constants.KServeNamespace
	igwName := ingressConfig.KserveIngressGateway
	igw := strings.Split(igwName, "/")
	if len(igw) == 2 {
		igwNs = igw[0]
		igwName = igw[1]
	}

	return &Config{
		SystemNamespace:         constants.KServeNamespace,
		IngressGatewayNamespace: igwNs,
		IngressGatewayName:      igwName,
		// TODO make it configurable
		IstioGatewayControllerNames: []string{
			"istio.io/gateway-controller",
			"istio.io/unmanaged-gateway",
			"openshift.io/gateway-controller",
		},
		StorageConfig: storageConfig,
	}
}

func LoadConfig(ctx context.Context, clientset kubernetes.Interface) (*Config, error) {
	isvcConfigMap, errCfgMap := v1beta1.GetInferenceServiceConfigMap(ctx, clientset) // Fetch directly from API Server
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

	return NewConfig(ingressConfig, storageInitializerConfig), nil
}

func (r *LLMISVCReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)
	log.Info("Reconciling LLMISVC", "name", req.Name, "namespace", req.Namespace)
	// Implement the reconciliation logic here
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *LLMISVCReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.LLMInferenceService{}).
		Complete(r)
}
