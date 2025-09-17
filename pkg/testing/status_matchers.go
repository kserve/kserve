/*
Copyright 2025 The KServe Authors.

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
	"encoding/json"
	"errors"
	"fmt"
	"reflect"

	v1 "knative.dev/pkg/apis/duck/v1"

	"github.com/onsi/gomega/types"
	"knative.dev/pkg/apis"
)

// HaveCondition returns a matcher that checks if a Status has a condition with the specified type and status
func HaveCondition(conditionType string, expectedStatus string) types.GomegaMatcher {
	return &haveConditionMatcher{
		conditionType:  conditionType,
		expectedStatus: expectedStatus,
	}
}

type haveConditionMatcher struct {
	conditionType    string
	expectedStatus   string
	actualConditions []apis.Condition
	foundCondition   *apis.Condition
}

func (matcher *haveConditionMatcher) Match(actual any) (success bool, err error) {
	conditions, err := matcher.extractConditions(actual)
	if err != nil {
		return false, err
	}

	matcher.actualConditions = conditions

	for i := range conditions {
		condition := &conditions[i]
		if string(condition.Type) == matcher.conditionType {
			matcher.foundCondition = condition
			return string(condition.Status) == matcher.expectedStatus, nil
		}
	}

	return false, nil
}

func (matcher *haveConditionMatcher) extractConditions(actual any) (v1.Conditions, error) {
	actualValue := reflect.ValueOf(actual)

	if actualValue.Kind() == reflect.Ptr {
		if actualValue.IsNil() {
			return nil, errors.New("expected a non-nil pointer, but got nil")
		}
		actualValue = actualValue.Elem()
	}

	if actualValue.Kind() != reflect.Struct {
		return nil, fmt.Errorf("expected a struct or pointer to struct, but got %T", actual)
	}

	conditionsField := actualValue.FieldByName("Conditions")
	if conditionsField.IsValid() && conditionsField.Kind() == reflect.Slice {
		return matcher.convertToConditions(conditionsField)
	}

	statusField := actualValue.FieldByName("Status")
	if statusField.IsValid() {
		if statusField.Kind() == reflect.Struct {
			conditionsField = statusField.FieldByName("Conditions")
			if conditionsField.IsValid() && conditionsField.Kind() == reflect.Slice {
				return matcher.convertToConditions(conditionsField)
			}
		}
	}

	return nil, fmt.Errorf("could not find Conditions field in %T", actual)
}

func (matcher *haveConditionMatcher) convertToConditions(conditionsValue reflect.Value) (v1.Conditions, error) {
	conditions, ok := conditionsValue.Interface().(v1.Conditions)
	if !ok {
		return nil, fmt.Errorf("expected v1.Conditions, but got %v", conditionsValue.Type())
	}

	return conditions, nil
}

func (matcher *haveConditionMatcher) FailureMessage(actual any) (message string) {
	if len(matcher.actualConditions) == 0 {
		return fmt.Sprintf("Expected %T to have condition %q with status %q, but no conditions were found",
			actual, matcher.conditionType, matcher.expectedStatus)
	}

	conditions, _ := json.MarshalIndent(matcher.actualConditions, "", "  ")
	if matcher.foundCondition != nil {
		return fmt.Sprintf("Expected %T to have condition %q with status %q, but found status %q (reason: %q, message: %q):conditions:\n%s",
			actual, matcher.conditionType, matcher.expectedStatus, string(matcher.foundCondition.Status), matcher.foundCondition.Reason, matcher.foundCondition.Message, string(conditions))
	}

	return fmt.Sprintf("Expected %T to have condition %q with status %q, but condition was not found. Available conditions:\n%s",
		actual, matcher.conditionType, matcher.expectedStatus, string(conditions))
}

func (matcher *haveConditionMatcher) NegatedFailureMessage(actual any) (message string) {
	if matcher.foundCondition != nil {
		return fmt.Sprintf("Expected %T to not have condition %q with status %q, but it was found",
			actual, matcher.conditionType, matcher.expectedStatus)
	}

	return fmt.Sprintf("Expected %T to not have condition %q with status %q, and it was not found (which is correct)",
		actual, matcher.conditionType, matcher.expectedStatus)
}
