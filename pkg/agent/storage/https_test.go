/*
Copyright 2023 The KServe Authors.

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

package storage

import (
	"os"
	"testing"
)

func TestCreateNewFileValidation(t *testing.T) {
	tests := []struct {
		name          string
		inputPath     string
		shouldSucceed bool
		description   string
	}{
		{
			name:          "Valid absolute path",
			inputPath:     "/tmp/test/model.bin",
			shouldSucceed: true,
			description:   "Should allow valid absolute paths",
		},
		{
			name:          "Valid relative path",
			inputPath:     "models/pytorch_model.bin",
			shouldSucceed: true,
			description:   "Should allow valid relative paths",
		},
		{
			name:          "Current directory reference",
			inputPath:     ".",
			shouldSucceed: false,
			description:   "Should reject current directory reference",
		},
		{
			name:          "Parent directory reference",
			inputPath:     "..",
			shouldSucceed: false,
			description:   "Should reject parent directory reference",
		},
		{
			name:          "Empty string",
			inputPath:     "",
			shouldSucceed: false,
			description:   "Should reject empty paths",
		},
		{
			name:          "Path that resolves to current dir",
			inputPath:     "./././.",
			shouldSucceed: false,
			description:   "Should reject paths that resolve to current directory",
		},
		{
			name:          "Path that resolves to parent dir",
			inputPath:     "../../../..",
			shouldSucceed: false,
			description:   "Should reject paths that resolve to parent directory",
		},
		{
			name:          "Path with traversal but valid destination",
			inputPath:     "/tmp/../model.bin",
			shouldSucceed: false,
			description:   "Should reject any path containing '..' even if it resolves to valid location",
		},
		// Protected paths test cases
		{
			name:          "Direct /etc path",
			inputPath:     "/etc",
			shouldSucceed: false,
			description:   "Should reject direct access to /etc directory",
		},
		{
			name:          "File in /etc directory",
			inputPath:     "/etc/passwd",
			shouldSucceed: false,
			description:   "Should reject files in /etc directory",
		},
		{
			name:          "Subdirectory in /etc",
			inputPath:     "/etc/ssl/certs/ca-bundle.crt",
			shouldSucceed: false,
			description:   "Should reject files in /etc subdirectories",
		},
		{
			name:          "Direct /bin path",
			inputPath:     "/bin",
			shouldSucceed: false,
			description:   "Should reject direct access to /bin directory",
		},
		{
			name:          "File in /bin directory",
			inputPath:     "/bin/bash",
			shouldSucceed: false,
			description:   "Should reject files in /bin directory",
		},
		{
			name:          "Direct /dev path",
			inputPath:     "/dev",
			shouldSucceed: false,
			description:   "Should reject direct access to /dev directory",
		},
		{
			name:          "File in /dev directory",
			inputPath:     "/dev/null",
			shouldSucceed: false,
			description:   "Should reject files in /dev directory",
		},
		{
			name:          "Direct /usr/bin path",
			inputPath:     "/usr/bin",
			shouldSucceed: false,
			description:   "Should reject direct access to /usr/bin directory",
		},
		{
			name:          "File in /usr/bin directory",
			inputPath:     "/usr/bin/python",
			shouldSucceed: false,
			description:   "Should reject files in /usr/bin directory",
		},
		{
			name:          "Direct /sbin path",
			inputPath:     "/sbin",
			shouldSucceed: false,
			description:   "Should reject direct access to /sbin directory",
		},
		{
			name:          "File in /sbin directory",
			inputPath:     "/sbin/init",
			shouldSucceed: false,
			description:   "Should reject files in /sbin directory",
		},
		{
			name:          "Direct /usr/sbin path",
			inputPath:     "/usr/sbin",
			shouldSucceed: false,
			description:   "Should reject direct access to /usr/sbin directory",
		},
		{
			name:          "File in /usr/sbin directory",
			inputPath:     "/usr/sbin/nginx",
			shouldSucceed: false,
			description:   "Should reject files in /usr/sbin directory",
		},
		// Valid paths that are NOT protected
		{
			name:          "Valid /tmp file",
			inputPath:     "/tmp/model.bin",
			shouldSucceed: true,
			description:   "Should allow files in /tmp directory",
		},
		{
			name:          "Valid /tmp subdirectory file",
			inputPath:     "/tmp/models/pytorch_model.bin",
			shouldSucceed: true,
			description:   "Should allow files in /tmp subdirectories",
		},
		{
			name:          "Valid relative path in current working dir",
			inputPath:     "workdir/model.bin",
			shouldSucceed: true,
			description:   "Should allow relative paths in working directory",
		},
		{
			name:          "Valid relative path with subdirs",
			inputPath:     "models/deep-learning/pytorch_model.bin",
			shouldSucceed: true,
			description:   "Should allow relative paths with subdirectories",
		},
		// Edge cases - paths that look similar but should be allowed
		{
			name:          "Path with 'etc' in middle",
			inputPath:     "etc-backup/config.json",
			shouldSucceed: true,
			description:   "Should allow paths that contain 'etc' but are not in /etc",
		},
		{
			name:          "Path with 'bin' in middle",
			inputPath:     "bin-files/binary.dat",
			shouldSucceed: true,
			description:   "Should allow paths that contain 'bin' but are not in /bin",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test the actual createNewFile method
			file, err := createNewFile(tt.inputPath)

			if tt.shouldSucceed {
				if err != nil {
					t.Errorf("Test %s: expected success but got error: %v", tt.name, err)
				} else {
					// Clean up the created file
					if file != nil {
						if closeErr := file.Close(); closeErr != nil {
							t.Logf("Failed to close file: %v", closeErr)
						}
						// Only remove if it was actually created
						if _, statErr := os.Stat(tt.inputPath); statErr == nil {
							if removeErr := os.Remove(tt.inputPath); removeErr != nil {
								t.Logf("Failed to remove file: %v", removeErr)
							}
						}
					}
					t.Logf("Test %s: SUCCESS - %s", tt.name, tt.inputPath)
				}
			} else {
				if err == nil {
					// Clean up if file was unexpectedly created
					if file != nil {
						if closeErr := file.Close(); closeErr != nil {
							t.Logf("Failed to close file: %v", closeErr)
						}
						if removeErr := os.Remove(tt.inputPath); removeErr != nil {
							t.Logf("Failed to remove file: %v", removeErr)
						}
					}
					t.Errorf("Test %s: expected error but file was created: %s", tt.name, tt.inputPath)
				} else {
					t.Logf("Test %s: REJECTED - %s (error: %v)", tt.name, tt.inputPath, err)
				}
			}
		})
	}
}
