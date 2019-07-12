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

	"github.com/knative/test-infra/tools/webhook-apicoverage/resourcetree"
)

// TODO(https://github.com/knative/test-infra/issues/448): evaluate refactoring common, shared code.
// coverage_rules.go contains all the apicoverage rules specified for knative serving.

// IgnoreLowerLevelMetaFields rule ignores metadata nodes that are at a level lower than 2.
// This is done to ensure we only have ObjectMeta and TypeMeta coverage details for higherlevel
// types like Service, Route and not for nodes which appear in spec.
func IgnoreLowerLevelMetaFields(node resourcetree.NodeInterface) bool {
	lowerCaseFieldName := strings.ToLower(node.GetData().Field)
	return !((strings.Contains(lowerCaseFieldName, "objectmeta") || strings.Contains(lowerCaseFieldName, "typemeta")) &&
		len(strings.Split(node.GetData().NodePath, ".")) > 2)
}

// NodeRules contains all resourcetree.NodeRules specified for knative serving.
var NodeRules = resourcetree.NodeRules{
	Rules: []func(node resourcetree.NodeInterface) bool{
		IgnoreLowerLevelMetaFields,
	},
}

// IgnoreDeprecatedFields ignores fields that are prefixed with the word "deprecated"
func IgnoreDeprecatedFields(fieldName string) bool {
	return !strings.HasPrefix(strings.ToLower(fieldName), "deprecated")
}

// FieldRules represent all resourcetree.FieldRules specified for knative serving.
var FieldRules = resourcetree.FieldRules{
	Rules: []func(fieldName string) bool{
		IgnoreDeprecatedFields,
	},
}
