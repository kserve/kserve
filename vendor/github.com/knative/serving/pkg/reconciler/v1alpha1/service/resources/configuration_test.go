/*
Copyright 2018 The Knative Authors

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

package resources

import (
	"testing"

	"github.com/knative/serving/pkg/apis/serving"
)

func TestRunLatest(t *testing.T) {
	s := createServiceWithRunLatest()
	c, _ := MakeConfiguration(s)
	if got, want := c.Name, testServiceName; got != want {
		t.Errorf("expected %q for service name got %q", want, got)
	}
	if got, want := c.Namespace, testServiceNamespace; got != want {
		t.Errorf("expected %q for service namespace got %q", want, got)
	}
	if got, want := c.Spec.GetTemplate().Spec.GetContainer().Name, testContainerNameRunLatest; got != want {
		t.Errorf("expected %q for container name got %q", want, got)
	}
	expectOwnerReferencesSetCorrectly(t, c.OwnerReferences)

	if got, want := len(c.Labels), 2; got != want {
		t.Errorf("expected %d labels got %d", want, got)
	}
	if got, want := c.Labels[testLabelKey], testLabelValueRunLatest; got != want {
		t.Errorf("expected %q labels got %q", want, got)
	}
	if got, want := c.Labels[serving.ServiceLabelKey], testServiceName; got != want {
		t.Errorf("expected %q labels got %q", want, got)
	}
}

func TestPinned(t *testing.T) {
	s := createServiceWithPinned()
	c, _ := MakeConfiguration(s)
	if got, want := c.Name, testServiceName; got != want {
		t.Errorf("expected %q for service name got %q", want, got)
	}
	if got, want := c.Namespace, testServiceNamespace; got != want {
		t.Errorf("expected %q for service namespace got %q", want, got)
	}
	if got, want := c.Spec.GetTemplate().Spec.GetContainer().Name, testContainerNamePinned; got != want {
		t.Errorf("expected %q for container name got %q", want, got)
	}
	expectOwnerReferencesSetCorrectly(t, c.OwnerReferences)

	if got, want := len(c.Labels), 2; got != want {
		t.Errorf("expected %d labels got %d", want, got)
	}
	if got, want := c.Labels[testLabelKey], testLabelValuePinned; got != want {
		t.Errorf("expected %q labels got %q", want, got)
	}
	if got, want := c.Labels[serving.ServiceLabelKey], testServiceName; got != want {
		t.Errorf("expected %q labels got %q", want, got)
	}
}

func TestRelease(t *testing.T) {
	s := createServiceWithRelease(1, 0)
	c, _ := MakeConfiguration(s)
	if got, want := c.Name, testServiceName; got != want {
		t.Errorf("expected %q for service name got %q", want, got)
	}
	if got, want := c.Namespace, testServiceNamespace; got != want {
		t.Errorf("expected %q for service namespace got %q", want, got)
	}
	if got, want := c.Spec.GetTemplate().Spec.GetContainer().Name, testContainerNameRelease; got != want {
		t.Errorf("expected %q for container name got %q", want, got)
	}
	expectOwnerReferencesSetCorrectly(t, c.OwnerReferences)

	if got, want := len(c.Labels), 2; got != want {
		t.Errorf("expected %d labels got %d", want, got)
	}
	if got, want := c.Labels[testLabelKey], testLabelValueRelease; got != want {
		t.Errorf("expected %q labels got %q", want, got)
	}
	if got, want := c.Labels[serving.ServiceLabelKey], testServiceName; got != want {
		t.Errorf("expected %q labels got %q", want, got)
	}
}

func TestManual(t *testing.T) {
	s := createServiceWithManual()
	c, err := MakeConfiguration(s)
	if err == nil {
		t.Errorf("MakeConfiguration(%v) = %v, wanted error", s, c)
	}
}

func TestMalformed(t *testing.T) {
	s := createServiceMeta()
	c, err := MakeConfiguration(s)
	if err == nil {
		t.Errorf("MakeConfiguration() = %v, wanted error", c)
	}
}
