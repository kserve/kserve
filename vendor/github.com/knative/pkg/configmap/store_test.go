/*
Copyright 2018 The Knative Authors.

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

package configmap

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	. "github.com/knative/pkg/logging/testing"
)

func TestStoreBadConstructors(t *testing.T) {
	tests := []struct {
		name        string
		constructor interface{}
	}{{
		name:        "not a function",
		constructor: "i'm pretending to be a function",
	}, {
		name:        "no function arguments",
		constructor: func() (bool, error) { return true, nil },
	}, {
		name:        "single argument is not a configmap",
		constructor: func(bool) (bool, error) { return true, nil },
	}, {
		name:        "single return",
		constructor: func(*corev1.ConfigMap) error { return nil },
	}, {
		name:        "wrong second return",
		constructor: func(*corev1.ConfigMap) (bool, bool) { return true, true },
	}}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r == nil {
					t.Error("expected NewUntypedStore to panic")
				}
			}()

			NewUntypedStore("store", nil, Constructors{
				"test": test.constructor,
			})
		})
	}
}

func TestStoreWatchConfigs(t *testing.T) {
	constructor := func(c *corev1.ConfigMap) (interface{}, error) {
		return c.Name, nil
	}

	store := NewUntypedStore(
		"name",
		TestLogger(t),
		Constructors{
			"config-name-1": constructor,
			"config-name-2": constructor,
		},
	)

	watcher := &mockWatcher{}
	store.WatchConfigs(watcher)

	want := []string{
		"config-name-1",
		"config-name-2",
	}

	got := watcher.watches

	if diff := cmp.Diff(want, got, sortStrings); diff != "" {
		t.Errorf("Unexpected configmap watches (-want, +got): %v", diff)
	}
}

func appendTo(evidence *[]string) func(int) func(string, interface{}) {
	return func(i int) func(string, interface{}) {
		return func(name string, value interface{}) {
			*evidence = append(*evidence, fmt.Sprintf("%s:%s:%v", name, value, i))
		}
	}
}

func listOf(f func(int) func(string, interface{}), count int, tail ...func(string, interface{})) []func(string, interface{}) {
	var result []func(string, interface{})
	for i := 0; i < count; i++ {
		result = append(result, f(i))
	}
	for _, f := range tail {
		result = append(result, f)
	}
	return result
}

func expectedEvidence(name string, count int) []string {
	var result []string
	for i := 0; i < count; i++ {
		result = append(result, fmt.Sprintf("%s:%s:%v", name, name, i))
	}
	return result
}

func signalComplete(done chan<- string) func(string, interface{}) {
	return func(name string, value interface{}) {
		done <- fmt.Sprintf("%s:%s", name, value)
	}
}

func TestOnAfterStore(t *testing.T) {
	constructor := func(c *corev1.ConfigMap) (interface{}, error) {
		return c.Name, nil
	}

	var evidence []string
	completeCh := make(chan string)

	store := NewUntypedStore(
		"name",
		TestLogger(t),
		Constructors{
			"config-name-1": constructor,
			"config-name-2": constructor,
		},
		listOf(appendTo(&evidence), 100, signalComplete(completeCh))...,
	)

	store.OnConfigChanged(&corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name: "config-name-1",
		},
	})

	if diff := cmp.Diff("config-name-1:config-name-1", <-completeCh); diff != "" {
		t.Fatalf("Expected value from completeCh diff: %s", diff)
	}

	if diff := cmp.Diff(expectedEvidence("config-name-1", 100), evidence); diff != "" {
		t.Fatalf("Expected evidence diff: %s", diff)
	}

	evidence = nil

	store.OnConfigChanged(&corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name: "config-name-2",
		},
	})

	if diff := cmp.Diff("config-name-2:config-name-2", <-completeCh); diff != "" {
		t.Fatalf("Expected value from completeCh diff: %s", diff)
	}

	if diff := cmp.Diff(expectedEvidence("config-name-2", 100), evidence); diff != "" {
		t.Fatalf("Expected evidence diff: %s", diff)
	}
}

func TestStoreConfigChange(t *testing.T) {
	constructor := func(c *corev1.ConfigMap) (interface{}, error) {
		return c.Name, nil
	}

	store := NewUntypedStore(
		"name",
		TestLogger(t),
		Constructors{
			"config-name-1": constructor,
			"config-name-2": constructor,
		},
	)

	store.OnConfigChanged(&corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name: "config-name-1",
		},
	})

	result := store.UntypedLoad("config-name-1")

	if diff := cmp.Diff(result, "config-name-1"); diff != "" {
		t.Errorf("Expected loaded value diff: %s", diff)
	}

	result = store.UntypedLoad("config-name-2")

	if diff := cmp.Diff(result, nil); diff != "" {
		t.Errorf("Unexpected loaded value diff: %s", diff)
	}

	store.OnConfigChanged(&corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name: "config-name-2",
		},
	})

	result = store.UntypedLoad("config-name-2")

	if diff := cmp.Diff(result, "config-name-2"); diff != "" {
		t.Errorf("Expected loaded value diff: %s", diff)
	}
}

func TestStoreFailedFirstConversionCrashes(t *testing.T) {
	if os.Getenv("CRASH") == "1" {
		constructor := func(c *corev1.ConfigMap) (interface{}, error) {
			return nil, errors.New("failure")
		}

		store := NewUntypedStore("name", TestLogger(t),
			Constructors{"config-name-1": constructor},
		)

		store.OnConfigChanged(&corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name: "config-name-1",
			},
		})
		return
	}

	cmd := exec.Command(os.Args[0], fmt.Sprintf("-test.run=%s", t.Name()))
	cmd.Env = append(os.Environ(), "CRASH=1")
	err := cmd.Run()
	if e, ok := err.(*exec.ExitError); ok && !e.Success() {
		return
	}
	t.Fatalf("process should have exited with status 1 - err %v", err)
}

func TestStoreFailedUpdate(t *testing.T) {
	induceConstructorFailure := false

	constructor := func(c *corev1.ConfigMap) (interface{}, error) {
		if induceConstructorFailure {
			return nil, errors.New("failure")
		}

		return time.Now().String(), nil
	}

	store := NewUntypedStore("name", TestLogger(t),
		Constructors{"config-name-1": constructor},
	)

	store.OnConfigChanged(&corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name: "config-name-1",
		},
	})

	firstLoad := store.UntypedLoad("config-name-1")

	induceConstructorFailure = true
	store.OnConfigChanged(&corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name: "config-name-1",
		},
	})

	secondLoad := store.UntypedLoad("config-name-1")

	if diff := cmp.Diff(firstLoad, secondLoad); diff != "" {
		t.Errorf("Expected loaded value to remain the same dff: %s", diff)
	}
}

type mockWatcher struct {
	watches []string
}

func (w *mockWatcher) Watch(config string, o Observer) {
	w.watches = append(w.watches, config)
}

func (*mockWatcher) Start(<-chan struct{}) error { return nil }

var _ Watcher = (*mockWatcher)(nil)

var sortStrings = cmpopts.SortSlices(func(x, y string) bool {
	return x < y
})
