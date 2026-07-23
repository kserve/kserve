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

package utils

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	logging "github.com/sirupsen/logrus"

	"github.com/kserve/kserve/mcv/pkg/constants"
)

// FilePathExists checks if the given file or directory exists.
func FilePathExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

// HasApp checks if the given app is available in the system PATH.
func HasApp(app string) bool {
	_, err := exec.LookPath(app)
	return err == nil
}

// CleanupMCVDirs removes the temporary MCV directory using os.RemoveAll.
func CleanupMCVDirs(ctx context.Context, path string) error {
	if path == "" {
		path = constants.MCVBuildDir
	}
	if err := os.RemoveAll(path); err != nil {
		return fmt.Errorf("failed to delete %s: %w", path, err)
	}
	logging.Debugf("Directory %s successfully deleted.", path)
	return nil
}

// SanitizeGroupJSON strips leading paths before ".triton/cache" in __grp__*.json child_paths.
func SanitizeGroupJSON(filePath string) error {
	data, err := os.ReadFile(filepath.Clean(filePath))
	if err != nil {
		return fmt.Errorf("failed to read %s: %w", filePath, err)
	}

	var parsed map[string]map[string]string
	if err := json.Unmarshal(data, &parsed); err != nil {
		return fmt.Errorf("failed to parse JSON in %s: %w", filePath, err)
	}

	for key, val := range parsed["child_paths"] {
		if idx := strings.Index(val, ".triton/cache"); idx != -1 {
			parsed["child_paths"][key] = val[idx:]
		}
	}

	return writeFormattedJSON(filePath, parsed)
}

// writeFormattedJSON writes the given data as pretty-formatted JSON to a file.
func writeFormattedJSON(filePath string, data interface{}) error {
	formatted, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal JSON: %w", err)
	}
	if err := os.WriteFile(filePath, formatted, 0o640); err != nil {
		return fmt.Errorf("failed to write JSON to %s: %w", filePath, err)
	}
	return nil
}
