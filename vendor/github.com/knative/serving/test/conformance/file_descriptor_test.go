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
)

// TestMustHaveCgroupConfigured verifies using the runtime test container that reading from the
// stdin file descriptor results in EOF.
func TestShouldHaveStdinEOF(t *testing.T) {
	clients := setup(t)

	ri, err := fetchRuntimeInfo(t, clients, &test.Options{})
	if err != nil {
		t.Fatalf("Error fetching runtime info: %v", err)
	}

	if ri.Host == nil {
		t.Fatal("Missing host information from runtime info.")
	}
	stdin := ri.Host.Stdin
	if stdin == nil {
		t.Fatal("Missing stdin information from host info.")
	}

	if stdin.Error != "" {
		t.Fatalf("Error reading stdin: %v", stdin.Error)
	}

	if got, want := *stdin.EOF, true; got != want {
		t.Errorf("Stdin.EOF = %t, expected: %t", got, want)
	}
}
