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
	"errors"
	"fmt"

	"github.com/onsi/gomega/types"
	appsv1 "k8s.io/api/apps/v1"
)

// HaveContainerImage returns a matcher that checks if a Deployment has a container with the specified image
func HaveContainerImage(expectedImage string) types.GomegaMatcher {
	return &haveContainerImageMatcher{
		expectedImage: expectedImage,
	}
}

type haveContainerImageMatcher struct {
	expectedImage string
	actualImages  []string
	foundImage    bool
}

func (matcher *haveContainerImageMatcher) Match(actual any) (success bool, err error) {
	var deployment *appsv1.Deployment
	switch v := actual.(type) {
	case *appsv1.Deployment:
		if v == nil {
			return false, errors.New("expected non-nil *appsv1.Deployment, but got nil")
		}
		deployment = v
	case appsv1.Deployment:
		deployment = &v
	default:
		return false, fmt.Errorf("expected *appsv1.Deployment or appsv1.Deployment, but got %T", actual)
	}

	containers := deployment.Spec.Template.Spec.Containers
	matcher.actualImages = make([]string, len(containers))
	for i, container := range containers {
		matcher.actualImages[i] = container.Image
		if container.Image == matcher.expectedImage {
			matcher.foundImage = true
		}
	}

	return matcher.foundImage, nil
}

func (matcher *haveContainerImageMatcher) FailureMessage(actual any) string {
	if len(matcher.actualImages) == 0 {
		return fmt.Sprintf("Expected %T to have container with image %q, but no containers were found",
			actual, matcher.expectedImage)
	}

	return fmt.Sprintf("Expected %T to have container with image %q, but found container images: %v",
		actual, matcher.expectedImage, matcher.actualImages)
}

func (matcher *haveContainerImageMatcher) NegatedFailureMessage(actual any) string {
	return fmt.Sprintf("Expected %T to not have container with image %q, but it was found",
		actual, matcher.expectedImage)
}
