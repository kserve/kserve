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
	"net/url"
	"testing"
	"time"

	"github.com/onsi/gomega"
	"google.golang.org/protobuf/proto"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"knative.dev/pkg/apis"
	"knative.dev/pkg/apis/duck"
	duckv1 "knative.dev/pkg/apis/duck/v1"
	duckv1beta1 "knative.dev/pkg/apis/duck/v1beta1"
	knservingv1 "knative.dev/serving/pkg/apis/serving/v1"

	"github.com/kserve/kserve/pkg/constants"
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
					Status: corev1.ConditionTrue,
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
					Status: corev1.ConditionFalse,
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
					Status: corev1.ConditionUnknown,
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
						Status: corev1.ConditionTrue,
					},
					{
						Type:   "RoutesReady",
						Status: corev1.ConditionTrue,
					},
					{
						Type:   knservingv1.ServiceConditionReady,
						Status: corev1.ConditionTrue,
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
						Status: corev1.ConditionTrue,
					},
					{
						Type:   "RoutesReady",
						Status: corev1.ConditionTrue,
					},
					{
						Type:   "ConfigurationsReady",
						Status: corev1.ConditionTrue,
					},
					{
						Type:   knservingv1.ServiceConditionReady,
						Status: corev1.ConditionTrue,
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
						Status: corev1.ConditionTrue,
					},
					{
						Type:   knservingv1.ConfigurationConditionReady,
						Status: corev1.ConditionFalse,
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
					Status:  corev1.ConditionTrue,
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
	targetStatus := corev1.ConditionFalse

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
							Status: corev1.ConditionTrue,
						},
						{
							Type:   "RoutesReady",
							Status: corev1.ConditionTrue,
						},
						{
							Type:   "ConfigurationsReady",
							Status: corev1.ConditionTrue,
						},
						{
							Type:   knservingv1.ServiceConditionReady,
							Status: corev1.ConditionTrue,
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
							Status: corev1.ConditionTrue,
						},
						{
							Type:   "RoutesReady",
							Status: corev1.ConditionTrue,
						},
						{
							Type:   "ConfigurationsReady",
							Status: corev1.ConditionTrue,
						},
						{
							Type:   knservingv1.ServiceConditionReady,
							Status: corev1.ConditionTrue,
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
		podList                  *corev1.PodList
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
							Status: corev1.ConditionFalse,
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
			podList: &corev1.PodList{
				TypeMeta: metav1.TypeMeta{},
				ListMeta: metav1.ListMeta{},
				Items:    []corev1.Pod{},
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
							Status: corev1.ConditionFalse,
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
			podList: &corev1.PodList{ // pod list is empty because the revision failed and scaled down to 0
				TypeMeta: metav1.TypeMeta{},
				ListMeta: metav1.ListMeta{},
				Items:    []corev1.Pod{},
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
							Status: corev1.ConditionFalse,
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
			podList: &corev1.PodList{
				TypeMeta: metav1.TypeMeta{},
				ListMeta: metav1.ListMeta{},
				Items: []corev1.Pod{
					{
						TypeMeta: metav1.TypeMeta{},
						ObjectMeta: metav1.ObjectMeta{
							Name: constants.InferenceServiceContainerName,
						},
						Spec: corev1.PodSpec{},
						Status: corev1.PodStatus{
							ContainerStatuses: []corev1.ContainerStatus{
								{
									Name:                 constants.InferenceServiceContainerName,
									State:                corev1.ContainerState{},
									LastTerminationState: corev1.ContainerState{},
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
							Status: corev1.ConditionFalse,
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
			podList: &corev1.PodList{
				TypeMeta: metav1.TypeMeta{},
				ListMeta: metav1.ListMeta{},
				Items: []corev1.Pod{
					{
						TypeMeta: metav1.TypeMeta{},
						ObjectMeta: metav1.ObjectMeta{
							Name: constants.InferenceServiceContainerName,
						},
						Spec: corev1.PodSpec{},
						Status: corev1.PodStatus{
							ContainerStatuses: []corev1.ContainerStatus{
								{
									Name: constants.InferenceServiceContainerName,
									State: corev1.ContainerState{
										Terminated: &corev1.ContainerStateTerminated{
											ExitCode: 1,
											Reason:   constants.StateReasonError,
											Message:  "For testing",
										},
									},
									LastTerminationState: corev1.ContainerState{},
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
							Status: corev1.ConditionFalse,
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
			podList: &corev1.PodList{
				TypeMeta: metav1.TypeMeta{},
				ListMeta: metav1.ListMeta{},
				Items: []corev1.Pod{
					{
						TypeMeta: metav1.TypeMeta{},
						ObjectMeta: metav1.ObjectMeta{
							Name: constants.InferenceServiceContainerName,
						},
						Spec: corev1.PodSpec{},
						Status: corev1.PodStatus{
							ContainerStatuses: []corev1.ContainerStatus{
								{
									Name: constants.InferenceServiceContainerName,
									State: corev1.ContainerState{
										Waiting: &corev1.ContainerStateWaiting{
											Reason:  constants.StateReasonCrashLoopBackOff,
											Message: "For testing",
										},
									},
									LastTerminationState: corev1.ContainerState{
										Terminated: &corev1.ContainerStateTerminated{
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
							Status: corev1.ConditionFalse,
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
			podList: &corev1.PodList{
				TypeMeta: metav1.TypeMeta{},
				ListMeta: metav1.ListMeta{},
				Items: []corev1.Pod{
					{
						TypeMeta: metav1.TypeMeta{},
						ObjectMeta: metav1.ObjectMeta{
							Name: constants.StorageInitializerContainerName,
						},
						Spec: corev1.PodSpec{},
						Status: corev1.PodStatus{
							InitContainerStatuses: []corev1.ContainerStatus{
								{
									Name: constants.StorageInitializerContainerName,
									State: corev1.ContainerState{
										Terminated: &corev1.ContainerStateTerminated{
											ExitCode: 1,
											Reason:   constants.StateReasonError,
											Message:  "For testing",
										},
									},
									LastTerminationState: corev1.ContainerState{},
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
							Status: corev1.ConditionFalse,
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
			podList: &corev1.PodList{
				TypeMeta: metav1.TypeMeta{},
				ListMeta: metav1.ListMeta{},
				Items: []corev1.Pod{
					{
						TypeMeta: metav1.TypeMeta{},
						ObjectMeta: metav1.ObjectMeta{
							Name: constants.StorageInitializerContainerName,
						},
						Spec: corev1.PodSpec{},
						Status: corev1.PodStatus{
							InitContainerStatuses: []corev1.ContainerStatus{
								{
									Name: constants.StorageInitializerContainerName,
									State: corev1.ContainerState{
										Waiting: &corev1.ContainerStateWaiting{
											Reason:  constants.StateReasonCrashLoopBackOff,
											Message: "For testing",
										},
									},
									LastTerminationState: corev1.ContainerState{
										Terminated: &corev1.ContainerStateTerminated{
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
							Status: corev1.ConditionFalse,
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
			podList: &corev1.PodList{
				TypeMeta: metav1.TypeMeta{},
				ListMeta: metav1.ListMeta{},
				Items: []corev1.Pod{
					{
						TypeMeta: metav1.TypeMeta{},
						ObjectMeta: metav1.ObjectMeta{
							Name: constants.StorageInitializerContainerName,
						},
						Spec: corev1.PodSpec{},
						Status: corev1.PodStatus{
							InitContainerStatuses: []corev1.ContainerStatus{
								{
									Name: constants.StorageInitializerContainerName,
									State: corev1.ContainerState{
										Running: &corev1.ContainerStateRunning{
											StartedAt: metav1.Time{},
										},
									},
									LastTerminationState: corev1.ContainerState{},
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
							Status: corev1.ConditionFalse,
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
			podList: &corev1.PodList{
				TypeMeta: metav1.TypeMeta{},
				ListMeta: metav1.ListMeta{},
				Items: []corev1.Pod{
					{
						TypeMeta: metav1.TypeMeta{},
						ObjectMeta: metav1.ObjectMeta{
							Name: constants.StorageInitializerContainerName,
						},
						Spec: corev1.PodSpec{},
						Status: corev1.PodStatus{
							InitContainerStatuses: []corev1.ContainerStatus{
								{
									Name: constants.StorageInitializerContainerName,
									State: corev1.ContainerState{
										Running: &corev1.ContainerStateRunning{
											StartedAt: metav1.Time{},
										},
									},
									LastTerminationState: corev1.ContainerState{
										Terminated: &corev1.ContainerStateTerminated{
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
							Status: corev1.ConditionTrue,
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
			podList: &corev1.PodList{
				TypeMeta: metav1.TypeMeta{},
				ListMeta: metav1.ListMeta{},
				Items: []corev1.Pod{
					{
						TypeMeta: metav1.TypeMeta{},
						ObjectMeta: metav1.ObjectMeta{
							Name: constants.InferenceServiceContainerName,
						},
						Spec: corev1.PodSpec{},
						Status: corev1.PodStatus{
							ContainerStatuses: []corev1.ContainerStatus{
								{
									Name: constants.InferenceServiceContainerName,
									State: corev1.ContainerState{
										Running: &corev1.ContainerStateRunning{},
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
							Status: corev1.ConditionTrue,
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
			podList: &corev1.PodList{
				TypeMeta: metav1.TypeMeta{},
				ListMeta: metav1.ListMeta{},
				Items: []corev1.Pod{
					{
						TypeMeta: metav1.TypeMeta{},
						ObjectMeta: metav1.ObjectMeta{
							Name: constants.InferenceServiceContainerName,
						},
						Spec: corev1.PodSpec{},
						Status: corev1.PodStatus{
							ContainerStatuses: []corev1.ContainerStatus{
								{
									Name: constants.InferenceServiceContainerName,
									State: corev1.ContainerState{
										Running: &corev1.ContainerStateRunning{},
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
							Status: corev1.ConditionFalse,
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
			podList: &corev1.PodList{
				TypeMeta: metav1.TypeMeta{},
				ListMeta: metav1.ListMeta{},
				Items: []corev1.Pod{
					{
						TypeMeta: metav1.TypeMeta{},
						ObjectMeta: metav1.ObjectMeta{
							Name: "test-container",
						},
						Spec: corev1.PodSpec{},
						Status: corev1.PodStatus{
							ContainerStatuses: []corev1.ContainerStatus{
								{
									Name:                 "test-container",
									State:                corev1.ContainerState{},
									LastTerminationState: corev1.ContainerState{},
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
							Status: corev1.ConditionFalse,
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
			podList: &corev1.PodList{
				TypeMeta: metav1.TypeMeta{},
				ListMeta: metav1.ListMeta{},
				Items: []corev1.Pod{
					{
						TypeMeta: metav1.TypeMeta{},
						ObjectMeta: metav1.ObjectMeta{
							Name: "test-container",
						},
						Spec: corev1.PodSpec{},
						Status: corev1.PodStatus{
							InitContainerStatuses: []corev1.ContainerStatus{
								{
									Name:                 "test-container",
									State:                corev1.ContainerState{},
									LastTerminationState: corev1.ContainerState{},
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
							Status: corev1.ConditionFalse,
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
							Status: corev1.ConditionFalse,
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
							Status: corev1.ConditionFalse,
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

func TestGetDeploymentCondition_SingleDeployment(t *testing.T) {
	now := metav1.Now()
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name: "predictor",
		},
		Status: appsv1.DeploymentStatus{
			Conditions: []appsv1.DeploymentCondition{
				{
					Type:               appsv1.DeploymentAvailable,
					Status:             corev1.ConditionTrue,
					Reason:             "MinimumReplicasAvailable",
					Message:            "Deployment has minimum availability.",
					LastTransitionTime: now,
				},
			},
		},
	}
	condition := getDeploymentCondition([]*appsv1.Deployment{deployment}, appsv1.DeploymentAvailable)
	if condition.Type != apis.ConditionType(appsv1.DeploymentAvailable) {
		t.Errorf("expected condition type %v, got %v", appsv1.DeploymentAvailable, condition.Type)
	}
	if condition.Status != corev1.ConditionTrue {
		t.Errorf("expected condition status %v, got %v", corev1.ConditionTrue, condition.Status)
	}
	if condition.Message != "Deployment has minimum availability." {
		t.Errorf("expected message %q, got %q", "Deployment has minimum availability.", condition.Message)
	}
	if condition.Reason != "MinimumReplicasAvailable" {
		t.Errorf("expected reason %q, got %q", "MinimumReplicasAvailable", condition.Reason)
	}
	if !condition.LastTransitionTime.Inner.Equal(&now) {
		t.Errorf("expected last transition time %v, got %v", now, condition.LastTransitionTime.Inner)
	}
}

func TestGetDeploymentCondition_MultiDeployment_AllTrue(t *testing.T) {
	now := metav1.Now()
	headDeployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name: "predictor",
		},
		Status: appsv1.DeploymentStatus{
			Conditions: []appsv1.DeploymentCondition{
				{
					Type:               appsv1.DeploymentAvailable,
					Status:             corev1.ConditionTrue,
					Reason:             "HeadReady",
					Message:            "Head node ready.",
					LastTransitionTime: now,
				},
			},
		},
	}
	workerDeployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name: "predictor-worker",
		},
		Status: appsv1.DeploymentStatus{
			Conditions: []appsv1.DeploymentCondition{
				{
					Type:               appsv1.DeploymentAvailable,
					Status:             corev1.ConditionTrue,
					Reason:             "WorkerReady",
					Message:            "Worker node ready.",
					LastTransitionTime: now,
				},
			},
		},
	}
	condition := getDeploymentCondition([]*appsv1.Deployment{headDeployment, workerDeployment}, appsv1.DeploymentAvailable)
	if condition.Type != apis.ConditionType(appsv1.DeploymentAvailable) {
		t.Errorf("expected condition type %v, got %v", appsv1.DeploymentAvailable, condition.Type)
	}
	if condition.Status != corev1.ConditionTrue {
		t.Errorf("expected condition status %v, got %v", corev1.ConditionTrue, condition.Status)
	}
	expectedMsg := "predictor-container: Head node ready., worker-container: Worker node ready."
	if condition.Message != expectedMsg {
		t.Errorf("expected message %q, got %q", expectedMsg, condition.Message)
	}
	if condition.Reason != "" {
		t.Errorf("expected reason to be empty, got %q", condition.Reason)
	}
	if !condition.LastTransitionTime.Inner.Equal(&now) {
		t.Errorf("expected last transition time %v, got %v", now, condition.LastTransitionTime.Inner)
	}
}

func TestGetDeploymentCondition_MultiDeployment_OneFalse(t *testing.T) {
	now := metav1.Now()
	headDeployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name: "predictor",
		},
		Status: appsv1.DeploymentStatus{
			Conditions: []appsv1.DeploymentCondition{
				{
					Type:               appsv1.DeploymentAvailable,
					Status:             corev1.ConditionTrue,
					Reason:             "HeadReady",
					Message:            "Head node ready.",
					LastTransitionTime: now,
				},
			},
		},
	}
	workerDeployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name: "predictor-worker",
		},
		Status: appsv1.DeploymentStatus{
			Conditions: []appsv1.DeploymentCondition{
				{
					Type:               appsv1.DeploymentAvailable,
					Status:             corev1.ConditionFalse,
					Reason:             "WorkerNotReady",
					Message:            "Worker node not ready.",
					LastTransitionTime: now,
				},
			},
		},
	}
	condition := getDeploymentCondition([]*appsv1.Deployment{headDeployment, workerDeployment}, appsv1.DeploymentAvailable)
	if condition.Type != apis.ConditionType(appsv1.DeploymentAvailable) {
		t.Errorf("expected condition type %v, got %v", appsv1.DeploymentAvailable, condition.Type)
	}
	if condition.Status != corev1.ConditionFalse {
		t.Errorf("expected condition status %v, got %v", corev1.ConditionFalse, condition.Status)
	}
	expectedMsg := "predictor-container: Head node ready., worker-container: Worker node not ready."
	if condition.Message != expectedMsg {
		t.Errorf("expected message %q, got %q", expectedMsg, condition.Message)
	}
	if condition.Reason != "" {
		t.Errorf("expected reason to be empty, got %q", condition.Reason)
	}
	if !condition.LastTransitionTime.Inner.Equal(&now) {
		t.Errorf("expected last transition time %v, got %v", now, condition.LastTransitionTime.Inner)
	}
}

func TestGetDeploymentCondition_SingleDeployment_NoMatchingCondition(t *testing.T) {
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name: "predictor",
		},
		Status: appsv1.DeploymentStatus{
			Conditions: []appsv1.DeploymentCondition{
				{
					Type:    appsv1.DeploymentProgressing,
					Status:  corev1.ConditionTrue,
					Reason:  "NewReplicaSetAvailable",
					Message: "ReplicaSet \"predictor-xxx\" has successfully progressed.",
				},
			},
		},
	}
	condition := getDeploymentCondition([]*appsv1.Deployment{deployment}, appsv1.DeploymentAvailable)
	if condition.Type != "" {
		t.Errorf("expected empty condition type, got %v", condition.Type)
	}
	if condition.Status != "" {
		t.Errorf("expected empty condition status, got %v", condition.Status)
	}
	if condition.Message != "" {
		t.Errorf("expected empty message, got %q", condition.Message)
	}
	if condition.Reason != "" {
		t.Errorf("expected empty reason, got %q", condition.Reason)
	}
}

func TestPropagateCrossComponentStatus(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	// Helper to create a status with a given condition
	setComponentCondition := func(ss *InferenceServiceStatus, condType apis.ConditionType, status corev1.ConditionStatus) {
		cond := &apis.Condition{
			Type:   condType,
			Status: status,
		}
		ss.SetCondition(condType, cond)
	}

	t.Run("All components ready sets RoutesReady True", func(t *testing.T) {
		ss := &InferenceServiceStatus{}
		ss.InitializeConditions()
		setComponentCondition(ss, PredictorRouteReady, corev1.ConditionTrue)
		setComponentCondition(ss, ExplainerRoutesReady, corev1.ConditionTrue)
		setComponentCondition(ss, TransformerRouteReady, corev1.ConditionTrue)

		ss.PropagateCrossComponentStatus([]ComponentType{PredictorComponent, ExplainerComponent, TransformerComponent}, RoutesReady)
		cond := ss.GetCondition(RoutesReady)
		g.Expect(cond).NotTo(gomega.BeNil())
		g.Expect(cond.Status).To(gomega.Equal(corev1.ConditionTrue))
	})

	t.Run("One component not ready sets RoutesReady False", func(t *testing.T) {
		ss := &InferenceServiceStatus{}
		ss.InitializeConditions()
		setComponentCondition(ss, PredictorRouteReady, corev1.ConditionTrue)
		setComponentCondition(ss, ExplainerRoutesReady, corev1.ConditionFalse)
		setComponentCondition(ss, TransformerRouteReady, corev1.ConditionTrue)

		ss.PropagateCrossComponentStatus([]ComponentType{PredictorComponent, ExplainerComponent, TransformerComponent}, RoutesReady)
		cond := ss.GetCondition(RoutesReady)
		g.Expect(cond).NotTo(gomega.BeNil())
		g.Expect(cond.Status).To(gomega.Equal(corev1.ConditionFalse))
		g.Expect(cond.Reason).To(gomega.Equal("ExplainerRoutesReady not ready"))
	})

	t.Run("One component unknown sets RoutesReady Unknown", func(t *testing.T) {
		ss := &InferenceServiceStatus{}
		ss.InitializeConditions()
		setComponentCondition(ss, PredictorRouteReady, corev1.ConditionTrue)
		setComponentCondition(ss, ExplainerRoutesReady, corev1.ConditionUnknown)
		setComponentCondition(ss, TransformerRouteReady, corev1.ConditionTrue)

		ss.PropagateCrossComponentStatus([]ComponentType{PredictorComponent, ExplainerComponent, TransformerComponent}, RoutesReady)
		cond := ss.GetCondition(RoutesReady)
		g.Expect(cond).NotTo(gomega.BeNil())
		g.Expect(cond.Status).To(gomega.Equal(corev1.ConditionUnknown))
		g.Expect(cond.Reason).To(gomega.Equal("ExplainerRoutesReady not ready"))
	})

	t.Run("No components sets nothing", func(t *testing.T) {
		ss := &InferenceServiceStatus{}
		ss.InitializeConditions()
		ss.PropagateCrossComponentStatus([]ComponentType{}, RoutesReady)
		cond := ss.GetCondition(RoutesReady)
		// Should still be initialized, but True by default
		g.Expect(cond).NotTo(gomega.BeNil())
		g.Expect(cond.Status).To(gomega.Equal(corev1.ConditionTrue))
	})

	t.Run("Unknown conditionType does nothing", func(t *testing.T) {
		ss := &InferenceServiceStatus{}
		ss.InitializeConditions()
		ss.PropagateCrossComponentStatus([]ComponentType{PredictorComponent}, "UnknownConditionType")
		cond := ss.GetCondition("UnknownConditionType")
		g.Expect(cond).To(gomega.BeNil())
	})

	t.Run("All components ready sets LatestDeploymentReady True", func(t *testing.T) {
		ss := &InferenceServiceStatus{}
		ss.InitializeConditions()
		setComponentCondition(ss, PredictorConfigurationReady, corev1.ConditionTrue)
		setComponentCondition(ss, ExplainerConfigurationReady, corev1.ConditionTrue)
		setComponentCondition(ss, TransformerConfigurationReady, corev1.ConditionTrue)

		ss.PropagateCrossComponentStatus([]ComponentType{PredictorComponent, ExplainerComponent, TransformerComponent}, LatestDeploymentReady)
		cond := ss.GetCondition(LatestDeploymentReady)
		g.Expect(cond).NotTo(gomega.BeNil())
		g.Expect(cond.Status).To(gomega.Equal(corev1.ConditionTrue))
	})

	t.Run("One component not ready sets LatestDeploymentReady False", func(t *testing.T) {
		ss := &InferenceServiceStatus{}
		ss.InitializeConditions()
		setComponentCondition(ss, PredictorConfigurationReady, corev1.ConditionTrue)
		setComponentCondition(ss, ExplainerConfigurationReady, corev1.ConditionFalse)
		setComponentCondition(ss, TransformerConfigurationReady, corev1.ConditionTrue)

		ss.PropagateCrossComponentStatus([]ComponentType{PredictorComponent, ExplainerComponent, TransformerComponent}, LatestDeploymentReady)
		cond := ss.GetCondition(LatestDeploymentReady)
		g.Expect(cond).NotTo(gomega.BeNil())
		g.Expect(cond.Status).To(gomega.Equal(corev1.ConditionFalse))
		g.Expect(cond.Reason).To(gomega.Equal("ExplainerConfigurationReady not ready"))
	})
}

func TestInferenceServiceStatus_ClearCondition(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	status := &InferenceServiceStatus{}
	status.InitializeConditions()

	// Mark PredictorReady as True
	status.SetCondition(PredictorReady, &apis.Condition{
		Status: corev1.ConditionTrue,
	})
	g.Expect(status.GetCondition(PredictorReady)).NotTo(gomega.BeNil())
	g.Expect(status.IsConditionReady(PredictorReady)).To(gomega.BeTrue())

	// Clear PredictorReady
	status.ClearCondition(PredictorReady)
	cond := status.GetCondition(PredictorReady)
	g.Expect(cond).NotTo(gomega.BeNil())

	// Try clearing a condition that was never set
	status.ClearCondition(TransformerReady)
}
