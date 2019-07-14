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

package rules

import (
	"strings"

	"github.com/knative/test-infra/tools/webhook-apicoverage/view"
)

// display_rules.go contains all the display rules specified by knative serving to display json type like result display.

// PackageDisplayRule rule specifies how package name needs to be displayed for json type like result display
func PackageDisplayRule(packageName string) string {
	if packageName != "" {
		tokens := strings.Split(packageName, "/")
		if len(tokens) >= 2 {
			// As package names are built using reflect.Type.PackagePath, they are long.
			// For better readability displaying only last two words of the package path. e.g. serving.v1alpha1
			return strings.Join(tokens[len(tokens)-2:], "/")
		}
	}
	return packageName
}

// GetDisplayRules returns the view.DisplayRules for knative serving.
func GetDisplayRules() view.DisplayRules {
	return view.DisplayRules{
		PackageNameRule: PackageDisplayRule,
	}
}
