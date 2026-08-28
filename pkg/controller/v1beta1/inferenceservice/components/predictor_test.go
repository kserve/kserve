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
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	"github.com/kserve/kserve/pkg/constants"
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

func TestAddStorageInitializerAnnotationsOciNative(t *testing.T) {
	// oci+native:// must pass ValidateStorageURI (it's in SupportedStorageURIPrefixList)
	// and set StorageInitializerSourceUriInternalAnnotationKey so InjectModelcar can fire.
	s := runtime.NewScheme()
	if err := v1alpha1.AddToScheme(s); err != nil {
		t.Fatalf("failed to add v1alpha1 to scheme: %v", err)
	}
	fakeClient := fake.NewClientBuilder().WithScheme(s).Build()
	p := &Predictor{client: fakeClient}

	ociNativeURI := constants.OciNativeURIPrefix + "ghcr.io/kserve/oci-native-test-fixture:v1"
	model := &v1beta1.ModelSpec{
		PredictorExtensionSpec: v1beta1.PredictorExtensionSpec{
			StorageURI: &ociNativeURI,
		},
	}
	annotations := map[string]string{}

	err := p.addStorageInitializerAnnotations(context.Background(), model, annotations, nil)
	assert.NoError(t, err, "oci+native:// must pass ValidateStorageURI without error")
	annotationVal, hasAnnotation := annotations[constants.StorageInitializerSourceUriInternalAnnotationKey]
	assert.True(t, hasAnnotation, "oci+native:// must set StorageInitializerSourceUriInternalAnnotationKey so InjectModelcar can inject the ImageVolume")
	assert.Equal(t, ociNativeURI, annotationVal)
}

func ptrInt32(v int32) *int32 { return &v }

func TestAdjustStableMinReplicasForCanaries(t *testing.T) {
	tests := []struct {
		name             string
		stableMinReplica int32
		canaries         []v1beta1.CanarySpec
		canaryStatuses   []v1beta1.CanaryStatus
		expectedStable   int32
	}{
		{
			name:             "no canaries - unchanged",
			stableMinReplica: 10,
			canaries:         nil,
			canaryStatuses:   nil,
			expectedStable:   10,
		},
		{
			name:             "canary not in status - stable unchanged",
			stableMinReplica: 10,
			canaries: []v1beta1.CanarySpec{
				{TrafficPercent: 20, Predictor: v1beta1.PredictorSpec{Name: "v2"}},
			},
			canaryStatuses: nil,
			expectedStable: 10,
		},
		{
			name:             "canary ready=false - stable unchanged",
			stableMinReplica: 10,
			canaries: []v1beta1.CanarySpec{
				{TrafficPercent: 20, Predictor: v1beta1.PredictorSpec{Name: "v2"}},
			},
			canaryStatuses: []v1beta1.CanaryStatus{
				{Name: "v2", Ready: false, TrafficPercent: 20},
			},
			expectedStable: 10,
		},
		{
			name:             "canary ready=true - stable reduced by computed count",
			stableMinReplica: 10,
			canaries: []v1beta1.CanarySpec{
				{TrafficPercent: 20, Predictor: v1beta1.PredictorSpec{Name: "v2"}},
			},
			canaryStatuses: []v1beta1.CanaryStatus{
				{Name: "v2", Ready: true, TrafficPercent: 20},
			},
			expectedStable: 8, // 10 - ceil(10*20/100)=2
		},
		{
			name:             "mixed readiness - only ready canary counted",
			stableMinReplica: 10,
			canaries: []v1beta1.CanarySpec{
				{TrafficPercent: 20, Predictor: v1beta1.PredictorSpec{Name: "v2"}},
				{TrafficPercent: 30, Predictor: v1beta1.PredictorSpec{Name: "v3"}},
			},
			canaryStatuses: []v1beta1.CanaryStatus{
				{Name: "v2", Ready: true, TrafficPercent: 20},
				{Name: "v3", Ready: false, TrafficPercent: 30},
			},
			expectedStable: 8, // 10 - ceil(10*20/100)=2
		},
		{
			name:             "all canaries ready - stable reduced by full sum",
			stableMinReplica: 10,
			canaries: []v1beta1.CanarySpec{
				{TrafficPercent: 20, Predictor: v1beta1.PredictorSpec{Name: "v2"}},
				{TrafficPercent: 30, Predictor: v1beta1.PredictorSpec{Name: "v3"}},
			},
			canaryStatuses: []v1beta1.CanaryStatus{
				{Name: "v2", Ready: true, TrafficPercent: 20},
				{Name: "v3", Ready: true, TrafficPercent: 30},
			},
			expectedStable: 5, // 10 - 2 - 3
		},
		{
			name:             "reduction floored at 1",
			stableMinReplica: 2,
			canaries: []v1beta1.CanarySpec{
				{TrafficPercent: 50, Predictor: v1beta1.PredictorSpec{Name: "v2"}},
			},
			canaryStatuses: []v1beta1.CanaryStatus{
				{Name: "v2", Ready: true, TrafficPercent: 50},
			},
			expectedStable: 1, // max(2-1, 1)
		},
		{
			name:             "explicit canary minReplicas honored when ready",
			stableMinReplica: 10,
			canaries: []v1beta1.CanarySpec{
				{TrafficPercent: 20, Predictor: v1beta1.PredictorSpec{Name: "v2", ComponentExtensionSpec: v1beta1.ComponentExtensionSpec{MinReplicas: ptrInt32(3)}}},
			},
			canaryStatuses: []v1beta1.CanaryStatus{
				{Name: "v2", Ready: true, TrafficPercent: 20},
			},
			expectedStable: 7, // 10 - 3 (explicit)
		},
		{
			name:             "explicit canary minReplicas ignored when not ready",
			stableMinReplica: 10,
			canaries: []v1beta1.CanarySpec{
				{TrafficPercent: 20, Predictor: v1beta1.PredictorSpec{Name: "v2", ComponentExtensionSpec: v1beta1.ComponentExtensionSpec{MinReplicas: ptrInt32(3)}}},
			},
			canaryStatuses: []v1beta1.CanaryStatus{
				{Name: "v2", Ready: false, TrafficPercent: 20},
			},
			expectedStable: 10,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isvc := &v1beta1.InferenceService{
				Spec: v1beta1.InferenceServiceSpec{
					Predictor: v1beta1.PredictorSpec{
						ComponentExtensionSpec: v1beta1.ComponentExtensionSpec{
							MinReplicas: ptrInt32(tt.stableMinReplica),
						},
					},
					Canary: tt.canaries,
				},
				Status: v1beta1.InferenceServiceStatus{
					CanaryStatuses: tt.canaryStatuses,
				},
			}
			componentExt := isvc.Spec.Predictor.ComponentExtensionSpec
			adjustStableMinReplicasForCanaries(isvc, &componentExt)
			assert.Equal(t, tt.expectedStable, *componentExt.MinReplicas)
		})
	}
}
