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

package llmisvc

import (
	"context"
	"fmt"
	"maps"
	"reflect"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	"github.com/kserve/kserve/pkg/constants"
	"github.com/kserve/kserve/pkg/localmodelcache"
	kservevalidation "github.com/kserve/kserve/pkg/validation"
	llmisvcwebhook "github.com/kserve/kserve/pkg/webhook/admission/llminferenceservice"
)

// ensureLocalModelCacheBinding re-runs the local model cache selection that
// the admission defaulter performs, for CacheRequired workloads that are
// still waiting without a binding. A workload created before its matching
// cache existed never received one from defaulting: reconciliation faithfully
// fails closed waiting for a cache (resolveBaseModelCachePVC /
// rewriteLoRAAdaptersFromLocalModelCache), but nothing re-selects, so the
// workload only recovers when its spec is edited again. Running the selection
// here binds the workload as soon as a matching cache appears; the persist
// step patches only metadata because llmSvc carries the merged (rendered)
// spec at this point in the reconcile, which must not be written back.
func (r *LLMISVCReconciler) ensureLocalModelCacheBinding(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService) error {
	if _, localModelDisabled := llmSvc.Annotations[constants.DisableLocalModelKey]; localModelDisabled {
		return nil
	}
	// The admission defaulter skips cache reads (and strips cache metadata)
	// when local models are globally disabled; the re-selection must not
	// read them either, so a cluster without the optional cache CRDs
	// installed can still reconcile CacheRequired workloads, including
	// pvc:// ones that never need a cache.
	configMap, err := v1beta1.GetInferenceServiceConfigMap(ctx, r.Clientset)
	if err != nil {
		return fmt.Errorf("cannot read inference service config: %w", err)
	}
	localModelConfig, err := v1beta1.NewLocalModelConfig(configMap)
	if err != nil {
		return fmt.Errorf("cannot parse local model config: %w", err)
	}
	if !localModelConfig.Enabled {
		return nil
	}
	// A malformed annotation already fails closed in attachModelArtifacts;
	// treating it as required here keeps the selection fail-closed too.
	cacheRequired, _ := kservevalidation.LocalModelCachePolicyRequired(llmSvc.Annotations)
	if !cacheRequired || localModelCacheFullyBound(llmSvc) {
		return nil
	}

	models := &v1alpha1.LocalModelCacheList{}
	if err := r.List(ctx, models); err != nil {
		return fmt.Errorf("cannot list local model caches: %w", err)
	}
	nsModels := &v1alpha1.LocalModelNamespaceCacheList{}
	if err := r.List(ctx, nsModels, client.InNamespace(llmSvc.Namespace)); err != nil {
		return fmt.Errorf("cannot list namespace-scoped local model caches: %w", err)
	}

	labelsBefore, annotationsBefore := maps.Clone(llmSvc.Labels), maps.Clone(llmSvc.Annotations)
	llmisvcwebhook.SetLocalModelLabel(llmSvc, models, nsModels)
	if reflect.DeepEqual(labelsBefore, llmSvc.Labels) && reflect.DeepEqual(annotationsBefore, llmSvc.Annotations) {
		// No matching cache yet: keep waiting for one (fail closed).
		return nil
	}

	// llmSvc.Spec holds the rendered configuration here; patch only the
	// metadata the selection changed, from a target whose spec matches the
	// base so the merge patch cannot touch anything else.
	base := &v1alpha2.LLMInferenceService{ObjectMeta: metav1.ObjectMeta{
		Namespace:   llmSvc.Namespace,
		Name:        llmSvc.Name,
		Labels:      labelsBefore,
		Annotations: annotationsBefore,
	}}
	target := base.DeepCopy()
	target.Labels = llmSvc.Labels
	target.Annotations = llmSvc.Annotations
	if err := r.Patch(ctx, target, client.MergeFrom(base)); err != nil {
		return fmt.Errorf("cannot persist local model cache binding: %w", err)
	}
	return nil
}

// localModelCacheFullyBound reports whether the base model and every LoRA
// adapter already carry a cache binding. Fully bound workloads have nothing
// to select, so the cache lists are not even read for them.
func localModelCacheFullyBound(llmSvc *v1alpha2.LLMInferenceService) bool {
	if _, bound := llmSvc.Labels[constants.LocalModelLabel]; !bound {
		return false
	}
	if llmSvc.Spec.Model.LoRA == nil || len(llmSvc.Spec.Model.LoRA.Adapters) == 0 {
		return true
	}
	entries, err := localmodelcache.ParseLoRACacheAnnotation(llmSvc.Annotations[constants.LocalModelLoRAAnnotationKey])
	if err != nil {
		// A malformed annotation is recomputed from scratch by the selection.
		return false
	}
	for _, adapter := range llmSvc.Spec.Model.LoRA.Adapters {
		if adapter.Name == nil {
			continue
		}
		if _, ok := entries[*adapter.Name]; !ok {
			return false
		}
	}
	return true
}
