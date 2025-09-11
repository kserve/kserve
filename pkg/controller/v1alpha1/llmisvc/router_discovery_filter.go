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
	"net"
	"strings"

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

// FilterInternalURLs returns only the URLs that are internal/private
func FilterInternalURLs(urls []*apis.URL) []*apis.URL {
	return utils.FilterSlice(urls, IsInternalURL)
}

// FilterExternalURLs returns only the URLs that are external/public
func FilterExternalURLs(urls []*apis.URL) []*apis.URL {
	return utils.FilterSlice(urls, IsExternalURL)
}

// isInternalIP checks if an IP address is in a private range
func isInternalIP(addr string) bool {
	ip := net.ParseIP(addr)
	if ip != nil && ip.IsPrivate() {
		return true
	}
	return false
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
