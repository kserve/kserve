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

package v1beta1

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
	"google.golang.org/protobuf/proto"
	"k8s.io/utils/ptr"
)

func TestComponentExtensionSpec_Validate(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	scenarios := map[string]struct {
		spec    ComponentExtensionSpec
		matcher types.GomegaMatcher
	}{
		"InvalidReplica": {
			spec: ComponentExtensionSpec{
				MinReplicas: ptr.To(int32(3)),
				MaxReplicas: 2,
			},
			matcher: gomega.Not(gomega.BeNil()),
		},
		"InvalidContainerConcurrency": {
			spec: ComponentExtensionSpec{
				ContainerConcurrency: proto.Int64(-1),
			},
			matcher: gomega.Not(gomega.BeNil()),
		},
	}

	for name, scenario := range scenarios {
		t.Run(name, func(t *testing.T) {
			res := scenario.spec.Validate()
			if !g.Expect(res).To(scenario.matcher) {
				t.Errorf("got %q, want %q", res, scenario.matcher)
			}
		})
	}
}

func TestComponentExtensionSpec_validateStorageSpec(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	scenarios := map[string]struct {
		spec       *StorageSpec
		storageUri *string
		matcher    types.GomegaMatcher
	}{
		"ValidStoragespec": {
			spec: &StorageSpec{
				Parameters: &map[string]string{
					"type": "s3",
				},
			},
			storageUri: nil,
			matcher:    gomega.BeNil(),
		},
		"ValidStoragespecWithoutParameters": {
			spec:       &StorageSpec{},
			storageUri: nil,
			matcher:    gomega.BeNil(),
		},
		"ValidStoragespecWithStorageURI": {
			spec: &StorageSpec{
				Parameters: &map[string]string{
					"type": "s3",
				},
			},
			storageUri: proto.String("s3://test/model"),
			matcher:    gomega.BeNil(),
		},
		"StorageSpecWithInvalidStorageURI": {
			spec: &StorageSpec{
				Parameters: &map[string]string{
					"type": "gs",
				},
			},
			storageUri: proto.String("gs://test/model"),
			matcher:    gomega.MatchError(fmt.Errorf(UnsupportedStorageURIFormatError, strings.Join(SupportedStorageSpecURIPrefixList, ", "), "gs://test/model")),
		},
		"InvalidStoragespec": {
			spec: &StorageSpec{
				Parameters: &map[string]string{
					"type": "gs",
				},
			},
			storageUri: nil,
			matcher:    gomega.MatchError(fmt.Errorf(UnsupportedStorageSpecFormatError, strings.Join(SupportedStorageSpecURIPrefixList, ", "), "gs")),
		},
	}
	for name, scenario := range scenarios {
		t.Run(name, func(t *testing.T) {
			g.Expect(validateStorageSpec(scenario.spec, scenario.storageUri)).To(scenario.matcher)
		})
	}
}

func TestComponentExtensionSpec_validateLogger(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	scenarios := map[string]struct {
		logger  *LoggerSpec
		matcher types.GomegaMatcher
	}{
		"LoggerWithLogAllMode": {
			logger: &LoggerSpec{
				Mode: LogAll,
			},
			matcher: gomega.BeNil(),
		},
		"LoggerWithLogRequestMode": {
			logger: &LoggerSpec{
				Mode: LogRequest,
			},
			matcher: gomega.BeNil(),
		},
		"LoggerWithLogResponseMode": {
			logger: &LoggerSpec{
				Mode: LogResponse,
			},
			matcher: gomega.BeNil(),
		},
		"LoggerWithHeaderMetadata": {
			logger: &LoggerSpec{
				Mode:            LogAll,
				MetadataHeaders: []string{"Foo", "Bar"},
			},
			matcher: gomega.BeNil(),
		},
		"InvalidLoggerMode": {
			logger: &LoggerSpec{
				Mode: "InvalidMode",
			},
			matcher: gomega.MatchError(errors.New(InvalidLoggerType)),
		},
		"LoggerIsNil": {
			logger:  nil,
			matcher: gomega.BeNil(),
		},
	}
	for name, scenario := range scenarios {
		t.Run(name, func(t *testing.T) {
			g.Expect(validateLogger(scenario.logger)).To(scenario.matcher)
		})
	}
}

func TestFirstNonNilComponent(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	spec := PredictorSpec{
		SKLearn: &SKLearnSpec{},
	}
	scenarios := map[string]struct {
		components []ComponentImplementation
		matcher    types.GomegaMatcher
	}{
		"WithNonNilComponent": {
			components: []ComponentImplementation{
				spec.PyTorch,
				spec.LightGBM,
				spec.SKLearn,
				spec.Tensorflow,
			},
			matcher: gomega.Equal(spec.SKLearn),
		},
		"NoNonNilComponents": {
			components: []ComponentImplementation{
				spec.PyTorch,
				spec.LightGBM,
				spec.Tensorflow,
				spec.PMML,
			},
			matcher: gomega.BeNil(),
		},
	}
	for name, scenario := range scenarios {
		t.Run(name, func(t *testing.T) {
			g.Expect(FirstNonNilComponent(scenario.components)).To(scenario.matcher)
		})
	}
}
