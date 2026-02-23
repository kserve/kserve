/*
Copyright 2025 The KServe Authors.

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

package localmodel

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

var _ = Describe("Node Ready Predicate", func() {
	var pred predicate.Funcs

	BeforeEach(func() {
		pred = nodeReadyPredicate()
	})

	Describe("UpdateFunc", func() {
		Context("when a node transitions from NotReady to Ready", func() {
			It("should return true", func() {
				oldNode := &corev1.Node{
					ObjectMeta: metav1.ObjectMeta{Name: "test-node"},
					Status: corev1.NodeStatus{
						Conditions: []corev1.NodeCondition{
							{
								Type:   corev1.NodeReady,
								Status: corev1.ConditionFalse,
							},
						},
					},
				}
				newNode := &corev1.Node{
					ObjectMeta: metav1.ObjectMeta{Name: "test-node"},
					Status: corev1.NodeStatus{
						Conditions: []corev1.NodeCondition{
							{
								Type:   corev1.NodeReady,
								Status: corev1.ConditionTrue,
							},
						},
					},
				}

				result := pred.Update(event.UpdateEvent{
					ObjectOld: oldNode,
					ObjectNew: newNode,
				})
				Expect(result).To(BeTrue())
			})
		})

		Context("when a node transitions from no conditions (new node) to Ready", func() {
			It("should return true", func() {
				oldNode := &corev1.Node{
					ObjectMeta: metav1.ObjectMeta{Name: "test-node"},
					Status:     corev1.NodeStatus{},
				}
				newNode := &corev1.Node{
					ObjectMeta: metav1.ObjectMeta{Name: "test-node"},
					Status: corev1.NodeStatus{
						Conditions: []corev1.NodeCondition{
							{
								Type:   corev1.NodeReady,
								Status: corev1.ConditionTrue,
							},
						},
					},
				}

				result := pred.Update(event.UpdateEvent{
					ObjectOld: oldNode,
					ObjectNew: newNode,
				})
				Expect(result).To(BeTrue())
			})
		})

		Context("when a node stays Ready", func() {
			It("should return false", func() {
				oldNode := &corev1.Node{
					ObjectMeta: metav1.ObjectMeta{Name: "test-node"},
					Status: corev1.NodeStatus{
						Conditions: []corev1.NodeCondition{
							{
								Type:   corev1.NodeReady,
								Status: corev1.ConditionTrue,
							},
						},
					},
				}
				newNode := &corev1.Node{
					ObjectMeta: metav1.ObjectMeta{Name: "test-node"},
					Status: corev1.NodeStatus{
						Conditions: []corev1.NodeCondition{
							{
								Type:   corev1.NodeReady,
								Status: corev1.ConditionTrue,
							},
						},
					},
				}

				result := pred.Update(event.UpdateEvent{
					ObjectOld: oldNode,
					ObjectNew: newNode,
				})
				Expect(result).To(BeFalse())
			})
		})

		Context("when a node transitions from Ready to NotReady", func() {
			It("should return false", func() {
				oldNode := &corev1.Node{
					ObjectMeta: metav1.ObjectMeta{Name: "test-node"},
					Status: corev1.NodeStatus{
						Conditions: []corev1.NodeCondition{
							{
								Type:   corev1.NodeReady,
								Status: corev1.ConditionTrue,
							},
						},
					},
				}
				newNode := &corev1.Node{
					ObjectMeta: metav1.ObjectMeta{Name: "test-node"},
					Status: corev1.NodeStatus{
						Conditions: []corev1.NodeCondition{
							{
								Type:   corev1.NodeReady,
								Status: corev1.ConditionFalse,
							},
						},
					},
				}

				result := pred.Update(event.UpdateEvent{
					ObjectOld: oldNode,
					ObjectNew: newNode,
				})
				Expect(result).To(BeFalse())
			})
		})

		Context("when a node stays NotReady", func() {
			It("should return false", func() {
				oldNode := &corev1.Node{
					ObjectMeta: metav1.ObjectMeta{Name: "test-node"},
					Status: corev1.NodeStatus{
						Conditions: []corev1.NodeCondition{
							{
								Type:   corev1.NodeReady,
								Status: corev1.ConditionFalse,
							},
						},
					},
				}
				newNode := &corev1.Node{
					ObjectMeta: metav1.ObjectMeta{Name: "test-node"},
					Status: corev1.NodeStatus{
						Conditions: []corev1.NodeCondition{
							{
								Type:   corev1.NodeReady,
								Status: corev1.ConditionFalse,
							},
						},
					},
				}

				result := pred.Update(event.UpdateEvent{
					ObjectOld: oldNode,
					ObjectNew: newNode,
				})
				Expect(result).To(BeFalse())
			})
		})
	})

	Describe("CreateFunc", func() {
		It("should return true for any node creation", func() {
			node := &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{Name: "test-node"},
			}
			result := pred.Create(event.CreateEvent{
				Object: node,
			})
			Expect(result).To(BeTrue())
		})
	})

	Describe("DeleteFunc", func() {
		It("should return false for any node deletion", func() {
			node := &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{Name: "test-node"},
			}
			result := pred.Delete(event.DeleteEvent{
				Object: node,
			})
			Expect(result).To(BeFalse())
		})
	})
})
