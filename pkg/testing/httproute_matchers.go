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

	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/utils/ptr"

	"github.com/onsi/gomega/types"
	gatewayapi "sigs.k8s.io/gateway-api/apis/v1"
)

// extractHTTPRoute safely extracts HTTPRoute from either pointer or value type
func extractHTTPRoute(actual any) (*gatewayapi.HTTPRoute, error) {
	switch v := actual.(type) {
	case gatewayapi.HTTPRouteSpec:
		return &gatewayapi.HTTPRoute{Spec: v}, nil
	case *gatewayapi.HTTPRouteSpec:
		if v == nil {
			return nil, errors.New("expected non-nil gatewayapi.HTTPRouteSpec, but got nil")
		}
		return &gatewayapi.HTTPRoute{Spec: *v}, nil
	case *gatewayapi.HTTPRoute:
		if v == nil {
			return nil, errors.New("expected non-nil gatewayapi.HTTPRoute, but got nil")
		}
		return v, nil
	case gatewayapi.HTTPRoute:
		return &v, nil
	default:
		return nil, fmt.Errorf("expected gatewayapi.HTTPRoute gatewayapi.HTTPRouteSpec, but got %T", actual)
	}
}

// HaveGatewayRefs returns a matcher that checks if an HTTPRoute has the specified gateway parent refs
func HaveGatewayRefs(expectedGateways ...gatewayapi.ParentReference) types.GomegaMatcher {
	return &haveGatewayRefsMatcher{
		expectedGatewayNames: expectedGateways,
	}
}

type haveGatewayRefsMatcher struct {
	expectedGatewayNames []gatewayapi.ParentReference
	actualParentRefs     []gatewayapi.ParentReference
	actualGatewayNames   []string
}

func (matcher *haveGatewayRefsMatcher) Match(actual any) (success bool, err error) {
	httpRoute, err := extractHTTPRoute(actual)
	if err != nil {
		return false, err
	}

	matcher.actualParentRefs = httpRoute.Spec.ParentRefs

	expectedSet := make(map[string]gatewayapi.ParentReference)
	for _, ref := range matcher.expectedGatewayNames {
		expectedSet[string(ref.Name)] = ref
	}

	for _, ref := range matcher.actualParentRefs {
		expectedRef, found := expectedSet[string(ref.Name)]
		if !found {
			return false, nil
		}

		if expectedRef.Namespace != nil {
			return ptr.Deref(expectedRef.Namespace, "") == ptr.Deref(ref.Namespace, ""), nil
		}
	}

	return true, nil
}

func (matcher *haveGatewayRefsMatcher) FailureMessage(actual any) string {
	return fmt.Sprintf("Expected %T to have gateway refs %v, but found %v",
		actual, matcher.expectedGatewayNames, matcher.actualGatewayNames)
}

func (matcher *haveGatewayRefsMatcher) NegatedFailureMessage(actual any) string {
	return fmt.Sprintf("Expected %T to not have gateway refs %v, but they were found",
		actual, matcher.expectedGatewayNames)
}

// HaveBackendRefs returns a matcher that checks if an HTTPRoute has the specified backend refs.
func HaveBackendRefs(backends ...gatewayapi.HTTPBackendRef) types.GomegaMatcher {
	return &haveBackendRefsMatcher{
		expectedBackendRefs: backends,
	}
}

type haveBackendRefsMatcher struct {
	expectedBackendRefs []gatewayapi.HTTPBackendRef
	actualBackendRefs   []gatewayapi.HTTPBackendRef
}

func (matcher *haveBackendRefsMatcher) Match(actual any) (success bool, err error) {
	httpRoute, err := extractHTTPRoute(actual)
	if err != nil {
		return false, err
	}

	for _, rule := range httpRoute.Spec.Rules {
		matcher.actualBackendRefs = append(matcher.actualBackendRefs, rule.BackendRefs...)
	}

	if len(matcher.actualBackendRefs) != len(matcher.expectedBackendRefs) {
		return false, nil
	}

	for _, want := range matcher.expectedBackendRefs {
		found := false
		for _, got := range matcher.actualBackendRefs {
			if equality.Semantic.DeepEqual(want, got) {
				found = true
				break
			}
		}
		if !found {
			return false, nil
		}
	}

	return true, nil
}

func (matcher *haveBackendRefsMatcher) FailureMessage(actual any) string {
	expected, _ := json.MarshalIndent(matcher.expectedBackendRefs, "", "  ")
	got, _ := json.MarshalIndent(matcher.actualBackendRefs, "", "  ")
	return fmt.Sprintf("Expected %T to have backend refs:\n%s\nbut found:\n%s",
		actual, expected, got)
}

func (matcher *haveBackendRefsMatcher) NegatedFailureMessage(actual any) string {
	expected, _ := json.MarshalIndent(matcher.expectedBackendRefs, "", "  ")
	got, _ := json.MarshalIndent(matcher.actualBackendRefs, "", "  ")
	return fmt.Sprintf("Expected %T to not have backend refs:\n%s, got:\n%s",
		actual, expected, got)
}
