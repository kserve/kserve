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
	"fmt"
	"strconv"

	"github.com/onsi/gomega/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// BeOwnedBy returns a matcher that checks if a Kubernetes object is owned by the specified owner
func BeOwnedBy(expectedOwner any) types.GomegaMatcher {
	return &beOwnedByMatcher{
		expectedOwner: expectedOwner,
	}
}

type beOwnedByMatcher struct {
	expectedOwner    any
	expectedName     string
	expectedKind     string
	actualOwnerRefs  []metav1.OwnerReference
	matchingOwnerRef *metav1.OwnerReference
}

func (matcher *beOwnedByMatcher) Match(actual any) (success bool, err error) {
	// Ensure actual is a Kubernetes object
	actualObj, ok := actual.(metav1.Object)
	if !ok {
		return false, fmt.Errorf("expected a Kubernetes object implementing metav1.Object, but got %T", actual)
	}

	// Extract expected owner name and kind
	expectedOwnerObj, ok := matcher.expectedOwner.(metav1.Object)
	if !ok {
		return false, fmt.Errorf("expected owner to be a Kubernetes object implementing metav1.Object, but got %T", matcher.expectedOwner)
	}

	matcher.expectedName = expectedOwnerObj.GetName()

	// Extract kind from the runtime object
	if runtimeObj, ok := matcher.expectedOwner.(runtime.Object); ok {
		gvk := runtimeObj.GetObjectKind().GroupVersionKind()
		if gvk.Kind != "" {
			matcher.expectedKind = gvk.Kind
		} else {
			// Fall back to type name if Kind is not set
			matcher.expectedKind = fmt.Sprintf("%T", matcher.expectedOwner)
			// Remove package path and pointer indicator if present
			if len(matcher.expectedKind) > 0 && matcher.expectedKind[0] == '*' {
				matcher.expectedKind = matcher.expectedKind[1:]
			}
			if idx := len(matcher.expectedKind) - 1; idx >= 0 {
				for i := idx; i >= 0; i-- {
					if matcher.expectedKind[i] == '.' {
						matcher.expectedKind = matcher.expectedKind[i+1:]
						break
					}
				}
			}
		}
	} else {
		return false, fmt.Errorf("expected owner to implement runtime.Object for kind extraction, but got %T", matcher.expectedOwner)
	}

	// Extract actual owner references
	matcher.actualOwnerRefs = actualObj.GetOwnerReferences()

	// Check if any owner reference matches
	for i := range matcher.actualOwnerRefs {
		ownerRef := &matcher.actualOwnerRefs[i]
		if ownerRef.Name == matcher.expectedName && ownerRef.Kind == matcher.expectedKind {
			matcher.matchingOwnerRef = ownerRef
			return true, nil
		}
	}

	return false, nil
}

func (matcher *beOwnedByMatcher) FailureMessage(actual any) string {
	if len(matcher.actualOwnerRefs) == 0 {
		return fmt.Sprintf("Expected %T to be owned by %q (Kind: %q), but no owner references were found",
			actual, matcher.expectedName, matcher.expectedKind)
	}

	ownerInfo := make([]string, len(matcher.actualOwnerRefs))
	for i, ref := range matcher.actualOwnerRefs {
		ownerInfo[i] = fmt.Sprintf("%s/%s", ref.Kind, ref.Name)
	}

	return fmt.Sprintf("Expected %T to be owned by %q (Kind: %q), but found owner references: %v",
		actual, matcher.expectedName, matcher.expectedKind, ownerInfo)
}

func (matcher *beOwnedByMatcher) NegatedFailureMessage(actual any) string {
	return fmt.Sprintf("Expected %T to not be owned by %q (Kind: %q), but it was found in owner references",
		actual, matcher.expectedName, matcher.expectedKind)
}

// BeControlledBy returns a matcher that checks if a Kubernetes object is owned AND controlled by the specified owner
func BeControlledBy(expectedOwner any) types.GomegaMatcher {
	return &beControlledByMatcher{
		expectedOwner:  expectedOwner,
		ownedByMatcher: BeOwnedBy(expectedOwner).(*beOwnedByMatcher),
	}
}

type beControlledByMatcher struct {
	expectedOwner    any
	ownedByMatcher   *beOwnedByMatcher
	controllerStatus string
}

func (matcher *beControlledByMatcher) Match(actual any) (success bool, err error) {
	owned, err := matcher.ownedByMatcher.Match(actual)
	if err != nil {
		return false, err
	}

	if !owned {
		return false, nil
	}

	if matcher.ownedByMatcher.matchingOwnerRef.Controller == nil {
		matcher.controllerStatus = "nil"
		return false, nil
	}

	isController := *matcher.ownedByMatcher.matchingOwnerRef.Controller
	matcher.controllerStatus = strconv.FormatBool(isController)

	return isController, nil
}

func (matcher *beControlledByMatcher) FailureMessage(actual any) string {
	owned, _ := matcher.ownedByMatcher.Match(actual)
	if !owned {
		return fmt.Sprintf("Expected %T to be controlled by %q, but %s",
			actual, matcher.ownedByMatcher.expectedName,
			matcher.ownedByMatcher.FailureMessage(actual)[len("Expected"):])
	}

	return fmt.Sprintf("Expected %T to be controlled by %q (Kind: %q), but controller field is %s",
		actual, matcher.ownedByMatcher.expectedName, matcher.ownedByMatcher.expectedKind, matcher.controllerStatus)
}

func (matcher *beControlledByMatcher) NegatedFailureMessage(actual any) string {
	return fmt.Sprintf("Expected %T to not be controlled by %q (Kind: %q), but it was found as a controller",
		actual, matcher.ownedByMatcher.expectedName, matcher.ownedByMatcher.expectedKind)
}
