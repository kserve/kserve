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

package cache_test

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	kcorev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	kmetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	kscheme "k8s.io/client-go/kubernetes/scheme"
	kcache "k8s.io/client-go/tools/cache"

	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const testNamespaceOne = "test-namespace-1"
const testNamespaceTwo = "test-namespace-2"

// TODO(community): Pull these helper functions into testenv.
// Restart policy is included to allow indexing on that field.
func createPod(name, namespace string, restartPolicy kcorev1.RestartPolicy) runtime.Object {
	three := int64(3)
	pod := &kcorev1.Pod{
		ObjectMeta: kmetav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				"test-label": name,
			},
		},
		Spec: kcorev1.PodSpec{
			Containers:            []kcorev1.Container{{Name: "nginx", Image: "nginx"}},
			RestartPolicy:         restartPolicy,
			ActiveDeadlineSeconds: &three,
		},
	}
	cl, err := client.New(cfg, client.Options{})
	Expect(err).NotTo(HaveOccurred())
	err = cl.Create(context.Background(), pod)
	Expect(err).NotTo(HaveOccurred())
	return pod
}

func deletePod(pod runtime.Object) {
	cl, err := client.New(cfg, client.Options{})
	Expect(err).NotTo(HaveOccurred())
	err = cl.Delete(context.Background(), pod)
	Expect(err).NotTo(HaveOccurred())
}

var _ = Describe("Informer Cache", func() {

	var (
		informerCache cache.Cache
		stop          chan struct{}
		knownPod1     runtime.Object
		knownPod2     runtime.Object
		knownPod3     runtime.Object
	)

	BeforeEach(func() {
		stop = make(chan struct{})
		Expect(cfg).NotTo(BeNil())

		By("creating three pods")
		// Includes restart policy since these objects are indexed on this field.
		knownPod1 = createPod("test-pod-1", testNamespaceOne, kcorev1.RestartPolicyNever)
		knownPod2 = createPod("test-pod-2", testNamespaceTwo, kcorev1.RestartPolicyAlways)
		knownPod3 = createPod("test-pod-3", testNamespaceTwo, kcorev1.RestartPolicyOnFailure)
		podGVK := schema.GroupVersionKind{
			Kind:    "Pod",
			Version: "v1",
		}
		knownPod1.GetObjectKind().SetGroupVersionKind(podGVK)
		knownPod2.GetObjectKind().SetGroupVersionKind(podGVK)
		knownPod3.GetObjectKind().SetGroupVersionKind(podGVK)

		By("creating the informer cache")
		var err error
		informerCache, err = cache.New(cfg, cache.Options{})
		Expect(err).NotTo(HaveOccurred())
		By("running the cache and waiting for it to sync")
		go func() {
			defer GinkgoRecover()
			Expect(informerCache.Start(stop)).To(Succeed())
		}()
		Expect(informerCache.WaitForCacheSync(stop)).To(BeTrue())
	})

	AfterEach(func() {
		By("cleaning up created pods")
		deletePod(knownPod1)
		deletePod(knownPod2)
		deletePod(knownPod3)

		close(stop)
	})

	Describe("as a Reader", func() {
		Context("with structured objects", func() {

			It("should be able to list objects that haven't been watched previously", func() {
				By("listing all services in the cluster")
				listObj := &kcorev1.ServiceList{}
				Expect(informerCache.List(context.Background(), nil, listObj)).To(Succeed())

				By("verifying that the returned list contains the Kubernetes service")
				// NB: kubernetes default service is automatically created in testenv.
				Expect(listObj.Items).NotTo(BeEmpty())
				hasKubeService := false
				for _, svc := range listObj.Items {
					if svc.Namespace == "default" && svc.Name == "kubernetes" {
						hasKubeService = true
						break
					}
				}
				Expect(hasKubeService).To(BeTrue())
			})

			It("should be able to get objects that haven't been watched previously", func() {
				By("getting the Kubernetes service")
				svc := &kcorev1.Service{}
				svcKey := client.ObjectKey{Namespace: "default", Name: "kubernetes"}
				Expect(informerCache.Get(context.Background(), svcKey, svc)).To(Succeed())

				By("verifying that the returned service looks reasonable")
				Expect(svc.Name).To(Equal("kubernetes"))
				Expect(svc.Namespace).To(Equal("default"))
			})

			It("should support filtering by labels in a single namespace", func() {
				By("listing pods with a particular label")
				// NB: each pod has a "test-label": <pod-name>
				out := kcorev1.PodList{}
				lo := &client.ListOptions{}
				lo.InNamespace(testNamespaceTwo)
				lo.MatchingLabels(map[string]string{"test-label": "test-pod-2"})
				Expect(informerCache.List(context.Background(), lo, &out)).To(Succeed())

				By("verifying the returned pods have the correct label")
				Expect(out.Items).NotTo(BeEmpty())
				Expect(out.Items).Should(HaveLen(1))
				actual := out.Items[0]
				Expect(actual.Labels["test-label"]).To(Equal("test-pod-2"))
			})

			It("should support filtering by labels from multiple namespaces", func() {
				By("creating another pod with the same label but different namespace")
				anotherPod := createPod("test-pod-2", testNamespaceOne, kcorev1.RestartPolicyAlways)

				By("listing pods with a particular label")
				// NB: each pod has a "test-label": <pod-name>
				out := kcorev1.PodList{}
				labels := map[string]string{"test-label": "test-pod-2"}
				lo := &client.ListOptions{}
				lo.MatchingLabels(labels)
				Expect(informerCache.List(context.Background(), lo, &out)).To(Succeed())

				By("verifying multiple pods with the same label in different namespaces are returned")
				Expect(out.Items).NotTo(BeEmpty())
				Expect(out.Items).Should(HaveLen(2))
				for _, actual := range out.Items {
					Expect(actual.Labels["test-label"]).To(Equal("test-pod-2"))
				}

				deletePod(anotherPod)
			})

			It("should be able to list objects by namespace", func() {
				By("listing pods in test-namespace-1")
				listObj := &kcorev1.PodList{}
				lo := &client.ListOptions{}
				lo.InNamespace(testNamespaceOne)
				Expect(informerCache.List(context.Background(), lo, listObj)).To(Succeed())

				By("verifying that the returned pods are in test-namespace-1")
				Expect(listObj.Items).NotTo(BeEmpty())
				Expect(listObj.Items).Should(HaveLen(1))
				actual := listObj.Items[0]
				Expect(actual.Namespace).To(Equal(testNamespaceOne))
			})

			It("should be able to restrict cache to a namespace", func() {
				By("creating a namespaced cache")
				namespacedCache, err := cache.New(cfg, cache.Options{Namespace: testNamespaceOne})
				Expect(err).NotTo(HaveOccurred())

				By("running the cache and waiting for it to sync")
				go func() {
					defer GinkgoRecover()
					Expect(namespacedCache.Start(stop)).To(Succeed())
				}()
				Expect(namespacedCache.WaitForCacheSync(stop)).NotTo(BeFalse())

				By("listing pods in all namespaces")
				out := &kcorev1.PodList{}
				Expect(namespacedCache.List(context.Background(), nil, out)).To(Succeed())

				By("verifying the returned pod is from the watched namespace")
				Expect(out.Items).NotTo(BeEmpty())
				Expect(out.Items).Should(HaveLen(1))
				Expect(out.Items[0].Namespace).To(Equal(testNamespaceOne))

				By("listing all namespaces - should still be able to get a cluster-scoped resource")
				namespaceList := &kcorev1.NamespaceList{}
				Expect(namespacedCache.List(context.Background(), nil, namespaceList)).To(Succeed())

				By("verifying the namespace list is not empty")
				Expect(namespaceList.Items).NotTo(BeEmpty())
			})

			It("should deep copy the object unless told otherwise", func() {
				By("retrieving a specific pod from the cache")
				out := &kcorev1.Pod{}
				podKey := client.ObjectKey{Name: "test-pod-2", Namespace: testNamespaceTwo}
				Expect(informerCache.Get(context.Background(), podKey, out)).To(Succeed())

				By("verifying the retrieved pod is equal to a known pod")
				Expect(out).To(Equal(knownPod2))

				By("altering a field in the retrieved pod")
				*out.Spec.ActiveDeadlineSeconds = 4

				By("verifying the pods are no longer equal")
				Expect(out).NotTo(Equal(knownPod2))
			})

			It("should return an error if the object is not found", func() {
				By("getting a service that does not exists")
				svc := &kcorev1.Service{}
				svcKey := client.ObjectKey{Namespace: "unknown", Name: "unknown"}

				By("verifying that an error is returned")
				err := informerCache.Get(context.Background(), svcKey, svc)
				Expect(err).To(HaveOccurred())
				Expect(errors.IsNotFound(err)).To(BeTrue())
			})
		})
		Context("with unstructured objects", func() {
			It("should be able to list objects that haven't been watched previously", func() {
				By("listing all services in the cluster")
				listObj := &unstructured.UnstructuredList{}
				listObj.SetGroupVersionKind(schema.GroupVersionKind{
					Group:   "",
					Version: "v1",
					Kind:    "ServiceList",
				})
				err := informerCache.List(context.Background(), nil, listObj)
				Expect(err).To(Succeed())

				By("verifying that the returned list contains the Kubernetes service")
				// NB: kubernetes default service is automatically created in testenv.
				Expect(listObj.Items).NotTo(BeEmpty())
				hasKubeService := false
				for _, svc := range listObj.Items {
					if svc.GetNamespace() == "default" && svc.GetName() == "kubernetes" {
						hasKubeService = true
						break
					}
				}
				Expect(hasKubeService).To(BeTrue())
			})

			It("should be able to get objects that haven't been watched previously", func() {
				By("getting the Kubernetes service")
				svc := &unstructured.Unstructured{}
				svc.SetGroupVersionKind(schema.GroupVersionKind{
					Group:   "",
					Version: "v1",
					Kind:    "Service",
				})
				svcKey := client.ObjectKey{Namespace: "default", Name: "kubernetes"}
				Expect(informerCache.Get(context.Background(), svcKey, svc)).To(Succeed())

				By("verifying that the returned service looks reasonable")
				Expect(svc.GetName()).To(Equal("kubernetes"))
				Expect(svc.GetNamespace()).To(Equal("default"))
			})

			It("should support filtering by labels in a single namespace", func() {
				By("listing pods with a particular label")
				// NB: each pod has a "test-label": <pod-name>
				out := unstructured.UnstructuredList{}
				out.SetGroupVersionKind(schema.GroupVersionKind{
					Group:   "",
					Version: "v1",
					Kind:    "PodList",
				})
				lo := &client.ListOptions{}
				lo.InNamespace(testNamespaceTwo)
				lo.MatchingLabels(map[string]string{"test-label": "test-pod-2"})
				err := informerCache.List(context.Background(), lo, &out)
				Expect(err).To(Succeed())

				By("verifying the returned pods have the correct label")
				Expect(out.Items).NotTo(BeEmpty())
				Expect(out.Items).Should(HaveLen(1))
				actual := out.Items[0]
				Expect(actual.GetLabels()["test-label"]).To(Equal("test-pod-2"))
			})

			It("should support filtering by labels from multiple namespaces", func() {
				By("creating another pod with the same label but different namespace")
				anotherPod := createPod("test-pod-2", testNamespaceOne, kcorev1.RestartPolicyAlways)

				By("listing pods with a particular label")
				// NB: each pod has a "test-label": <pod-name>
				out := unstructured.UnstructuredList{}
				out.SetGroupVersionKind(schema.GroupVersionKind{
					Group:   "",
					Version: "v1",
					Kind:    "PodList",
				})
				labels := map[string]string{"test-label": "test-pod-2"}
				lo := &client.ListOptions{}
				lo.MatchingLabels(labels)
				err := informerCache.List(context.Background(), lo, &out)
				Expect(err).To(Succeed())

				By("verifying multiple pods with the same label in different namespaces are returned")
				Expect(out.Items).NotTo(BeEmpty())
				Expect(out.Items).Should(HaveLen(2))
				for _, actual := range out.Items {
					Expect(actual.GetLabels()["test-label"]).To(Equal("test-pod-2"))
				}

				deletePod(anotherPod)
			})

			It("should be able to list objects by namespace", func() {
				By("listing pods in test-namespace-1")
				listObj := &unstructured.UnstructuredList{}
				listObj.SetGroupVersionKind(schema.GroupVersionKind{
					Group:   "",
					Version: "v1",
					Kind:    "PodList",
				})
				lo := &client.ListOptions{}
				lo.InNamespace(testNamespaceOne)
				err := informerCache.List(context.Background(), lo, listObj)
				Expect(err).To(Succeed())

				By("verifying that the returned pods are in test-namespace-1")
				Expect(listObj.Items).NotTo(BeEmpty())
				Expect(listObj.Items).Should(HaveLen(1))
				actual := listObj.Items[0]
				Expect(actual.GetNamespace()).To(Equal(testNamespaceOne))
			})

			It("should be able to restrict cache to a namespace", func() {
				By("creating a namespaced cache")
				namespacedCache, err := cache.New(cfg, cache.Options{Namespace: testNamespaceOne})
				Expect(err).NotTo(HaveOccurred())

				By("running the cache and waiting for it to sync")
				go func() {
					defer GinkgoRecover()
					Expect(namespacedCache.Start(stop)).To(Succeed())
				}()
				Expect(namespacedCache.WaitForCacheSync(stop)).NotTo(BeFalse())

				By("listing pods in all namespaces")
				out := &unstructured.UnstructuredList{}
				out.SetGroupVersionKind(schema.GroupVersionKind{
					Group:   "",
					Version: "v1",
					Kind:    "PodList",
				})
				Expect(namespacedCache.List(context.Background(), nil, out)).To(Succeed())

				By("verifying the returned pod is from the watched namespace")
				Expect(out.Items).NotTo(BeEmpty())
				Expect(out.Items).Should(HaveLen(1))
				Expect(out.Items[0].GetNamespace()).To(Equal(testNamespaceOne))

				By("listing all namespaces - should still be able to get a cluster-scoped resource")
				namespaceList := &unstructured.UnstructuredList{}
				namespaceList.SetGroupVersionKind(schema.GroupVersionKind{
					Group:   "",
					Version: "v1",
					Kind:    "NamespaceList",
				})
				Expect(namespacedCache.List(context.Background(), nil, namespaceList)).To(Succeed())

				By("verifying the namespace list is not empty")
				Expect(namespaceList.Items).NotTo(BeEmpty())
			})

			It("should deep copy the object unless told otherwise", func() {
				By("retrieving a specific pod from the cache")
				out := &unstructured.Unstructured{}
				out.SetGroupVersionKind(schema.GroupVersionKind{
					Group:   "",
					Version: "v1",
					Kind:    "Pod",
				})
				uKnownPod2 := &unstructured.Unstructured{}
				kscheme.Scheme.Convert(knownPod2, uKnownPod2, nil)

				podKey := client.ObjectKey{Name: "test-pod-2", Namespace: testNamespaceTwo}
				Expect(informerCache.Get(context.Background(), podKey, out)).To(Succeed())

				By("verifying the retrieved pod is equal to a known pod")
				Expect(out).To(Equal(uKnownPod2))

				By("altering a field in the retrieved pod")
				m, _ := out.Object["spec"].(map[string]interface{})
				m["activeDeadlineSeconds"] = 4

				By("verifying the pods are no longer equal")
				Expect(out).NotTo(Equal(knownPod2))
			})

			It("should return an error if the object is not found", func() {
				By("getting a service that does not exists")
				svc := &unstructured.Unstructured{}
				svc.SetGroupVersionKind(schema.GroupVersionKind{
					Group:   "",
					Version: "v1",
					Kind:    "Service",
				})
				svcKey := client.ObjectKey{Namespace: "unknown", Name: "unknown"}

				By("verifying that an error is returned")
				err := informerCache.Get(context.Background(), svcKey, svc)
				Expect(err).To(HaveOccurred())
				Expect(errors.IsNotFound(err)).To(BeTrue())
			})
		})
	})

	Describe("as an Informer", func() {
		Context("with structured objects", func() {
			It("should be able to get informer for the object", func(done Done) {
				By("getting a shared index informer for a pod")
				pod := &kcorev1.Pod{
					ObjectMeta: kmetav1.ObjectMeta{
						Name:      "informer-obj",
						Namespace: "default",
					},
					Spec: kcorev1.PodSpec{
						Containers: []kcorev1.Container{
							{
								Name:  "nginx",
								Image: "nginx",
							},
						},
					},
				}
				sii, err := informerCache.GetInformer(pod)
				Expect(err).NotTo(HaveOccurred())
				Expect(sii).NotTo(BeNil())
				Expect(sii.HasSynced()).To(BeTrue())

				By("adding an event handler listening for object creation which sends the object to a channel")
				out := make(chan interface{})
				addFunc := func(obj interface{}) {
					out <- obj
				}
				sii.AddEventHandler(kcache.ResourceEventHandlerFuncs{AddFunc: addFunc})

				By("adding an object")
				cl, err := client.New(cfg, client.Options{})
				Expect(err).NotTo(HaveOccurred())
				Expect(cl.Create(context.Background(), pod)).To(Succeed())

				By("verifying the object is received on the channel")
				Eventually(out).Should(Receive(Equal(pod)))
				close(done)
			})

			// TODO: Add a test for when GVK is not in Scheme. Does code support informer for unstructured object?
			It("should be able to get an informer by group/version/kind", func(done Done) {
				By("getting an shared index informer for gvk = core/v1/pod")
				gvk := schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Pod"}
				sii, err := informerCache.GetInformerForKind(gvk)
				Expect(err).NotTo(HaveOccurred())
				Expect(sii).NotTo(BeNil())
				Expect(sii.HasSynced()).To(BeTrue())

				By("adding an event handler listening for object creation which sends the object to a channel")
				out := make(chan interface{})
				addFunc := func(obj interface{}) {
					out <- obj
				}
				sii.AddEventHandler(kcache.ResourceEventHandlerFuncs{AddFunc: addFunc})

				By("adding an object")
				cl, err := client.New(cfg, client.Options{})
				Expect(err).NotTo(HaveOccurred())
				pod := &kcorev1.Pod{
					ObjectMeta: kmetav1.ObjectMeta{
						Name:      "informer-gvk",
						Namespace: "default",
					},
					Spec: kcorev1.PodSpec{
						Containers: []kcorev1.Container{
							{
								Name:  "nginx",
								Image: "nginx",
							},
						},
					},
				}
				Expect(cl.Create(context.Background(), pod)).To(Succeed())

				By("verifying the object is received on the channel")
				Eventually(out).Should(Receive(Equal(pod)))
				close(done)
			})

			It("should be able to index an object field then retrieve objects by that field", func() {
				By("creating the cache")
				informer, err := cache.New(cfg, cache.Options{})
				Expect(err).NotTo(HaveOccurred())

				By("indexing the restartPolicy field of the Pod object before starting")
				pod := &kcorev1.Pod{}
				indexFunc := func(obj runtime.Object) []string {
					return []string{string(obj.(*kcorev1.Pod).Spec.RestartPolicy)}
				}
				Expect(informer.IndexField(pod, "spec.restartPolicy", indexFunc)).To(Succeed())

				By("running the cache and waiting for it to sync")
				go func() {
					defer GinkgoRecover()
					Expect(informer.Start(stop)).To(Succeed())
				}()
				Expect(informer.WaitForCacheSync(stop)).NotTo(BeFalse())

				By("listing Pods with restartPolicyOnFailure")
				listObj := &kcorev1.PodList{}
				lo := &client.ListOptions{}
				lo.MatchingField("spec.restartPolicy", "OnFailure")
				Expect(informer.List(context.Background(), lo, listObj)).To(Succeed())

				By("verifying that the returned pods have correct restart policy")
				Expect(listObj.Items).NotTo(BeEmpty())
				Expect(listObj.Items).Should(HaveLen(1))
				actual := listObj.Items[0]
				Expect(actual.Name).To(Equal("test-pod-3"))
			})
		})
		Context("with unstructured objects", func() {
			It("should be able to get informer for the object", func(done Done) {
				By("getting a shared index informer for a pod")

				pod := &unstructured.Unstructured{
					Object: map[string]interface{}{
						"spec": map[string]interface{}{
							"containers": []map[string]interface{}{
								map[string]interface{}{
									"name":  "nginx",
									"image": "nginx",
								},
							},
						},
					},
				}
				pod.SetName("informer-obj2")
				pod.SetNamespace("default")
				pod.SetGroupVersionKind(schema.GroupVersionKind{
					Group:   "",
					Version: "v1",
					Kind:    "Pod",
				})
				sii, err := informerCache.GetInformer(pod)
				Expect(err).NotTo(HaveOccurred())
				Expect(sii).NotTo(BeNil())
				Expect(sii.HasSynced()).To(BeTrue())

				By("adding an event handler listening for object creation which sends the object to a channel")
				out := make(chan interface{})
				addFunc := func(obj interface{}) {
					out <- obj
				}
				sii.AddEventHandler(kcache.ResourceEventHandlerFuncs{AddFunc: addFunc})

				By("adding an object")
				cl, err := client.New(cfg, client.Options{})
				Expect(err).NotTo(HaveOccurred())
				Expect(cl.Create(context.Background(), pod)).To(Succeed())

				By("verifying the object is received on the channel")
				Eventually(out).Should(Receive(Equal(pod)))
				close(done)
			}, 3)

			It("should be able to index an object field then retrieve objects by that field", func() {
				By("creating the cache")
				informer, err := cache.New(cfg, cache.Options{})
				Expect(err).NotTo(HaveOccurred())

				By("indexing the restartPolicy field of the Pod object before starting")
				pod := &unstructured.Unstructured{}
				pod.SetGroupVersionKind(schema.GroupVersionKind{
					Group:   "",
					Version: "v1",
					Kind:    "Pod",
				})
				indexFunc := func(obj runtime.Object) []string {
					s, ok := obj.(*unstructured.Unstructured).Object["spec"]
					if !ok {
						return []string{}
					}
					m, ok := s.(map[string]interface{})
					if !ok {
						return []string{}
					}
					return []string{fmt.Sprintf("%v", m["restartPolicy"])}
				}
				Expect(informer.IndexField(pod, "spec.restartPolicy", indexFunc)).To(Succeed())

				By("running the cache and waiting for it to sync")
				go func() {
					defer GinkgoRecover()
					Expect(informer.Start(stop)).To(Succeed())
				}()
				Expect(informer.WaitForCacheSync(stop)).NotTo(BeFalse())

				By("listing Pods with restartPolicyOnFailure")
				listObj := &unstructured.UnstructuredList{}
				listObj.SetGroupVersionKind(schema.GroupVersionKind{
					Group:   "",
					Version: "v1",
					Kind:    "PodList",
				})
				lo := &client.ListOptions{}
				lo.MatchingField("spec.restartPolicy", "OnFailure")
				err = informer.List(context.Background(), lo, listObj)
				Expect(err).To(Succeed())

				By("verifying that the returned pods have correct restart policy")
				Expect(listObj.Items).NotTo(BeEmpty())
				Expect(listObj.Items).Should(HaveLen(1))
				actual := listObj.Items[0]
				Expect(actual.GetName()).To(Equal("test-pod-3"))
			}, 3)
		})
	})
})
