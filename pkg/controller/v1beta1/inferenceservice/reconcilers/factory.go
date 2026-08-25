// Copyright 2025 The KServe Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package reconcilers

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	"github.com/kserve/kserve/pkg/constants"
	"github.com/kserve/kserve/pkg/controller/v1beta1/inferenceservice/reconcilers/deployment"
	"github.com/kserve/kserve/pkg/controller/v1beta1/inferenceservice/reconcilers/ingress"
	"github.com/kserve/kserve/pkg/controller/v1beta1/inferenceservice/reconcilers/service"
)

// WorkloadReconcilerParams contains parameters for workload reconciler creation
type WorkloadReconcilerParams struct {
	Client              client.Client
	Scheme              *runtime.Scheme
	ComponentMeta       metav1.ObjectMeta
	WorkerComponentMeta metav1.ObjectMeta
	ComponentExt        *v1beta1.ComponentExtensionSpec
	PodSpec             *corev1.PodSpec
	WorkerPodSpec       *corev1.PodSpec
	DeployConfig        *v1beta1.DeployConfig
}

// ServiceReconcilerParams contains parameters for service reconciler creation
type ServiceReconcilerParams struct {
	Client           client.Client
	Scheme           *runtime.Scheme
	ComponentMeta    metav1.ObjectMeta
	ComponentExt     *v1beta1.ComponentExtensionSpec
	PodSpec          *corev1.PodSpec
	MultiNodeEnabled bool
	ServiceConfig    *v1beta1.ServiceConfig
}

// IngressReconcilerParams contains parameters for ingress reconciler creation
type IngressReconcilerParams struct {
	Client        client.Client
	Clientset     kubernetes.Interface
	Scheme        *runtime.Scheme
	IngressConfig *v1beta1.IngressConfig
	IsvcConfig    *v1beta1.InferenceServicesConfig
	// IsVirtualServiceAvailable indicates whether the Istio VirtualService CRD
	// exists in the cluster and should be used by the Ingress reconciler.
	IsVirtualServiceAvailable bool
}

// ReconcilerFactory creates appropriate reconcilers based on deployment mode
type ReconcilerFactory struct{}

// NewReconcilerFactory creates a new ReconcilerFactory
func NewReconcilerFactory() *ReconcilerFactory {
	return &ReconcilerFactory{}
}

// CreateWorkloadReconciler creates the appropriate workload reconciler
// Phase 1: Only supports Deployment (RawDeployment, Standard)
func (f *ReconcilerFactory) CreateWorkloadReconciler(
	ctx context.Context,
	deploymentMode constants.DeploymentModeType,
	params WorkloadReconcilerParams,
) (WorkloadReconciler, error) {
	switch deploymentMode {
	case constants.Standard, constants.LegacyRawDeployment:
		deploymentRec, err := deployment.NewDeploymentReconciler(
			params.Client, params.Scheme, params.ComponentMeta, params.WorkerComponentMeta,
			params.ComponentExt, params.PodSpec, params.WorkerPodSpec, params.DeployConfig,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to create deployment reconciler: %w", err)
		}
		return deploymentRec, nil

	default:
		return nil, fmt.Errorf("unsupported deployment mode for workload: %s", deploymentMode)
	}
}

// CreateServiceReconciler creates the appropriate service reconciler
// Phase 1: Only supports standard ServiceReconciler
func (f *ReconcilerFactory) CreateServiceReconciler(
	deploymentMode constants.DeploymentModeType,
	params ServiceReconcilerParams,
) (ServiceReconciler, error) {
	switch deploymentMode {
	case constants.Standard, constants.LegacyRawDeployment:
		return service.NewServiceReconciler(
			params.Client, params.Scheme, params.ComponentMeta, params.ComponentExt,
			params.PodSpec, params.MultiNodeEnabled, params.ServiceConfig,
		), nil

	default:
		return nil, fmt.Errorf("unsupported deployment mode for service: %s", deploymentMode)
	}
}

// CreateIngressReconciler creates the appropriate ingress reconciler
func (f *ReconcilerFactory) CreateIngressReconciler(
	deploymentMode constants.DeploymentModeType,
	params IngressReconcilerParams,
) (IngressReconciler, error) {
	switch deploymentMode {
	case constants.Standard, constants.LegacyRawDeployment:
		if params.IngressConfig.EnableGatewayAPI {
			// Gateway API HTTPRoute
			return ingress.NewRawHTTPRouteReconciler(
				params.Client, params.Scheme, params.IngressConfig, params.IsvcConfig,
			), nil
		} else {
			// Kubernetes Ingress
			rawIngress, err := ingress.NewRawIngressReconciler(
				params.Client, params.Scheme, params.IngressConfig, params.IsvcConfig,
			)
			if err != nil {
				return nil, fmt.Errorf("failed to create ingress reconciler: %w", err)
			}
			return rawIngress, nil
		}

	case constants.Knative, constants.LegacyServerless, constants.ModelMeshDeployment:
		// Knative Service
		return ingress.NewIngressReconciler(
			params.Client, params.Clientset, params.Scheme, params.IngressConfig, params.IsvcConfig,
			params.IsVirtualServiceAvailable,
		), nil

	default:
		return nil, fmt.Errorf("unsupported deployment mode for ingress: %s", deploymentMode)
	}
}
