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

// +kubebuilder:rbac:groups=serving.kserve.io,resources=kernelcaches,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=serving.kserve.io,resources=kernelcaches/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=serving.kserve.io,resources=kernelcachenodes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=serving.kserve.io,resources=kernelcachenodes/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=core,resources=nodes,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=nodes/status,verbs=get;watch
// +kubebuilder:rbac:groups=core,resources=persistentvolumes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=persistentvolumeclaims,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch;delete
// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;delete
package kernelcache

import (
	"github.com/kserve/kserve/pkg/controller/v1alpha1/kernelcache/reconcilers"
)

// Re-export reconciler type for backwards compatibility
type (
	KernelCacheReconciler = reconcilers.KernelCacheReconciler
)
