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

// BackendFromLLMInferenceService returns the first usable backend for svc.
// Prefer BackendsFromLLMInferenceService: each status.addresses entry has its
// own URL and model list.
func BackendFromLLMInferenceService(svc *v1alpha2.LLMInferenceService) (Backend, bool) {
	backends := BackendsFromLLMInferenceService(svc)
	if len(backends) == 0 {
		return Backend{}, false
	}
	return backends[0], true
}

// BackendsFromLLMInferenceService maps a Ready, non-stopped LLMInferenceService
// to one backend per reachable address.
//
// status.addresses is per-model: each entry has its own URL and model list.
// When both cluster-local and public addresses exist, only the highest-preference
// class is kept (cluster-local, then internal, then any) so the in-cluster
// aggregator does not fan out to duplicate public URLs.
func BackendsFromLLMInferenceService(svc *v1alpha2.LLMInferenceService) []Backend {
	if svc == nil || utils.GetForceStopRuntime(svc) || !isLLMInferenceServiceReady(svc) {
		return nil
	}

	type candidate struct {
		url     string
		address string
		models  []string
		pref    int
	}

	best := -1
	var cands []candidate
	for i := range svc.Status.Addresses {
		addr := svc.Status.Addresses[i]
		if addr.URL == nil {
			continue
		}
		pref := urlPreference(addr.URL)
		if pref > best {
			best = pref
		}
		name := ""
		if addr.Name != nil {
			name = *addr.Name
		}
		var models []string
		for _, m := range addr.Models {
			if m.Name != "" {
				models = append(models, m.Name)
			}
		}
		cands = append(cands, candidate{
			url:     strings.TrimRight(addr.URL.String(), "/"),
			address: name,
			models:  models,
			pref:    pref,
		})
	}

	if len(cands) == 0 {
		if svc.Status.URL == nil {
			return nil
		}
		return []Backend{{
			Name:      svc.Name,
			Namespace: svc.Namespace,
			URL:       strings.TrimRight(svc.Status.URL.String(), "/"),
			Models:    collectModelNames(svc),
			Kind:      "LLMInferenceService",
		}}
	}

	out := make([]Backend, 0, len(cands))
	for _, c := range cands {
		if c.pref != best {
			continue
		}
		out = append(out, Backend{
			Name:      svc.Name,
			Namespace: svc.Namespace,
			URL:       c.url,
			Models:    c.models,
			Kind:      "LLMInferenceService",
			Address:   c.address,
		})
	}
	return out
}

func urlPreference(u *apis.URL) int {
	switch {
	case isClusterLocalURL(u):
		return 2
	case isInternalHostnameURL(u):
		return 1
	default:
		return 0
	}
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
