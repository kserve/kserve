/*
Copyright 2023 The KServe Authors.

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
	"context"
	"strings"
	"time"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	k8serr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Cleaner is a struct to perform deletion of resources,
// enforcing removal of finalizers. Otherwise, deletion of namespaces wouldn't be possible.
// See: https://book.kubebuilder.io/reference/envtest.html#namespace-usage-limitation
// Based on https://github.com/kubernetes-sigs/controller-runtime/issues/880#issuecomment-749742403
type Cleaner struct {
	clientset         *kubernetes.Clientset
	client            client.Client
	timeout, interval time.Duration
	namespacedGVKs    map[string]schema.GroupVersionKind
}

func CreateCleaner(k8sClient client.Client, config *rest.Config, timeout, interval time.Duration) *Cleaner {
	k8sClientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		panic(err)
	}

	namespacedGVKs := lookupNamespacedResources(k8sClientset)

	return &Cleaner{
		clientset:      k8sClientset,
		client:         k8sClient,
		namespacedGVKs: namespacedGVKs,
		timeout:        timeout,
		interval:       interval,
	}
}

func (c *Cleaner) DeleteAll(objects ...client.Object) { //nolint:gocognit //reason it is what is ;)
	for _, o := range objects {
		obj := o
		Expect(client.IgnoreNotFound(c.client.Delete(context.Background(), obj))).Should(Succeed())

		if namespace, ok := obj.(*corev1.Namespace); ok {
			// Normally the kube-controller-manager would handle finalization
			// and garbage collection of namespaces, but with envtest, we aren't
			// running a kube-controller-manager. Instead we're gonna approximate
			// (poorly) the kube-controller-manager by explicitly deleting some
			// resources within the namespace and then removing the `kubernetes`
			// finalizer from the namespace resource so it can finish deleting.
			// Note that any resources within the namespace that we don't
			// successfully delete could reappear if the namespace is ever
			// recreated with the same name.
			// Delete all namespaced resources in this namespace
			for _, gvk := range c.namespacedGVKs {
				u := unstructured.Unstructured{}
				u.SetGroupVersionKind(gvk)

				deleteErr := c.client.DeleteAllOf(context.Background(), &u, client.InNamespace(namespace.Name))
				Expect(client.IgnoreNotFound(ignoreMethodNotAllowed(deleteErr))).ShouldNot(HaveOccurred())
			}

			Eventually(func() error {
				key := client.ObjectKeyFromObject(namespace)

				if getErr := c.client.Get(context.Background(), key, namespace); getErr != nil {
					return client.IgnoreNotFound(getErr)
				}
				// remove `kubernetes` finalizer
				const k8s = "kubernetes"
				finalizers := []corev1.FinalizerName{}
				for _, f := range namespace.Spec.Finalizers {
					if f != k8s {
						finalizers = append(finalizers, f)
					}
				}
				namespace.Spec.Finalizers = finalizers

				// We have to use the k8s.io/client-go library here to expose
				// ability to patch the /finalize subresource on the namespace
				_, err := c.clientset.CoreV1().Namespaces().Finalize(context.Background(), namespace, metav1.UpdateOptions{})

				return err
			}, c.timeout, c.interval).Should(Succeed())
		}

		Eventually(func() metav1.StatusReason {
			key := client.ObjectKeyFromObject(obj)
			if err := c.client.Get(context.Background(), key, obj); err != nil {
				return k8serr.ReasonForError(err)
			}

			return ""
		}, c.timeout, c.interval).Should(Equal(metav1.StatusReasonNotFound))
	}
}

func lookupNamespacedResources(clientset *kubernetes.Clientset) map[string]schema.GroupVersionKind {
	namespacedGVKs := make(map[string]schema.GroupVersionKind)

	// Look up all namespaced resources under the discovery API
	_, apiResources, listErr := clientset.DiscoveryClient.ServerGroupsAndResources()
	if listErr != nil {
		panic(listErr)
	}

	for _, apiResourceList := range apiResources {
		resources := apiResourceList

		defaultGV, parseErr := schema.ParseGroupVersion(resources.GroupVersion)
		Expect(parseErr).ShouldNot(HaveOccurred())

		for i := range resources.APIResources {
			resource := resources.APIResources[i]
			if !resource.Namespaced || strings.Contains(resource.Name, "/") {
				// skip non-namespaced and sub-resources
				continue
			}

			gvk := schema.GroupVersionKind{
				Group:   defaultGV.Group,
				Version: defaultGV.Version,
				Kind:    resource.Kind,
			}

			if resource.Group != "" {
				gvk.Group = resource.Group
			}

			if resource.Version != "" {
				gvk.Version = resource.Version
			}

			namespacedGVKs[gvk.String()] = gvk
		}
	}

	return namespacedGVKs
}

func ignoreMethodNotAllowed(err error) error {
	if k8serr.ReasonForError(err) == metav1.StatusReasonMethodNotAllowed {
		return nil
	}

	return err
}
