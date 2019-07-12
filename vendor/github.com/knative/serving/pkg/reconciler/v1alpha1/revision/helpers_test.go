/*
Copyright 2018 The Knative Authors.

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

package revision

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	duckv1alpha1 "github.com/knative/pkg/apis/duck/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
)

func TestGetBuildDoneCondition(t *testing.T) {
	tests := []struct {
		description string
		build       *duckv1alpha1.KResource
		cond        *duckv1alpha1.Condition
	}{{
		// If there are no build conditions, we should get nil.
		description: "no conditions",
		build:       &duckv1alpha1.KResource{},
	}, {
		// If the conditions indicate that things are running, we should get nil.
		description: "build running",
		build: &duckv1alpha1.KResource{
			Status: duckv1alpha1.Status{
				Conditions: []duckv1alpha1.Condition{{
					Type:   duckv1alpha1.ConditionSucceeded,
					Status: corev1.ConditionUnknown,
				}},
			},
		},
	}, {
		// If the build succeeded, return the success condition.
		description: "build succeeded",
		build: &duckv1alpha1.KResource{
			Status: duckv1alpha1.Status{
				Conditions: []duckv1alpha1.Condition{{
					Type:   duckv1alpha1.ConditionSucceeded,
					Status: corev1.ConditionTrue,
				}},
			},
		},
		cond: &duckv1alpha1.Condition{
			Type:   duckv1alpha1.ConditionSucceeded,
			Status: corev1.ConditionTrue,
		},
	}, {
		// If the build failed, return the failure condition.
		description: "build failed",
		build: &duckv1alpha1.KResource{
			Status: duckv1alpha1.Status{
				Conditions: []duckv1alpha1.Condition{{
					Type:    duckv1alpha1.ConditionSucceeded,
					Status:  corev1.ConditionTrue,
					Reason:  "TheReason",
					Message: "something super descriptive",
				}},
			},
		},
		cond: &duckv1alpha1.Condition{
			Type:    duckv1alpha1.ConditionSucceeded,
			Status:  corev1.ConditionTrue,
			Reason:  "TheReason",
			Message: "something super descriptive",
		},
	}}

	for _, test := range tests {
		t.Run(test.description, func(t *testing.T) {
			cond := getBuildDoneCondition(test.build)
			if diff := cmp.Diff(test.cond, cond); diff != "" {
				t.Errorf("getBuildDoneCondition(%v); (-want +got) = %v", test.build, diff)
			}
		})
	}

}

func TestGetDeploymentProgressCondition(t *testing.T) {
	tests := []struct {
		description string
		deploy      *appsv1.Deployment
		timedOut    bool
	}{{
		description: "no conditions",
		deploy:      &appsv1.Deployment{},
	}, {
		description: "other conditions",
		deploy: &appsv1.Deployment{
			Status: appsv1.DeploymentStatus{
				Conditions: []appsv1.DeploymentCondition{{
					Type:   appsv1.DeploymentAvailable,
					Status: corev1.ConditionTrue,
				}},
			},
		},
	}, {
		description: "progressing",
		deploy: &appsv1.Deployment{
			Status: appsv1.DeploymentStatus{
				Conditions: []appsv1.DeploymentCondition{{
					Type:   appsv1.DeploymentProgressing,
					Status: corev1.ConditionTrue,
				}},
			},
		},
	}, {
		description: "not progressing for other reasons",
		deploy: &appsv1.Deployment{
			Status: appsv1.DeploymentStatus{
				Conditions: []appsv1.DeploymentCondition{{
					Type:   appsv1.DeploymentProgressing,
					Status: corev1.ConditionFalse,
					Reason: "OnHoliday",
				}},
			},
		},
	}, {
		description: "progress deadline exceeded",
		deploy: &appsv1.Deployment{
			Status: appsv1.DeploymentStatus{
				Conditions: []appsv1.DeploymentCondition{{
					Type:   appsv1.DeploymentProgressing,
					Status: corev1.ConditionFalse,
					Reason: "ProgressDeadlineExceeded",
				}},
			},
		},
		timedOut: true,
	}}

	for _, test := range tests {
		t.Run(test.description, func(t *testing.T) {
			timedOut := hasDeploymentTimedOut(test.deploy)
			if diff := cmp.Diff(test.timedOut, timedOut); diff != "" {
				t.Errorf("hasDeploymentTimedOut(%v); (-want +got) = %v", test.deploy, diff)
			}
		})
	}
}
