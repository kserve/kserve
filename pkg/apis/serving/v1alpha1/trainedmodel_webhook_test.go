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
	"testing"

	"github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
		tm              TrainedModel
		update          map[string]string
		errMatcher      types.GomegaMatcher
		warningsMatcher types.GomegaMatcher
	}{
		"simple": {
			tm:              makeTestTrainModel(),
			errMatcher:      gomega.MatchError(nil),
			warningsMatcher: gomega.BeEmpty(),
		},
		"alphanumeric model name": {
			tm: makeTestTrainModel(),
			update: map[string]string{
				name: "Abc-123",
			},
			errMatcher:      gomega.MatchError(nil),
			warningsMatcher: gomega.BeEmpty(),
		},
		"name starts with number": {
			tm: makeTestTrainModel(),
			update: map[string]string{
				name: "4abc-3",
			},
			errMatcher:      gomega.MatchError(nil),
			warningsMatcher: gomega.BeEmpty(),
		},
		"name starts with dash": {
			tm: makeTestTrainModel(),
			update: map[string]string{
				name: "-abc-3",
			},
			errMatcher:      gomega.MatchError(nil),
			warningsMatcher: gomega.BeEmpty(),
		},
		"name ends with dash": {
			tm: makeTestTrainModel(),
			update: map[string]string{
				name: "abc-3-",
			},
			errMatcher:      gomega.MatchError(nil),
			warningsMatcher: gomega.BeEmpty(),
		},
		"name includes dot": {
			tm: makeTestTrainModel(),
			update: map[string]string{
				name: "abc.123",
			},
			errMatcher:      gomega.MatchError(fmt.Errorf(InvalidTmNameFormatError, "abc.123", TmRegexp)),
			warningsMatcher: gomega.BeEmpty(),
		},
		"name includes spaces": {
			tm: makeTestTrainModel(),
			update: map[string]string{
				name: "abc 123",
			},
			errMatcher:      gomega.MatchError(fmt.Errorf(InvalidTmNameFormatError, "abc 123", TmRegexp)),
			warningsMatcher: gomega.BeEmpty(),
		},
		"invalid storageURI prefix": {
			tm: makeTestTrainModel(),
			update: map[string]string{
				storageURI: "foo://kfserving/sklearn/iris",
			},
			errMatcher:      gomega.MatchError(fmt.Errorf(InvalidStorageUriFormatError, "bar", StorageUriProtocols, "foo://kfserving/sklearn/iris")),
			warningsMatcher: gomega.BeEmpty(),
		},
	}

	validator := TrainedModelValidator{}
	for testName, scenario := range scenarios {
		t.Run(testName, func(t *testing.T) {
			tm := &scenario.tm
			for tmField, value := range scenario.update {
				tm.update(tmField, value)
			}
			warnings, err := validator.ValidateCreate(t.Context(), tm)
			if !g.Expect(gomega.MatchError(err)).To(gomega.Equal(scenario.errMatcher)) {
				t.Errorf("got %t, want %t", err, scenario.errMatcher)
			}
			if !g.Expect(warnings).To(scenario.warningsMatcher) {
				t.Errorf("got %s, want %t", warnings, scenario.warningsMatcher)
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
		tm              TrainedModel
		update          map[string]string
		errMatcher      types.GomegaMatcher
		warningsMatcher types.GomegaMatcher
	}{
		"no change": {
			tm:              makeTestTrainModel(),
			errMatcher:      gomega.MatchError(nil),
			warningsMatcher: gomega.BeEmpty(),
		},
		"alphanumeric model name": {
			tm: makeTestTrainModel(),
			update: map[string]string{
				name: "Abc-123",
			},
			errMatcher:      gomega.MatchError(nil),
			warningsMatcher: gomega.BeEmpty(),
		},
		"name starts with number": {
			tm: makeTestTrainModel(),
			update: map[string]string{
				name: "4abc-3",
			},
			errMatcher:      gomega.MatchError(nil),
			warningsMatcher: gomega.BeEmpty(),
		},
		"name starts with dash": {
			tm: makeTestTrainModel(),
			update: map[string]string{
				name: "-abc-3",
			},
			errMatcher:      gomega.MatchError(nil),
			warningsMatcher: gomega.BeEmpty(),
		},
		"name ends with dash": {
			tm: makeTestTrainModel(),
			update: map[string]string{
				name: "abc-3-",
			},
			errMatcher:      gomega.MatchError(nil),
			warningsMatcher: gomega.BeEmpty(),
		},
		"name includes dot": {
			tm: makeTestTrainModel(),
			update: map[string]string{
				name: "abc.123",
			},
			errMatcher:      gomega.MatchError(fmt.Errorf(InvalidTmNameFormatError, "abc.123", TmRegexp)),
			warningsMatcher: gomega.BeEmpty(),
		},
		"name includes spaces": {
			tm: makeTestTrainModel(),
			update: map[string]string{
				name: "abc 123",
			},
			errMatcher:      gomega.MatchError(fmt.Errorf(InvalidTmNameFormatError, "abc 123", TmRegexp)),
			warningsMatcher: gomega.BeEmpty(),
		},
		"inference service": {
			tm: makeTestTrainModel(),
			update: map[string]string{
				infereceservice: "parent2",
			},
			errMatcher:      gomega.MatchError(nil),
			warningsMatcher: gomega.BeEmpty(),
		},
		"storageURI": {
			tm: makeTestTrainModel(),
			update: map[string]string{
				storageURI: "gs://kfserving/sklearn2/iris",
			},
			errMatcher:      gomega.MatchError(nil),
			warningsMatcher: gomega.BeEmpty(),
		},
		"invalid storageURI prefix": {
			tm: makeTestTrainModel(),
			update: map[string]string{
				storageURI: "foo://kfserving/sklearn/iris",
			},
			errMatcher:      gomega.MatchError(fmt.Errorf(InvalidStorageUriFormatError, "bar", StorageUriProtocols, "foo://kfserving/sklearn/iris")),
			warningsMatcher: gomega.BeEmpty(),
		},
		"framework": {
			tm: makeTestTrainModel(),
			update: map[string]string{
				framework: "sklearn2",
			},
			errMatcher:      gomega.MatchError(nil),
			warningsMatcher: gomega.BeEmpty(),
		},
		"change immutable memory": {
			tm: makeTestTrainModel(),
			update: map[string]string{
				memory: newMemory,
			},
			errMatcher:      gomega.MatchError(fmt.Errorf(InvalidTmMemoryModification, temptTm.Name, temptTm.Spec.Model.Memory.String(), newMemory)),
			warningsMatcher: gomega.BeEmpty(),
		},
	}

	validator := TrainedModelValidator{}
	for testName, scenario := range scenarios {
		t.Run(testName, func(t *testing.T) {
			tm := &scenario.tm
			for tmField, value := range scenario.update {
				tm.update(tmField, value)
			}
			warnings, err := validator.ValidateUpdate(t.Context(), old, tm)
			if !g.Expect(gomega.MatchError(err)).To(gomega.Equal(scenario.errMatcher)) {
				t.Errorf("got %t, want %t", err, scenario.errMatcher)
			}
			if !g.Expect(warnings).To(scenario.warningsMatcher) {
				t.Errorf("got %s, want %t", warnings, scenario.warningsMatcher)
			}
		})
	}
}

func TestValidateDelete(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	scenarios := map[string]struct {
		tm              TrainedModel
		update          map[string]string
		errMatcher      types.GomegaMatcher
		warningsMatcher types.GomegaMatcher
	}{
		"simple": {
			tm:              makeTestTrainModel(),
			errMatcher:      gomega.MatchError(nil),
			warningsMatcher: gomega.BeEmpty(),
		},
	}

	validator := TrainedModelValidator{}
	for testName, scenario := range scenarios {
		t.Run(testName, func(t *testing.T) {
			warnings, err := validator.ValidateDelete(t.Context(), &scenario.tm)
			if !g.Expect(gomega.MatchError(err)).To(gomega.Equal(scenario.errMatcher)) {
				t.Errorf("got %t, want %t", err, scenario.errMatcher)
			}
			if !g.Expect(warnings).To(scenario.warningsMatcher) {
				t.Errorf("got %s, want %t", warnings, scenario.warningsMatcher)
			}
		})
	}
}

func (tm *TrainedModel) update(tmField string, value string) {
	switch tmField {
	case name:
		tm.Name = value
	case infereceservice:
		tm.Spec.InferenceService = value
	case storageURI:
		tm.Spec.Model.StorageURI = value
	case framework:
		tm.Spec.Model.Framework = value
	case memory:
		tm.Spec.Model.Memory = resource.MustParse(value)
	default:
		// do nothing
	}
}
