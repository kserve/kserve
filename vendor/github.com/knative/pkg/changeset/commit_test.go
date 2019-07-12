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

package changeset

import (
	"errors"
	"fmt"
	"os"
	"testing"
)

const (
	nonCommittedHeadContext = "ref: refs/heads/non_committed_branch"
	testCommitID            = "a2d1bdf"
)

func TestReadFile(t *testing.T) {
	tests := []struct {
		name                      string
		koDataPath                string
		koDataPathEnvDoesNotExist bool
		want                      string
		wantErr                   bool
		err                       error
	}{{
		name:       "committed branch",
		koDataPath: "testdata",
		want:       testCommitID,
	}, {
		name:       "non committed branch",
		koDataPath: "testdata/noncommitted",
		wantErr:    true,
		err:        fmt.Errorf("%q is not a valid GitHub commit ID", nonCommittedHeadContext),
	}, {
		name:       "KO_DATA_PATH is empty",
		koDataPath: "",
		wantErr:    true,
		err:        fmt.Errorf("%q does not exist or is empty", koDataPathEnvName),
	}, {
		name:                      "KO_DATA_PATH does not exist",
		koDataPath:                "",
		wantErr:                   true,
		koDataPathEnvDoesNotExist: true,
		err:                       fmt.Errorf("%q does not exist or is empty", koDataPathEnvName),
	}, {
		name:       "HEAD file does not exist",
		koDataPath: "testdata/nonexisting",
		wantErr:    true,
		err:        errors.New("open testdata/nonexisting/HEAD: no such file or directory"),
	}}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.koDataPathEnvDoesNotExist {
				os.Clearenv()
			} else {
				os.Setenv(koDataPathEnvName, test.koDataPath)
			}

			got, err := Get()

			if (err != nil) != test.wantErr {
				t.Errorf("Get() = %v", err)
			}
			if !test.wantErr {
				if test.want != got {
					t.Errorf("wanted %q but got %q", test.want, got)
				}
			} else {
				if err == nil || err.Error() != test.err.Error() {
					t.Errorf("wanted error %v but got: %v", test.err, err)
				}
			}
		})
	}
}
