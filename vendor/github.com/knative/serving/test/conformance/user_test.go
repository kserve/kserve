// +build e2e

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

package conformance

import (
	"testing"

	"github.com/knative/serving/test"

	corev1 "k8s.io/api/core/v1"
)

const userID = 2020

// TestMustRunAsUser verifies that a supplied runAsUser through securityContext takes
// effect as delared by "MUST" in the runtime-contract.
func TestMustRunAsUser(t *testing.T) {
	t.Parallel()
	clients := setup(t)

	runAsUser := int64(userID)
	securityContext := &corev1.SecurityContext{
		RunAsUser: &runAsUser,
	}

	ri, err := fetchRuntimeInfo(t, clients, &test.Options{SecurityContext: securityContext})
	if err != nil {
		t.Fatalf("Error fetching runtime info: %v", err)
	}

	if ri.Host == nil {
		t.Fatal("Missing host information from runtime info.")
	}

	if ri.Host.User == nil {
		t.Fatal("Missing user information from runtime info.")
	}

	if got, want := ri.Host.User.UID, userID; got != want {
		t.Errorf("uid = %d, want: %d", got, want)
	}

	// We expect the effective userID to match the userID as we
	// did not use setuid.
	if got, want := ri.Host.User.EUID, userID; got != want {
		t.Errorf("euid = %d, want: %d", got, want)
	}
}
