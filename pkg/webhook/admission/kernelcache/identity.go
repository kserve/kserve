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

package kernelcache

import (
	"errors"

	corev1 "k8s.io/api/core/v1"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/constants"
	cacheidentity "github.com/kserve/kserve/pkg/kernelcache/identity"
)

type workloadRef struct {
	Kind string
	Name string
}

// identityForPod builds the KernelCache identity from the Pod workload, runtime container, and model URI.
func identityForPod(pod *corev1.Pod, modelURI string) (v1alpha1.KernelCacheIdentity, error) {
	workload, err := workloadRefForPod(pod)
	if err != nil {
		return v1alpha1.KernelCacheIdentity{}, err
	}

	container, err := runtimeContainerForPod(pod, workload)
	if err != nil {
		return v1alpha1.KernelCacheIdentity{}, err
	}

	identityFactors := buildRuntimeIdentityFactors(container, modelURI)
	identityFactors.Namespace = pod.Namespace
	identityFactors.WorkloadKind = workload.Kind
	identityFactors.WorkloadName = workload.Name
	return cacheidentity.Build(identityFactors)
}

// workloadRefForPod extracts the workload kind and name from the Pod labels.
func workloadRefForPod(pod *corev1.Pod) (workloadRef, error) {
	if name := pod.Labels[constants.InferenceServicePodLabelKey]; name != "" {
		return workloadRef{Kind: "InferenceService", Name: name}, nil
	}

	if name := pod.Labels[constants.KubernetesAppNameLabelKey]; name != "" &&
		pod.Labels[constants.KubernetesPartOfLabelKey] == constants.LLMInferenceServicePartOfValue {
		return workloadRef{Kind: v1alpha2.LLMInferenceServiceGVK.Kind, Name: name}, nil
	}

	return workloadRef{}, errors.New("workload identity labels are required")
}

// runtimeContainerForPod finds the runtime container associated with the workload kind.
func runtimeContainerForPod(pod *corev1.Pod, workload workloadRef) (*corev1.Container, error) {
	containerNames := []string{constants.InferenceServiceContainerName, constants.LLMInferenceServiceContainerName}
	if workload.Kind == v1alpha2.LLMInferenceServiceGVK.Kind {
		containerNames[0], containerNames[1] = containerNames[1], containerNames[0]
	}
	for _, name := range containerNames {
		if index := findContainerIndex(pod.Spec.Containers, name); index >= 0 {
			return &pod.Spec.Containers[index], nil
		}
	}
	return nil, errors.New("runtime container was not found")
}

// buildRuntimeIdentityFactors builds runtime identity factors from a container and model URI.
func buildRuntimeIdentityFactors(container *corev1.Container, modelURI string) cacheidentity.KernelCacheIdentityFactors {
	runtimeConfigFactors := cacheidentity.ExtractRuntimeConfigFactors(container)
	identityFactors := cacheidentity.KernelCacheIdentityFactors{
		RuntimeImage:         container.Image,
		ModelURIHash:         cacheidentity.ModelURIHash(modelURI),
		RuntimeConfigFactors: runtimeConfigFactors,
	}
	if len(container.Command) > 0 {
		identityFactors.CommandHash = cacheidentity.HashStrings(container.Command)
	}
	if len(container.Args) > 0 {
		identityFactors.ArgsHash = cacheidentity.HashStrings(container.Args)
	}
	return identityFactors
}
