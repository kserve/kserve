//go:build distro

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

package main

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/fields"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"

	"github.com/kserve/kserve/pkg/controller/v1alpha2/llmisvc"
)

func customizeManagerOptions(opts *ctrl.Options) {
	// Replace the simple label-based Secret cache with a namespace-aware one that
	// also watches the platform CA signing secret used for workload TLS certificates.
	for obj, cfg := range opts.Cache.ByObject {
		if _, ok := obj.(*corev1.Secret); ok {
			opts.Cache.ByObject[obj] = cache.ByObject{
				Namespaces: map[string]cache.Config{
					llmisvc.ServiceCASigningSecretNamespace: {
						FieldSelector: fields.SelectorFromSet(map[string]string{
							"metadata.name": llmisvc.ServiceCASigningSecretName,
						}),
					},
					cache.AllNamespaces: {
						LabelSelector: cfg.Label,
					},
				},
			}
			return
		}
	}

	setupLog.WithValues("distro", "opendatahub").Info("WARNING: Secret entry not found in cache.ByObject; CA signing secret will not be watched")
}
