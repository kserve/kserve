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

package serving

import (
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"

	"knative.dev/pkg/apis"
	duckv1 "knative.dev/pkg/apis/duck/v1"
)

var depCondSet = apis.NewLivingConditionSet(
	DeploymentConditionProgressing,
	DeploymentConditionReplicaSetReady,
)

const (
	// DeploymentConditionReady means the underlying deployment is ready.
	DeploymentConditionReady = apis.ConditionReady
	// DeploymentConditionReplicaSetready inverts the underlying deployment's
	// ReplicaSetFailure condition.
	DeploymentConditionReplicaSetReady apis.ConditionType = "ReplicaSetReady"
	// DeploymentConditionProgressing reflects the underlying deployment's
	// Progressing condition.
	DeploymentConditionProgressing apis.ConditionType = "Progressing"
)

// transformDeploymentStatus transforms the kubernetes DeploymentStatus into a
// duckv1.Status that uses ConditionSets to propagate failures and expose
// a top-level happy state, per our condition conventions.
func TransformDeploymentStatus(ds *appsv1.DeploymentStatus) *duckv1.Status {
	s := &duckv1.Status{}

	depCondSet.Manage(s).InitializeConditions()
	// The absence of this condition means no failure has occurred. If we find it
	// below, we'll ovewrwrite this.
	depCondSet.Manage(s).MarkTrue(DeploymentConditionReplicaSetReady)

	for _, cond := range ds.Conditions {
		// TODO(jonjohnsonjr): Should we care about appsv1.DeploymentAvailable here?
		switch cond.Type {
		case appsv1.DeploymentProgressing:
			switch cond.Status {
			case corev1.ConditionUnknown:
				depCondSet.Manage(s).MarkUnknown(DeploymentConditionProgressing, cond.Reason, cond.Message)
			case corev1.ConditionTrue:
				depCondSet.Manage(s).MarkTrue(DeploymentConditionProgressing)
			case corev1.ConditionFalse:
				depCondSet.Manage(s).MarkFalse(DeploymentConditionProgressing, cond.Reason, cond.Message)
			}
		case appsv1.DeploymentReplicaFailure:
			switch cond.Status {
			case corev1.ConditionUnknown:
				depCondSet.Manage(s).MarkUnknown(DeploymentConditionReplicaSetReady, cond.Reason, cond.Message)
			case corev1.ConditionTrue:
				depCondSet.Manage(s).MarkFalse(DeploymentConditionReplicaSetReady, cond.Reason, cond.Message)
			case corev1.ConditionFalse:
				depCondSet.Manage(s).MarkTrue(DeploymentConditionReplicaSetReady)
			}
		}
	}
	return s
}
