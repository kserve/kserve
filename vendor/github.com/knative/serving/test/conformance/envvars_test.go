// +build e2e

/*
Copyright 2018 The Knative Authors

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
	"encoding/json"
	"strconv"
	"testing"

	"github.com/knative/serving/test"
)

// TestShouldEnvVars verifies environment variables that are declared as "SHOULD be set" in runtime-contract
func TestShouldEnvVars(t *testing.T) {
	t.Parallel()
	clients := setup(t)
	resp, names, err := fetchEnvInfo(t, clients, test.EnvImageEnvVarsPath, &test.Options{})
	if err != nil {
		t.Fatal(err)
	}

	var respValues ShouldEnvvars
	if err := json.Unmarshal(resp, &respValues); err != nil {
		t.Fatalf("Failed to unmarshall response : %v", err)
	}

	expectedValues := ShouldEnvvars{
		Service:       names.Service,
		Configuration: names.Config,
		Revision:      names.Revision,
	}
	if respValues != expectedValues {
		t.Fatalf("Received response failed to match execpted response. Received: %v Expected: %v", respValues, expectedValues)
	}
}

// TestMustEnvVars verifies environment variables that are declared as "MUST be set" in runtime-contract
func TestMustEnvVars(t *testing.T) {
	t.Parallel()
	clients := setup(t)
	resp, _, err := fetchEnvInfo(t, clients, test.EnvImageEnvVarsPath, &test.Options{})
	if err != nil {
		t.Fatal(err)
	}

	var respValues MustEnvvars
	if err := json.Unmarshal(resp, &respValues); err != nil {
		t.Fatalf("Failed to unmarshall response : %v", err)
	}

	expectedValues := MustEnvvars{
		// The port value needs to match the port exposed by the test-image.
		// We currently control them by using a common constant, but any change needs synchronization between this check
		// and the value used by the test-image.
		Port: strconv.Itoa(test.EnvImageServerPort),
	}
	if respValues != expectedValues {
		t.Fatalf("Received response failed to match execpted response. Received: %v Expected: %v", respValues, expectedValues)
	}
}
