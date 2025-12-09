/*
Copyright 2024 The KServe Authors.

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

// +kubebuilder:rbac:groups=serving.kserve.io,resources=inferenceservices,verbs=get;list;watch
// +kubebuilder:rbac:groups=serving.kserve.io,resources=localmodelnodegroups,verbs=get;list;watch
// +kubebuilder:rbac:groups=serving.kserve.io,resources=localmodelcaches,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=serving.kserve.io,resources=localmodelcaches/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=serving.kserve.io,resources=localmodelnamespacecaches,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=serving.kserve.io,resources=localmodelnamespacecaches/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=serving.kserve.io,resources=localmodelnodes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=serving.kserve.io,resources=localmodelnodes/status,verbs=get;watch
// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get
// +kubebuilder:rbac:groups=core,resources=nodes,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=nodes/status,verbs=get;watch
// +kubebuilder:rbac:groups=core,resources=persistentvolumes,verbs=get;list;watch;create;update;patch
// +kubebuilder:rbac:groups=core,resources=persistentvolumeclaims,verbs=get;list;watch;create;update;patch
package localmodel

import (
	"github.com/kserve/kserve/pkg/controller/v1alpha1/localmodel/reconcilers"
)

// Re-export reconciler types for backwards compatibility
type (
	LocalModelReconciler               = reconcilers.LocalModelReconciler
	LocalModelNamespaceCacheReconciler = reconcilers.LocalModelNamespaceCacheReconciler
)
