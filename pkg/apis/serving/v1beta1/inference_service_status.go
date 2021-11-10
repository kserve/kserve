/*

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
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
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
	// URL holds the url that will distribute traffic over the provided traffic targets.
	// It generally has the form http[s]://{route-name}.{route-namespace}.{cluster-level-suffix}
	// +optional
	URL *apis.URL `json:"url,omitempty"`
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
	// PredictorRoutesReady is set when network configuration has completed.
	PredictorRouteReady apis.ConditionType = "PredictorRouteReady"
	// TransformerRoutesReady is set when network configuration has completed.
	TransformerRouteReady apis.ConditionType = "TransformerRouteReady"
	// ExplainerRoutesReady is set when network configuration has completed.
	ExplainerRoutesReady apis.ConditionType = "ExplainerRoutesReady"
	// PredictorConfigurationReady is set when predictor pods are ready.
	PredictorConfigurationReady apis.ConditionType = "PredictorConfigurationReady"
	// TransformerConfigurationeReady is set when transformer pods are ready.
	TransformerConfigurationeReady apis.ConditionType = "TransformerConfigurationeReady"
	// ExplainerConfigurationReady is set when explainer pods are ready.
	ExplainerConfigurationReady apis.ConditionType = "ExplainerConfigurationReady"
	// PredictorReady is set when predictor has reported readiness.
	PredictorReady apis.ConditionType = "PredictorReady"
	// TransformerReady is set when transformer has reported readiness.
	TransformerReady apis.ConditionType = "TransformerReady"
	// ExplainerReady is set when explainer has reported readiness.
	ExplainerReady apis.ConditionType = "ExplainerReady"
	// Ingress is created
	IngressReady apis.ConditionType = "IngressReady"
)

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
	TransformerComponent: TransformerConfigurationeReady,
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
						statusSpec.LatestRolledoutRevision = statusSpec.PreviousRolledoutRevision
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
	routeCondition := serviceStatus.GetCondition("ConfigurationsReady")
	routeConditionType := routeConditionsMap[component]
	ss.SetCondition(routeConditionType, routeCondition)
	// propagate configuration condition for each component
	configurationCondition := serviceStatus.GetCondition("RoutesReady")
	configurationConditionType := configurationConditionsMap[component]
	// propagate traffic status for each component
	statusSpec.Traffic = serviceStatus.Traffic
	ss.SetCondition(configurationConditionType, configurationCondition)

	ss.Components[component] = statusSpec
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
