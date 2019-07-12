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

package source_test

import (
	"k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

var ctrl controller.Controller

// This example Watches for Pod Events (e.g. Create / Update / Delete) and enqueues a reconcile.Request
// with the Name and Namespace of the Pod.
func ExampleKind() {
	ctrl.Watch(&source.Kind{Type: &v1.Pod{}}, &handler.EnqueueRequestForObject{})
}

// This example reads GenericEvents from a channel and enqueues a reconcile.Request containing the Name and Namespace
// provided by the event.
func ExampleChannel() {
	events := make(chan event.GenericEvent)

	ctrl.Watch(
		&source.Channel{Source: events},
		&handler.EnqueueRequestForObject{},
	)
}
