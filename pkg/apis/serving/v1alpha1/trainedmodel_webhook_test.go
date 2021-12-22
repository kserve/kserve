/*
Copyright 2021 The KServe Authors.

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
	"fmt"
	"github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"testing"
)

var (
	name            = "Name"
	infereceservice = "infrenceservice"
	storageURI      = "storageURI"
	framework       = "framework"
	memory          = "memory"
)

func makeTestTrainModel() TrainedModel {
	quantity := resource.MustParse("100Mi")
	tm := TrainedModel{
		TypeMeta: metav1.TypeMeta{
			Kind:       "TrainedModel",
			APIVersion: "v1alpha1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "bar",
		},
		Spec: TrainedModelSpec{
			InferenceService: "Parent",
			Model: ModelSpec{
				StorageURI: "gs://kfserving/sklearn/iris",
				Framework:  "sklearn",
				Memory:     quantity,
			},
		},
	}

	return tm
}

func TestValidateCreate(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	scenarios := map[string]struct {
		tm      TrainedModel
		update  map[string]string
		matcher types.GomegaMatcher
	}{
		"simple": {
			tm:      makeTestTrainModel(),
			matcher: gomega.MatchError(nil),
		},
		"alphanumeric model name": {
			tm: makeTestTrainModel(),
			update: map[string]string{
				name: "Abc-123",
			},
			matcher: gomega.MatchError(nil),
		},
		"name starts with number": {
			tm: makeTestTrainModel(),
			update: map[string]string{
				name: "4abc-3",
			},
			matcher: gomega.MatchError(nil),
		},
		"name starts with dash": {
			tm: makeTestTrainModel(),
			update: map[string]string{
				name: "-abc-3",
			},
			matcher: gomega.MatchError(nil),
		},
		"name ends with dash": {
			tm: makeTestTrainModel(),
			update: map[string]string{
				name: "abc-3-",
			},
			matcher: gomega.MatchError(nil),
		},
		"name includes dot": {
			tm: makeTestTrainModel(),
			update: map[string]string{
				name: "abc.123",
			},
			matcher: gomega.MatchError(fmt.Errorf(InvalidTmNameFormatError, "abc.123", TmRegexp)),
		},
		"name includes spaces": {
			tm: makeTestTrainModel(),
			update: map[string]string{
				name: "abc 123",
			},
			matcher: gomega.MatchError(fmt.Errorf(InvalidTmNameFormatError, "abc 123", TmRegexp)),
		},
		"invalid storageURI prefix": {
			tm: makeTestTrainModel(),
			update: map[string]string{
				storageURI: "foo://kfserving/sklearn/iris",
			},
			matcher: gomega.MatchError(fmt.Errorf(InvalidStorageUriFormatError, "bar", StorageUriProtocols, "foo://kfserving/sklearn/iris")),
		},
	}

	for testName, scenario := range scenarios {
		t.Run(testName, func(t *testing.T) {
			tm := &scenario.tm
			for tmField, value := range scenario.update {
				tm.update(tmField, value)
			}
			res := scenario.tm.ValidateCreate()
			if !g.Expect(gomega.MatchError(res)).To(gomega.Equal(scenario.matcher)) {
				t.Errorf("got %t, want %t", res, scenario.matcher)
			}
		})
	}
}

func TestValidateUpdate(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	temptTm := makeTestTrainModel()
	old := temptTm.DeepCopyObject()
	newMemory := "300Mi"

	scenarios := map[string]struct {
		tm      TrainedModel
		update  map[string]string
		matcher types.GomegaMatcher
	}{
		"no change": {
			tm:      makeTestTrainModel(),
			matcher: gomega.MatchError(nil),
		},
		"alphanumeric model name": {
			tm: makeTestTrainModel(),
			update: map[string]string{
				name: "Abc-123",
			},
			matcher: gomega.MatchError(nil),
		},
		"name starts with number": {
			tm: makeTestTrainModel(),
			update: map[string]string{
				name: "4abc-3",
			},
			matcher: gomega.MatchError(nil),
		},
		"name starts with dash": {
			tm: makeTestTrainModel(),
			update: map[string]string{
				name: "-abc-3",
			},
			matcher: gomega.MatchError(nil),
		},
		"name ends with dash": {
			tm: makeTestTrainModel(),
			update: map[string]string{
				name: "abc-3-",
			},
			matcher: gomega.MatchError(nil),
		},
		"name includes dot": {
			tm: makeTestTrainModel(),
			update: map[string]string{
				name: "abc.123",
			},
			matcher: gomega.MatchError(fmt.Errorf(InvalidTmNameFormatError, "abc.123", TmRegexp)),
		},
		"name includes spaces": {
			tm: makeTestTrainModel(),
			update: map[string]string{
				name: "abc 123",
			},
			matcher: gomega.MatchError(fmt.Errorf(InvalidTmNameFormatError, "abc 123", TmRegexp)),
		},
		"inference service": {
			tm: makeTestTrainModel(),
			update: map[string]string{
				infereceservice: "parent2",
			},
			matcher: gomega.MatchError(nil),
		},
		"storageURI": {
			tm: makeTestTrainModel(),
			update: map[string]string{
				storageURI: "gs://kfserving/sklearn2/iris",
			},
			matcher: gomega.MatchError(nil),
		},
		"invalid storageURI prefix": {
			tm: makeTestTrainModel(),
			update: map[string]string{
				storageURI: "foo://kfserving/sklearn/iris",
			},
			matcher: gomega.MatchError(fmt.Errorf(InvalidStorageUriFormatError, "bar", StorageUriProtocols, "foo://kfserving/sklearn/iris")),
		},
		"framework": {
			tm: makeTestTrainModel(),
			update: map[string]string{
				framework: "sklearn2",
			},
			matcher: gomega.MatchError(nil),
		},
		"change immutable memory": {
			tm: makeTestTrainModel(),
			update: map[string]string{
				memory: newMemory,
			},
			matcher: gomega.MatchError(fmt.Errorf(InvalidTmMemoryModification, temptTm.Name, temptTm.Spec.Model.Memory.String(), newMemory)),
		},
	}

	for testName, scenario := range scenarios {
		t.Run(testName, func(t *testing.T) {
			tm := &scenario.tm
			for tmField, value := range scenario.update {
				tm.update(tmField, value)
			}
			res := scenario.tm.ValidateUpdate(old)
			if !g.Expect(gomega.MatchError(res)).To(gomega.Equal(scenario.matcher)) {
				t.Errorf("got %t, want %t", res, scenario.matcher)
			}
		})
	}
}

func TestValidateDelete(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	scenarios := map[string]struct {
		tm      TrainedModel
		update  map[string]string
		matcher types.GomegaMatcher
	}{
		"simple": {
			tm:      makeTestTrainModel(),
			matcher: gomega.MatchError(nil),
		},
	}

	for testName, scenario := range scenarios {
		t.Run(testName, func(t *testing.T) {
			res := scenario.tm.ValidateDelete()
			if !g.Expect(gomega.MatchError(res)).To(gomega.Equal(scenario.matcher)) {
				t.Errorf("got %t, want %t", res, scenario.matcher)
			}
		})
	}
}

func (tm *TrainedModel) update(tmField string, value string) {
	if tmField == name {
		tm.Name = value
	} else if tmField == infereceservice {
		tm.Spec.InferenceService = value
	} else if tmField == storageURI {
		tm.Spec.Model.StorageURI = value
	} else if tmField == framework {
		tm.Spec.Model.Framework = value
	} else if tmField == memory {
		tm.Spec.Model.Memory = resource.MustParse(value)
	}
}
