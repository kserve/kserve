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

package v1

import (
	"k8s.io/apimachinery/pkg/runtime/schema"

	"knative.dev/pkg/apis"
	"knative.dev/serving/pkg/apis/config"
)

const (
	// DefaultUserPort is the system default port value exposed on the user-container.
	DefaultUserPort = 8080
)

var revisionCondSet = apis.NewLivingConditionSet()

// GetGroupVersionKind returns the GroupVersionKind.
func (r *Revision) GetGroupVersionKind() schema.GroupVersionKind {
	return SchemeGroupVersion.WithKind("Revision")
}

// IsReady returns if the revision is ready to serve the requested configuration.
func (rs *RevisionStatus) IsReady() bool {
	return revisionCondSet.Manage(rs).IsHappy()
}

// GetContainerConcurrency returns the container concurrency. If
// container concurrency is not set, the default value will be returned.
// We use the original default (0) here for backwards compatibility.
// Previous versions of Knative equated unspecified and zero, so to avoid
// changing the value used by Revisions with unspecified values when a different
// default is configured, we use the original default instead of the configured
// default to remain safe across upgrades.
func (rs *RevisionSpec) GetContainerConcurrency() int64 {
	if rs.ContainerConcurrency == nil {
		return config.DefaultContainerConcurrency
	}
	return *rs.ContainerConcurrency
}
