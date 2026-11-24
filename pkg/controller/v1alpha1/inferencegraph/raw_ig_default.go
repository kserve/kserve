//go:build !distro

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

package inferencegraph

import (
	corev1 "k8s.io/api/core/v1"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
)

// customizeRouterPodSpec is a hook for platform-specific customization of the router pod spec
// in raw deployment mode. Distribution-specific builds (compiled with -tags distro) can provide
// their own implementation; the default does nothing. Implementations must tolerate a nil podSpec.
func customizeRouterPodSpec(_ *v1alpha1.InferenceGraph, _ *corev1.PodSpec) {}
