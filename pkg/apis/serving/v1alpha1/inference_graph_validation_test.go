package v1alpha1

import (
	"fmt"
	"github.com/golang/protobuf/proto"
	"github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"testing"
)

func makeTestInferenceGraph() InferenceGraph {
	ig := InferenceGraph{
		TypeMeta: metav1.TypeMeta{
			Kind:       "InferenceGraph",
			APIVersion: "v1alpha1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "foo-bar",
		},
		Spec: InferenceGraphSpec{},
	}
	return ig
}

func TestInferenceGraph_ValidateCreate(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	scenarios := map[string]struct {
		ig      InferenceGraph
		update  map[string]string
		nodes   map[string]InferenceRouter
		matcher types.GomegaMatcher
	}{
		"simple": {
			ig: makeTestInferenceGraph(),
			nodes: map[string]InferenceRouter{
				GraphRootNodeName: {},
			},
			matcher: gomega.MatchError(nil),
		},
		"alphanumeric model name": {
			ig: makeTestInferenceGraph(),
			update: map[string]string{
				name: "Abc-123",
			},
			matcher: gomega.MatchError(fmt.Errorf(InvalidGraphNameFormatError, "Abc-123", GraphNameFmt)),
		},
		"name starts with number": {
			ig: makeTestInferenceGraph(),
			update: map[string]string{
				name: "4abc-3",
			},
			nodes: map[string]InferenceRouter{
				GraphRootNodeName: {},
			},
			matcher: gomega.MatchError(fmt.Errorf(InvalidGraphNameFormatError, "4abc-3", GraphNameFmt)),
		},
		"name starts with dash": {
			ig: makeTestInferenceGraph(),
			update: map[string]string{
				name: "-abc-3",
			},
			nodes: map[string]InferenceRouter{
				GraphRootNodeName: {},
			},
			matcher: gomega.MatchError(fmt.Errorf(InvalidGraphNameFormatError, "-abc-3", GraphNameFmt)),
		},
		"name ends with dash": {
			ig: makeTestInferenceGraph(),
			update: map[string]string{
				name: "abc-3-",
			},
			nodes: map[string]InferenceRouter{
				GraphRootNodeName: {},
			},
			matcher: gomega.MatchError(fmt.Errorf(InvalidGraphNameFormatError, "abc-3-", GraphNameFmt)),
		},
		"name includes dot": {
			ig: makeTestInferenceGraph(),
			update: map[string]string{
				name: "abc.123",
			},
			nodes: map[string]InferenceRouter{
				GraphRootNodeName: {},
			},
			matcher: gomega.MatchError(fmt.Errorf(InvalidGraphNameFormatError, "abc.123", GraphNameFmt)),
		},
		"name includes spaces": {
			ig: makeTestInferenceGraph(),
			update: map[string]string{
				name: "abc 123",
			},
			nodes: map[string]InferenceRouter{
				GraphRootNodeName: {},
			},
			matcher: gomega.MatchError(fmt.Errorf(InvalidGraphNameFormatError, "abc 123", GraphNameFmt)),
		},
		"without root node": {
			ig:      makeTestInferenceGraph(),
			nodes:   make(map[string]InferenceRouter),
			matcher: gomega.MatchError(fmt.Errorf(RootNodeNotFoundError)),
		},
		"with root node": {
			ig: makeTestInferenceGraph(),
			nodes: map[string]InferenceRouter{
				GraphRootNodeName: {},
			},
			matcher: gomega.MatchError(nil),
		},
		"invalid weight for splitter": {
			ig: makeTestInferenceGraph(),
			nodes: map[string]InferenceRouter{
				GraphRootNodeName: {
					RouterType: "Splitter",
					Steps: []InferenceStep{
						{
							Weight: proto.Int64(80),
							InferenceTarget: InferenceTarget{
								NodeName: "test",
							},
						},
						{
							Weight: proto.Int64(30),
							InferenceTarget: InferenceTarget{
								ServiceURL: "http://foo-bar.local/",
							},
						},
					},
				},
			},
			matcher: gomega.MatchError(fmt.Errorf(InvalidWeightError, "foo-bar", GraphRootNodeName)),
		},
		"weight missing in splitter": {
			ig: makeTestInferenceGraph(),
			nodes: map[string]InferenceRouter{
				GraphRootNodeName: {
					RouterType: "Splitter",
					Steps: []InferenceStep{
						{
							InferenceTarget: InferenceTarget{
								ServiceName: "test",
							},
						},
					},
				},
			},
			matcher: gomega.MatchError(fmt.Errorf(WeightNotProvidedError, "foo-bar", GraphRootNodeName, "test")),
		},
		"simple splitter": {
			ig: makeTestInferenceGraph(),
			nodes: map[string]InferenceRouter{
				GraphRootNodeName: {
					RouterType: "Splitter",
					Steps: []InferenceStep{
						{
							Weight: proto.Int64(80),
							InferenceTarget: InferenceTarget{
								ServiceName: "service1",
							},
						},
						{
							Weight: proto.Int64(20),
							InferenceTarget: InferenceTarget{
								ServiceName: "service2",
							},
						},
					},
				},
			},
			matcher: gomega.MatchError(nil),
		},
		"step inference target not provided": {
			ig: makeTestInferenceGraph(),
			nodes: map[string]InferenceRouter{
				GraphRootNodeName: {
					RouterType: "Splitter",
					Steps: []InferenceStep{
						{
							Weight: proto.Int64(100),
						},
					},
				},
			},
			matcher: gomega.MatchError(fmt.Errorf(TargetNotProvidedError, 0, "", GraphRootNodeName, "foo-bar")),
		},
		"invalid inference graph target": {
			ig: makeTestInferenceGraph(),
			nodes: map[string]InferenceRouter{
				GraphRootNodeName: {
					RouterType: "Splitter",
					Steps: []InferenceStep{
						{
							Weight: proto.Int64(100),
							InferenceTarget: InferenceTarget{
								ServiceName: "service",
								NodeName:    "test",
							},
						},
					},
				},
			},
			matcher: gomega.MatchError(fmt.Errorf(InvalidTargetError, 0, "", GraphRootNodeName, "foo-bar")),
		},
		"duplicate step name": {
			ig: makeTestInferenceGraph(),
			nodes: map[string]InferenceRouter{
				GraphRootNodeName: {
					RouterType: "Splitter",
					Steps: []InferenceStep{
						{
							StepName: "step1",
							Weight:   proto.Int64(80),
							InferenceTarget: InferenceTarget{
								ServiceName: "service1",
							},
						},
						{
							StepName: "step1",
							Weight:   proto.Int64(20),
							InferenceTarget: InferenceTarget{
								ServiceName: "service2",
							},
						},
					},
				},
			},
			matcher: gomega.MatchError(fmt.Errorf(DuplicateStepNameError, GraphRootNodeName, "foo-bar", "step1")),
		},
	}

	for testName, scenario := range scenarios {
		t.Run(testName, func(t *testing.T) {
			ig := &scenario.ig
			for igField, value := range scenario.update {
				ig.update(igField, value)
			}
			ig.Spec.Nodes = scenario.nodes
			res := scenario.ig.ValidateCreate()
			if !g.Expect(gomega.MatchError(res)).To(gomega.Equal(scenario.matcher)) {
				t.Errorf("got %t, want %t", res, scenario.matcher)
			}
		})
	}
}

func TestInferenceGraph_ValidateUpdate(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	temptIg := makeTestTrainModel()
	old := temptIg.DeepCopyObject()
	scenarios := map[string]struct {
		ig      InferenceGraph
		update  map[string]string
		nodes   map[string]InferenceRouter
		matcher types.GomegaMatcher
	}{
		"no change": {
			ig: makeTestInferenceGraph(),
			nodes: map[string]InferenceRouter{
				GraphRootNodeName: {},
			},
			matcher: gomega.MatchError(nil),
		},
	}

	for testName, scenario := range scenarios {
		t.Run(testName, func(t *testing.T) {
			ig := &scenario.ig
			for igField, value := range scenario.update {
				ig.update(igField, value)
			}
			ig.Spec.Nodes = scenario.nodes
			res := scenario.ig.ValidateUpdate(old)
			if !g.Expect(gomega.MatchError(res)).To(gomega.Equal(scenario.matcher)) {
				t.Errorf("got %t, want %t", res, scenario.matcher)
			}
		})
	}
}

func TestInferenceGraph_ValidateDelete(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	scenarios := map[string]struct {
		ig      InferenceGraph
		update  map[string]string
		nodes   map[string]InferenceRouter
		matcher types.GomegaMatcher
	}{
		"simple": {
			ig: makeTestInferenceGraph(),
			nodes: map[string]InferenceRouter{
				GraphRootNodeName: {},
			},
			matcher: gomega.MatchError(nil),
		},
	}

	for testName, scenario := range scenarios {
		t.Run(testName, func(t *testing.T) {
			ig := &scenario.ig
			for igField, value := range scenario.update {
				ig.update(igField, value)
			}
			ig.Spec.Nodes = scenario.nodes
			res := scenario.ig.ValidateDelete()
			if !g.Expect(gomega.MatchError(res)).To(gomega.Equal(scenario.matcher)) {
				t.Errorf("got %t, want %t", res, scenario.matcher)
			}
		})
	}
}

func (ig *InferenceGraph) update(igField string, value string) {
	if igField == "Name" {
		ig.Name = value
	}
}
