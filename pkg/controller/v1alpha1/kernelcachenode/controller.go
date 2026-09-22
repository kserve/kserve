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

// +kubebuilder:rbac:groups=serving.kserve.io,resources=kernelcaches,verbs=get;list;watch
// +kubebuilder:rbac:groups=serving.kserve.io,resources=kernelcachenodes,verbs=get;list;watch
// +kubebuilder:rbac:groups=serving.kserve.io,resources=kernelcachenodes/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=serving.kserve.io,resources=kernelcachenodegroups,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=nodes,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch
// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch
package kernelcachenode

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	kernelcacheconfig "github.com/kserve/kserve/pkg/kernelcache/config"
)

const (
	// Labels identify KernelCache prefetch Jobs and their target node.
	kernelCacheNameLabel      = "serving.kserve.io/kernel-cache-name"
	kernelCacheNamespaceLabel = "serving.kserve.io/kernel-cache-namespace"
	kernelCacheNodeLabel      = "serving.kserve.io/kernel-cache-node"

	// Reconciliation and image validation intervals.
	defaultReconcileInterval = 5 * time.Minute
	cacheImageCheckInterval  = time.Hour
	missingImageMessage      = "OCI artifact is not present in Node.status.images"
)

// KernelCacheNodeReconciler reports cache preparation status for one node.
type KernelCacheNodeReconciler struct {
	client.Client
	Reader   client.Reader // Reads prefetch Pods outside the filtered manager cache.
	NodeName string
	Log      logr.Logger
}

// Main reconciliation loop.
func (r *KernelCacheNodeReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	if r.NodeName == "" {
		return ctrl.Result{}, errors.New("node name is required")
	}
	if req.Name != r.NodeName {
		return ctrl.Result{}, nil
	}

	kernelCacheNode := &v1alpha1.KernelCacheNode{}
	if err := r.Get(ctx, client.ObjectKey{Name: r.NodeName}, kernelCacheNode); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, fmt.Errorf("get KernelCacheNode %q: %w", r.NodeName, err)
	}

	config, err := kernelcacheconfig.Load(ctx, r.Client)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("load KernelCache configuration: %w", err)
	}
	if !config.Enabled {
		return ctrl.Result{}, nil
	}

	if err := r.reconcileStatus(ctx, config, false); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	interval := time.Duration(*config.ReconcileIntervalSeconds) * time.Second
	return ctrl.Result{RequeueAfter: interval}, nil
}
