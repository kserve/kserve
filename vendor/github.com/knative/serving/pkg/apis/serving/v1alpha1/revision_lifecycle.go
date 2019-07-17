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

	"github.com/knative/pkg/apis"
	duckv1beta1 "github.com/knative/pkg/apis/duck/v1beta1"
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

	// QueueAdminPortName specifies the port name for
	// health check and lifecycle hooks for queue-proxy.
	QueueAdminPortName string = "queueadm-port"

	// AutoscalingQueueMetricsPortName specifies the port name to use for metrics
	// emitted by queue-proxy for autoscaler.
	AutoscalingQueueMetricsPortName = "queue-metrics"

	// UserQueueMetricsPortName specifies the port name to use for metrics
	// emitted by queue-proxy for end user.
	UserQueueMetricsPortName = "user-metrics"

	// ServiceQueueMetricsPortName is the name of the port that serves metrics
	// on the Kubernetes service.
	ServiceQueueMetricsPortName = "metrics"
)

var revCondSet = apis.NewLivingConditionSet(
	RevisionConditionResourcesAvailable,
	RevisionConditionContainerHealthy,
)

func (r *Revision) GetGroupVersionKind() schema.GroupVersionKind {
	return SchemeGroupVersion.WithKind("Revision")
}

// GetContainer returns a pointer to the relevant corev1.Container field.
// It is never nil and should be exactly the specified container as guaranteed
// by validation.
func (rs *RevisionSpec) GetContainer() *corev1.Container {
	if rs.DeprecatedContainer != nil {
		return rs.DeprecatedContainer
	}
	if len(rs.Containers) > 0 {
		return &rs.Containers[0]
	}
	// Should be unreachable post-validation, but here to ease testing.
	return &corev1.Container{}
}

func (r *Revision) DeprecatedBuildRef() *corev1.ObjectReference {
	if r.Spec.DeprecatedBuildRef != nil {
		buildRef := r.Spec.DeprecatedBuildRef.DeepCopy()
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
	ports := r.Spec.GetContainer().Ports
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

func (rs *RevisionStatus) GetCondition(t apis.ConditionType) *apis.Condition {
	return revCondSet.Manage(rs).GetCondition(t)
}

func (rs *RevisionStatus) InitializeConditions() {
	revCondSet.Manage(rs).InitializeConditions()
}

// MarkResourceNotConvertible adds a Warning-severity condition to the resource noting that
// it cannot be converted to a higher version.
func (rs *RevisionStatus) MarkResourceNotConvertible(err *CannotConvertError) {
	revCondSet.Manage(rs).SetCondition(apis.Condition{
		Type:     ConditionTypeConvertible,
		Status:   corev1.ConditionFalse,
		Severity: apis.ConditionSeverityWarning,
		Reason:   err.Field,
		Message:  err.Message,
	})
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

// MarkResourcesUnavailable changes "ResourcesAvailable" condition to false to reflect that the
// resources of the given kind and name cannot be created.
func (rs *RevisionStatus) MarkResourcesUnavailable(reason, message string) {
	revCondSet.Manage(rs).MarkFalse(RevisionConditionResourcesAvailable, reason, message)
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

func (rs *RevisionStatus) duck() *duckv1beta1.Status {
	return &rs.Status
}
