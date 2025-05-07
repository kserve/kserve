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
	"github.com/kserve/kserve/pkg/constants"
	"github.com/onsi/gomega"
	"net/url"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"knative.dev/pkg/apis"
	"knative.dev/pkg/apis/duck"
	duckv1 "knative.dev/pkg/apis/duck/v1"
	duckv1beta1 "knative.dev/pkg/apis/duck/v1beta1"
	knservingv1 "knative.dev/serving/pkg/apis/serving/v1"
)

func TestInferenceServiceDuckType(t *testing.T) {
	cases := []struct {
		name string
		t    duck.Implementable
	}{{
		name: "conditions",
		t:    &duckv1beta1.Conditions{},
	}}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			err := duck.VerifyType(&InferenceService{}, test.t)
			if err != nil {
				t.Errorf("VerifyType(InferenceService, %T) = %v", test.t, err)
			}
		})
	}
}

func TestInferenceServiceIsReady(t *testing.T) {
	cases := []struct {
		name          string
		ServiceStatus knservingv1.ServiceStatus
		routeStatus   knservingv1.RouteStatus
		isReady       bool
	}{{
		name:          "empty status should not be ready",
		ServiceStatus: knservingv1.ServiceStatus{},
		isReady:       false,
	}, {
		name: "Different condition type should not be ready",
		ServiceStatus: knservingv1.ServiceStatus{
			Status: duckv1.Status{
				Conditions: duckv1.Conditions{{
					Type:   "Foo",
					Status: v1.ConditionTrue,
				}},
			},
		},
		isReady: false,
	}, {
		name: "False condition status should not be ready",
		ServiceStatus: knservingv1.ServiceStatus{
			Status: duckv1.Status{
				Conditions: duckv1.Conditions{{
					Type:   knservingv1.ServiceConditionReady,
					Status: v1.ConditionFalse,
				}},
			},
		},
		isReady: false,
	}, {
		name: "Unknown condition status should not be ready",
		ServiceStatus: knservingv1.ServiceStatus{
			Status: duckv1.Status{
				Conditions: duckv1.Conditions{{
					Type:   knservingv1.ServiceConditionReady,
					Status: v1.ConditionUnknown,
				}},
			},
		},
		isReady: false,
	}, {
		name: "Missing condition status should not be ready",
		ServiceStatus: knservingv1.ServiceStatus{
			Status: duckv1.Status{
				Conditions: duckv1.Conditions{{
					Type: knservingv1.ConfigurationConditionReady,
				}},
			},
		},
		isReady: false,
	}, {
		name: "True condition status should be ready",
		ServiceStatus: knservingv1.ServiceStatus{
			Status: duckv1.Status{
				Conditions: duckv1.Conditions{
					{
						Type:   "ConfigurationsReady",
						Status: v1.ConditionTrue,
					},
					{
						Type:   "RoutesReady",
						Status: v1.ConditionTrue,
					},
					{
						Type:   knservingv1.ServiceConditionReady,
						Status: v1.ConditionTrue,
					},
				},
			},
		},
		isReady: true,
	}, {
		name: "Conditions with ready status should be ready",
		ServiceStatus: knservingv1.ServiceStatus{
			Status: duckv1.Status{
				Conditions: duckv1.Conditions{
					{
						Type:   "Foo",
						Status: v1.ConditionTrue,
					},
					{
						Type:   "RoutesReady",
						Status: v1.ConditionTrue,
					},
					{
						Type:   "ConfigurationsReady",
						Status: v1.ConditionTrue,
					},
					{
						Type:   knservingv1.ServiceConditionReady,
						Status: v1.ConditionTrue,
					},
				},
			},
		},
		isReady: true,
	}, {
		name: "Multiple conditions with ready status false should not be ready",
		ServiceStatus: knservingv1.ServiceStatus{
			Status: duckv1.Status{
				Conditions: duckv1.Conditions{
					{
						Type:   "Foo",
						Status: v1.ConditionTrue,
					},
					{
						Type:   knservingv1.ConfigurationConditionReady,
						Status: v1.ConditionFalse,
					},
				},
			},
		},
		isReady: false,
	}}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			status := InferenceServiceStatus{}
			status.PropagateStatus(PredictorComponent, &tc.ServiceStatus)
			if e, a := tc.isReady, status.IsConditionReady(PredictorReady); e != a {
				t.Errorf("%q expected: %v got: %v conditions: %v", tc.name, e, a, status.Conditions)
			}
		})
	}
}

func TestPropagateRawStatus(t *testing.T) {
	deployment := &appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				"deployment.kubernetes.io/revision": "1",
			},
		},
		Spec: appsv1.DeploymentSpec{},
		Status: appsv1.DeploymentStatus{
			Conditions: []appsv1.DeploymentCondition{
				{
					Type:    appsv1.DeploymentAvailable,
					Status:  v1.ConditionTrue,
					Reason:  "MinimumReplicasAvailable",
					Message: "Deployment has minimum availability.",
					LastTransitionTime: metav1.Time{
						Time: time.Now(),
					},
				},
			},
		},
	}
	status := &InferenceServiceStatus{
		Status:      duckv1.Status{},
		Address:     &duckv1.Addressable{},
		URL:         &apis.URL{},
		ModelStatus: ModelStatus{},
	}
	parsedUrl, _ := url.Parse("http://test-predictor-default.default.example.com")
	url := (*apis.URL)(parsedUrl)
	deploymentList := []*appsv1.Deployment{deployment}
	status.PropagateRawStatus(PredictorComponent, deploymentList, url)
	if res := status.IsConditionReady(PredictorReady); !res {
		t.Errorf("expected: %v got: %v conditions: %v", true, res, status.Conditions)
	}
}

func TestPropagateRawStatusWithMessages(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	errorMsg := "test message"
	reason := "test reason"
	targetStatus := v1.ConditionFalse

	status := &InferenceServiceStatus{
		Status:      duckv1.Status{},
		Address:     nil,
		URL:         nil,
		ModelStatus: ModelStatus{},
	}

	status.PropagateRawStatusWithMessages(PredictorComponent, reason, errorMsg, targetStatus)
	g.Expect(status.IsConditionFalse(PredictorReady)).To(gomega.BeTrue())
	g.Expect(status.Conditions[0].Message).To(gomega.Equal(errorMsg))
	g.Expect(status.Conditions[0].Reason).To(gomega.Equal(reason))
}

func TestPropagateStatus(t *testing.T) {
	parsedUrl, _ := url.Parse("http://test-predictor-default.default.example.com")
	cases := []struct {
		name          string
		ServiceStatus knservingv1.ServiceStatus
		status        InferenceServiceStatus
		isReady       bool
	}{
		{
			name: "Status with Traffic Routing for Latest Revision",
			ServiceStatus: knservingv1.ServiceStatus{
				Status: duckv1.Status{
					Conditions: duckv1.Conditions{
						{
							Type:   "Foo",
							Status: v1.ConditionTrue,
						},
						{
							Type:   "RoutesReady",
							Status: v1.ConditionTrue,
						},
						{
							Type:   "ConfigurationsReady",
							Status: v1.ConditionTrue,
						},
						{
							Type:   knservingv1.ServiceConditionReady,
							Status: v1.ConditionTrue,
						},
					},
				},
				ConfigurationStatusFields: knservingv1.ConfigurationStatusFields{
					LatestReadyRevisionName: "test-predictor-default-0001",
				},
				RouteStatusFields: knservingv1.RouteStatusFields{
					Traffic: []knservingv1.TrafficTarget{
						{
							RevisionName:   "test-predictor-default-0001",
							Percent:        proto.Int64(100),
							LatestRevision: proto.Bool(true),
						},
					},
					Address: &duckv1.Addressable{},
					URL:     (*apis.URL)(parsedUrl),
				},
			},
			status:  InferenceServiceStatus{},
			isReady: true,
		},
		{
			name: "Status with Traffic Routing for Rolledout Revision",
			ServiceStatus: knservingv1.ServiceStatus{
				Status: duckv1.Status{
					Conditions: duckv1.Conditions{
						{
							Type:   "Foo",
							Status: v1.ConditionTrue,
						},
						{
							Type:   "RoutesReady",
							Status: v1.ConditionTrue,
						},
						{
							Type:   "ConfigurationsReady",
							Status: v1.ConditionTrue,
						},
						{
							Type:   knservingv1.ServiceConditionReady,
							Status: v1.ConditionTrue,
						},
					},
				},
				ConfigurationStatusFields: knservingv1.ConfigurationStatusFields{
					LatestReadyRevisionName:   "test-predictor-default-0001",
					LatestCreatedRevisionName: "test-predictor-default-0001",
				},
				RouteStatusFields: knservingv1.RouteStatusFields{
					Traffic: []knservingv1.TrafficTarget{
						{
							RevisionName:   "test-predictor-default-0001",
							Percent:        proto.Int64(90),
							LatestRevision: proto.Bool(true),
						},
					},
					Address: &duckv1.Addressable{},
					URL:     (*apis.URL)(parsedUrl),
				},
			},
			status: InferenceServiceStatus{
				Components: map[ComponentType]ComponentStatusSpec{
					PredictorComponent: {
						LatestRolledoutRevision: "test-predictor-default-0001",
					},
				},
			},
			isReady: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tc.status.PropagateStatus(PredictorComponent, &tc.ServiceStatus)
			if e, a := tc.isReady, tc.status.IsConditionReady(PredictorReady); e != a {
				t.Errorf("%q expected: %v got: %v conditions: %v", tc.name, e, a, tc.status.Conditions)
			}
			if e, a := tc.status.Components[PredictorComponent].Traffic[0], tc.ServiceStatus.Traffic[0]; e != a {
				t.Errorf("%q expected: %v got: %v", tc.name, e, a)
			}
		})
	}
}

func TestInferenceServiceStatus_PropagateModelStatus(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	scenarios := map[string]struct {
		isvcStatus               *InferenceServiceStatus
		statusSpec               ComponentStatusSpec
		podList                  *v1.PodList
		rawDeployment            bool
		serviceStatus            *knservingv1.ServiceStatus
		expectedRevisionStates   *ModelRevisionStates
		expectedTransitionStatus TransitionStatus
		expectedFailureInfo      *FailureInfo
		expectedReturnValue      bool
	}{
		"pod list is empty": {
			isvcStatus: &InferenceServiceStatus{
				Status: duckv1.Status{
					Conditions: duckv1.Conditions{
						{
							Type:   "Ready",
							Status: v1.ConditionFalse,
						},
					},
				},
				Address: &duckv1.Addressable{},
				URL:     &apis.URL{},
				Components: map[ComponentType]ComponentStatusSpec{
					PredictorComponent: {
						LatestRolledoutRevision: "test-predictor-default-0001",
					},
				},
				ModelStatus: ModelStatus{},
			},
			statusSpec: ComponentStatusSpec{
				LatestReadyRevision:       "",
				LatestCreatedRevision:     "",
				PreviousRolledoutRevision: "",
				LatestRolledoutRevision:   "",
				Traffic:                   nil,
				URL:                       nil,
				RestURL:                   nil,
				GrpcURL:                   nil,
				Address:                   nil,
			},
			podList: &v1.PodList{
				TypeMeta: metav1.TypeMeta{},
				ListMeta: metav1.ListMeta{},
				Items:    []v1.Pod{},
			},
			rawDeployment: false,
			serviceStatus: &knservingv1.ServiceStatus{
				Status: duckv1.Status{
					Conditions: duckv1.Conditions{
						{
							Type:   "Ready",
							Status: "True",
						},
					},
				},
			},
			expectedRevisionStates: &ModelRevisionStates{
				ActiveModelState: "",
				TargetModelState: Pending,
			},
			expectedTransitionStatus: InProgress,
			expectedFailureInfo:      nil,
			expectedReturnValue:      true,
		},
		"pod list is empty but knative has an error": {
			isvcStatus: &InferenceServiceStatus{
				Status: duckv1.Status{
					Conditions: duckv1.Conditions{
						{
							Type:   "Ready",
							Status: v1.ConditionFalse,
						},
					},
				},
				Address: &duckv1.Addressable{},
				URL:     &apis.URL{},
				Components: map[ComponentType]ComponentStatusSpec{
					PredictorComponent: {
						LatestRolledoutRevision: "test-predictor-default-0001",
					},
				},
				ModelStatus: ModelStatus{},
			},
			statusSpec: ComponentStatusSpec{
				LatestReadyRevision:       "",
				LatestCreatedRevision:     "",
				PreviousRolledoutRevision: "",
				LatestRolledoutRevision:   "",
				Traffic:                   nil,
				URL:                       nil,
				RestURL:                   nil,
				GrpcURL:                   nil,
				Address:                   nil,
			},
			podList: &v1.PodList{ // pod list is empty because the revision failed and scaled down to 0
				TypeMeta: metav1.TypeMeta{},
				ListMeta: metav1.ListMeta{},
				Items:    []v1.Pod{},
			},
			rawDeployment: false,
			serviceStatus: &knservingv1.ServiceStatus{
				Status: duckv1.Status{
					Conditions: duckv1.Conditions{
						{
							Type:    "Ready",
							Status:  "False",
							Reason:  "RevisionFailed",
							Message: "For testing",
						},
					},
				},
			},
			expectedRevisionStates: &ModelRevisionStates{
				ActiveModelState: "",
				TargetModelState: FailedToLoad,
			},
			expectedTransitionStatus: BlockedByFailedLoad,
			expectedFailureInfo:      nil,
			expectedReturnValue:      true,
		},
		"kserve container in pending state": {
			isvcStatus: &InferenceServiceStatus{
				Status: duckv1.Status{
					Conditions: duckv1.Conditions{
						{
							Type:   "Ready",
							Status: v1.ConditionFalse,
						},
					},
				},
				Address: &duckv1.Addressable{},
				URL:     &apis.URL{},
				Components: map[ComponentType]ComponentStatusSpec{
					PredictorComponent: {
						LatestRolledoutRevision: "test-predictor-default-0001",
					},
				},
				ModelStatus: ModelStatus{},
			},
			statusSpec: ComponentStatusSpec{
				LatestReadyRevision:       "",
				LatestCreatedRevision:     "",
				PreviousRolledoutRevision: "",
				LatestRolledoutRevision:   "",
				Traffic:                   nil,
				URL:                       nil,
				RestURL:                   nil,
				GrpcURL:                   nil,
				Address:                   nil,
			},
			podList: &v1.PodList{
				TypeMeta: metav1.TypeMeta{},
				ListMeta: metav1.ListMeta{},
				Items: []v1.Pod{
					{
						TypeMeta: metav1.TypeMeta{},
						ObjectMeta: metav1.ObjectMeta{
							Name: constants.InferenceServiceContainerName,
						},
						Spec: v1.PodSpec{},
						Status: v1.PodStatus{
							ContainerStatuses: []v1.ContainerStatus{
								{
									Name:                 constants.InferenceServiceContainerName,
									State:                v1.ContainerState{},
									LastTerminationState: v1.ContainerState{},
									Ready:                false,
									RestartCount:         0,
									Image:                "",
									ImageID:              "",
									ContainerID:          "",
									Started:              nil,
								},
							},
						},
					},
				},
			},
			rawDeployment: false,
			serviceStatus: &knservingv1.ServiceStatus{
				Status: duckv1.Status{
					Conditions: duckv1.Conditions{
						{
							Type:   "Ready",
							Status: "True",
						},
					},
				},
			},
			expectedRevisionStates: &ModelRevisionStates{
				ActiveModelState: "",
				TargetModelState: Pending,
			},
			expectedTransitionStatus: InProgress,
			expectedFailureInfo:      nil,
			expectedReturnValue:      true,
		},
		"kserve container failed due to an error": {
			isvcStatus: &InferenceServiceStatus{
				Status: duckv1.Status{
					Conditions: duckv1.Conditions{
						{
							Type:   "Ready",
							Status: v1.ConditionFalse,
						},
					},
				},
				Address: &duckv1.Addressable{},
				URL:     &apis.URL{},
				Components: map[ComponentType]ComponentStatusSpec{
					PredictorComponent: {
						LatestRolledoutRevision: "test-predictor-default-0001",
					},
				},
				ModelStatus: ModelStatus{},
			},
			statusSpec: ComponentStatusSpec{
				LatestReadyRevision:       "",
				LatestCreatedRevision:     "",
				PreviousRolledoutRevision: "",
				LatestRolledoutRevision:   "",
				Traffic:                   nil,
				URL:                       nil,
				RestURL:                   nil,
				GrpcURL:                   nil,
				Address:                   nil,
			},
			podList: &v1.PodList{
				TypeMeta: metav1.TypeMeta{},
				ListMeta: metav1.ListMeta{},
				Items: []v1.Pod{
					{
						TypeMeta: metav1.TypeMeta{},
						ObjectMeta: metav1.ObjectMeta{
							Name: constants.InferenceServiceContainerName,
						},
						Spec: v1.PodSpec{},
						Status: v1.PodStatus{
							ContainerStatuses: []v1.ContainerStatus{
								{
									Name: constants.InferenceServiceContainerName,
									State: v1.ContainerState{
										Terminated: &v1.ContainerStateTerminated{
											ExitCode: 1,
											Reason:   constants.StateReasonError,
											Message:  "For testing",
										},
									},
									LastTerminationState: v1.ContainerState{},
									Ready:                false,
									RestartCount:         0,
									Image:                "",
									ImageID:              "",
									ContainerID:          "",
									Started:              nil,
								},
							},
						},
					},
				},
			},
			rawDeployment: false,
			serviceStatus: &knservingv1.ServiceStatus{
				Status: duckv1.Status{
					Conditions: duckv1.Conditions{
						{
							Type:   "Ready",
							Status: "True",
						},
					},
				},
			},
			expectedRevisionStates: &ModelRevisionStates{
				ActiveModelState: "",
				TargetModelState: FailedToLoad,
			},
			expectedTransitionStatus: BlockedByFailedLoad,
			expectedFailureInfo: &FailureInfo{
				Reason:   ModelLoadFailed,
				Message:  "For testing",
				ExitCode: 1,
			},
			expectedReturnValue: true,
		},
		"kserve container failed due to crash loopBackOff": {
			isvcStatus: &InferenceServiceStatus{
				Status: duckv1.Status{
					Conditions: duckv1.Conditions{
						{
							Type:   "Ready",
							Status: v1.ConditionFalse,
						},
					},
				},
				Address: &duckv1.Addressable{},
				URL:     &apis.URL{},
				Components: map[ComponentType]ComponentStatusSpec{
					PredictorComponent: {
						LatestRolledoutRevision: "test-predictor-default-0001",
					},
				},
				ModelStatus: ModelStatus{},
			},
			statusSpec: ComponentStatusSpec{
				LatestReadyRevision:       "",
				LatestCreatedRevision:     "",
				PreviousRolledoutRevision: "",
				LatestRolledoutRevision:   "",
				Traffic:                   nil,
				URL:                       nil,
				RestURL:                   nil,
				GrpcURL:                   nil,
				Address:                   nil,
			},
			podList: &v1.PodList{
				TypeMeta: metav1.TypeMeta{},
				ListMeta: metav1.ListMeta{},
				Items: []v1.Pod{
					{
						TypeMeta: metav1.TypeMeta{},
						ObjectMeta: metav1.ObjectMeta{
							Name: constants.InferenceServiceContainerName,
						},
						Spec: v1.PodSpec{},
						Status: v1.PodStatus{
							ContainerStatuses: []v1.ContainerStatus{
								{
									Name: constants.InferenceServiceContainerName,
									State: v1.ContainerState{
										Waiting: &v1.ContainerStateWaiting{
											Reason:  constants.StateReasonCrashLoopBackOff,
											Message: "For testing",
										},
									},
									LastTerminationState: v1.ContainerState{
										Terminated: &v1.ContainerStateTerminated{
											Reason:   constants.StateReasonCrashLoopBackOff,
											Message:  "For testing",
											ExitCode: 1,
										},
									},
									Ready:        false,
									RestartCount: 0,
									Image:        "",
									ImageID:      "",
									ContainerID:  "",
									Started:      nil,
								},
							},
						},
					},
				},
			},
			rawDeployment: false,
			serviceStatus: &knservingv1.ServiceStatus{
				Status: duckv1.Status{
					Conditions: duckv1.Conditions{
						{
							Type:   "Ready",
							Status: "True",
						},
					},
				},
			},
			expectedRevisionStates: &ModelRevisionStates{
				ActiveModelState: "",
				TargetModelState: FailedToLoad,
			},
			expectedTransitionStatus: BlockedByFailedLoad,
			expectedFailureInfo: &FailureInfo{
				Reason:   ModelLoadFailed,
				Message:  "For testing",
				ExitCode: 1,
			},
			expectedReturnValue: true,
		},
		"storage initializer failed due to an error": {
			isvcStatus: &InferenceServiceStatus{
				Status: duckv1.Status{
					Conditions: duckv1.Conditions{
						{
							Type:   "Ready",
							Status: v1.ConditionFalse,
						},
					},
				},
				Address: &duckv1.Addressable{},
				URL:     &apis.URL{},
				Components: map[ComponentType]ComponentStatusSpec{
					PredictorComponent: {
						LatestRolledoutRevision: "test-predictor-default-0001",
					},
				},
				ModelStatus: ModelStatus{},
			},
			statusSpec: ComponentStatusSpec{
				LatestReadyRevision:       "",
				LatestCreatedRevision:     "",
				PreviousRolledoutRevision: "",
				LatestRolledoutRevision:   "",
				Traffic:                   nil,
				URL:                       nil,
				RestURL:                   nil,
				GrpcURL:                   nil,
				Address:                   nil,
			},
			podList: &v1.PodList{
				TypeMeta: metav1.TypeMeta{},
				ListMeta: metav1.ListMeta{},
				Items: []v1.Pod{
					{
						TypeMeta: metav1.TypeMeta{},
						ObjectMeta: metav1.ObjectMeta{
							Name: constants.StorageInitializerContainerName,
						},
						Spec: v1.PodSpec{},
						Status: v1.PodStatus{
							InitContainerStatuses: []v1.ContainerStatus{
								{
									Name: constants.StorageInitializerContainerName,
									State: v1.ContainerState{
										Terminated: &v1.ContainerStateTerminated{
											ExitCode: 1,
											Reason:   constants.StateReasonError,
											Message:  "For testing",
										},
									},
									LastTerminationState: v1.ContainerState{},
									Ready:                false,
									RestartCount:         0,
									Image:                "",
									ImageID:              "",
									ContainerID:          "",
									Started:              nil,
								},
							},
						},
					},
				},
			},
			rawDeployment: false,
			serviceStatus: &knservingv1.ServiceStatus{
				Status: duckv1.Status{
					Conditions: duckv1.Conditions{
						{
							Type:   "Ready",
							Status: "True",
						},
					},
				},
			},
			expectedRevisionStates: &ModelRevisionStates{
				ActiveModelState: "",
				TargetModelState: FailedToLoad,
			},
			expectedTransitionStatus: BlockedByFailedLoad,
			expectedFailureInfo: &FailureInfo{
				Reason:   ModelLoadFailed,
				Message:  "For testing",
				ExitCode: 1,
			},
			expectedReturnValue: true,
		},
		"storage initializer failed due to crash loopBackOff": {
			isvcStatus: &InferenceServiceStatus{
				Status: duckv1.Status{
					Conditions: duckv1.Conditions{
						{
							Type:   "Ready",
							Status: v1.ConditionFalse,
						},
					},
				},
				Address: &duckv1.Addressable{},
				URL:     &apis.URL{},
				Components: map[ComponentType]ComponentStatusSpec{
					PredictorComponent: {
						LatestRolledoutRevision: "test-predictor-default-0001",
					},
				},
				ModelStatus: ModelStatus{},
			},
			statusSpec: ComponentStatusSpec{
				LatestReadyRevision:       "",
				LatestCreatedRevision:     "",
				PreviousRolledoutRevision: "",
				LatestRolledoutRevision:   "",
				Traffic:                   nil,
				URL:                       nil,
				RestURL:                   nil,
				GrpcURL:                   nil,
				Address:                   nil,
			},
			podList: &v1.PodList{
				TypeMeta: metav1.TypeMeta{},
				ListMeta: metav1.ListMeta{},
				Items: []v1.Pod{
					{
						TypeMeta: metav1.TypeMeta{},
						ObjectMeta: metav1.ObjectMeta{
							Name: constants.StorageInitializerContainerName,
						},
						Spec: v1.PodSpec{},
						Status: v1.PodStatus{
							InitContainerStatuses: []v1.ContainerStatus{
								{
									Name: constants.StorageInitializerContainerName,
									State: v1.ContainerState{
										Waiting: &v1.ContainerStateWaiting{
											Reason:  constants.StateReasonCrashLoopBackOff,
											Message: "For testing",
										},
									},
									LastTerminationState: v1.ContainerState{
										Terminated: &v1.ContainerStateTerminated{
											Reason:   constants.StateReasonCrashLoopBackOff,
											Message:  "For testing",
											ExitCode: 1,
										},
									},
									Ready:        false,
									RestartCount: 0,
									Image:        "",
									ImageID:      "",
									ContainerID:  "",
									Started:      nil,
								},
							},
						},
					},
				},
			},
			rawDeployment: false,
			serviceStatus: &knservingv1.ServiceStatus{
				Status: duckv1.Status{
					Conditions: duckv1.Conditions{
						{
							Type:   "Ready",
							Status: "Unknown",
						},
					},
				},
			},
			expectedRevisionStates: &ModelRevisionStates{
				ActiveModelState: "",
				TargetModelState: FailedToLoad,
			},
			expectedTransitionStatus: BlockedByFailedLoad,
			expectedFailureInfo: &FailureInfo{
				Reason:   ModelLoadFailed,
				Message:  "For testing",
				ExitCode: 1,
			},
			expectedReturnValue: true,
		},
		"storage initializer in running state": {
			isvcStatus: &InferenceServiceStatus{
				Status: duckv1.Status{
					Conditions: duckv1.Conditions{
						{
							Type:   "Ready",
							Status: v1.ConditionFalse,
						},
					},
				},
				Address: &duckv1.Addressable{},
				URL:     &apis.URL{},
				Components: map[ComponentType]ComponentStatusSpec{
					PredictorComponent: {
						LatestRolledoutRevision: "test-predictor-default-0001",
					},
				},
				ModelStatus: ModelStatus{},
			},
			statusSpec: ComponentStatusSpec{
				LatestReadyRevision:       "",
				LatestCreatedRevision:     "",
				PreviousRolledoutRevision: "",
				LatestRolledoutRevision:   "",
				Traffic:                   nil,
				URL:                       nil,
				RestURL:                   nil,
				GrpcURL:                   nil,
				Address:                   nil,
			},
			podList: &v1.PodList{
				TypeMeta: metav1.TypeMeta{},
				ListMeta: metav1.ListMeta{},
				Items: []v1.Pod{
					{
						TypeMeta: metav1.TypeMeta{},
						ObjectMeta: metav1.ObjectMeta{
							Name: constants.StorageInitializerContainerName,
						},
						Spec: v1.PodSpec{},
						Status: v1.PodStatus{
							InitContainerStatuses: []v1.ContainerStatus{
								{
									Name: constants.StorageInitializerContainerName,
									State: v1.ContainerState{
										Running: &v1.ContainerStateRunning{
											StartedAt: metav1.Time{},
										},
									},
									LastTerminationState: v1.ContainerState{},
									Ready:                false,
									RestartCount:         0,
									Image:                "",
									ImageID:              "",
									ContainerID:          "",
									Started:              proto.Bool(true),
								},
							},
						},
					},
				},
			},
			rawDeployment: false,
			serviceStatus: &knservingv1.ServiceStatus{
				Status: duckv1.Status{
					Conditions: duckv1.Conditions{
						{
							Type:   "Ready",
							Status: "True",
						},
					},
				},
			},
			expectedRevisionStates: &ModelRevisionStates{
				ActiveModelState: "",
				TargetModelState: Loading,
			},
			expectedTransitionStatus: InProgress,
			expectedFailureInfo:      nil,
			expectedReturnValue:      true,
		},
		"storage initializer in running state but it has a previous error": {
			isvcStatus: &InferenceServiceStatus{
				Status: duckv1.Status{
					Conditions: duckv1.Conditions{
						{
							Type:   "Ready",
							Status: v1.ConditionFalse,
						},
					},
				},
				Address: &duckv1.Addressable{},
				URL:     &apis.URL{},
				Components: map[ComponentType]ComponentStatusSpec{
					PredictorComponent: {
						LatestRolledoutRevision: "test-predictor-default-0001",
					},
				},
				ModelStatus: ModelStatus{},
			},
			statusSpec: ComponentStatusSpec{
				LatestReadyRevision:       "",
				LatestCreatedRevision:     "",
				PreviousRolledoutRevision: "",
				LatestRolledoutRevision:   "",
				Traffic:                   nil,
				URL:                       nil,
				RestURL:                   nil,
				GrpcURL:                   nil,
				Address:                   nil,
			},
			podList: &v1.PodList{
				TypeMeta: metav1.TypeMeta{},
				ListMeta: metav1.ListMeta{},
				Items: []v1.Pod{
					{
						TypeMeta: metav1.TypeMeta{},
						ObjectMeta: metav1.ObjectMeta{
							Name: constants.StorageInitializerContainerName,
						},
						Spec: v1.PodSpec{},
						Status: v1.PodStatus{
							InitContainerStatuses: []v1.ContainerStatus{
								{
									Name: constants.StorageInitializerContainerName,
									State: v1.ContainerState{
										Running: &v1.ContainerStateRunning{
											StartedAt: metav1.Time{},
										},
									},
									LastTerminationState: v1.ContainerState{
										Terminated: &v1.ContainerStateTerminated{
											Reason:   constants.StateReasonCrashLoopBackOff,
											Message:  "For testing",
											ExitCode: 1,
										},
									},
									Ready:        false,
									RestartCount: 0,
									Image:        "",
									ImageID:      "",
									ContainerID:  "",
									Started:      proto.Bool(true),
								},
							},
						},
					},
				},
			},
			rawDeployment: false,
			serviceStatus: &knservingv1.ServiceStatus{
				Status: duckv1.Status{
					Conditions: duckv1.Conditions{
						{
							Type:    "Ready",
							Status:  "False",
							Reason:  "RevisionFailed",
							Message: "For testing",
						},
					},
				},
			},
			expectedRevisionStates:   nil, // This field is not changed in this use case
			expectedTransitionStatus: "",  // This field is not changed in this use case
			expectedFailureInfo:      nil, // This field is not changed in this use case
			expectedReturnValue:      false,
		},
		"kserve container is ready": {
			isvcStatus: &InferenceServiceStatus{
				Status: duckv1.Status{
					Conditions: duckv1.Conditions{
						{
							Type:   "Ready",
							Status: v1.ConditionTrue,
						},
					},
				},
				Address: &duckv1.Addressable{},
				URL:     &apis.URL{},
				Components: map[ComponentType]ComponentStatusSpec{
					PredictorComponent: {
						LatestRolledoutRevision: "test-predictor-default-0001",
					},
				},
				ModelStatus: ModelStatus{},
			},
			statusSpec: ComponentStatusSpec{
				LatestReadyRevision:       "test-predictor-default-0001",
				LatestCreatedRevision:     "test-predictor-default-0001",
				PreviousRolledoutRevision: "",
				LatestRolledoutRevision:   "test-predictor-default-0001",
				Traffic:                   nil,
				URL:                       nil,
				RestURL:                   nil,
				GrpcURL:                   nil,
				Address:                   nil,
			},
			podList: &v1.PodList{
				TypeMeta: metav1.TypeMeta{},
				ListMeta: metav1.ListMeta{},
				Items: []v1.Pod{
					{
						TypeMeta: metav1.TypeMeta{},
						ObjectMeta: metav1.ObjectMeta{
							Name: constants.InferenceServiceContainerName,
						},
						Spec: v1.PodSpec{},
						Status: v1.PodStatus{
							ContainerStatuses: []v1.ContainerStatus{
								{
									Name: constants.InferenceServiceContainerName,
									State: v1.ContainerState{
										Running: &v1.ContainerStateRunning{},
									},
									Ready:        true,
									RestartCount: 0,
									Image:        "",
									ImageID:      "",
									ContainerID:  "",
									Started:      proto.Bool(true),
								},
							},
						},
					},
				},
			},
			rawDeployment: false,
			serviceStatus: &knservingv1.ServiceStatus{
				Status: duckv1.Status{
					Conditions: duckv1.Conditions{
						{
							Type:   "Ready",
							Status: "True",
						},
					},
				},
			},
			expectedRevisionStates: &ModelRevisionStates{
				ActiveModelState: Loaded,
				TargetModelState: Loaded,
			},
			expectedTransitionStatus: UpToDate,
			expectedFailureInfo:      nil,
			expectedReturnValue:      true,
		},
		"raw deployment is ready": {
			isvcStatus: &InferenceServiceStatus{
				Status: duckv1.Status{
					Conditions: duckv1.Conditions{
						{
							Type:   "Ready",
							Status: v1.ConditionTrue,
						},
					},
				},
				Address: &duckv1.Addressable{},
				URL:     &apis.URL{},
				Components: map[ComponentType]ComponentStatusSpec{
					PredictorComponent: {
						LatestRolledoutRevision: "test-predictor-default-0001",
					},
				},
				ModelStatus: ModelStatus{},
			},
			statusSpec: ComponentStatusSpec{
				LatestReadyRevision:       "test-predictor-default-0001",
				LatestCreatedRevision:     "test-predictor-default-0001",
				PreviousRolledoutRevision: "",
				LatestRolledoutRevision:   "test-predictor-default-0001",
				Traffic:                   nil,
				URL:                       nil,
				RestURL:                   nil,
				GrpcURL:                   nil,
				Address:                   nil,
			},
			podList: &v1.PodList{
				TypeMeta: metav1.TypeMeta{},
				ListMeta: metav1.ListMeta{},
				Items: []v1.Pod{
					{
						TypeMeta: metav1.TypeMeta{},
						ObjectMeta: metav1.ObjectMeta{
							Name: constants.InferenceServiceContainerName,
						},
						Spec: v1.PodSpec{},
						Status: v1.PodStatus{
							ContainerStatuses: []v1.ContainerStatus{
								{
									Name: constants.InferenceServiceContainerName,
									State: v1.ContainerState{
										Running: &v1.ContainerStateRunning{},
									},
									Ready:        true,
									RestartCount: 0,
									Image:        "",
									ImageID:      "",
									ContainerID:  "",
									Started:      proto.Bool(true),
								},
							},
						},
					},
				},
			},
			rawDeployment: true,
			serviceStatus: &knservingv1.ServiceStatus{},
			expectedRevisionStates: &ModelRevisionStates{
				ActiveModelState: Loaded,
				TargetModelState: Loaded,
			},
			expectedTransitionStatus: UpToDate,
			expectedFailureInfo:      nil,
			expectedReturnValue:      true,
		},
		"skip containers other than kserve": {
			isvcStatus: &InferenceServiceStatus{
				Status: duckv1.Status{
					Conditions: duckv1.Conditions{
						{
							Type:   "Ready",
							Status: v1.ConditionFalse,
						},
					},
				},
				Address: &duckv1.Addressable{},
				URL:     &apis.URL{},
				Components: map[ComponentType]ComponentStatusSpec{
					PredictorComponent: {
						LatestRolledoutRevision: "test-predictor-default-0001",
					},
				},
				ModelStatus: ModelStatus{},
			},
			statusSpec: ComponentStatusSpec{
				LatestReadyRevision:       "",
				LatestCreatedRevision:     "",
				PreviousRolledoutRevision: "",
				LatestRolledoutRevision:   "",
				Traffic:                   nil,
				URL:                       nil,
				RestURL:                   nil,
				GrpcURL:                   nil,
				Address:                   nil,
			},
			podList: &v1.PodList{
				TypeMeta: metav1.TypeMeta{},
				ListMeta: metav1.ListMeta{},
				Items: []v1.Pod{
					{
						TypeMeta: metav1.TypeMeta{},
						ObjectMeta: metav1.ObjectMeta{
							Name: "test-container",
						},
						Spec: v1.PodSpec{},
						Status: v1.PodStatus{
							ContainerStatuses: []v1.ContainerStatus{
								{
									Name:                 "test-container",
									State:                v1.ContainerState{},
									LastTerminationState: v1.ContainerState{},
									Ready:                false,
									RestartCount:         0,
									Image:                "",
									ImageID:              "",
									ContainerID:          "",
									Started:              nil,
								},
							},
						},
					},
				},
			},
			rawDeployment: false,
			serviceStatus: &knservingv1.ServiceStatus{
				Status: duckv1.Status{
					Conditions: duckv1.Conditions{
						{
							Type:   "Ready",
							Status: "True",
						},
					},
				},
			},
			expectedRevisionStates:   nil,
			expectedTransitionStatus: "",
			expectedFailureInfo:      nil,
			expectedReturnValue:      true,
		},
		"skip initcontainers other than storage initializer": {
			isvcStatus: &InferenceServiceStatus{
				Status: duckv1.Status{
					Conditions: duckv1.Conditions{
						{
							Type:   "Ready",
							Status: v1.ConditionFalse,
						},
					},
				},
				Address: &duckv1.Addressable{},
				URL:     &apis.URL{},
				Components: map[ComponentType]ComponentStatusSpec{
					PredictorComponent: {
						LatestRolledoutRevision: "test-predictor-default-0001",
					},
				},
				ModelStatus: ModelStatus{},
			},
			statusSpec: ComponentStatusSpec{
				LatestReadyRevision:       "",
				LatestCreatedRevision:     "",
				PreviousRolledoutRevision: "",
				LatestRolledoutRevision:   "",
				Traffic:                   nil,
				URL:                       nil,
				RestURL:                   nil,
				GrpcURL:                   nil,
				Address:                   nil,
			},
			podList: &v1.PodList{
				TypeMeta: metav1.TypeMeta{},
				ListMeta: metav1.ListMeta{},
				Items: []v1.Pod{
					{
						TypeMeta: metav1.TypeMeta{},
						ObjectMeta: metav1.ObjectMeta{
							Name: "test-container",
						},
						Spec: v1.PodSpec{},
						Status: v1.PodStatus{
							InitContainerStatuses: []v1.ContainerStatus{
								{
									Name:                 "test-container",
									State:                v1.ContainerState{},
									LastTerminationState: v1.ContainerState{},
									Ready:                false,
									RestartCount:         0,
									Image:                "",
									ImageID:              "",
									ContainerID:          "",
									Started:              nil,
								},
							},
						},
					},
				},
			},
			rawDeployment: false,
			serviceStatus: &knservingv1.ServiceStatus{
				Status: duckv1.Status{
					Conditions: duckv1.Conditions{
						{
							Type:   "Ready",
							Status: "True",
						},
					},
				},
			},
			expectedRevisionStates:   nil,
			expectedTransitionStatus: "",
			expectedFailureInfo:      nil,
			expectedReturnValue:      true,
		},
	}

	for name, scenario := range scenarios {
		t.Run(name, func(t *testing.T) {
			rstatus := scenario.isvcStatus.PropagateModelStatus(scenario.statusSpec, scenario.podList, scenario.rawDeployment, scenario.serviceStatus)

			g.Expect(rstatus).To(gomega.Equal(scenario.expectedReturnValue))
			g.Expect(scenario.isvcStatus.ModelStatus.ModelRevisionStates).To(gomega.Equal(scenario.expectedRevisionStates))
			g.Expect(scenario.isvcStatus.ModelStatus.TransitionStatus).To(gomega.Equal(scenario.expectedTransitionStatus))
			g.Expect(scenario.isvcStatus.ModelStatus.LastFailureInfo).To(gomega.Equal(scenario.expectedFailureInfo))
		})
	}
}

func TestInferenceServiceStatus_UpdateModelRevisionStates(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	scenarios := map[string]struct {
		isvcStatus       *InferenceServiceStatus
		transitionStatus TransitionStatus
		failureInfo      *FailureInfo
		expected         ModelStatus
	}{
		"simple": {
			isvcStatus: &InferenceServiceStatus{
				Status: duckv1.Status{
					Conditions: duckv1.Conditions{
						{
							Type:   "Ready",
							Status: v1.ConditionFalse,
						},
					},
				},
				Address: &duckv1.Addressable{},
				URL:     &apis.URL{},
				Components: map[ComponentType]ComponentStatusSpec{
					PredictorComponent: {
						LatestRolledoutRevision: "test-predictor-default-0001",
					},
				},
				ModelStatus: ModelStatus{},
			},
			transitionStatus: InProgress,
			failureInfo:      nil,
			expected: ModelStatus{
				TransitionStatus: InProgress,
				LastFailureInfo:  nil,
			},
		},
		"invalid spec with nil modelRevisionStates": {
			isvcStatus: &InferenceServiceStatus{
				Status: duckv1.Status{
					Conditions: duckv1.Conditions{
						{
							Type:   "Ready",
							Status: v1.ConditionFalse,
						},
					},
				},
				Address: &duckv1.Addressable{},
				URL:     &apis.URL{},
				Components: map[ComponentType]ComponentStatusSpec{
					PredictorComponent: {
						LatestRolledoutRevision: "test-predictor-default-0001",
					},
				},
				ModelStatus: ModelStatus{},
			},
			transitionStatus: InvalidSpec,
			failureInfo: &FailureInfo{
				Reason:   ModelLoadFailed,
				Message:  "For testing",
				ExitCode: 1,
			},
			expected: ModelStatus{
				TransitionStatus:    InvalidSpec,
				ModelRevisionStates: &ModelRevisionStates{TargetModelState: FailedToLoad},
				LastFailureInfo: &FailureInfo{
					Reason:   ModelLoadFailed,
					Message:  "For testing",
					ExitCode: 1,
				},
			},
		},
		"invalid spec with modelRevisionStates": {
			isvcStatus: &InferenceServiceStatus{
				Status: duckv1.Status{
					Conditions: duckv1.Conditions{
						{
							Type:   "Ready",
							Status: v1.ConditionFalse,
						},
					},
				},
				Address: &duckv1.Addressable{},
				URL:     &apis.URL{},
				Components: map[ComponentType]ComponentStatusSpec{
					PredictorComponent: {
						LatestRolledoutRevision: "test-predictor-default-0001",
					},
				},
				ModelStatus: ModelStatus{
					ModelRevisionStates: &ModelRevisionStates{TargetModelState: Loading},
				},
			},
			transitionStatus: InvalidSpec,
			failureInfo: &FailureInfo{
				Reason:   ModelLoadFailed,
				Message:  "For testing",
				ExitCode: 1,
			},
			expected: ModelStatus{
				TransitionStatus:    InvalidSpec,
				ModelRevisionStates: &ModelRevisionStates{TargetModelState: FailedToLoad},
				LastFailureInfo: &FailureInfo{
					Reason:   ModelLoadFailed,
					Message:  "For testing",
					ExitCode: 1,
				},
			},
		},
	}

	for name, scenario := range scenarios {
		t.Run(name, func(t *testing.T) {
			scenario.isvcStatus.UpdateModelTransitionStatus(scenario.transitionStatus, scenario.failureInfo)

			g.Expect(scenario.isvcStatus.ModelStatus).To(gomega.Equal(scenario.expected))
		})
	}
}
