/*
Copyright 2024 The KServe Authors.

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
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestComputeMpNodeAndGPUs(t *testing.T) {
	tests := []struct {
		name                 string
		pipelineParallelSize int
		tensorParallelSize   int
		expectedNodeCount    int
		expectedWorkerGPU    int
		expectedHeadGPU      int
	}{
		{
			name:                 "PP=2, TP=2: 2 nodes, 2 GPUs per node",
			pipelineParallelSize: 2,
			tensorParallelSize:   2,
			expectedNodeCount:    2,
			expectedWorkerGPU:    2,
			expectedHeadGPU:      2,
		},
		{
			name:                 "PP=4, TP=8: 4 nodes, 8 GPUs per node",
			pipelineParallelSize: 4,
			tensorParallelSize:   8,
			expectedNodeCount:    4,
			expectedWorkerGPU:    8,
			expectedHeadGPU:      8,
		},
		{
			name:                 "PP=1, TP=4: 1 node, 4 GPUs",
			pipelineParallelSize: 1,
			tensorParallelSize:   4,
			expectedNodeCount:    1,
			expectedWorkerGPU:    4,
			expectedHeadGPU:      4,
		},
		{
			name:                 "PP=2, TP=1: 2 nodes, 1 GPU per node",
			pipelineParallelSize: 2,
			tensorParallelSize:   1,
			expectedNodeCount:    2,
			expectedWorkerGPU:    1,
			expectedHeadGPU:      1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			nodeCount, workerGPU, headGPU := computeMpNodeAndGPUs(tt.pipelineParallelSize, tt.tensorParallelSize)
			assert.Equal(t, tt.expectedNodeCount, nodeCount)
			assert.Equal(t, tt.expectedWorkerGPU, workerGPU)
			assert.Equal(t, tt.expectedHeadGPU, headGPU)
		})
	}
}
