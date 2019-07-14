/*
Copyright 2019 The Knative Authors

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

package resources

import (
	"strings"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	corev1listers "k8s.io/client-go/listers/core/v1"
)

// FetchReadyAddressCount fetches endpoints and returns the total number of addresses ready for them.
func FetchReadyAddressCount(lister corev1listers.EndpointsLister, ns, name string) (int, error) {
	endpoints, err := lister.Endpoints(ns).Get(name)
	if apierrors.IsNotFound(err) {
		// Treat a not-found error as 0 ready addresses
		return 0, nil
	} else if err != nil {
		return 0, err
	}

	return ReadyAddressCount(endpoints), nil
}

// ReadyAddressCount returns the total number of addresses ready for the given endpoint.
func ReadyAddressCount(endpoints *corev1.Endpoints) int {
	var total int
	for _, subset := range endpoints.Subsets {
		total += len(subset.Addresses)
	}
	return total
}

// ParentResourceFromService returns the parent resource name from
// endpoints or k8s service resource.
// The function is based upon knowledge that all knative built services
// have `parent-resource-<svc-unique-suffix` format.
func ParentResourceFromService(name string) string {
	li := strings.LastIndex(name, "-")
	if li == -1 {
		// Presume same.
		return name
	}
	return name[:li]
}
