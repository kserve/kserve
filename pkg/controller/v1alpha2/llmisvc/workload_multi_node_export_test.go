/*
Copyright 2023 The KServe Authors.

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

package llmisvc

import (
	"context"

	corev1 "k8s.io/api/core/v1"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
)

// ReconcileMultiNodeMainServiceAccountForTest exposes reconcileMultiNodeMainServiceAccount for testing.
func ReconcileMultiNodeMainServiceAccountForTest(
	ctx context.Context,
	r *LLMISVCReconciler,
	llmSvc *v1alpha2.LLMInferenceService,
	cfg *Config,
) error {
	return r.reconcileMultiNodeMainServiceAccount(ctx, llmSvc, cfg)
}

// ReconcileMultiNodePrefillServiceAccountForTest exposes reconcileMultiNodePrefillServiceAccount for testing.
func ReconcileMultiNodePrefillServiceAccountForTest(
	ctx context.Context,
	r *LLMISVCReconciler,
	llmSvc *v1alpha2.LLMInferenceService,
) error {
	return r.reconcileMultiNodePrefillServiceAccount(ctx, llmSvc)
}

// ExpectedMultiNodePrefillLeaderWorkerTemplateForTest exposes expectedMultiNodePrefillLeaderWorkerTemplate for testing.
func ExpectedMultiNodePrefillLeaderWorkerTemplateForTest(
	ctx context.Context,
	r *LLMISVCReconciler,
	llmSvc *v1alpha2.LLMInferenceService,
	config *Config,
	deployedLeader *corev1.PodSpec,
	deployedWorker corev1.PodSpec,
) (*multiNodeTemplate, error) {
	return r.expectedMultiNodePrefillLeaderWorkerTemplate(ctx, llmSvc, config, deployedLeader, deployedWorker)
}
