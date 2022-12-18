/*
Copyright 2021 The KServe Authors.

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

package v1beta1

import (
	"reflect"

	"github.com/kserve/kserve/pkg/constants"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"knative.dev/pkg/apis"
	duckv1 "knative.dev/pkg/apis/duck/v1"
	knservingv1 "knative.dev/serving/pkg/apis/serving/v1"
)

// InferenceServiceStatus defines the observed state of InferenceService
type InferenceServiceStatus struct {
	// Conditions for the InferenceService <br/>
	// - PredictorReady: predictor readiness condition; <br/>
	// - TransformerReady: transformer readiness condition; <br/>
	// - ExplainerReady: explainer readiness condition; <br/>
	// - RoutesReady: aggregated routing condition; <br/>
	// - Ready: aggregated condition; <br/>
	duckv1.Status `json:",inline"`
	// Addressable endpoint for the InferenceService
	// +optional
	Address *duckv1.Addressable `json:"address,omitempty"`
	// URL holds the url that will distribute traffic over the provided traffic targets.
	// It generally has the form http[s]://{route-name}.{route-namespace}.{cluster-level-suffix}
	// +optional
	URL *apis.URL `json:"url,omitempty"`
	// Statuses for the components of the InferenceService
	Components map[ComponentType]ComponentStatusSpec `json:"components,omitempty"`
	// Model related statuses
	ModelStatus ModelStatus `json:"modelStatus,omitempty"`
}

// ComponentStatusSpec describes the state of the component
type ComponentStatusSpec struct {
	// Latest revision name that is in ready state
	// +optional
	LatestReadyRevision string `json:"latestReadyRevision,omitempty"`
	// Latest revision name that is created
	// +optional
	LatestCreatedRevision string `json:"latestCreatedRevision,omitempty"`
	// Previous revision name that is rolled out with 100 percent traffic
	// +optional
	PreviousRolledoutRevision string `json:"previousRolledoutRevision,omitempty"`
	// Latest revision name that is rolled out with 100 percent traffic
	// +optional
	LatestRolledoutRevision string `json:"latestRolledoutRevision,omitempty"`
	// Traffic holds the configured traffic distribution for latest ready revision and previous rolled out revision.
	// +optional
	Traffic []knservingv1.TrafficTarget `json:"traffic,omitempty"`
	// URL holds the primary url that will distribute traffic over the provided traffic targets.
	// This will be one the REST or gRPC endpoints that are available.
	// It generally has the form http[s]://{route-name}.{route-namespace}.{cluster-level-suffix}
	// +optional
	URL *apis.URL `json:"url,omitempty"`
	// REST endpoint of the component if available.
	// +optional
	RestURL *apis.URL `json:"restUrl,omitempty"`
	// gRPC endpoint of the component if available.
	// +optional
	GrpcURL *apis.URL `json:"grpcUrl,omitempty"`
	// Addressable endpoint for the InferenceService
	// +optional
	Address *duckv1.Addressable `json:"address,omitempty"`
}

// ComponentType contains the different types of components of the service
type ComponentType string

// ComponentType Enum
const (
	PredictorComponent   ComponentType = "predictor"
	ExplainerComponent   ComponentType = "explainer"
	TransformerComponent ComponentType = "transformer"
)

// ConditionType represents a Service condition value
const (
	// PredictorRouteReady  is set when network configuration has completed.
	PredictorRouteReady apis.ConditionType = "PredictorRouteReady"
	// TransformerRouteReady is set when network configuration has completed.
	TransformerRouteReady apis.ConditionType = "TransformerRouteReady"
	// ExplainerRoutesReady is set when network configuration has completed.
	ExplainerRoutesReady apis.ConditionType = "ExplainerRoutesReady"
	// PredictorConfigurationReady is set when predictor pods are ready.
	PredictorConfigurationReady apis.ConditionType = "PredictorConfigurationReady"
	// TransformerConfigurationReady is set when transformer pods are ready.
	TransformerConfigurationReady  apis.ConditionType = "TransformerConfigurationReady"
	TransformerConfigurationeReady apis.ConditionType = "TransformerConfigurationeReady"
	// ExplainerConfigurationReady is set when explainer pods are ready.
	ExplainerConfigurationReady apis.ConditionType = "ExplainerConfigurationReady"
	// PredictorReady is set when predictor has reported readiness.
	PredictorReady apis.ConditionType = "PredictorReady"
	// TransformerReady is set when transformer has reported readiness.
	TransformerReady apis.ConditionType = "TransformerReady"
	// ExplainerReady is set when explainer has reported readiness.
	ExplainerReady apis.ConditionType = "ExplainerReady"
	// IngressReady is set when Ingress is created
	IngressReady apis.ConditionType = "IngressReady"
)

type ModelStatus struct {
	// Whether the available predictor endpoints reflect the current Spec or is in transition
	// +kubebuilder:default=UpToDate
	TransitionStatus TransitionStatus `json:"transitionStatus"`

	// State information of the predictor's model.
	// +optional
	ModelRevisionStates *ModelRevisionStates `json:"states,omitempty"`

	// Details of last failure, when load of target model is failed or blocked.
	// +optional
	LastFailureInfo *FailureInfo `json:"lastFailureInfo,omitempty"`

	// Model copy information of the predictor's model.
	// +optional
	ModelCopies *ModelCopies `json:"copies,omitempty"`
}

type ModelRevisionStates struct {
	// High level state string: Pending, Standby, Loading, Loaded, FailedToLoad
	// +kubebuilder:default=Pending
	ActiveModelState ModelState `json:"activeModelState"`
	// +kubebuilder:default=""
	TargetModelState ModelState `json:"targetModelState,omitempty"`
}

type ModelCopies struct {
	// How many copies of this predictor's models failed to load recently
	// +kubebuilder:default=0
	FailedCopies int `json:"failedCopies"`
	// Total number copies of this predictor's models that are currently loaded
	// +optional
	TotalCopies int `json:"totalCopies,omitempty"`
}

// TransitionStatus enum
// +kubebuilder:validation:Enum="";UpToDate;InProgress;BlockedByFailedLoad;InvalidSpec
type TransitionStatus string

// TransitionStatus Enum values
const (
	// Predictor is up-to-date (reflects current spec)
	UpToDate TransitionStatus = "UpToDate"
	// Waiting for target model to reach state of active model
	InProgress TransitionStatus = "InProgress"
	// Target model failed to load
	BlockedByFailedLoad TransitionStatus = "BlockedByFailedLoad"
	// Target predictor spec failed validation
	InvalidSpec TransitionStatus = "InvalidSpec"
)

// ModelState enum
// +kubebuilder:validation:Enum="";Pending;Standby;Loading;Loaded;FailedToLoad
type ModelState string

// ModelState Enum values
const (
	// Model is not yet registered
	Pending ModelState = "Pending"
	// Model is available but not loaded (will load when used)
	Standby ModelState = "Standby"
	// Model is loading
	Loading ModelState = "Loading"
	// At least one copy of the model is loaded
	Loaded ModelState = "Loaded"
	// All copies of the model failed to load
	FailedToLoad ModelState = "FailedToLoad"
)

// FailureReason enum
// +kubebuilder:validation:Enum=ModelLoadFailed;RuntimeUnhealthy;RuntimeDisabled;NoSupportingRuntime;RuntimeNotRecognized;InvalidPredictorSpec
type FailureReason string

// FailureReason enum values
const (
	// The model failed to load within a ServingRuntime container
	ModelLoadFailed FailureReason = "ModelLoadFailed"
	// Corresponding ServingRuntime containers failed to start or are unhealthy
	RuntimeUnhealthy FailureReason = "RuntimeUnhealthy"
	// The ServingRuntime is disabled
	RuntimeDisabled FailureReason = "RuntimeDisabled"
	// There are no ServingRuntime which support the specified model type
	NoSupportingRuntime FailureReason = "NoSupportingRuntime"
	// There is no ServingRuntime defined with the specified runtime name
	RuntimeNotRecognized FailureReason = "RuntimeNotRecognized"
	// The current Predictor Spec is invalid or unsupported
	InvalidPredictorSpec FailureReason = "InvalidPredictorSpec"
)

type FailureInfo struct {
	// Name of component to which the failure relates (usually Pod name)
	//+optional
	Location string `json:"location,omitempty"`
	// High level class of failure
	//+optional
	Reason FailureReason `json:"reason,omitempty"`
	// Detailed error message
	//+optional
	Message string `json:"message,omitempty"`
	// Internal Revision/ID of model, tied to specific Spec contents
	//+optional
	ModelRevisionName string `json:"modelRevisionName,omitempty"`
	// Time failure occurred or was discovered
	//+optional
	Time *metav1.Time `json:"time,omitempty"`
	// Exit status from the last termination of the container
	//+optional
	ExitCode int32 `json:"exitCode,omitempty"`
}

var conditionsMap = map[ComponentType]apis.ConditionType{
	PredictorComponent:   PredictorReady,
	ExplainerComponent:   ExplainerReady,
	TransformerComponent: TransformerReady,
}

var routeConditionsMap = map[ComponentType]apis.ConditionType{
	PredictorComponent:   PredictorRouteReady,
	ExplainerComponent:   ExplainerRoutesReady,
	TransformerComponent: TransformerRouteReady,
}

var configurationConditionsMap = map[ComponentType]apis.ConditionType{
	PredictorComponent:   PredictorConfigurationReady,
	ExplainerComponent:   ExplainerConfigurationReady,
	TransformerComponent: TransformerConfigurationReady,
}

// InferenceService Ready condition is depending on predictor and route readiness condition
var conditionSet = apis.NewLivingConditionSet(
	PredictorReady,
	IngressReady,
)

var _ apis.ConditionsAccessor = (*InferenceServiceStatus)(nil)

func (ss *InferenceServiceStatus) InitializeConditions() {
	conditionSet.Manage(ss).InitializeConditions()
}

// IsReady returns if the service is ready to serve the requested configuration.
func (ss *InferenceServiceStatus) IsReady() bool {
	return conditionSet.Manage(ss).IsHappy()
}

// GetCondition returns the condition by name.
func (ss *InferenceServiceStatus) GetCondition(t apis.ConditionType) *apis.Condition {
	return conditionSet.Manage(ss).GetCondition(t)
}

// IsConditionReady returns the readiness for a given condition
func (ss *InferenceServiceStatus) IsConditionReady(t apis.ConditionType) bool {
	return conditionSet.Manage(ss).GetCondition(t) != nil && conditionSet.Manage(ss).GetCondition(t).Status == v1.ConditionTrue
}

func (ss *InferenceServiceStatus) PropagateRawStatus(
	component ComponentType,
	deployment *appsv1.Deployment,
	url *apis.URL) {
	if len(ss.Components) == 0 {
		ss.Components = make(map[ComponentType]ComponentStatusSpec)
	}
	statusSpec, ok := ss.Components[component]
	if !ok {
		ss.Components[component] = ComponentStatusSpec{}
	}

	statusSpec.LatestCreatedRevision = deployment.GetObjectMeta().GetAnnotations()["deployment.kubernetes.io/revision"]
	condition := getDeploymentCondition(deployment, appsv1.DeploymentAvailable)
	if condition != nil && condition.Status == v1.ConditionTrue {
		statusSpec.URL = url
	}
	readyCondition := conditionsMap[component]
	ss.SetCondition(readyCondition, condition)
	ss.Components[component] = statusSpec
	ss.ObservedGeneration = deployment.Status.ObservedGeneration
}

func getDeploymentCondition(deployment *appsv1.Deployment, conditionType appsv1.DeploymentConditionType) *apis.Condition {
	condition := apis.Condition{}
	for _, con := range deployment.Status.Conditions {
		if con.Type == conditionType {
			condition.Type = apis.ConditionType(conditionType)
			condition.Status = con.Status
			condition.Message = con.Message
			condition.LastTransitionTime = apis.VolatileTime{
				Inner: con.LastTransitionTime,
			}
			condition.Reason = con.Reason
			break
		}
	}
	return &condition
}

func (ss *InferenceServiceStatus) PropagateStatus(component ComponentType, serviceStatus *knservingv1.ServiceStatus) {
	if len(ss.Components) == 0 {
		ss.Components = make(map[ComponentType]ComponentStatusSpec)
	}
	statusSpec, ok := ss.Components[component]
	if !ok {
		ss.Components[component] = ComponentStatusSpec{}
	}
	statusSpec.LatestCreatedRevision = serviceStatus.LatestCreatedRevisionName
	revisionTraffic := map[string]int64{}
	for _, traffic := range serviceStatus.Traffic {
		if traffic.Percent != nil {
			revisionTraffic[traffic.RevisionName] += *traffic.Percent
		}
	}
	for _, traffic := range serviceStatus.Traffic {
		if traffic.RevisionName == serviceStatus.LatestReadyRevisionName && traffic.LatestRevision != nil &&
			*traffic.LatestRevision {
			if statusSpec.LatestRolledoutRevision != serviceStatus.LatestReadyRevisionName {
				if traffic.Percent != nil && *traffic.Percent == 100 {
					// track the last revision that's rolled out
					statusSpec.PreviousRolledoutRevision = statusSpec.LatestRolledoutRevision
					statusSpec.LatestRolledoutRevision = serviceStatus.LatestReadyRevisionName
				}
			} else {
				// This is to handle case when the latest ready revision is rolled out with 100% and then rolled back
				// so here we need to rollback the LatestRolledoutRevision to PreviousRolledoutRevision
				if serviceStatus.LatestReadyRevisionName == serviceStatus.LatestCreatedRevisionName {
					if traffic.Percent != nil && *traffic.Percent < 100 {
						// check the possibility that the traffic is split over the same revision
						if val, ok := revisionTraffic[traffic.RevisionName]; ok {
							if val == 100 && statusSpec.PreviousRolledoutRevision != "" {
								statusSpec.LatestRolledoutRevision = statusSpec.PreviousRolledoutRevision
							}
						}
					}
				}
			}
		}
	}

	if serviceStatus.LatestReadyRevisionName != statusSpec.LatestReadyRevision {
		statusSpec.LatestReadyRevision = serviceStatus.LatestReadyRevisionName
	}
	// propagate overall service condition
	serviceCondition := serviceStatus.GetCondition(knservingv1.ServiceConditionReady)
	if serviceCondition != nil && serviceCondition.Status == v1.ConditionTrue {
		if serviceStatus.Address != nil {
			statusSpec.Address = serviceStatus.Address
		}
		if serviceStatus.URL != nil {
			statusSpec.URL = serviceStatus.URL
		}
	}
	// propagate ready condition for each component
	readyCondition := conditionsMap[component]
	ss.SetCondition(readyCondition, serviceCondition)
	// propagate route condition for each component
	routeCondition := serviceStatus.GetCondition("RoutesReady")
	routeConditionType := routeConditionsMap[component]
	ss.SetCondition(routeConditionType, routeCondition)
	// propagate configuration condition for each component
	configurationCondition := serviceStatus.GetCondition("ConfigurationsReady")
	configurationConditionType := configurationConditionsMap[component]
	// propagate traffic status for each component
	statusSpec.Traffic = serviceStatus.Traffic
	ss.SetCondition(configurationConditionType, configurationCondition)
	// Fix previously incorrectly named condition type
	ss.ClearCondition(TransformerConfigurationeReady)

	ss.Components[component] = statusSpec
	ss.ObservedGeneration = serviceStatus.ObservedGeneration
}

func (ss *InferenceServiceStatus) SetCondition(conditionType apis.ConditionType, condition *apis.Condition) {
	switch {
	case condition == nil:
	case condition.Status == v1.ConditionUnknown:
		conditionSet.Manage(ss).MarkUnknown(conditionType, condition.Reason, condition.Message)
	case condition.Status == v1.ConditionTrue:
		conditionSet.Manage(ss).MarkTrue(conditionType)
	case condition.Status == v1.ConditionFalse:
		conditionSet.Manage(ss).MarkFalse(conditionType, condition.Reason, condition.Message)
	}
}

func (ss *InferenceServiceStatus) ClearCondition(conditionType apis.ConditionType) {
	if conditionSet.Manage(ss).GetCondition(conditionType) != nil {
		conditionSet.Manage(ss).ClearCondition(conditionType)
	}
}

func (ss *InferenceServiceStatus) UpdateModelRevisionStates(modelState ModelState, totalCopies int, info *FailureInfo) {
	if ss.ModelStatus.ModelRevisionStates == nil {
		ss.ModelStatus.ModelRevisionStates = &ModelRevisionStates{TargetModelState: modelState}
	} else {
		ss.ModelStatus.ModelRevisionStates.TargetModelState = modelState
	}
	// Update transition status, failure info based on new model state
	if modelState == Pending || modelState == Loading {
		ss.ModelStatus.TransitionStatus = InProgress
	} else if modelState == Loaded {
		ss.ModelStatus.TransitionStatus = UpToDate
		ss.ModelStatus.ModelCopies = &ModelCopies{TotalCopies: totalCopies}
		ss.ModelStatus.ModelRevisionStates.ActiveModelState = Loaded
	} else if modelState == FailedToLoad {
		ss.ModelStatus.TransitionStatus = BlockedByFailedLoad
	}
	if info != nil {
		ss.SetModelFailureInfo(info)
	}
}

func (ss *InferenceServiceStatus) UpdateModelTransitionStatus(status TransitionStatus, info *FailureInfo) {
	ss.ModelStatus.TransitionStatus = status
	// Update model state to 'FailedToLoad' in case of invalid spec provided
	if ss.ModelStatus.TransitionStatus == InvalidSpec {
		if ss.ModelStatus.ModelRevisionStates == nil {
			ss.ModelStatus.ModelRevisionStates = &ModelRevisionStates{TargetModelState: FailedToLoad}
		} else {
			ss.ModelStatus.ModelRevisionStates.TargetModelState = FailedToLoad
		}
	}
	if info != nil {
		ss.SetModelFailureInfo(info)
	}
}

func (ss *InferenceServiceStatus) SetModelFailureInfo(info *FailureInfo) bool {
	if reflect.DeepEqual(info, ss.ModelStatus.LastFailureInfo) {
		return false
	}
	ss.ModelStatus.LastFailureInfo = info
	return true
}

func (ss *InferenceServiceStatus) PropagateModelStatus(statusSpec ComponentStatusSpec, podList *v1.PodList, rawDeplyment bool) {
	// Check at least one pod is running for the latest revision of inferenceservice
	totalCopies := len(podList.Items)
	if totalCopies == 0 {
		ss.UpdateModelRevisionStates(Pending, totalCopies, nil)
		return
	}
	// Update model state to 'Loaded' if inferenceservice status is ready.
	// For serverless deployment, the latest created revision and the latest ready revision should be equal
	if ss.IsReady() {
		if rawDeplyment {
			ss.UpdateModelRevisionStates(Loaded, totalCopies, nil)
			return
		} else if statusSpec.LatestCreatedRevision == statusSpec.LatestReadyRevision {
			ss.UpdateModelRevisionStates(Loaded, totalCopies, nil)
			return
		}
	}
	// Update model state to 'Loading' if storage initializer is running.
	// If the storage initializer is terminated due to error or crashloopbackoff, update model
	// state to 'ModelLoadFailed' with failure info.
	for _, cs := range podList.Items[0].Status.InitContainerStatuses {
		if cs.Name == constants.StorageInitializerContainerName {
			if cs.State.Running != nil {
				ss.UpdateModelRevisionStates(Loading, totalCopies, nil)
				return
			} else if cs.State.Terminated != nil &&
				cs.State.Terminated.Reason == constants.StateReasonError {
				ss.UpdateModelRevisionStates(FailedToLoad, totalCopies, &FailureInfo{
					Reason:   ModelLoadFailed,
					Message:  cs.State.Terminated.Message,
					ExitCode: cs.State.Terminated.ExitCode,
				})
				return
			} else if cs.State.Waiting != nil &&
				cs.State.Waiting.Reason == constants.StateReasonCrashLoopBackOff {
				ss.UpdateModelRevisionStates(FailedToLoad, totalCopies, &FailureInfo{
					Reason:   ModelLoadFailed,
					Message:  cs.LastTerminationState.Terminated.Message,
					ExitCode: cs.LastTerminationState.Terminated.ExitCode,
				})
				return
			}
		}
	}
	// If the kserve container is terminated due to error or crashloopbackoff, update model
	// state to 'ModelLoadFailed' with failure info.
	for _, cs := range podList.Items[0].Status.ContainerStatuses {
		if cs.Name == constants.InferenceServiceContainerName {
			if cs.State.Terminated != nil &&
				cs.State.Terminated.Reason == constants.StateReasonError {
				ss.UpdateModelRevisionStates(FailedToLoad, totalCopies, &FailureInfo{
					Reason:   ModelLoadFailed,
					Message:  cs.State.Terminated.Message,
					ExitCode: cs.State.Terminated.ExitCode,
				})
			} else if cs.State.Waiting != nil &&
				cs.State.Waiting.Reason == constants.StateReasonCrashLoopBackOff {
				ss.UpdateModelRevisionStates(FailedToLoad, totalCopies, &FailureInfo{
					Reason:   ModelLoadFailed,
					Message:  cs.LastTerminationState.Terminated.Message,
					ExitCode: cs.LastTerminationState.Terminated.ExitCode,
				})
			} else {
				ss.UpdateModelRevisionStates(Pending, totalCopies, nil)
			}
		}
	}
}
