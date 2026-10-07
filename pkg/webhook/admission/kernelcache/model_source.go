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
	"context"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kserve/kserve/pkg/constants"
	cacheidentity "github.com/kserve/kserve/pkg/kernelcache/identity"
)

// workloadModelSourceResolver resolves the effective model source for a workload Pod.
type workloadModelSourceResolver interface {
	Resolve(context.Context, client.Reader, *corev1.Pod, workloadRef) (string, bool, error)
}

type inferenceServiceModelSourceResolver struct{}

func (inferenceServiceModelSourceResolver) Resolve(
	_ context.Context,
	_ client.Reader,
	pod *corev1.Pod,
	_ workloadRef,
) (string, bool, error) {
	modelURI := strings.TrimSpace(pod.Annotations[constants.StorageInitializerSourceUriInternalAnnotationKey])
	if modelURI == "" || cacheidentity.ModelURIHash(modelURI) == "" {
		return "", false, nil
	}
	return modelURI, true, nil
}

var workloadModelSourceResolvers = map[string]workloadModelSourceResolver{
	"InferenceService": inferenceServiceModelSourceResolver{},
}

// resolveWorkloadModelSource dispatches model source resolution by workload kind.
func resolveWorkloadModelSource(
	ctx context.Context,
	reader client.Reader,
	pod *corev1.Pod,
	workload workloadRef,
) (string, bool, error) {
	resolver, ok := workloadModelSourceResolvers[workload.Kind]
	if !ok {
		return "", false, nil
	}
	return resolver.Resolve(ctx, reader, pod, workload)
}
