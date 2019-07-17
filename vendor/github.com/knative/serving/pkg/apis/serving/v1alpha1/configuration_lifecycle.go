/*
Copyright 2019 The Knative Authors.

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

package v1alpha1

import (
	"github.com/knative/pkg/apis"
	duckv1beta1 "github.com/knative/pkg/apis/duck/v1beta1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var confCondSet = apis.NewLivingConditionSet()

func (r *Configuration) GetGroupVersionKind() schema.GroupVersionKind {
	return SchemeGroupVersion.WithKind("Configuration")
}

// MarkResourceNotConvertible adds a Warning-severity condition to the resource noting that
// it cannot be converted to a higher version.
func (cs *ConfigurationStatus) MarkResourceNotConvertible(err *CannotConvertError) {
	confCondSet.Manage(cs).SetCondition(apis.Condition{
		Type:     ConditionTypeConvertible,
		Status:   corev1.ConditionFalse,
		Severity: apis.ConditionSeverityWarning,
		Reason:   err.Field,
		Message:  err.Message,
	})
}

// GetTemplate returns a pointer to the relevant RevisionTemplateSpec field.
// It is never nil and should be exactly the specified template as guaranteed
// by validation.
func (cs *ConfigurationSpec) GetTemplate() *RevisionTemplateSpec {
	if cs.DeprecatedRevisionTemplate != nil {
		return cs.DeprecatedRevisionTemplate
	}
	if cs.Template != nil {
		return cs.Template
	}
	// Should be unreachable post-validation, but here to ease testing.
	return &RevisionTemplateSpec{}
}

// IsReady looks at the conditions to see if they are happy.
func (cs *ConfigurationStatus) IsReady() bool {
	return confCondSet.Manage(cs).IsHappy()
}

// IsLatestReadyRevisionNameUpToDate returns true if the Configuration is ready
// and LatestCreateRevisionName is equal to LatestReadyRevisionName. Otherwise
// it returns false.
func (cs *ConfigurationStatus) IsLatestReadyRevisionNameUpToDate() bool {
	return cs.IsReady() &&
		cs.LatestCreatedRevisionName == cs.LatestReadyRevisionName
}

func (cs *ConfigurationStatus) GetCondition(t apis.ConditionType) *apis.Condition {
	return confCondSet.Manage(cs).GetCondition(t)
}

func (cs *ConfigurationStatus) InitializeConditions() {
	confCondSet.Manage(cs).InitializeConditions()
}

func (cs *ConfigurationStatus) SetLatestCreatedRevisionName(name string) {
	cs.LatestCreatedRevisionName = name
	if cs.LatestReadyRevisionName != name {
		confCondSet.Manage(cs).MarkUnknown(
			ConfigurationConditionReady,
			"",
			"")
	}
}

func (cs *ConfigurationStatus) SetLatestReadyRevisionName(name string) {
	cs.LatestReadyRevisionName = name
	confCondSet.Manage(cs).MarkTrue(ConfigurationConditionReady)
}

func (cs *ConfigurationStatus) MarkLatestCreatedFailed(name, message string) {
	confCondSet.Manage(cs).MarkFalse(
		ConfigurationConditionReady,
		"RevisionFailed",
		"Revision %q failed with message: %s.", name, message)
}

func (cs *ConfigurationStatus) MarkRevisionCreationFailed(message string) {
	confCondSet.Manage(cs).MarkFalse(
		ConfigurationConditionReady,
		"RevisionFailed",
		"Revision creation failed with message: %s.", message)
}

func (cs *ConfigurationStatus) MarkLatestReadyDeleted() {
	confCondSet.Manage(cs).MarkFalse(
		ConfigurationConditionReady,
		"RevisionDeleted",
		"Revision %q was deleted.", cs.LatestReadyRevisionName)
}

func (cs *ConfigurationStatus) duck() *duckv1beta1.Status {
	return &cs.Status
}
