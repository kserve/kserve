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

package localmodel

import (
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
)

func NewCacheOptions() cache.Options {
	// This selector contains only fixed, valid label keys.
	jobSelector, _ := labels.Parse("model,modelNamespace")
	return cache.Options{
		ReaderFailOnMissingInformer: true,
		ByObject: map[client.Object]cache.ByObject{
			&batchv1.Job{}: {Label: jobSelector},
		},
		// Nodes can match arbitrary affinity rules. LLMInferenceServices can
		// reference LoRA caches through annotations alone; the KernelCache
		// capture controllers select them through annotations as well, so the
		// shared InferenceService informer must stay unfiltered. Neither has
		// a safe static label filter. PV/PVC watches use metadata only;
		// user-provided shared PVCs must remain visible without KServe labels.
	}
}

func NewClientOptions() client.Options {
	return client.Options{Cache: &client.CacheOptions{DisableFor: []client.Object{
		// Name-conflict checks must see Jobs without our cache labels.
		&batchv1.Job{},
		// Typed volume reads must not create a second, full-object informer.
		&corev1.PersistentVolume{}, &corev1.PersistentVolumeClaim{},
		// registryauth.Revoke Get+Deletes ephemeral pull Secrets; nothing
		// watches Secrets in this binary, so keep those reads direct.
		&corev1.Secret{},
		&v1alpha1.ClusterStorageContainer{},
	}}}
}
