/*
Copyright 2026 The KServe Authors.

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

package main

import (
	"flag"
	"os"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/manager/signals"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	"github.com/kserve/kserve/pkg/constants"
	kernelcachenodecontroller "github.com/kserve/kserve/pkg/controller/v1alpha1/kernelcachenode"
	kservescheme "github.com/kserve/kserve/pkg/scheme"
)

var setupLog = ctrl.Log.WithName("setup")

func main() {
	var zapOpts zap.Options
	zapOpts.BindFlags(flag.CommandLine)
	flag.Parse()
	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&zapOpts)))

	if os.Getenv("NODE_NAME") == "" {
		setupLog.Error(nil, "NODE_NAME environment variable must be set")
		os.Exit(1)
	}

	cfg, err := config.GetConfig()
	if err != nil {
		setupLog.Error(err, "unable to set up client config")
		os.Exit(1)
	}

	isvcPodLabelRequirement, err := labels.NewRequirement(constants.InferenceServicePodLabelKey, selection.Exists, nil)
	if err != nil {
		setupLog.Error(err, "unable to create InferenceService Pod label selector")
		os.Exit(1)
	}
	isvcPodLabelSelector := labels.NewSelector().Add(*isvcPodLabelRequirement)

	mgr, err := manager.New(cfg, manager.Options{
		Metrics:                metricsserver.Options{BindAddress: ":8080"},
		HealthProbeBindAddress: ":8081",
		Cache: cache.Options{
			ByObject: map[client.Object]cache.ByObject{
				&corev1.Pod{}: {Label: isvcPodLabelSelector},
				&corev1.ConfigMap{}: {
					Namespaces: map[string]cache.Config{
						constants.KServeNamespace: {
							FieldSelector: fields.OneTermEqualSelector("metadata.name", constants.InferenceServiceConfigMapName),
						},
					},
				},
			},
		},
	})
	if err != nil {
		setupLog.Error(err, "unable to set up manager")
		os.Exit(1)
	}

	if err := kservescheme.AddControllerAPIs(mgr.GetScheme()); err != nil {
		setupLog.Error(err, "unable to add controller APIs to scheme")
		os.Exit(1)
	}

	setupLog.Info("Setting up KernelCacheNode controller")
	reconciler := &kernelcachenodecontroller.KernelCacheNodeReconciler{
		Client:   mgr.GetClient(),
		Reader:   mgr.GetAPIReader(),
		NodeName: os.Getenv("NODE_NAME"),
		Log:      ctrl.Log.WithName("kernelcachenode-agent"),
	}
	if err := reconciler.SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create KernelCacheNode controller")
		os.Exit(1)
	}
	if err := mgr.Add(manager.RunnableFunc(reconciler.RunPeriodicImageValidation)); err != nil {
		setupLog.Error(err, "unable to create KernelCacheNode image validation runnable")
		os.Exit(1)
	}

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	setupLog.Info("Starting KernelCacheNode agent", "node", os.Getenv("NODE_NAME"))
	if err := mgr.Start(signals.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "unable to run KernelCacheNode agent")
		os.Exit(1)
	}
}
