/*
Copyright 2018 The Kubernetes Authors.

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

package client_test

import (
	"context"
	"fmt"
	"os"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
)

var (
	c client.Client
)

func ExampleNew() {
	cl, err := client.New(config.GetConfigOrDie(), client.Options{})
	if err != nil {
		fmt.Println("failed to create client")
		os.Exit(1)
	}

	podList := &corev1.PodList{}

	err = cl.List(context.Background(), client.InNamespace("default"), podList)
	if err != nil {
		fmt.Printf("failed to list pods in namespace default: %v\n", err)
		os.Exit(1)
	}
}

// This example shows how to use the client with typed and unstructured objects to retrieve a objects.
func ExampleClient_get() {
	// Using a typed object.
	pod := &corev1.Pod{}
	// c is a created client.
	_ = c.Get(context.Background(), client.ObjectKey{
		Namespace: "namespace",
		Name:      "name",
	}, pod)

	// Using a unstructured object.
	u := &unstructured.Unstructured{}
	u.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "apps",
		Kind:    "Deployment",
		Version: "v1",
	})
	_ = c.Get(context.Background(), client.ObjectKey{
		Namespace: "namespace",
		Name:      "name",
	}, u)
}

// This example shows how to use the client with typed and unstrucurted objects to create objects.
func ExampleClient_create() {
	// Using a typed object.
	pod := &corev1.Pod{
		ObjectMeta: v1.ObjectMeta{
			Namespace: "namespace",
			Name:      "name",
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				corev1.Container{
					Image: "nginx",
					Name:  "nginx",
				},
			},
		},
	}
	// c is a created client.
	_ = c.Create(context.Background(), pod)

	// Using a unstructured object.
	u := &unstructured.Unstructured{}
	u.Object = map[string]interface{}{
		"name":      "name",
		"namespace": "namespace",
		"spec": map[string]interface{}{
			"replicas": 2,
			"selector": map[string]interface{}{
				"matchLabels": map[string]interface{}{
					"foo": "bar",
				},
			},
			"template": map[string]interface{}{
				"labels": map[string]interface{}{
					"foo": "bar",
				},
				"spec": map[string]interface{}{
					"containers": []map[string]interface{}{
						{
							"name":  "nginx",
							"image": "nginx",
						},
					},
				},
			},
		},
	}
	u.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "apps",
		Kind:    "Deployment",
		Version: "v1",
	})
	_ = c.Create(context.Background(), u)
}

// This example shows how to use the client with typed and unstrucurted objects to list objects.
func ExampleClient_list() {
	// Using a typed object.
	pod := &corev1.PodList{}
	// c is a created client.
	_ = c.List(context.Background(), nil, pod)

	// Using a unstructured object.
	u := &unstructured.UnstructuredList{}
	u.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "apps",
		Kind:    "DeploymentList",
		Version: "v1",
	})
	_ = c.List(context.Background(), nil, u)
}

// This example shows how to use the client with typed and unstrucurted objects to update objects.
func ExampleClient_update() {
	// Using a typed object.
	pod := &corev1.Pod{}
	// c is a created client.
	_ = c.Get(context.Background(), client.ObjectKey{
		Namespace: "namespace",
		Name:      "name",
	}, pod)
	pod.SetFinalizers(append(pod.GetFinalizers(), "new-finalizer"))
	_ = c.Update(context.Background(), pod)

	// Using a unstructured object.
	u := &unstructured.Unstructured{}
	u.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "apps",
		Kind:    "Deployment",
		Version: "v1",
	})
	_ = c.Get(context.Background(), client.ObjectKey{
		Namespace: "namespace",
		Name:      "name",
	}, u)
	u.SetFinalizers(append(u.GetFinalizers(), "new-finalizer"))
	_ = c.Update(context.Background(), u)
}

// This example shows how to use the client with typed and unstrucurted objects to delete objects.
func ExampleClient_delete() {
	// Using a typed object.
	pod := &corev1.Pod{
		ObjectMeta: v1.ObjectMeta{
			Namespace: "namespace",
			Name:      "name",
		},
	}
	// c is a created client.
	_ = c.Delete(context.Background(), pod)

	// Using a unstructured object.
	u := &unstructured.Unstructured{}
	u.Object = map[string]interface{}{
		"name":      "name",
		"namespace": "namespace",
	}
	u.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "apps",
		Kind:    "Deployment",
		Version: "v1",
	})
	_ = c.Delete(context.Background(), u)
}
