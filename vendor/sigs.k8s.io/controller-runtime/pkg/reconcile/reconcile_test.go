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

package reconcile_test

import (
	"fmt"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var _ = Describe("reconcile", func() {
	Describe("Func", func() {
		It("should call the function with the request and return a nil error.", func() {
			request := reconcile.Request{
				NamespacedName: types.NamespacedName{Name: "foo", Namespace: "bar"},
			}
			result := reconcile.Result{
				Requeue: true,
			}

			instance := reconcile.Func(func(r reconcile.Request) (reconcile.Result, error) {
				defer GinkgoRecover()
				Expect(r).To(Equal(request))

				return result, nil
			})
			actualResult, actualErr := instance.Reconcile(request)
			Expect(actualResult).To(Equal(result))
			Expect(actualErr).NotTo(HaveOccurred())
		})

		It("should call the function with the request and return an error.", func() {
			request := reconcile.Request{
				NamespacedName: types.NamespacedName{Name: "foo", Namespace: "bar"},
			}
			result := reconcile.Result{
				Requeue: false,
			}
			err := fmt.Errorf("hello world")

			instance := reconcile.Func(func(r reconcile.Request) (reconcile.Result, error) {
				defer GinkgoRecover()
				Expect(r).To(Equal(request))

				return result, err
			})
			actualResult, actualErr := instance.Reconcile(request)
			Expect(actualResult).To(Equal(result))
			Expect(actualErr).To(Equal(err))
		})
	})
})
