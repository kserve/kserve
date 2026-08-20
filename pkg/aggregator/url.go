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

package aggregator

import (
	"strings"

	corev1 "k8s.io/api/core/v1"
	"knative.dev/pkg/apis"
	"knative.dev/pkg/network"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/utils"
)

// ResolveBackendURL picks a base URL for an LLMInferenceService.
// Preference order:
//  1. First cluster-local address in status.addresses
//  2. First internal (non-public-looking) address in status.addresses
//  3. First address in status.addresses
//  4. status.url
func ResolveBackendURL(svc *v1alpha2.LLMInferenceService) string {
	if svc == nil {
		return ""
	}

	var clusterLocal, internal, anyURL *apis.URL
	for i := range svc.Status.Addresses {
		u := svc.Status.Addresses[i].URL
		if u == nil {
			continue
		}
		if anyURL == nil {
			anyURL = u
		}
		if isClusterLocalURL(u) {
			if clusterLocal == nil {
				clusterLocal = u
			}
			continue
		}
		if isInternalHostnameURL(u) && internal == nil {
			internal = u
		}
	}

	switch {
	case clusterLocal != nil:
		return strings.TrimRight(clusterLocal.String(), "/")
	case internal != nil:
		return strings.TrimRight(internal.String(), "/")
	case anyURL != nil:
		return strings.TrimRight(anyURL.String(), "/")
	case svc.Status.URL != nil:
		return strings.TrimRight(svc.Status.URL.String(), "/")
	default:
		return ""
	}
}

// BackendFromLLMInferenceService converts a CR into a Backend when it is usable.
// Returns ok=false when the service is stopped, not Ready, or has no resolvable URL.
func BackendFromLLMInferenceService(svc *v1alpha2.LLMInferenceService) (Backend, bool) {
	if svc == nil || utils.GetForceStopRuntime(svc) || !isLLMInferenceServiceReady(svc) {
		return Backend{}, false
	}
	base := ResolveBackendURL(svc)
	if base == "" {
		return Backend{}, false
	}

	return Backend{
		Name:      svc.Name,
		Namespace: svc.Namespace,
		URL:       base,
		Models:    collectModelNames(svc),
	}, true
}

func isLLMInferenceServiceReady(svc *v1alpha2.LLMInferenceService) bool {
	cond := svc.Status.GetCondition(apis.ConditionReady)
	return cond != nil && cond.Status == corev1.ConditionTrue
}

func collectModelNames(svc *v1alpha2.LLMInferenceService) []string {
	seen := map[string]struct{}{}
	var models []string
	add := func(name string) {
		if name == "" {
			return
		}
		if _, ok := seen[name]; ok {
			return
		}
		seen[name] = struct{}{}
		models = append(models, name)
	}
	for _, addr := range svc.Status.Addresses {
		for _, m := range addr.Models {
			add(m.Name)
		}
	}
	if svc.Spec.Model.Name != nil {
		add(*svc.Spec.Model.Name)
	}
	return models
}

func isClusterLocalURL(u *apis.URL) bool {
	if u == nil {
		return false
	}
	host := strings.ToLower(u.URL().Hostname())
	return strings.HasSuffix(host, network.GetClusterDomainName())
}

func isInternalHostnameURL(u *apis.URL) bool {
	if u == nil {
		return false
	}
	if isClusterLocalURL(u) {
		return true
	}
	host := strings.ToLower(u.URL().Hostname())
	for _, suffix := range []string{".local", ".localhost", ".internal"} {
		if strings.HasSuffix(host, suffix) {
			return true
		}
	}
	return false
}
