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

package reconciler

import (
	"testing"
	"time"

	fakesharedclientset "github.com/knative/pkg/client/clientset/versioned/fake"
	logtesting "github.com/knative/pkg/logging/testing"
	_ "github.com/knative/pkg/system/testing"
	fakekubeclientset "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"

	fakeclientset "github.com/knative/serving/pkg/client/clientset/versioned/fake"
)

var reconcilerName = "test-reconciler"

func TestNewOptions(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("Didn't expect to Die! %v", r)
		}
	}()
	stopCh := make(chan struct{})
	defer close(stopCh)

	cfg := &rest.Config{}
	NewOptionsOrDie(cfg, nil, stopCh)

	resetPeriod = 100 * time.Millisecond
	time.Sleep(300 * time.Millisecond)

	// TODO(mattmoor): There is no definitive way to know if the restmapper is reset...
}

func TestNew(t *testing.T) {
	defer logtesting.ClearAll()
	kubeClient := fakekubeclientset.NewSimpleClientset()
	sharedClient := fakesharedclientset.NewSimpleClientset()
	servingClient := fakeclientset.NewSimpleClientset()
	sc := make(chan struct{})
	defer close(sc)

	r := NewBase(Options{
		KubeClientSet:    kubeClient,
		SharedClientSet:  sharedClient,
		ServingClientSet: servingClient,
		Logger:           logtesting.TestLogger(t),
		StopChannel:      sc,
	}, reconcilerName)

	if r == nil {
		t.Fatal("Expected NewBase to return a non-nil value")
	}
	if r.Recorder == nil {
		t.Fatal("Expected NewBase to add a Recorder")
	}
	if r.StatsReporter == nil {
		t.Fatal("Expected NewBase to add a StatsReporter")
	}
}
