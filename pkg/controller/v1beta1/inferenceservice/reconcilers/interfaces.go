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

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
)

// WorkloadReconciler reconciles workload resources (Deployment or Rollout)
type WorkloadReconciler interface {
	// Reconcile reconciles the workload and returns Deployments
	Reconcile(ctx context.Context) ([]*appsv1.Deployment, error)

	// GetWorkloads returns workload resources as generic Objects for controller references
	GetWorkloads() []metav1.Object

	// SetControllerReferences sets owner references on all workloads
	SetControllerReferences(owner metav1.Object, scheme *runtime.Scheme) error
}

// ServiceReconciler reconciles service resources
type ServiceReconciler interface {
	// Reconcile reconciles services
	Reconcile(ctx context.Context) ([]*corev1.Service, error)

	// GetServiceList returns all managed services
	GetServiceList() []*corev1.Service

	// SetControllerReferences sets owner references on all services
	SetControllerReferences(owner metav1.Object, scheme *runtime.Scheme) error
}

// IngressReconciler reconciles ingress/routing resources
type IngressReconciler interface {
	// Reconcile reconciles ingress resources
	// Returns ctrl.Result for status propagation and requeueing
	Reconcile(ctx context.Context, isvc *v1beta1.InferenceService) (ctrl.Result, error)
}
