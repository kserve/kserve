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
	"context"
	"errors"
	"flag"
	"os"

	"k8s.io/apimachinery/pkg/runtime"
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

	localmodelnodecontroller "github.com/kserve/kserve/pkg/controller/v1alpha1/localmodelnode"
	kservemetrics "github.com/kserve/kserve/pkg/metrics"
	"github.com/kserve/kserve/pkg/oteljson"
	kservescheme "github.com/kserve/kserve/pkg/scheme"
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
	metricsSecure        bool
	metricsCertPath      string
	tlsMinVersion        string
	tlsCipherSuites      string
	zapOpts              zap.Options
	logFormat            oteljson.Format
}

// DefaultOptions returns the default values for the program options.
func DefaultOptions() Options {
	return Options{
		metricsAddr:          ":8080",
		webhookPort:          9443,
		enableLeaderElection: false,
		probeAddr:            ":8081",
		zapOpts:              zap.Options{},
		logFormat:            oteljson.FormatZap,
	}
}

// GetOptions parses the program flags and returns them as Options.
func GetOptions() Options {
	opts := DefaultOptions()
	flag.StringVar(&opts.metricsAddr, "metrics-addr", opts.metricsAddr, "The address the metric endpoint binds to.")
	flag.BoolVar(&opts.enableLeaderElection, "leader-elect", opts.enableLeaderElection,
		"Enable leader election for kserve controller manager. "+
			"Enabling this will ensure there is only one active kserve controller manager.")
	flag.BoolVar(&opts.metricsSecure, "metrics-secure", opts.metricsSecure, "Serve metrics over HTTPS with Kubernetes authentication and authorization.")
	flag.StringVar(&opts.metricsCertPath, "metrics-cert-path", opts.metricsCertPath, "Directory containing tls.crt and tls.key for the metrics server. If empty, self-signed certificates are generated.")
	flag.StringVar(&opts.tlsMinVersion, "tls-min-version", opts.tlsMinVersion, "Minimum TLS version (VersionTLS12, VersionTLS13). Defaults to VersionTLS12.")
	flag.StringVar(&opts.tlsCipherSuites, "tls-cipher-suites", opts.tlsCipherSuites, "Comma-separated list of TLS cipher suites (Go names). If empty, Go defaults are used.")
	opts.zapOpts.BindFlags(flag.CommandLine)
	oteljson.BindFlags(flag.CommandLine, &opts.logFormat)
	flag.Parse()
	return opts
}

func main() {
	options := GetOptions()
	if options.logFormat == oteljson.FormatOTelJSON {
		oteljson.Apply(&options.zapOpts, "kserve-localmodelnode-controller", os.Stdout)
	}
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

	tlsOpts, err := resolveTLS(context.Background(), options.tlsMinVersion, options.tlsCipherSuites)
	if err != nil {
		setupLog.Error(err, "unable to resolve TLS configuration")
		os.Exit(1)
	}

	// Create a new Cmd to provide shared dependencies and start components
	setupLog.Info("Setting up manager")
	currentNode := os.Getenv("NODE_NAME")
	if currentNode == "" {
		setupLog.Error(errors.New("NODE_NAME must be set"), "unable to configure node cache")
		os.Exit(1)
	}
	cacheOpts, err := localmodelnodecontroller.NewCacheOptions(currentNode)
	if err != nil {
		setupLog.Error(err, "unable to configure node cache")
		os.Exit(1)
	}
	// Cache policies resolve custom resource types while the manager is created.
	scheme := runtime.NewScheme()
	if err := kservescheme.AddControllerAPIs(scheme); err != nil {
		setupLog.Error(err, "unable to add controller APIs to scheme")
		os.Exit(1)
	}
	metricsServerOptions, err := kservemetrics.ConfigureServerOptions(metricsserver.Options{
		BindAddress:   options.metricsAddr,
		SecureServing: options.metricsSecure,
		CertDir:       options.metricsCertPath,
		TLSOpts:       tlsOpts,
	})
	if err != nil {
		setupLog.Error(err, "unable to configure metrics server")
		os.Exit(1)
	}

	mgr, err := manager.New(cfg, manager.Options{
		Scheme:  scheme,
		Metrics: metricsServerOptions,
		WebhookServer: webhook.NewServer(webhook.Options{
			Port:    options.webhookPort,
			TLSOpts: tlsOpts,
		}),
		LeaderElection:         options.enableLeaderElection,
		LeaderElectionID:       LeaderLockName,
		HealthProbeBindAddress: options.probeAddr,
		Cache:                  cacheOpts,
		Client:                 localmodelnodecontroller.NewClientOptions(),
	})
	if err != nil {
		setupLog.Error(err, "unable to set up overall controller manager")
		os.Exit(1)
	}

	setupLog.Info("Registering Components.")

	// Setup LocalModelNode controller
	localModelNodeEventBroadcaster := record.NewBroadcaster()
	setupLog.Info("Setting up v1alpha1 LocalModelNode controller")
	localModelNodeEventBroadcaster.StartRecordingToSink(&typedcorev1.EventSinkImpl{Interface: clientSet.CoreV1().Events("")})
	reconciler := &localmodelnodecontroller.LocalModelNodeReconciler{
		Client:    mgr.GetClient(),
		Clientset: clientSet,
		Log:       ctrl.Log.WithName("v1alpha1Controllers").WithName("LocalModelNode"),
		Scheme:    mgr.GetScheme(),
	}

	if err = reconciler.SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "v1alpha1Controllers", "LocalModelNode")
		os.Exit(1)
	}

	// Start the Cmd
	setupLog.Info("Starting the Cmd.")
	startCtx, err := setupDistroStartup(signals.SetupSignalHandler(), mgr)
	if err != nil {
		setupLog.Error(err, "Failed to set up distro startup; profile changes will not trigger a restart")
	}
	if err := mgr.Start(startCtx); err != nil {
		setupLog.Error(err, "unable to run the manager")
		os.Exit(1)
	}
}
