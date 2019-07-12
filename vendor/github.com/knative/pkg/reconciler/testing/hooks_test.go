/*
Copyright 2019 The Knative Authors

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
	"log"
	"testing"
	"time"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
)

func ExampleHooks() {
	h := NewHooks()
	f := fake.NewSimpleClientset()

	h.OnCreate(&f.Fake, "pods", func(obj runtime.Object) HookResult {
		pod := obj.(*v1.Pod)
		fmt.Printf("Pod %s has restart policy %v\n", pod.Name, pod.Spec.RestartPolicy)
		return true
	})

	h.OnUpdate(&f.Fake, "pods", func(obj runtime.Object) HookResult {
		pod := obj.(*v1.Pod)
		fmt.Printf("Pod %s restart policy was updated to %v\n", pod.Name, pod.Spec.RestartPolicy)
		return true
	})

	h.OnDelete(&f.Fake, "pods", func(name string) HookResult {
		fmt.Printf("Pod %s was deleted\n", name)
		return true
	})

	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-pod",
		},
		Spec: v1.PodSpec{
			RestartPolicy: v1.RestartPolicyAlways,
		},
	}
	f.CoreV1().Pods("test").Create(pod)

	updatedPod := pod.DeepCopy()
	updatedPod.Spec.RestartPolicy = v1.RestartPolicyNever
	f.CoreV1().Pods("test").Update(updatedPod)

	f.CoreV1().Pods("test").Delete(pod.Name, &metav1.DeleteOptions{})
	if err := h.WaitForHooks(time.Second); err != nil {
		log.Fatal(err)
	}

	// Output:
	// Pod test-pod has restart policy Always
	// Pod test-pod restart policy was updated to Never
	// Pod test-pod was deleted
}

func TestWaitWithoutHooks(t *testing.T) {
	h := NewHooks()
	if err := h.WaitForHooks(time.Second); err != nil {
		t.Errorf("Expected no error without hooks, but got: %v", err)
	}
}

func TestWaitTimeout(t *testing.T) {
	h := NewHooks()
	f := fake.NewSimpleClientset()

	h.OnCreate(&f.Fake, "pods", func(obj runtime.Object) HookResult {
		return true
	})

	err := h.WaitForHooks(time.Millisecond)
	if err == nil {
		t.Error("expected uncalled hook to cause a timeout error")
	}
}

func TestWaitPartialCompletion(t *testing.T) {
	h := NewHooks()
	f := fake.NewSimpleClientset()

	createCalled := false
	h.OnCreate(&f.Fake, "pods", func(obj runtime.Object) HookResult {
		createCalled = true
		return true
	})

	h.OnUpdate(&f.Fake, "pods", func(obj runtime.Object) HookResult {
		return true
	})

	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-pod",
		},
		Spec: v1.PodSpec{
			RestartPolicy: v1.RestartPolicyAlways,
		},
	}
	f.CoreV1().Pods("test").Create(pod)

	err := h.WaitForHooks(time.Millisecond)
	if err == nil {
		t.Error("expected uncalled hook to cause a timeout error")
	}
	if createCalled == false {
		t.Error("expected create hook to be called")
	}
}

func TestMultiUpdate(t *testing.T) {
	h := NewHooks()
	f := fake.NewSimpleClientset()

	updates := 0
	h.OnUpdate(&f.Fake, "pods", func(obj runtime.Object) HookResult {
		updates = updates + 1
		switch updates {
		case 1:
		case 2:
			return HookComplete
		}
		return HookIncomplete
	})

	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-pod",
		},
		Spec: v1.PodSpec{
			RestartPolicy: v1.RestartPolicyAlways,
		},
	}
	f.CoreV1().Pods("test").Create(pod)

	updatedPod := pod.DeepCopy()
	updatedPod.Spec.RestartPolicy = v1.RestartPolicyNever
	f.CoreV1().Pods("test").Update(updatedPod)

	updatedPod = pod.DeepCopy()
	updatedPod.Spec.RestartPolicy = v1.RestartPolicyAlways
	f.CoreV1().Pods("test").Update(updatedPod)

	f.CoreV1().Pods("test").Delete(pod.Name, &metav1.DeleteOptions{})
	if err := h.WaitForHooks(time.Second); err != nil {
		t.Error(err)
	}

	if updates != 2 {
		t.Errorf("Unexpected number of Update events; want 2, got %d", updates)
	}
}

func TestWaitNoExecutionAfterWait(t *testing.T) {
	h := NewHooks()
	f := fake.NewSimpleClientset()

	createCalled := false
	h.OnCreate(&f.Fake, "pods", func(obj runtime.Object) HookResult {
		createCalled = true
		return true
	})

	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-pod",
		},
		Spec: v1.PodSpec{
			RestartPolicy: v1.RestartPolicyAlways,
		},
	}
	err := h.WaitForHooks(time.Millisecond)

	f.CoreV1().Pods("test").Create(pod)

	if err == nil {
		t.Error("expected uncalled hook to cause a timeout error")
	}
	if createCalled == true {
		t.Error("expected create hook not to be called")
	}
}
