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
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/manager/signals"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	kernelcachecontroller "github.com/kserve/kserve/pkg/controller/v1alpha1/kernelcache"
	kservescheme "github.com/kserve/kserve/pkg/scheme"
)

var setupLog = ctrl.Log.WithName("setup")

const (
	LeaderLockName = "kserve-kernelcache-manager-leader-lock"
)

// Options defines the program configurable options that may be passed on the command line.
type Options struct {
	metricsAddr          string
	enableLeaderElection bool
	probeAddr            string
	webhookPort          int
	zapOpts              zap.Options
}

// DefaultOptions returns the default values for the program options.
func DefaultOptions() Options {
	return Options{
		metricsAddr:          ":8080",
		enableLeaderElection: false,
		probeAddr:            ":8081",
		webhookPort:          9443,
		zapOpts:              zap.Options{},
	}
}

// GetOptions parses the program flags and returns them as Options.
func GetOptions() Options {
	opts := DefaultOptions()
	flag.StringVar(&opts.metricsAddr, "metrics-addr", opts.metricsAddr, "The address the metric endpoint binds to.")
	flag.BoolVar(&opts.enableLeaderElection, "leader-elect", opts.enableLeaderElection,
		"Enable leader election for kernelcache controller manager. "+
			"Enabling this will ensure there is only one active kernelcache controller manager.")
	flag.StringVar(&opts.probeAddr, "health-probe-addr", opts.probeAddr, "The address the probe endpoint binds to.")
	flag.IntVar(&opts.webhookPort, "webhook-port", opts.webhookPort, "The port that the webhook server binds to.")
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

	// Create a new Cmd to provide shared dependencies and start components
	setupLog.Info("Setting up manager")
	mgr, err := manager.New(cfg, manager.Options{
		Metrics: metricsserver.Options{
			BindAddress: options.metricsAddr,
		},
		LeaderElection:         options.enableLeaderElection,
		LeaderElectionID:       LeaderLockName,
		HealthProbeBindAddress: options.probeAddr,
		WebhookServer: webhook.NewServer(webhook.Options{
			Port: options.webhookPort,
		}),
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

	// Setup KernelCache controller
	kernelCacheEventBroadcaster := record.NewBroadcaster()
	setupLog.Info("Setting up v1alpha1 KernelCache controller")
	kernelCacheEventBroadcaster.StartRecordingToSink(&typedcorev1.EventSinkImpl{Interface: clientSet.CoreV1().Events("")})
	if err = (&kernelcachecontroller.KernelCacheReconciler{
		Client:    mgr.GetClient(),
		Clientset: clientSet,
		Log:       ctrl.Log.WithName("v1alpha1Controllers").WithName("KernelCache"),
		Scheme:    mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "v1alpha1Controllers", "KernelCache")
		os.Exit(1)
	}

	// Setup KernelCache webhook
	setupLog.Info("Setting up v1alpha1 KernelCache webhook")
	if err = (&v1alpha1.KernelCache{}).SetupWebhookWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create webhook", "webhook", "v1alpha1.KernelCache")
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

	// Start the Cmd
	setupLog.Info("Starting the Cmd.")
	if err := mgr.Start(signals.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "unable to run the manager")
		os.Exit(1)
	}
}
