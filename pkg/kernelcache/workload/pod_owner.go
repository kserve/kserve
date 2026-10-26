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

// Package workload resolves Kubernetes-managed workload ownership for kernel cache captures.
package workload

import (
	"context"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
)

const (
	replicaSetKind       = "ReplicaSet"
	deploymentKind       = "Deployment"
	inferenceServiceKind = "InferenceService"
)

// ResolveDeploymentBackedInferenceService resolves the InferenceService owning
// a Deployment-backed Pod without requiring a revision label.
func ResolveDeploymentBackedInferenceService(ctx context.Context, reader client.Reader, pod *corev1.Pod) (*v1beta1.InferenceService, error) {
	resolved, err := resolveDeploymentBackedWorkload(ctx, reader, pod)
	if err != nil || resolved == nil {
		return nil, err
	}
	return resolved.Source, nil
}

func resolveDeploymentBackedWorkload(ctx context.Context, reader client.Reader, pod *corev1.Pod) (*WorkloadRevision, error) {
	if reader == nil || pod == nil {
		return nil, nil
	}

	replicaSetRef := controllerOwner(pod, appsv1.SchemeGroupVersion.String(), replicaSetKind)
	if replicaSetRef == nil {
		return nil, nil
	}
	replicaSet := &appsv1.ReplicaSet{}
	if err := reader.Get(ctx, client.ObjectKey{Namespace: pod.Namespace, Name: replicaSetRef.Name}, replicaSet); err != nil {
		return nil, err
	}
	if replicaSet.UID != replicaSetRef.UID {
		return nil, nil
	}

	deploymentRef := controllerOwner(replicaSet, appsv1.SchemeGroupVersion.String(), deploymentKind)
	if deploymentRef == nil {
		return nil, nil
	}
	deployment := &appsv1.Deployment{}
	if err := reader.Get(ctx, client.ObjectKey{Namespace: replicaSet.Namespace, Name: deploymentRef.Name}, deployment); err != nil {
		return nil, err
	}
	if deployment.UID != deploymentRef.UID {
		return nil, nil
	}

	inferenceServiceRef := controllerOwner(deployment, v1beta1.SchemeGroupVersion.String(), inferenceServiceKind)
	if inferenceServiceRef == nil {
		return nil, nil
	}
	inferenceService := &v1beta1.InferenceService{}
	if err := reader.Get(ctx, client.ObjectKey{Namespace: deployment.Namespace, Name: inferenceServiceRef.Name}, inferenceService); err != nil {
		return nil, err
	}
	if inferenceService.UID != inferenceServiceRef.UID {
		return nil, nil
	}

	revisionID := pod.Labels[appsv1.DefaultDeploymentUniqueLabelKey]
	if revisionID == "" {
		revisionID = replicaSet.Labels[appsv1.DefaultDeploymentUniqueLabelKey]
	}
	return &WorkloadRevision{Source: inferenceService, ReplicaSet: replicaSet, RevisionID: revisionID}, nil
}

func controllerOwner(obj metav1.Object, apiVersion, kind string) *metav1.OwnerReference {
	ref := metav1.GetControllerOf(obj)
	if ref == nil || ref.APIVersion != apiVersion || ref.Kind != kind || ref.Name == "" || ref.UID == "" {
		return nil
	}
	return ref
}
