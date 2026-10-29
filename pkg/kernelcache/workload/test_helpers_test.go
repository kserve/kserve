/*
Copyright 2026 The KServe Authors.

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

package workload

import (
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
)

type deploymentBackedWorkloadFixture struct {
	scheme           *runtime.Scheme
	reader           client.Client
	pod              *corev1.Pod
	replicaSet       *appsv1.ReplicaSet
	deployment       *appsv1.Deployment
	inferenceService *v1beta1.InferenceService
}

func newDeploymentBackedWorkloadFixture(t *testing.T) *deploymentBackedWorkloadFixture {
	t.Helper()

	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := appsv1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := v1beta1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}

	inferenceService := &v1beta1.InferenceService{ObjectMeta: metav1.ObjectMeta{
		Name: "model", Namespace: "team", UID: types.UID("inferenceservice-uid"),
	}}
	deployment := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{
		Name: "model-predictor", Namespace: "team", UID: types.UID("deployment-uid"),
		OwnerReferences: []metav1.OwnerReference{*metav1.NewControllerRef(
			inferenceService, v1beta1.SchemeGroupVersion.WithKind(inferenceServiceKind),
		)},
	}}
	replicaSet := &appsv1.ReplicaSet{ObjectMeta: metav1.ObjectMeta{
		Name: "model-predictor-abc", Namespace: "team", UID: types.UID("replicaset-uid"),
		Labels: map[string]string{
			appsv1.DefaultDeploymentUniqueLabelKey: "abc123",
		},
		OwnerReferences: []metav1.OwnerReference{*metav1.NewControllerRef(
			deployment, appsv1.SchemeGroupVersion.WithKind(deploymentKind),
		)},
	}}
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
		Name: "model-predictor-abc-xyz", Namespace: "team",
		Labels: map[string]string{
			appsv1.DefaultDeploymentUniqueLabelKey: "abc123",
		},
		OwnerReferences: []metav1.OwnerReference{*metav1.NewControllerRef(
			replicaSet, appsv1.SchemeGroupVersion.WithKind(replicaSetKind),
		)},
	}}

	fixture := &deploymentBackedWorkloadFixture{
		scheme:           scheme,
		pod:              pod,
		replicaSet:       replicaSet,
		deployment:       deployment,
		inferenceService: inferenceService,
	}
	fixture.rebuildReader()
	return fixture
}

func (f *deploymentBackedWorkloadFixture) rebuildReader() {
	f.reader = fake.NewClientBuilder().WithScheme(f.scheme).
		WithObjects(f.inferenceService, f.deployment, f.replicaSet).Build()
}
