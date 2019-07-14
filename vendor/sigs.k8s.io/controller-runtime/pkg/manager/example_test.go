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

package manager_test

import (
	"os"

	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/controller-runtime/pkg/runtime/signals"
)

var (
	mgr manager.Manager
	// NB: don't call SetLogger in init(), or else you'll mess up logging in the main suite.
	log = logf.Log.WithName("manager-examples")
)

// This example creates a new Manager that can be used with controller.New to create Controllers.
func ExampleNew() {
	cfg, err := config.GetConfig()
	if err != nil {
		log.Error(err, "unable to get kubeconfig")
		os.Exit(1)
	}

	mgr, err := manager.New(cfg, manager.Options{})
	if err != nil {
		log.Error(err, "unable to set up manager")
		os.Exit(1)
	}
	log.Info("created manager", "manager", mgr)
}

// This example adds a Runnable for the Manager to Start.
func ExampleManager_Add() {
	err := mgr.Add(manager.RunnableFunc(func(<-chan struct{}) error {
		// Do something
		return nil
	}))
	if err != nil {
		log.Error(err, "unable add a runnable to the manager")
		os.Exit(1)
	}
}

// This example starts a Manager that has had Runnables added.
func ExampleManager_Start() {
	err := mgr.Start(signals.SetupSignalHandler())
	if err != nil {
		log.Error(err, "unable start the manager")
		os.Exit(1)
	}
}
