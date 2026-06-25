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

package llmisvc

import (
	"context"
	"fmt"
	"net"
	"strings"

	"k8s.io/utils/ptr"

	duckv1 "knative.dev/pkg/apis/duck/v1"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/utils"

	"knative.dev/pkg/apis"
	"knative.dev/pkg/network"
)

// IsInternalURL determines if a URL points to an internal/private endpoint
// This is used to classify URLs as internal vs external for routing decisions
func IsInternalURL(url *apis.URL) bool {
	host := url.URL().Hostname()

	if isInternalIP(host) {
		return true
	}

	return isInternalHostname(host)
}

// IsExternalURL determines if a URL points to an external/public endpoint
func IsExternalURL(url *apis.URL) bool {
	return !IsInternalURL(url)
}

// FilterInternalURLs returns only the discovered URLs that are internal/private
func FilterInternalURLs(urls []DiscoveredURL) []DiscoveredURL {
	return utils.FilterSlice(urls, func(d DiscoveredURL) bool {
		return IsInternalURL(d.URL)
	})
}

// FilterExternalURLs returns only the discovered URLs that are external/public
func FilterExternalURLs(urls []DiscoveredURL) []DiscoveredURL {
	return utils.FilterSlice(urls, func(d DiscoveredURL) bool {
		return IsExternalURL(d.URL)
	})
}

// isInternalIP checks if an IP address is in a private range
func isInternalIP(addr string) bool {
	ip := net.ParseIP(addr)
	if ip != nil && ip.IsPrivate() {
		return true
	}
	return false
}

// IsClusterLocalURL returns true if the URL uses a Kubernetes cluster-local hostname
// (e.g., service.namespace.svc.cluster.local)
func IsClusterLocalURL(url *apis.URL) bool {
	host := strings.ToLower(url.URL().Hostname())
	return strings.HasSuffix(host, network.GetClusterDomainName())
}

func IsModelRoutingURL(url *apis.URL) bool {
	return url.Path == "/" || url.Path == ""
}

func SourcedAddress(ctx context.Context, d DiscoveredURL, llmSvc *v1alpha2.LLMInferenceService) v1alpha2.SourcedAddress {
	typeName := "gateway-external"

	if IsClusterLocalURL(d.URL) {
		typeName = "gateway-internal"
	} else if IsInternalURL(d.URL) {
		typeName = "internal"
	}

	models := make([]v1alpha2.ModelSourcedAddressStatus, 0, 2)
	const modelRoutingFmt = "publishers/%s/models/%s"

	// Ensure llmSvc.Spec.Model.Name is set.
	llmSvc.Spec.Model.Name = ptr.To(ptr.Deref(llmSvc.Spec.Model.Name, llmSvc.GetName()))

	if IsModelRoutingURL(d.URL) {
		typeName += "-model-routing"

		models = append(models, v1alpha2.ModelSourcedAddressStatus{
			Name: fmt.Sprintf(modelRoutingFmt, llmSvc.GetNamespace(), *llmSvc.Spec.Model.Name),
		})
		if llmSvc.Spec.Model.LoRA != nil {
			for _, m := range llmSvc.Spec.Model.LoRA.Adapters {
				if m.Name == nil {
					continue
				}
				models = append(models, v1alpha2.ModelSourcedAddressStatus{
					Name: fmt.Sprintf(modelRoutingFmt, llmSvc.GetNamespace(), *m.Name),
				})
			}
		}
	} else {
		models = append(models,
			v1alpha2.ModelSourcedAddressStatus{
				Name: fmt.Sprintf(modelRoutingFmt, llmSvc.GetNamespace(), *llmSvc.Spec.Model.Name),
			},
			v1alpha2.ModelSourcedAddressStatus{
				Name: *llmSvc.Spec.Model.Name,
			},
		)
		if llmSvc.Spec.Model.LoRA != nil {
			for _, m := range llmSvc.Spec.Model.LoRA.Adapters {
				if m.Name == nil {
					continue
				}

				models = append(models,
					v1alpha2.ModelSourcedAddressStatus{
						Name: fmt.Sprintf(modelRoutingFmt, llmSvc.GetNamespace(), *m.Name),
					},
					v1alpha2.ModelSourcedAddressStatus{
						Name: *m.Name,
					},
				)
			}
		}
	}

	sa := v1alpha2.SourcedAddress{
		Addressable: duckv1.Addressable{
			Name: &typeName,
			URL:  d.URL,
		},
		Origin: d.Origin,
		Models: models,
	}

	return sa
}

// isInternalHostname checks if a hostname appears to be internal
// This includes cluster-local domains and localhost variants
func isInternalHostname(hostname string) bool {
	hostname = strings.ToLower(hostname)

	localSuffixes := []string{network.GetClusterDomainName(), ".local", ".localhost", ".internal"}
	for _, localSuffix := range localSuffixes {
		if strings.HasSuffix(hostname, localSuffix) {
			return true
		}
	}

	return hostname == "localhost"
}
