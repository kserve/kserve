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

package components

import (
	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	"github.com/kserve/kserve/pkg/controller/v1beta1/inferenceservice/reconcilers"
)

// propagatePlatformWorkloadStatus is a hook for surfacing the outcome of platform-specific workload
// customization (e.g. DeploymentReconciler.PlatformConditions) on the InferenceService status. It runs
// for the stable predictor in Standard mode, once its workload is reconciled and before the predictor
// readiness is propagated, and owns both setting and clearing such conditions. It is not called when
// the workload reconcile fails, nor for canary, transformer or explainer workloads. Distribution-specific
// builds (compiled with -tags distro) can provide their own implementation; the default does nothing.
func propagatePlatformWorkloadStatus(_ *v1beta1.InferenceService, _ reconcilers.WorkloadReconciler) {}
