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
	duckv1alpha1 "github.com/knative/pkg/apis/duck/v1alpha1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var confCondSet = duckv1alpha1.NewLivingConditionSet()

func (r *Configuration) GetGroupVersionKind() schema.GroupVersionKind {
	return SchemeGroupVersion.WithKind("Configuration")
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

func (cs *ConfigurationStatus) GetCondition(t duckv1alpha1.ConditionType) *duckv1alpha1.Condition {
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
