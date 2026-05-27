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

package main

import (
	"flag"
	"os"

	"k8s.io/client-go/kubernetes"
	typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/manager/signals"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	mcvClient "github.com/redhat-et/GKM/mcv/pkg/client"
	kernelcachenodecontroller "github.com/kserve/kserve/pkg/controller/v1alpha1/kernelcachenode"
	kservescheme "github.com/kserve/kserve/pkg/scheme"
)

var setupLog = ctrl.Log.WithName("setup")

const (
	LeaderLockName = "kserve-kernel-cache-node-manager-leader-lock"
)

// Options defines the program configurable options that may be passed on the command line.
type Options struct {
	metricsAddr          string
	webhookPort          int
	enableLeaderElection bool
	probeAddr            string
	zapOpts              zap.Options
}

// DefaultOptions returns the default values for the program options.
func DefaultOptions() Options {
	return Options{
		metricsAddr:          ":8080",
		webhookPort:          9443,
		enableLeaderElection: false,
		probeAddr:            ":8081",
		zapOpts:              zap.Options{},
	}
}

// GetOptions parses the program flags and returns them as Options.
func GetOptions() Options {
	opts := DefaultOptions()
	flag.BoolVar(&opts.enableLeaderElection, "leader-elect", opts.enableLeaderElection,
		"Enable leader election for kserve controller manager. "+
			"Enabling this will ensure there is only one active kserve controller manager.")
	opts.zapOpts.BindFlags(flag.CommandLine)
	flag.Parse()
	return opts
}

func main() {
	options := GetOptions()
	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&options.zapOpts)))

	// Get a config to talk to the apiserver
	setupLog.Info("Setting up client for manager")
	cfg, err := config.GetConfig()
	if err != nil {
		setupLog.Error(err, "unable to set up client config")
		os.Exit(1)
	}

	// Setup clientset to directly talk to the api server
	clientSet, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		setupLog.Error(err, "unable to create clientSet")
		os.Exit(1)
	}

	// Warm up MCV GPU detection cache before starting manager
	// Pattern from GKM: Call MCV at startup to seed the cache
	// First call is slow (hardware detection), subsequent calls are fast (cached)
	setupLog.Info("Warming up MCV GPU detection cache")
	noGPU := false
	if os.Getenv("NO_GPU") == "true" {
		noGPU = true
		setupLog.Info("NO_GPU environment variable set - will use stub GPU data")
	}

	detected := false
	disableTimeout := 0 // Use default MCV timeout
	for i := 1; i < 8; i++ {
		_, err := mcvClient.GetSystemGPUInfo(mcvClient.HwOptions{
			EnableStub: &noGPU,
			Timeout:    disableTimeout,
		})
		if err == nil {
			detected = true
			setupLog.Info("GPU detection cache warmed up", "attempts", i, "stubMode", noGPU)
			break
		}
		setupLog.V(1).Info("GPU detection attempt failed, retrying", "attempt", i, "error", err)
	}
	if !detected {
		setupLog.Info("GPU detection cache warmup failed - will retry during reconciliation")
		// Non-fatal - reconciler will retry when KernelCacheNode is created
	}

	// Create a new Cmd to provide shared dependencies and start components
	setupLog.Info("Setting up manager")
	mgr, err := manager.New(cfg, manager.Options{
		Metrics: metricsserver.Options{
			BindAddress: options.metricsAddr,
		},
		WebhookServer: webhook.NewServer(webhook.Options{
			Port: options.webhookPort,
		}),
		LeaderElection:         options.enableLeaderElection,
		LeaderElectionID:       LeaderLockName,
		HealthProbeBindAddress: options.probeAddr,
	})
	if err != nil {
		setupLog.Error(err, "unable to set up overall controller manager")
		os.Exit(1)
	}

	setupLog.Info("Registering Components.")

	setupLog.Info("Setting up controller schemes")
	if err := kservescheme.AddControllerAPIs(mgr.GetScheme()); err != nil {
		setupLog.Error(err, "unable to add controller APIs to scheme")
		os.Exit(1)
	}

	// Setup KernelCacheNode controller
	kernelCacheNodeEventBroadcaster := record.NewBroadcaster()
	setupLog.Info("Setting up v1alpha1 KernelCacheNode controller")
	kernelCacheNodeEventBroadcaster.StartRecordingToSink(&typedcorev1.EventSinkImpl{Interface: clientSet.CoreV1().Events("")})
	reconciler := &kernelcachenodecontroller.KernelCacheNodeReconciler{
		Client:    mgr.GetClient(),
		Clientset: clientSet,
		Log:       ctrl.Log.WithName("v1alpha1Controllers").WithName("KernelCacheNode"),
		Scheme:    mgr.GetScheme(),
	}

	if err = reconciler.SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "v1alpha1Controllers", "KernelCacheNode")
		os.Exit(1)
	}

	// Start the Cmd
	setupLog.Info("Starting the Cmd.")
	if err := mgr.Start(signals.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "unable to run the manager")
		os.Exit(1)
	}
}
