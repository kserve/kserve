/*
Copyright 2022 The KServe Authors.

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

package v1alpha1

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
	"google.golang.org/protobuf/proto"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
		ig              InferenceGraph
		update          map[string]string
		nodes           map[string]InferenceRouter
		errMatcher      types.GomegaMatcher
		warningsMatcher types.GomegaMatcher
	}{
		"simple": {
			ig: makeTestInferenceGraph(),
			nodes: map[string]InferenceRouter{
				GraphRootNodeName: {},
			},
			errMatcher:      gomega.MatchError(nil),
			warningsMatcher: gomega.BeEmpty(),
		},
		"alphanumeric model name": {
			ig: makeTestInferenceGraph(),
			update: map[string]string{
				name: "Abc-123",
			},
			errMatcher:      gomega.MatchError(fmt.Errorf(InvalidGraphNameFormatError, "Abc-123", GraphNameFmt)),
			warningsMatcher: gomega.BeEmpty(),
		},
		"name starts with number": {
			ig: makeTestInferenceGraph(),
			update: map[string]string{
				name: "4abc-3",
			},
			nodes: map[string]InferenceRouter{
				GraphRootNodeName: {},
			},
			errMatcher:      gomega.MatchError(fmt.Errorf(InvalidGraphNameFormatError, "4abc-3", GraphNameFmt)),
			warningsMatcher: gomega.BeEmpty(),
		},
		"name starts with dash": {
			ig: makeTestInferenceGraph(),
			update: map[string]string{
				name: "-abc-3",
			},
			nodes: map[string]InferenceRouter{
				GraphRootNodeName: {},
			},
			errMatcher:      gomega.MatchError(fmt.Errorf(InvalidGraphNameFormatError, "-abc-3", GraphNameFmt)),
			warningsMatcher: gomega.BeEmpty(),
		},
		"name ends with dash": {
			ig: makeTestInferenceGraph(),
			update: map[string]string{
				name: "abc-3-",
			},
			nodes: map[string]InferenceRouter{
				GraphRootNodeName: {},
			},
			errMatcher:      gomega.MatchError(fmt.Errorf(InvalidGraphNameFormatError, "abc-3-", GraphNameFmt)),
			warningsMatcher: gomega.BeEmpty(),
		},
		"name includes dot": {
			ig: makeTestInferenceGraph(),
			update: map[string]string{
				name: "abc.123",
			},
			nodes: map[string]InferenceRouter{
				GraphRootNodeName: {},
			},
			errMatcher:      gomega.MatchError(fmt.Errorf(InvalidGraphNameFormatError, "abc.123", GraphNameFmt)),
			warningsMatcher: gomega.BeEmpty(),
		},
		"name includes spaces": {
			ig: makeTestInferenceGraph(),
			update: map[string]string{
				name: "abc 123",
			},
			nodes: map[string]InferenceRouter{
				GraphRootNodeName: {},
			},
			errMatcher:      gomega.MatchError(fmt.Errorf(InvalidGraphNameFormatError, "abc 123", GraphNameFmt)),
			warningsMatcher: gomega.BeEmpty(),
		},
		"without root node": {
			ig:              makeTestInferenceGraph(),
			nodes:           make(map[string]InferenceRouter),
			errMatcher:      gomega.MatchError(errors.New(RootNodeNotFoundError)),
			warningsMatcher: gomega.BeEmpty(),
		},
		"with root node": {
			ig: makeTestInferenceGraph(),
			nodes: map[string]InferenceRouter{
				GraphRootNodeName: {},
			},
			errMatcher:      gomega.MatchError(nil),
			warningsMatcher: gomega.BeEmpty(),
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
			errMatcher:      gomega.MatchError(fmt.Errorf(InvalidWeightError, "foo-bar", GraphRootNodeName)),
			warningsMatcher: gomega.BeEmpty(),
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
			errMatcher:      gomega.MatchError(fmt.Errorf(WeightNotProvidedError, "foo-bar", GraphRootNodeName, "test")),
			warningsMatcher: gomega.BeEmpty(),
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
			errMatcher:      gomega.MatchError(nil),
			warningsMatcher: gomega.BeEmpty(),
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
			errMatcher:      gomega.MatchError(fmt.Errorf(TargetNotProvidedError, 0, "", GraphRootNodeName, "foo-bar")),
			warningsMatcher: gomega.BeEmpty(),
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
			errMatcher:      gomega.MatchError(fmt.Errorf(InvalidTargetError, 0, "", GraphRootNodeName, "foo-bar")),
			warningsMatcher: gomega.BeEmpty(),
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
			errMatcher:      gomega.MatchError(fmt.Errorf(DuplicateStepNameError, GraphRootNodeName, "foo-bar", "step1")),
			warningsMatcher: gomega.BeEmpty(),
		},
	}

	validator := InferenceGraphValidator{}
	for testName, scenario := range scenarios {
		t.Run(testName, func(t *testing.T) {
			ig := &scenario.ig
			for igField, value := range scenario.update {
				ig.update(igField, value)
			}
			ig.Spec.Nodes = scenario.nodes
			warnings, err := validator.ValidateCreate(context.Background(), ig)
			if !g.Expect(gomega.MatchError(err)).To(gomega.Equal(scenario.errMatcher)) {
				t.Errorf("got %t, want %t", err, scenario.errMatcher)
			}
			if !g.Expect(warnings).To(scenario.warningsMatcher) {
				t.Errorf("got %s, want %t", warnings, scenario.warningsMatcher)
			}
		})
	}
}

func TestInferenceGraph_ValidateUpdate(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	temptIg := makeTestTrainModel()
	old := temptIg.DeepCopyObject()
	scenarios := map[string]struct {
		ig              InferenceGraph
		update          map[string]string
		nodes           map[string]InferenceRouter
		errMatcher      types.GomegaMatcher
		warningsMatcher types.GomegaMatcher
	}{
		"no change": {
			ig: makeTestInferenceGraph(),
			nodes: map[string]InferenceRouter{
				GraphRootNodeName: {},
			},
			errMatcher:      gomega.MatchError(nil),
			warningsMatcher: gomega.BeEmpty(),
		},
	}

	validator := InferenceGraphValidator{}
	for testName, scenario := range scenarios {
		t.Run(testName, func(t *testing.T) {
			ig := &scenario.ig
			for igField, value := range scenario.update {
				ig.update(igField, value)
			}
			ig.Spec.Nodes = scenario.nodes
			warnings, err := validator.ValidateUpdate(context.Background(), old, ig)
			if !g.Expect(gomega.MatchError(err)).To(gomega.Equal(scenario.errMatcher)) {
				t.Errorf("got %t, want %t", err, scenario.errMatcher)
			}
			if !g.Expect(warnings).To(scenario.warningsMatcher) {
				t.Errorf("got %s, want %t", warnings, scenario.warningsMatcher)
			}
		})
	}
}

func TestInferenceGraph_ValidateDelete(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	scenarios := map[string]struct {
		ig              InferenceGraph
		update          map[string]string
		nodes           map[string]InferenceRouter
		errMatcher      types.GomegaMatcher
		warningsMatcher types.GomegaMatcher
	}{
		"simple": {
			ig: makeTestInferenceGraph(),
			nodes: map[string]InferenceRouter{
				GraphRootNodeName: {},
			},
			errMatcher:      gomega.MatchError(nil),
			warningsMatcher: gomega.BeEmpty(),
		},
	}

	validator := InferenceGraphValidator{}
	for testName, scenario := range scenarios {
		t.Run(testName, func(t *testing.T) {
			ig := &scenario.ig
			for igField, value := range scenario.update {
				ig.update(igField, value)
			}
			ig.Spec.Nodes = scenario.nodes
			warnings, err := validator.ValidateDelete(context.Background(), ig)
			if !g.Expect(gomega.MatchError(err)).To(gomega.Equal(scenario.errMatcher)) {
				t.Errorf("got %t, want %t", err, scenario.errMatcher)
			}
			if !g.Expect(warnings).To(scenario.warningsMatcher) {
				t.Errorf("got %s, want %t", warnings, scenario.warningsMatcher)
			}
		})
	}
}

func (ig *InferenceGraph) update(igField string, value string) {
	if igField == "Name" {
		ig.Name = value
	}
}
