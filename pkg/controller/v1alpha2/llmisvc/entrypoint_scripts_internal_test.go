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

	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/ptr"
)

func TestComputeShutdownTimeout(t *testing.T) {
	// tgps=120, preStop=15 => 120 - 15 - min(5,120) = 100
	got := computeShutdownTimeout(&corev1.PodSpec{
		TerminationGracePeriodSeconds: ptr.To(int64(120)),
	}, 15)
	if got != 100 {
		t.Errorf("computeShutdownTimeout() = %d, want 100", got)
	}
}
