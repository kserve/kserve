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

// namespace.go provides methods to create and delete namespaces.

package test

import (
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// CreateNamespace creates and returns the namespace that was successfully created otherwise error
func CreateNamespace(clients *Clients, name string) error {
	namespaceSpec := &v1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}

	_, err := clients.KubeClient.Kube.CoreV1().Namespaces().Create(namespaceSpec)
	return err
}

// DeleteNamespace deletes the namespace specified otherwise error
func DeleteNamespace(clients *Clients, name string) error {
	return clients.KubeClient.Kube.CoreV1().Namespaces().Delete(name, &metav1.DeleteOptions{})
}
