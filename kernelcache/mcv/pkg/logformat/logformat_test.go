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

package logformat

import (
	"testing"

	logging "github.com/sirupsen/logrus"
)

func TestConfigureLogging(t *testing.T) {
	tests := []struct {
		name      string
		logLevel  string
		expectErr bool
	}{
		{"Valid log level: info", "info", false},
		{"Valid log level: debug", "debug", false},
		{"Invalid log level", "invalid", true},
		{"Empty log level", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ConfigureLogging(tt.logLevel)
			if (err != nil) != tt.expectErr {
				t.Errorf("ConfigureLogging(%q) error = %v, expectErr = %v", tt.logLevel, err, tt.expectErr)
			}

			if err == nil && tt.logLevel != "" {
				level, _ := logging.ParseLevel(tt.logLevel)
				if logging.GetLevel() != level {
					t.Errorf("Expected log level %v, got %v", level, logging.GetLevel())
				}
			}
		})
	}
}
