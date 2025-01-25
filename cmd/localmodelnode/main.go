/*
Copyright 2024 The KServe Authors.

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

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	localmodelnodecontroller "github.com/kserve/kserve/pkg/controller/v1alpha1/localmodelnode"
)

var setupLog = ctrl.Log.WithName("setup")

const (
	LeaderLockName = "kserve-local-model-node-manager-leader-lock"
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

	setupLog.Info("Setting up KServe v1alpha1 scheme")
	if err := v1alpha1.AddToScheme(mgr.GetScheme()); err != nil {
		setupLog.Error(err, "unable to add KServe v1alpha1 to scheme")
		os.Exit(1)
	}

	setupLog.Info("Setting up KServe v1beta1 scheme")
	if err := v1beta1.AddToScheme(mgr.GetScheme()); err != nil {
		setupLog.Error(err, "unable to add KServe v1beta1 to scheme")
		os.Exit(1)
	}

	setupLog.Info("Setting up core scheme")
	if err := corev1.AddToScheme(mgr.GetScheme()); err != nil {
		setupLog.Error(err, "unable to add Core APIs to scheme")
		os.Exit(1)
	}

	// Setup LocalModelNode controller
	localModelNodeEventBroadcaster := record.NewBroadcaster()
	setupLog.Info("Setting up v1alpha1 LocalModelNode controller")
	localModelNodeEventBroadcaster.StartRecordingToSink(&typedcorev1.EventSinkImpl{Interface: clientSet.CoreV1().Events("")})
	if err = (&localmodelnodecontroller.LocalModelNodeReconciler{
		Client:    mgr.GetClient(),
		Clientset: clientSet,
		Log:       ctrl.Log.WithName("v1alpha1Controllers").WithName("LocalModelNode"),
		Scheme:    mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "v1alpha1Controllers", "LocalModelNode")
		os.Exit(1)
	}

	// Start the Cmd
	setupLog.Info("Starting the Cmd.")
	if err := mgr.Start(signals.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "unable to run the manager")
		os.Exit(1)
	}
}
