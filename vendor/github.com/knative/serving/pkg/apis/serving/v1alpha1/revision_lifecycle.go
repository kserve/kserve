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
	"fmt"
	"strconv"
	"time"

	duckv1alpha1 "github.com/knative/pkg/apis/duck/v1alpha1"
	net "github.com/knative/serving/pkg/apis/networking"
	"github.com/knative/serving/pkg/apis/serving"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

const (
	// UserPortName is the name that will be used for the Port on the
	// Deployment and Pod created by a Revision. This name will be set regardless of if
	// a user specifies a port or the default value is chosen.
	UserPortName = "user-port"

	// DefaultUserPort is the default port value the QueueProxy will
	// use for connecting to the user container.
	DefaultUserPort = 8080

	// RequestQueuePortName specifies the port name to use for http requests
	// in queue-proxy container.
	RequestQueuePortName string = "queue-port"

	// RequestQueuePort specifies the port number to use for http requests
	// in queue-proxy container.
	RequestQueuePort = 8012

	// RequestQueueAdminPortName specifies the port name for
	// health check and lifecyle hooks for queue-proxy.
	RequestQueueAdminPortName string = "queueadm-port"

	// RequestQueueAdminPort specifies the port number for
	// health check and lifecyle hooks for queue-proxy.
	RequestQueueAdminPort = 8022

	// RequestQueueMetricsPort specifies the port number for metrics emitted
	// by queue-proxy.
	RequestQueueMetricsPort = 9090

	// RequestQueueMetricsPortName specifies the port name to use for metrics
	// emitted by queue-proxy.
	RequestQueueMetricsPortName = "queue-metrics"
)

var revCondSet = duckv1alpha1.NewLivingConditionSet(
	RevisionConditionResourcesAvailable,
	RevisionConditionContainerHealthy,
	RevisionConditionBuildSucceeded,
)

var buildCondSet = duckv1alpha1.NewBatchConditionSet()

func (r *Revision) GetGroupVersionKind() schema.GroupVersionKind {
	return SchemeGroupVersion.WithKind("Revision")
}

func (r *Revision) BuildRef() *corev1.ObjectReference {
	if r.Spec.BuildRef != nil {
		buildRef := r.Spec.BuildRef.DeepCopy()
		if buildRef.Namespace == "" {
			buildRef.Namespace = r.Namespace
		}
		return buildRef
	}

	if r.Spec.DeprecatedBuildName != "" {
		return &corev1.ObjectReference{
			APIVersion: "build.knative.dev/v1alpha1",
			Kind:       "Build",
			Namespace:  r.Namespace,
			Name:       r.Spec.DeprecatedBuildName,
		}
	}

	return nil
}

// GetProtocol returns the app level network protocol.
func (r *Revision) GetProtocol() net.ProtocolType {
	ports := r.Spec.Container.Ports
	if len(ports) > 0 && ports[0].Name == string(net.ProtocolH2C) {
		return net.ProtocolH2C
	}

	return net.ProtocolHTTP1
}

// IsReady looks at the conditions and if the Status has a condition
// RevisionConditionReady returns true if ConditionStatus is True
func (rs *RevisionStatus) IsReady() bool {
	return revCondSet.Manage(rs).IsHappy()
}

// IsActivationRequired returns true if activation is required.
func (rs *RevisionStatus) IsActivationRequired() bool {
	if c := revCondSet.Manage(rs).GetCondition(RevisionConditionActive); c != nil {
		return c.Status != corev1.ConditionTrue
	}
	return false
}

func (rs *RevisionStatus) GetCondition(t duckv1alpha1.ConditionType) *duckv1alpha1.Condition {
	return revCondSet.Manage(rs).GetCondition(t)
}

func (rs *RevisionStatus) InitializeConditions() {
	revCondSet.Manage(rs).InitializeConditions()
}

func (rs *RevisionStatus) PropagateBuildStatus(bs duckv1alpha1.Status) {
	bc := buildCondSet.Manage(&bs).GetCondition(duckv1alpha1.ConditionSucceeded)
	if bc == nil {
		return
	}
	switch {
	case bc.Status == corev1.ConditionUnknown:
		revCondSet.Manage(rs).MarkUnknown(RevisionConditionBuildSucceeded, "Building", bc.Message)
	case bc.Status == corev1.ConditionTrue:
		revCondSet.Manage(rs).MarkTrue(RevisionConditionBuildSucceeded)
	case bc.Status == corev1.ConditionFalse:
		revCondSet.Manage(rs).MarkFalse(RevisionConditionBuildSucceeded, bc.Reason, bc.Message)
	}
}

// MarkResourceNotOwned changes the "ResourcesAvailable" condition to false to reflect that the
// resource of the given kind and name has already been created, and we do not own it.
func (rs *RevisionStatus) MarkResourceNotOwned(kind, name string) {
	revCondSet.Manage(rs).MarkFalse(RevisionConditionResourcesAvailable, "NotOwned",
		fmt.Sprintf("There is an existing %s %q that we do not own.", kind, name))
}

func (rs *RevisionStatus) MarkDeploying(reason string) {
	revCondSet.Manage(rs).MarkUnknown(RevisionConditionResourcesAvailable, reason, "")
	revCondSet.Manage(rs).MarkUnknown(RevisionConditionContainerHealthy, reason, "")
}

func (rs *RevisionStatus) MarkServiceTimeout() {
	revCondSet.Manage(rs).MarkFalse(RevisionConditionResourcesAvailable, "ServiceTimeout",
		"Timed out waiting for a service endpoint to become ready")
}

func (rs *RevisionStatus) MarkProgressDeadlineExceeded(message string) {
	revCondSet.Manage(rs).MarkFalse(RevisionConditionResourcesAvailable, "ProgressDeadlineExceeded", message)
}

func (rs *RevisionStatus) MarkContainerHealthy() {
	revCondSet.Manage(rs).MarkTrue(RevisionConditionContainerHealthy)
}

func (rs *RevisionStatus) MarkContainerExiting(exitCode int32, message string) {
	exitCodeString := fmt.Sprintf("ExitCode%d", exitCode)
	revCondSet.Manage(rs).MarkFalse(RevisionConditionContainerHealthy, exitCodeString, RevisionContainerExitingMessage(message))
}

func (rs *RevisionStatus) MarkResourcesAvailable() {
	revCondSet.Manage(rs).MarkTrue(RevisionConditionResourcesAvailable)
}

func (rs *RevisionStatus) MarkActive() {
	revCondSet.Manage(rs).MarkTrue(RevisionConditionActive)
}

func (rs *RevisionStatus) MarkActivating(reason, message string) {
	revCondSet.Manage(rs).MarkUnknown(RevisionConditionActive, reason, message)
}

func (rs *RevisionStatus) MarkInactive(reason, message string) {
	revCondSet.Manage(rs).MarkFalse(RevisionConditionActive, reason, message)
}

func (rs *RevisionStatus) MarkContainerMissing(message string) {
	revCondSet.Manage(rs).MarkFalse(RevisionConditionContainerHealthy, "ContainerMissing", message)
}

// RevisionContainerMissingMessage constructs the status message if a given image
// cannot be pulled correctly.
func RevisionContainerMissingMessage(image string, message string) string {
	return fmt.Sprintf("Unable to fetch image %q: %s", image, message)
}

// RevisionContainerExitingMessage constructs the status message if a container
// fails to come up.
func RevisionContainerExitingMessage(message string) string {
	return fmt.Sprintf("Container failed with: %s", message)
}

const (
	AnnotationParseErrorTypeMissing = "Missing"
	AnnotationParseErrorTypeInvalid = "Invalid"
	LabelParserErrorTypeMissing     = "Missing"
	LabelParserErrorTypeInvalid     = "Invalid"
)

// +k8s:deepcopy-gen=false
type AnnotationParseError struct {
	Type  string
	Value string
	Err   error
}

// +k8s:deepcopy-gen=false
type LastPinnedParseError AnnotationParseError

func (e LastPinnedParseError) Error() string {
	return fmt.Sprintf("%v lastPinned value: %q", e.Type, e.Value)
}

// +k8s:deepcopy-gen=false
type configurationGenerationParseError AnnotationParseError

func (e configurationGenerationParseError) Error() string {
	return fmt.Sprintf("%v configurationGeneration value: %q", e.Type, e.Value)
}

func RevisionLastPinnedString(t time.Time) string {
	return fmt.Sprintf("%d", t.Unix())
}

func (r *Revision) SetLastPinned(t time.Time) {
	if r.ObjectMeta.Annotations == nil {
		r.ObjectMeta.Annotations = make(map[string]string)
	}

	r.ObjectMeta.Annotations[serving.RevisionLastPinnedAnnotationKey] = RevisionLastPinnedString(t)
}

func (r *Revision) GetLastPinned() (time.Time, error) {
	if r.Annotations == nil {
		return time.Time{}, LastPinnedParseError{
			Type: AnnotationParseErrorTypeMissing,
		}
	}

	str, ok := r.ObjectMeta.Annotations[serving.RevisionLastPinnedAnnotationKey]
	if !ok {
		// If a revision is past the create delay without an annotation it is stale
		return time.Time{}, LastPinnedParseError{
			Type: AnnotationParseErrorTypeMissing,
		}
	}

	secs, err := strconv.ParseInt(str, 10, 64)
	if err != nil {
		return time.Time{}, LastPinnedParseError{
			Type:  AnnotationParseErrorTypeInvalid,
			Value: str,
			Err:   err,
		}
	}

	return time.Unix(secs, 0), nil
}
