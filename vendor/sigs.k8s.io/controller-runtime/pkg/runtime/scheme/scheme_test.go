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

package scheme_test

import (
	"reflect"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/runtime/scheme"
)

var _ = Describe("Scheme", func() {
	Describe("Builder", func() {
		It("should provide a Scheme with the types registered", func() {
			gv := schema.GroupVersion{Group: "core", Version: "v1"}

			s, err := (&scheme.Builder{GroupVersion: gv}).
				Register(&corev1.Pod{}, &corev1.PodList{}).
				Build()
			Expect(err).NotTo(HaveOccurred())

			Expect(s.AllKnownTypes()).To(HaveLen(15))
			Expect(s.AllKnownTypes()[gv.WithKind("Pod")]).To(Equal(reflect.TypeOf(corev1.Pod{})))
			Expect(s.AllKnownTypes()[gv.WithKind("PodList")]).To(Equal(reflect.TypeOf(corev1.PodList{})))

			// Base types
			Expect(s.AllKnownTypes()).To(HaveKey(gv.WithKind("DeleteOptions")))
			Expect(s.AllKnownTypes()).To(HaveKey(gv.WithKind("ExportOptions")))
			Expect(s.AllKnownTypes()).To(HaveKey(gv.WithKind("GetOptions")))
			Expect(s.AllKnownTypes()).To(HaveKey(gv.WithKind("ListOptions")))
			Expect(s.AllKnownTypes()).To(HaveKey(gv.WithKind("WatchEvent")))

			internalGv := schema.GroupVersion{Group: "core", Version: "__internal"}
			Expect(s.AllKnownTypes()).To(HaveKey(internalGv.WithKind("WatchEvent")))

			emptyGv := schema.GroupVersion{Group: "", Version: "v1"}
			Expect(s.AllKnownTypes()).To(HaveKey(emptyGv.WithKind("APIGroup")))
			Expect(s.AllKnownTypes()).To(HaveKey(emptyGv.WithKind("APIGroupList")))
			Expect(s.AllKnownTypes()).To(HaveKey(emptyGv.WithKind("APIResourceList")))
			Expect(s.AllKnownTypes()).To(HaveKey(emptyGv.WithKind("APIVersions")))
			Expect(s.AllKnownTypes()).To(HaveKey(emptyGv.WithKind("Status")))
		})

		It("should be able to add types from other Builders", func() {
			gv1 := schema.GroupVersion{Group: "core", Version: "v1"}
			b1 := (&scheme.Builder{GroupVersion: gv1}).Register(&corev1.Pod{}, &corev1.PodList{})

			gv2 := schema.GroupVersion{Group: "apps", Version: "v1"}
			s, err := (&scheme.Builder{GroupVersion: gv2}).
				Register(&appsv1.Deployment{}).
				Register(&appsv1.DeploymentList{}).
				RegisterAll(b1).
				Build()

			Expect(err).NotTo(HaveOccurred())
			Expect(s.AllKnownTypes()).To(HaveLen(25))

			// Types from b1
			Expect(s.AllKnownTypes()[gv1.WithKind("Pod")]).To(Equal(reflect.TypeOf(corev1.Pod{})))
			Expect(s.AllKnownTypes()[gv1.WithKind("PodList")]).To(Equal(reflect.TypeOf(corev1.PodList{})))

			// Types from b2
			Expect(s.AllKnownTypes()[gv2.WithKind("Deployment")]).To(Equal(reflect.TypeOf(appsv1.Deployment{})))
			Expect(s.AllKnownTypes()[gv2.WithKind("Deployment")]).To(Equal(reflect.TypeOf(appsv1.Deployment{})))

			// Base types
			Expect(s.AllKnownTypes()).To(HaveKey(gv1.WithKind("DeleteOptions")))
			Expect(s.AllKnownTypes()).To(HaveKey(gv1.WithKind("ExportOptions")))
			Expect(s.AllKnownTypes()).To(HaveKey(gv1.WithKind("GetOptions")))
			Expect(s.AllKnownTypes()).To(HaveKey(gv1.WithKind("ListOptions")))
			Expect(s.AllKnownTypes()).To(HaveKey(gv1.WithKind("WatchEvent")))

			internalGv1 := schema.GroupVersion{Group: "core", Version: "__internal"}
			Expect(s.AllKnownTypes()).To(HaveKey(internalGv1.WithKind("WatchEvent")))

			Expect(s.AllKnownTypes()).To(HaveKey(gv2.WithKind("DeleteOptions")))
			Expect(s.AllKnownTypes()).To(HaveKey(gv2.WithKind("ExportOptions")))
			Expect(s.AllKnownTypes()).To(HaveKey(gv2.WithKind("GetOptions")))
			Expect(s.AllKnownTypes()).To(HaveKey(gv2.WithKind("ListOptions")))
			Expect(s.AllKnownTypes()).To(HaveKey(gv2.WithKind("WatchEvent")))

			internalGv2 := schema.GroupVersion{Group: "apps", Version: "__internal"}
			Expect(s.AllKnownTypes()).To(HaveKey(internalGv2.WithKind("WatchEvent")))

			emptyGv := schema.GroupVersion{Group: "", Version: "v1"}
			Expect(s.AllKnownTypes()).To(HaveKey(emptyGv.WithKind("APIGroup")))
			Expect(s.AllKnownTypes()).To(HaveKey(emptyGv.WithKind("APIGroupList")))
			Expect(s.AllKnownTypes()).To(HaveKey(emptyGv.WithKind("APIResourceList")))
			Expect(s.AllKnownTypes()).To(HaveKey(emptyGv.WithKind("APIVersions")))
			Expect(s.AllKnownTypes()).To(HaveKey(emptyGv.WithKind("Status")))
		})
	})
})
