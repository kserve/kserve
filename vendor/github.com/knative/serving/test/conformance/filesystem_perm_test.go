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
	"fmt"
	"testing"

	"github.com/knative/serving/test"
)

func verifyPermString(resp string, expected string) error {
	if len(resp) != len(expected) {
		return fmt.Errorf("Length of expected and response string don't match. Expected Response: %q Received Response: %q Expected length: %d Received length: %d",
			expected, resp, len(expected), len(resp))
	}

	for index := range expected {
		if string(expected[index]) != "*" && expected[index] != resp[index] {
			return fmt.Errorf("Permission strings don't match at Index: %d. Expected: %q Received Response: %q",
				index, expected, resp)
		}
	}

	return nil
}

func testFileSystemPermissions(t *testing.T, clients *test.Clients, paths map[string]FilePathInfo) error {
	for key, value := range paths {
		resp, _, err := fetchEnvInfo(t, clients,
			fmt.Sprintf("%s?%s=%s", test.EnvImageFilePathInfoPath, test.EnvImageFilePathQueryParam, key),
			&test.Options{})
		if err != nil {
			return err
		}

		var f FilePathInfo
		if err := json.Unmarshal(resp, &f); err != nil {
			return fmt.Errorf("Error unmarshalling response: %v", err)
		}

		if f.IsDirectory != value.IsDirectory {
			return fmt.Errorf("%s isDirectory = %t, want: %t", key, f.IsDirectory, value.IsDirectory)
		}

		if err := verifyPermString(f.PermString[1:], value.PermString); err != nil {
			return err
		}
	}
	return nil
}

// TestMustHaveFileSystemPermissions asserts that the file system has all the MUST have paths and they have appropriate permissions.
func TestMustHaveFileSystemPermissions(t *testing.T) {
	t.Parallel()
	clients := setup(t)
	if err := testFileSystemPermissions(t, clients, MustFilePathSpecs); err != nil {
		t.Error(err)
	}
}

// TestShouldHaveFileSystemPermissions asserts that the file system has all the SHOULD have paths and they have appropriate permissions.
func TestShouldHaveFileSystemPermissions(t *testing.T) {
	t.Parallel()
	clients := setup(t)
	if err := testFileSystemPermissions(t, clients, ShouldFilePathSpecs); err != nil {
		t.Error(err)
	}
}
