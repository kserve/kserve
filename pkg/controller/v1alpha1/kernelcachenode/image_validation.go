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

package kernelcachenode

import (
	"context"
	"fmt"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	kernelcacheconfig "github.com/kserve/kserve/pkg/kernelcache/config"
)

// RunPeriodicImageValidation performs an image check at startup and then at a fixed interval.
func (r *KernelCacheNodeReconciler) RunPeriodicImageValidation(ctx context.Context) error {
	r.runImageValidation(ctx)

	ticker := time.NewTicker(cacheImageCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			r.runImageValidation(ctx)
		}
	}
}

func (r *KernelCacheNodeReconciler) runImageValidation(ctx context.Context) {
	if err := r.validateNodeImages(ctx); err != nil {
		r.Log.Error(err, "KernelCache image validation failed", "node", r.NodeName, "operation", "periodic-image-validation")
		if r.Recorder == nil {
			return
		}
		kernelCacheNode := &v1alpha1.KernelCacheNode{}
		if getErr := r.Get(ctx, client.ObjectKey{Name: r.NodeName}, kernelCacheNode); getErr != nil {
			r.Log.Error(getErr, "unable to get KernelCacheNode for image validation event", "node", r.NodeName)
			return
		}
		r.recordErrorEvent(kernelCacheNode, "ImageValidationError", err)
	}
}

func (r *KernelCacheNodeReconciler) validateNodeImages(ctx context.Context) error {
	config, err := kernelcacheconfig.Load(ctx, r.Client)
	if err != nil {
		return fmt.Errorf("load KernelCache configuration for image validation: %w", err)
	}
	if !config.Enabled {
		return nil
	}
	if err := r.reconcileStatus(ctx, config, true); err != nil {
		return fmt.Errorf("reconcile image validation status for node %q: %w", r.NodeName, err)
	}
	return nil
}
