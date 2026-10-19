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

package llmisvc

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
)

func TestGetFailConditions(t *testing.T) {
	svc := &v1alpha2.LLMInferenceService{}
	svc.MarkHTTPRoutesNotReady("RefsInvalid", "gateway missing")
	svc.MarkMainWorkloadNotReady("Pending", "not scheduled")
	svc.MarkGroupNotReady("ModelNameMismatch", "members diverge")
	svc.MarkPerModelPathsKept("NotSetOnAllGateways", "partial setting")

	got := GetFailConditions(svc)

	assert.Contains(t, got, string(v1alpha2.HTTPRoutesReady), "sub-conditions outside the condition set still roll up into Ready")
	assert.Contains(t, got, string(v1alpha2.MainWorkloadReady))
	assert.NotContains(t, got, string(v1alpha2.GroupReady), "informational: routing keeps working")
	assert.NotContains(t, got, string(v1alpha2.PerModelPathsDropped), "informational: kept paths still serve every client")
}
