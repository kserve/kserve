/*
Copyright 2026 The KServe Authors.

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

// +kubebuilder:rbac:groups=serving.kserve.io,resources=kernelcachecaptures,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=serving.kserve.io,resources=kernelcachecaptures/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=serving.kserve.io,resources=kernelcachecaptures/finalizers,verbs=update
// +kubebuilder:rbac:groups=serving.kserve.io,resources=kernelcaches,verbs=get;list;watch;create;update;patch
// +kubebuilder:rbac:groups=serving.kserve.io,resources=inferenceservices,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=pods/exec,verbs=create
// +kubebuilder:rbac:groups=core,resources=events,verbs=create;patch
package kernelcachecapture

import (
	"context"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
)

// KernelCacheCaptureReconciler reconciles KernelCacheCapture objects.
// It runs in the local-model controller manager alongside the LocalModel controller.
type KernelCacheCaptureReconciler struct {
	client.Client
	Clientset    *kubernetes.Clientset
	ClientConfig *rest.Config
	Log          logr.Logger
	Scheme       *runtime.Scheme
	Recorder     record.EventRecorder
}

func (r *KernelCacheCaptureReconciler) Reconcile(_ context.Context, _ ctrl.Request) (ctrl.Result, error) {
	return ctrl.Result{}, nil
}

func (r *KernelCacheCaptureReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.KernelCacheCapture{}).
		Complete(r)
}
