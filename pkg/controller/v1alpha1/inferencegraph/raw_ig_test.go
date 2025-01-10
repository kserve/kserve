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

package inferencegraph

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"google.golang.org/protobuf/proto"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"knative.dev/pkg/apis"
	duckv1 "knative.dev/pkg/apis/duck/v1"

	. "github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	"github.com/kserve/kserve/pkg/constants"
	"github.com/kserve/kserve/pkg/utils"
)

func TestCreateInferenceGraphPodSpec(t *testing.T) {
	type args struct {
		graph  *InferenceGraph
		config *RouterConfig
	}

	routerConfig := RouterConfig{
		Image:         "kserve/router:v0.10.0",
		CpuRequest:    "100m",
		CpuLimit:      "100m",
		MemoryRequest: "100Mi",
		MemoryLimit:   "500Mi",
	}

	routerConfigWithHeaders := RouterConfig{
		Image:         "kserve/router:v0.10.0",
		CpuRequest:    "100m",
		CpuLimit:      "100m",
		MemoryRequest: "100Mi",
		MemoryLimit:   "500Mi",
		Headers: map[string][]string{
			"propagate": {
				"Authorization",
				"Intuit_tid",
			},
		},
	}

	testIGSpecs := map[string]*InferenceGraph{
		"basic": {
			ObjectMeta: metav1.ObjectMeta{
				Name:      "basic-ig",
				Namespace: "basic-ig-namespace",
			},
			Spec: InferenceGraphSpec{
				Nodes: map[string]InferenceRouter{
					GraphRootNodeName: {
						RouterType: Sequence,
						Steps: []InferenceStep{
							{
								InferenceTarget: InferenceTarget{
									ServiceURL: "http://someservice.exmaple.com",
								},
							},
						},
					},
				},
			},
		},
		"withresource": {
			ObjectMeta: metav1.ObjectMeta{
				Name:      "resource-ig",
				Namespace: "resource-ig-namespace",
				Annotations: map[string]string{
					"serving.kserve.io/deploymentMode": string(constants.RawDeployment),
				},
			},

			Spec: InferenceGraphSpec{
				Nodes: map[string]InferenceRouter{
					GraphRootNodeName: {
						RouterType: Sequence,
						Steps: []InferenceStep{
							{
								InferenceTarget: InferenceTarget{
									ServiceURL: "http://someservice.exmaple.com",
								},
							},
						},
					},
				},
				Resources: v1.ResourceRequirements{
					Limits: v1.ResourceList{
						v1.ResourceCPU:    resource.MustParse("100m"),
						v1.ResourceMemory: resource.MustParse("500Mi"),
					},
					Requests: v1.ResourceList{
						v1.ResourceCPU:    resource.MustParse("100m"),
						v1.ResourceMemory: resource.MustParse("100Mi"),
					},
				},
			},
		},

		"withenv": {
			ObjectMeta: metav1.ObjectMeta{
				Name:      "env-ig",
				Namespace: "env-ig-namespace",
				Annotations: map[string]string{
					"serving.kserve.io/deploymentMode": string(constants.RawDeployment),
				},
			},

			Spec: InferenceGraphSpec{
				Nodes: map[string]InferenceRouter{
					GraphRootNodeName: {
						RouterType: Sequence,
						Steps: []InferenceStep{
							{
								InferenceTarget: InferenceTarget{
									ServiceURL: "http://someservice.exmaple.com",
								},
							},
						},
					},
				},
			},
		},
	}

	expectedPodSpecs := map[string]*v1.PodSpec{
		"basicgraph": {
			Containers: []v1.Container{
				{
					Image: "kserve/router:v0.10.0",
					Name:  "basic-ig",
					Args: []string{
						"--graph-json",
						"{\"nodes\":{\"root\":{\"routerType\":\"Sequence\",\"steps\":[{\"serviceUrl\":\"http://someservice.exmaple.com\"}]}},\"resources\":{}}",
					},
					Resources: v1.ResourceRequirements{
						Limits: v1.ResourceList{
							v1.ResourceCPU:    resource.MustParse("100m"),
							v1.ResourceMemory: resource.MustParse("500Mi"),
						},
						Requests: v1.ResourceList{
							v1.ResourceCPU:    resource.MustParse("100m"),
							v1.ResourceMemory: resource.MustParse("100Mi"),
						},
					},
					SecurityContext: &v1.SecurityContext{
						Privileged:               proto.Bool(false),
						RunAsNonRoot:             proto.Bool(true),
						ReadOnlyRootFilesystem:   proto.Bool(true),
						AllowPrivilegeEscalation: proto.Bool(false),
						Capabilities: &v1.Capabilities{
							Drop: []v1.Capability{v1.Capability("ALL")},
						},
					},
				},
			},
			AutomountServiceAccountToken: proto.Bool(false),
		},
		"basicgraphwithheaders": {
			Containers: []v1.Container{
				{
					Image: "kserve/router:v0.10.0",
					Name:  "basic-ig",
					Args: []string{
						"--graph-json",
						"{\"nodes\":{\"root\":{\"routerType\":\"Sequence\",\"steps\":[{\"serviceUrl\":\"http://someservice.exmaple.com\"}]}},\"resources\":{}}",
					},
					Env: []v1.EnvVar{
						{
							Name:  "PROPAGATE_HEADERS",
							Value: "Authorization,Intuit_tid",
						},
					},
					Resources: v1.ResourceRequirements{
						Limits: v1.ResourceList{
							v1.ResourceCPU:    resource.MustParse("100m"),
							v1.ResourceMemory: resource.MustParse("500Mi"),
						},
						Requests: v1.ResourceList{
							v1.ResourceCPU:    resource.MustParse("100m"),
							v1.ResourceMemory: resource.MustParse("100Mi"),
						},
					},
					SecurityContext: &v1.SecurityContext{
						Privileged:               proto.Bool(false),
						RunAsNonRoot:             proto.Bool(true),
						ReadOnlyRootFilesystem:   proto.Bool(true),
						AllowPrivilegeEscalation: proto.Bool(false),
						Capabilities: &v1.Capabilities{
							Drop: []v1.Capability{v1.Capability("ALL")},
						},
					},
				},
			},
			AutomountServiceAccountToken: proto.Bool(false),
		},
		"withresource": {
			Containers: []v1.Container{
				{
					Image: "kserve/router:v0.10.0",
					Name:  "resource-ig",
					Args: []string{
						"--graph-json",
						"{\"nodes\":{\"root\":{\"routerType\":\"Sequence\",\"steps\":[{\"serviceUrl\":\"http://someservice.exmaple.com\"}]}},\"resources\":{\"limits\":{\"cpu\":\"100m\",\"memory\":\"500Mi\"},\"requests\":{\"cpu\":\"100m\",\"memory\":\"100Mi\"}}}",
					},
					Resources: v1.ResourceRequirements{
						Limits: v1.ResourceList{
							v1.ResourceCPU:    resource.MustParse("100m"),
							v1.ResourceMemory: resource.MustParse("500Mi"),
						},
						Requests: v1.ResourceList{
							v1.ResourceCPU:    resource.MustParse("100m"),
							v1.ResourceMemory: resource.MustParse("100Mi"),
						},
					},
					SecurityContext: &v1.SecurityContext{
						Privileged:               proto.Bool(false),
						RunAsNonRoot:             proto.Bool(true),
						ReadOnlyRootFilesystem:   proto.Bool(true),
						AllowPrivilegeEscalation: proto.Bool(false),
						Capabilities: &v1.Capabilities{
							Drop: []v1.Capability{v1.Capability("ALL")},
						},
					},
				},
			},
			AutomountServiceAccountToken: proto.Bool(false),
		},
	}

	scenarios := []struct {
		name     string
		args     args
		expected *v1.PodSpec
	}{
		{
			name: "Basic Inference graph",
			args: args{
				graph:  testIGSpecs["basic"],
				config: &routerConfig,
			},
			expected: expectedPodSpecs["basicgraph"],
		},
		{
			name:     "Inference graph with resource requirements",
			args:     args{testIGSpecs["withresource"], &routerConfig},
			expected: expectedPodSpecs["withresource"],
		},
		{
			name: "Inference graph with propagate headers",
			args: args{
				graph:  testIGSpecs["basic"],
				config: &routerConfigWithHeaders,
			},
			expected: expectedPodSpecs["basicgraphwithheaders"],
		},
	}

	for _, tt := range scenarios {
		t.Run(tt.name, func(t *testing.T) {
			result := createInferenceGraphPodSpec(tt.args.graph, tt.args.config)
			if diff := cmp.Diff(tt.expected, result); diff != "" {
				t.Errorf("Test %q unexpected result (-want +got): %v", t.Name(), diff)
			}
		})
	}
}

func TestConstructGraphObjectMeta(t *testing.T) {
	type args struct {
		graph *InferenceGraph
	}

	type metaAndExt struct {
		objectMeta   metav1.ObjectMeta
		componentExt v1beta1.ComponentExtensionSpec
	}

	cpuResource := v1beta1.MetricCPU

	scenarios := []struct {
		name     string
		args     args
		expected metaAndExt
	}{
		{
			name: "Basic Inference graph",
			args: args{
				graph: &InferenceGraph{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "basic-ig",
						Namespace: "basic-ig-namespace",
					},
				},
			},
			expected: metaAndExt{
				objectMeta: metav1.ObjectMeta{
					Name:      "basic-ig",
					Namespace: "basic-ig-namespace",
					Labels: map[string]string{
						"serving.kserve.io/inferencegraph": "basic-ig",
					},
					Annotations: map[string]string{},
				},

				componentExt: v1beta1.ComponentExtensionSpec{
					MaxReplicas: 0,
					MinReplicas: nil,
					ScaleMetric: nil,
					ScaleTarget: nil,
				},
			},
		},
		{
			name: "Inference graph with annotations , min and max replicas ",
			args: args{
				graph: &InferenceGraph{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "basic-ig",
						Namespace: "basic-ig-namespace",
						Annotations: map[string]string{
							"test": "test",
						},
					},
					Spec: InferenceGraphSpec{
						MinReplicas: utils.ToPointer(int32(2)),
						MaxReplicas: 5,
					},
				},
			},
			expected: metaAndExt{
				objectMeta: metav1.ObjectMeta{
					Name:      "basic-ig",
					Namespace: "basic-ig-namespace",
					Labels: map[string]string{
						"serving.kserve.io/inferencegraph": "basic-ig",
					},
					Annotations: map[string]string{
						"test": "test",
					},
				},

				componentExt: v1beta1.ComponentExtensionSpec{
					MaxReplicas: 5,
					MinReplicas: utils.ToPointer(int32(2)),
					ScaleMetric: nil,
					ScaleTarget: nil,
				},
			},
		},
		{
			name: "Inference graph with labels",
			args: args{
				graph: &InferenceGraph{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "basic-ig",
						Namespace: "basic-ig-namespace",
						Labels: map[string]string{
							"test": "test",
						},
					},
				},
			},
			expected: metaAndExt{
				objectMeta: metav1.ObjectMeta{
					Name:      "basic-ig",
					Namespace: "basic-ig-namespace",
					Labels: map[string]string{
						"serving.kserve.io/inferencegraph": "basic-ig",
						"test":                             "test",
					},
					Annotations: map[string]string{},
				},
				componentExt: v1beta1.ComponentExtensionSpec{
					MaxReplicas: 0,
					MinReplicas: nil,
					ScaleMetric: nil,
					ScaleTarget: nil,
				},
			},
		},
		{
			name: "Inference graph with annotations and labels",
			args: args{
				graph: &InferenceGraph{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "basic-ig",
						Namespace: "basic-ig-namespace",
						Annotations: map[string]string{
							"test": "test",
						},
						Labels: map[string]string{
							"test": "test",
						},
					},
					Spec: InferenceGraphSpec{
						MinReplicas: utils.ToPointer(int32(5)),
						MaxReplicas: 10,
						ScaleTarget: utils.ToPointer(int32(50)),
						ScaleMetric: (*ScaleMetric)(&cpuResource),
					},
				},
			},
			expected: metaAndExt{
				objectMeta: metav1.ObjectMeta{
					Name:      "basic-ig",
					Namespace: "basic-ig-namespace",
					Labels: map[string]string{
						"serving.kserve.io/inferencegraph": "basic-ig",
						"test":                             "test",
					},
					Annotations: map[string]string{
						"test": "test",
					},
				},
				componentExt: v1beta1.ComponentExtensionSpec{
					MinReplicas: utils.ToPointer(int32(5)),
					MaxReplicas: 10,
					ScaleTarget: utils.ToPointer(int32(50)),
					ScaleMetric: &cpuResource,
				},
			},
		},
	}

	for _, tt := range scenarios {
		t.Run(tt.name, func(t *testing.T) {
			objMeta, componentExt := constructForRawDeployment(tt.args.graph)
			if diff := cmp.Diff(tt.expected.objectMeta, objMeta); diff != "" {
				t.Errorf("Test %q unexpected result (-want +got): %v", t.Name(), diff)
			}
			if diff := cmp.Diff(tt.expected.componentExt, componentExt); diff != "" {
				t.Errorf("Test %q unexpected result (-want +got): %v", t.Name(), diff)
			}
		})
	}
}

func TestPropagateRawStatus(t *testing.T) {
	type args struct {
		graphStatus *InferenceGraphStatus
		deployment  *appsv1.Deployment
		url         *apis.URL
	}

	scenarios := []struct {
		name     string
		args     args
		expected *InferenceGraphStatus
	}{
		{
			name: "Basic Inference graph with with graph status as ready and deployment available",
			args: args{
				graphStatus: &InferenceGraphStatus{
					Status: duckv1.Status{
						Conditions: duckv1.Conditions{
							{
								Type:   apis.ConditionReady,
								Status: v1.ConditionTrue,
							},
						},
					},
				},
				deployment: &appsv1.Deployment{
					Status: appsv1.DeploymentStatus{
						AvailableReplicas: 1,
					},
				},
				url: &apis.URL{
					Scheme: "http",
					Host:   "test.com",
				},
			},
			expected: &InferenceGraphStatus{
				Status: duckv1.Status{
					Conditions: duckv1.Conditions{
						{
							Type:   apis.ConditionReady,
							Status: v1.ConditionTrue,
						},
					},
				},
			},
		},

		{
			name: "Basic Inference graph with with Inferencegraph status as not ready and deployment unavailable",
			args: args{
				graphStatus: &InferenceGraphStatus{
					Status: duckv1.Status{
						Conditions: duckv1.Conditions{
							{
								Type:   apis.ConditionReady,
								Status: v1.ConditionFalse,
							},
						},
					},
				},
				deployment: &appsv1.Deployment{
					Status: appsv1.DeploymentStatus{
						AvailableReplicas: 0,
					},
				},
			},
			expected: &InferenceGraphStatus{
				Status: duckv1.Status{
					Conditions: duckv1.Conditions{
						{
							Type:   apis.ConditionReady,
							Status: v1.ConditionFalse,
						},
					},
				},
			},
		},
	}

	for _, tt := range scenarios {
		t.Run(tt.name, func(t *testing.T) {
			PropagateRawStatus(tt.args.graphStatus, tt.args.deployment, tt.args.url)
			if diff := cmp.Diff(tt.expected, tt.args.graphStatus); diff != "" {
				t.Errorf("Test for graphstatus %q unexpected result (-want +got): %v", t.Name(), diff)
			}
		})
	}
}
