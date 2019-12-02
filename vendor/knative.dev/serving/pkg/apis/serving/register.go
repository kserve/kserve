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

package serving

const (
	// GroupName is the group name for knative labels and annotations
	GroupName = "serving.knative.dev"

	// GroupNamePrefix is the prefix for label key and annotation key
	GroupNamePrefix = GroupName + "/"

	// ConfigurationLabelKey is the label key attached to a Revision indicating by
	// which Configuration it is created.
	ConfigurationLabelKey = GroupName + "/configuration"

	// RevisionLastPinnedAnnotationKey is the annotation key used for determining when a route has
	// pinned a revision
	RevisionLastPinnedAnnotationKey = GroupName + "/lastPinned"

	// RouteLabelKey is the label key attached to a Configuration indicating by
	// which Route it is configured as traffic target.
	// The key can also be attached to ClusterIngress resources to indicate
	// which Route triggered their creation.
	// The key is also attached to k8s Service resources to indicate which Route
	// triggered their creation.
	RouteLabelKey = GroupName + "/route"

	// RouteNamespaceLabelKey is the label key attached to a ClusterIngress
	// by a Route to indicate which namespace the Route was created in.
	RouteNamespaceLabelKey = GroupName + "/routeNamespace"

	// RevisionLabelKey is the label key attached to k8s resources to indicate
	// which Revision triggered their creation.
	RevisionLabelKey = GroupName + "/revision"

	// RevisionUID is the label key attached to a revision to indicate
	// its unique identifier
	RevisionUID = GroupName + "/revisionUID"

	// ServiceLabelKey is the label key attached to a Route and Configuration indicating by
	// which Service they are created.
	ServiceLabelKey = GroupName + "/service"

	// ConfigurationGenerationLabelKey is the label key attached to a Revision indicating the
	// metadata generation of the Configuration that created this revision
	ConfigurationGenerationLabelKey = GroupName + "/configurationGeneration"

	// CreatorAnnotation is the annotation key to describe the user that
	// created the resource.
	CreatorAnnotation = GroupName + "/creator"
	// UpdaterAnnotation is the annotation key to describe the user that
	// last updated the resource.
	UpdaterAnnotation = GroupName + "/lastModifier"

	// QueueSideCarResourcePercentageAnnotation is the percentage of user container resources to be used for queue-proxy
	// It has to be in [0.1,100]
	QueueSideCarResourcePercentageAnnotation = "queue.sidecar." + GroupName + "/resourcePercentage"
)
