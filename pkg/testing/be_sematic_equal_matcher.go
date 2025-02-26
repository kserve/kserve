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

package testing

import (
	"errors"
	"reflect"

	"github.com/onsi/gomega/format"
	"github.com/onsi/gomega/types"
	"k8s.io/apimachinery/pkg/api/equality"
)

// Gomega does not support semantic equal so adding this here in testing util

func BeSematicEqual(expected interface{}) types.GomegaMatcher {
	return &BeSematicEqualMatcher{
		Expected: expected,
	}
}

type BeSematicEqualMatcher struct {
	Expected interface{}
}

func (matcher *BeSematicEqualMatcher) Match(actual interface{}) (success bool, err error) {
	if actual == nil && matcher.Expected == nil {
		return false, errors.New("Both actual and expected must not be nil.")
	}

	convertedActual := actual

	if actual != nil && matcher.Expected != nil && reflect.TypeOf(actual).ConvertibleTo(reflect.TypeOf(matcher.Expected)) {
		convertedActual = reflect.ValueOf(actual).Convert(reflect.TypeOf(matcher.Expected)).Interface()
	}

	return equality.Semantic.DeepEqual(convertedActual, matcher.Expected), nil
}

func (matcher *BeSematicEqualMatcher) FailureMessage(actual interface{}) (message string) {
	return format.Message(actual, "to be equivalent to", matcher.Expected)
}

func (matcher *BeSematicEqualMatcher) NegatedFailureMessage(actual interface{}) (message string) {
	return format.Message(actual, "not to be equivalent to", matcher.Expected)
}
