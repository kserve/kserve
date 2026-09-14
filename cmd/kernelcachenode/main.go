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

	"k8s.io/client-go/kubernetes"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/manager/signals"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	kernelcachenodecontroller "github.com/kserve/kserve/pkg/controller/v1alpha1/kernelcachenode"
	kservescheme "github.com/kserve/kserve/pkg/scheme"
)

var setupLog = ctrl.Log.WithName("setup")

const (
	LeaderLockName = "kserve-kernel-cache-node-manager-leader-lock"
)

type Options struct {
	metricsAddr          string
	enableLeaderElection bool
	probeAddr            string
	zapOpts              zap.Options
}

func DefaultOptions() Options {
	return Options{
		metricsAddr: ":8080",
		probeAddr:   ":8081",
		zapOpts:     zap.Options{},
	}
}

func GetOptions() Options {
	opts := DefaultOptions()
	flag.StringVar(&opts.metricsAddr, "metrics-addr", opts.metricsAddr, "The address the metric endpoint binds to.")
	flag.BoolVar(&opts.enableLeaderElection, "leader-elect", opts.enableLeaderElection,
		"Enable leader election for the kernel cache node controller manager.")
	flag.StringVar(&opts.probeAddr, "health-probe-addr", opts.probeAddr, "The address the probe endpoint binds to.")
	opts.zapOpts.BindFlags(flag.CommandLine)
	flag.Parse()
	return opts
}

func main() {
	options := GetOptions()
	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&options.zapOpts)))

	if os.Getenv("NODE_NAME") == "" {
		setupLog.Error(nil, "NODE_NAME environment variable must be set")
		os.Exit(1)
	}

	setupLog.Info("Setting up client for manager")
	cfg, err := config.GetConfig()
	if err != nil {
		setupLog.Error(err, "unable to set up client config")
		os.Exit(1)
	}

	clientSet, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		setupLog.Error(err, "unable to create clientSet")
		os.Exit(1)
	}

	setupLog.Info("Setting up manager")
	mgr, err := manager.New(cfg, manager.Options{
		Metrics: metricsserver.Options{
			BindAddress: options.metricsAddr,
		},
		LeaderElection:         options.enableLeaderElection,
		LeaderElectionID:       LeaderLockName,
		HealthProbeBindAddress: options.probeAddr,
	})
	if err != nil {
		setupLog.Error(err, "unable to set up overall controller manager")
		os.Exit(1)
	}

	setupLog.Info("Setting up controller schemes")
	if err := kservescheme.AddControllerAPIs(mgr.GetScheme()); err != nil {
		setupLog.Error(err, "unable to add controller APIs to scheme")
		os.Exit(1)
	}

	setupLog.Info("Setting up v1alpha1 KernelCacheNode controller")
	if err = (&kernelcachenodecontroller.KernelCacheNodeReconciler{
		Client:    mgr.GetClient(),
		Clientset: clientSet,
		Log:       ctrl.Log.WithName("v1alpha1Controllers").WithName("KernelCacheNode"),
		Scheme:    mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "v1alpha1Controllers", "KernelCacheNode")
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

	setupLog.Info("Starting the Cmd.")
	if err := mgr.Start(signals.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "unable to run the manager")
		os.Exit(1)
	}
}
