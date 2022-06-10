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

	"github.com/golang/protobuf/proto"
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
	status.PropagateRawStatus(PredictorComponent, deployment, url)
	if res := status.IsConditionReady(PredictorReady); !res {
		t.Errorf("expected: %v got: %v conditions: %v", true, res, status.Conditions)
	}
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
