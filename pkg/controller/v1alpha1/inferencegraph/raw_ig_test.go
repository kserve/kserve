package inferencegraph

import (
	"github.com/google/go-cmp/cmp"
	v1alpha1 "github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"github.com/kserve/kserve/pkg/constants"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"testing"
)

func TestCreateInferenceGraphPodSpec(t *testing.T) {
	type args struct {
		graph  *v1alpha1.InferenceGraph
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

	testIGSpecs := map[string]*v1alpha1.InferenceGraph{
		"basic": {
			ObjectMeta: metav1.ObjectMeta{
				Name:      "basic-ig",
				Namespace: "basic-ig-namespace",
			},
			Spec: v1alpha1.InferenceGraphSpec{
				Nodes: map[string]v1alpha1.InferenceRouter{
					v1alpha1.GraphRootNodeName: {
						RouterType: v1alpha1.Sequence,
						Steps: []v1alpha1.InferenceStep{
							{
								InferenceTarget: v1alpha1.InferenceTarget{
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
				Name:      "resouce-ig",
				Namespace: "resource-ig-namespace",
				Annotations: map[string]string{
					"serving.kserve.io/deploymentMode": string(constants.Serverless),
				},
			},

			Spec: v1alpha1.InferenceGraphSpec{
				Nodes: map[string]v1alpha1.InferenceRouter{
					v1alpha1.GraphRootNodeName: {
						RouterType: v1alpha1.Sequence,
						Steps: []v1alpha1.InferenceStep{
							{
								InferenceTarget: v1alpha1.InferenceTarget{
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

		"withaffinity": {
			ObjectMeta: metav1.ObjectMeta{
				Name:      "affinity-ig",
				Namespace: "affinity-ig-namespace",
				Annotations: map[string]string{
					"serving.kserve.io/deploymentMode": string(constants.Serverless),
				},
			},

			Spec: v1alpha1.InferenceGraphSpec{
				Affinity: &v1.Affinity{
					PodAffinity: &v1.PodAffinity{
						PreferredDuringSchedulingIgnoredDuringExecution: []v1.WeightedPodAffinityTerm{
							{
								Weight: 100,
								PodAffinityTerm: v1.PodAffinityTerm{
									LabelSelector: &metav1.LabelSelector{
										MatchExpressions: []metav1.LabelSelectorRequirement{
											{
												Key:      "serving.kserve.io/inferencegraph",
												Operator: metav1.LabelSelectorOpIn,
												Values: []string{
													"affinity-ig",
												},
											},
										},
									},
									TopologyKey: "topology.kubernetes.io/zone",
								},
							},
						},
					},
				},
				Nodes: map[string]v1alpha1.InferenceRouter{
					v1alpha1.GraphRootNodeName: {
						RouterType: v1alpha1.Sequence,
						Steps: []v1alpha1.InferenceStep{
							{
								InferenceTarget: v1alpha1.InferenceTarget{
									ServiceURL: "http://someservice.exmaple.com",
								},
							},
						},
					},
				},
			},
		},

		"withenv": {
			ObjectMeta: metav1.ObjectMeta{
				Name:      "env-ig",
				Namespace: "env-ig-namespace",
				Annotations: map[string]string{
					"serving.kserve.io/deploymentMode": string(constants.Serverless),
				},
			},

			Spec: v1alpha1.InferenceGraphSpec{
				Nodes: map[string]v1alpha1.InferenceRouter{
					v1alpha1.GraphRootNodeName: {
						RouterType: v1alpha1.Sequence,
						Steps: []v1alpha1.InferenceStep{
							{
								InferenceTarget: v1alpha1.InferenceTarget{
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
				},
			},
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
				},
			},
		},
	}

	//obj := metav1.ObjectMeta{
	//	Name:      "model",
	//	Namespace: "test",
	//	Annotations: map[string]string{
	//		"annotation": "annotation-value",
	//	},
	//	Labels: map[string]string{
	//		"label": "label-value",
	//	},
	//}

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
		//{
		//	name:     "Inference graph with resource requirements",
		//	args:     args{nil, nil},
		//	expected: nil,
		//},
		//{
		//	name:     "Inference graph with pod affinity",
		//	args:     args{nil, nil},
		//	expected: nil,
		//},
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
