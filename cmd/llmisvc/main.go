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
	"context"
	"crypto/tls"
	"flag"
	"fmt"
	"os"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	apixclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/record"
	"knative.dev/pkg/apiextensions/storageversion"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/manager/signals"
	"sigs.k8s.io/controller-runtime/pkg/metrics/filters"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/controller/v1alpha2/llmisvc"
	kservescheme "github.com/kserve/kserve/pkg/scheme"
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

// leaderRunnable is a function that implements both Runnable and
// LeaderElectionRunnable, ensuring it only runs on the elected leader
// and starts after webhooks and caches are ready.
type leaderRunnable func(context.Context) error

func (r leaderRunnable) Start(ctx context.Context) error { return r(ctx) }
func (r leaderRunnable) NeedLeaderElection() bool        { return true }

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(kservescheme.AddLLMISVCAPIs(scheme))
}

type Options struct {
	metricsAddr          string
	webhookPort          int
	enableLeaderElection bool
	probeAddr            string
	metricsSecure        bool
	enableHTTP2          bool
	zapOpts              zap.Options
}

func DefaultOptions() Options {
	return Options{
		metricsAddr:          ":8443",
		webhookPort:          9443,
		enableLeaderElection: false,
		probeAddr:            ":8081",
		metricsSecure:        true,
		enableHTTP2:          false,
		zapOpts:              zap.Options{},
	}
}

// GetOptions parses the program flags and returns them as Options.
func GetOptions() Options {
	opts := DefaultOptions()
	flag.StringVar(&opts.metricsAddr, "metrics-addr", opts.metricsAddr, "The address the metric endpoint binds to.")
	flag.IntVar(&opts.webhookPort, "webhook-port", opts.webhookPort, "The port that the webhook server binds to.")
	flag.BoolVar(&opts.enableLeaderElection, "leader-elect", opts.enableLeaderElection,
		"Enable leader election for kserve controller manager. "+
			"Enabling this will ensure there is only one active kserve controller manager.")
	flag.StringVar(&opts.probeAddr, "health-probe-addr", opts.probeAddr, "The address the probe endpoint binds to.")
	flag.BoolVar(&opts.metricsSecure, "metrics-secure", opts.metricsSecure, "Whether to serve metric via HTTPS.")
	flag.BoolVar(&opts.enableHTTP2, "enable-http2", false, "If set, HTTP/2 will be enabled for the metrics and webhook servers")
	opts.zapOpts.BindFlags(flag.CommandLine)
	flag.Parse()
	return opts
}

func main() {
	ctx := signals.SetupSignalHandler()
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

	// http/2 should be disabled due to its vulnerabilities. More specifically, disabling http/2 will
	// prevent from being vulnerable to the HTTP/2 Stream Cancellation and
	// Rapid Reset CVEs. For more information see:
	// - https://github.com/advisories/GHSA-qppj-fm5r-hxr3
	// - https://github.com/advisories/GHSA-4374-p667-p6c8
	disableHTTP2 := func(c *tls.Config) {
		setupLog.Info("disabling http/2")
		c.NextProtos = []string{"http/1.1"}
	}

	var tlsOpts []func(*tls.Config)
	if !options.enableHTTP2 {
		tlsOpts = append(tlsOpts, disableHTTP2)
	}
	// Metrics endpoint is enabled in 'config/default/kustomization.yaml'. The Metrics options configure the server.
	// More info:
	// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.21.0/pkg/metrics/server
	// - https://book.kubebuilder.io/reference/metrics.html
	metricsServerOptions := metricsserver.Options{
		BindAddress:   options.metricsAddr,
		SecureServing: options.metricsSecure,
		TLSOpts:       tlsOpts,
	}

	if options.metricsSecure {
		// FilterProvider is used to protect the metrics endpoint with authn/authz.
		// These configurations ensure that only authorized users and service accounts
		// can access the metrics endpoint. The RBAC are configured in 'config/rbac/kustomization.yaml'. More info:
		// https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.21.0/pkg/metrics/filters#WithAuthenticationAndAuthorization
		metricsServerOptions.FilterProvider = filters.WithAuthenticationAndAuthorization
	}

	llmSvcCacheSelector, _ := metav1.LabelSelectorAsSelector(&llmisvc.ChildResourcesLabelSelector)

	mgrOpts := ctrl.Options{
		Scheme:                 scheme,
		Metrics:                metricsServerOptions,
		WebhookServer:          webhook.NewServer(webhook.Options{Port: options.webhookPort, TLSOpts: tlsOpts}),
		HealthProbeBindAddress: options.probeAddr,
		LeaderElection:         options.enableLeaderElection,
		LeaderElectionID:       "llminferenceservice-kserve-controller-manager",
		Cache: cache.Options{
			ByObject: map[client.Object]cache.ByObject{
				&corev1.Secret{}: {
					Label: llmSvcCacheSelector,
				},
				&corev1.ConfigMap{}: {
					Label: llmSvcCacheSelector,
				},
				&appsv1.Deployment{}: {
					Label: llmSvcCacheSelector,
				},
				&corev1.Pod{}: {
					Label: llmSvcCacheSelector,
				},
				&autoscalingv2.HorizontalPodAutoscaler{}: {
					Label: llmSvcCacheSelector,
				},
			},
		},
	}

	if err := customizeManagerOptions(&mgrOpts); err != nil {
		setupLog.Error(err, "failed to apply distribution-specific manager options")
		os.Exit(1)
	}
	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), mgrOpts)
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	// Register webhooks: validation (v1alpha1, v1alpha2) and conversion
	v1alpha2LLMValidator := &v1alpha2.LLMInferenceServiceValidator{}
	if err = v1alpha2LLMValidator.SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create webhook", "webhook", "llminferenceservice-v1alpha2")
		os.Exit(1)
	}
	v1alpha1LLMValidator := &v1alpha1.LLMInferenceServiceValidator{}
	if err = v1alpha1LLMValidator.SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create webhook", "webhook", "llminferenceservice-v1alpha1")
		os.Exit(1)
	}
	v1alpha1ConfigValidator := &v1alpha1.LLMInferenceServiceConfigValidator{
		ConfigValidationFunc:   createV1Alpha1ConfigValidationFunc(clientSet),
		WellKnownConfigChecker: wellKnownConfigChecker,
	}
	if err = v1alpha1ConfigValidator.SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create webhook", "webhook", "llminferenceserviceconfig-v1alpha1")
		os.Exit(1)
	}
	v1alpha2ConfigValidator := &v1alpha2.LLMInferenceServiceConfigValidator{
		ConfigValidationFunc:   createV1Alpha2ConfigValidationFunc(clientSet),
		WellKnownConfigChecker: wellKnownConfigChecker,
	}
	if err = v1alpha2ConfigValidator.SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create webhook", "webhook", "llminferenceserviceconfig-v1alpha2")
		os.Exit(1)
	}
	if err = ctrl.NewWebhookManagedBy(mgr).
		For(&v1alpha2.LLMInferenceService{}).
		Complete(); err != nil {
		setupLog.Error(err, "unable to create conversion webhook", "webhook", "llminferenceservice")
		os.Exit(1)
	}
	if err = ctrl.NewWebhookManagedBy(mgr).
		For(&v1alpha2.LLMInferenceServiceConfig{}).
		Complete(); err != nil {
		setupLog.Error(err, "unable to create conversion webhook", "webhook", "llminferenceserviceconfig")
		os.Exit(1)
	}

	setupLog.Info("Setting up LLMInferenceService controller")
	llmEventBroadcaster := record.NewBroadcaster()
	llmEventBroadcaster.StartRecordingToSink(&typedcorev1.EventSinkImpl{Interface: clientSet.CoreV1().Events("")})
	if err = (&llmisvc.LLMISVCReconciler{
		Client:        mgr.GetClient(),
		Config:        mgr.GetConfig(),
		Clientset:     clientSet,
		EventRecorder: llmEventBroadcaster.NewRecorder(scheme, corev1.EventSource{Component: "LLMInferenceServiceController"}),
		Validator: func(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService) error {
			_, err := v1alpha2LLMValidator.ValidateCreate(ctx, llmSvc)
			return err
		},
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "LLMInferenceService")
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

	// Storage version migration runs as a LeaderElection runnable, which starts
	// after the webhook server and cache sync are ready. This avoids the
	// chicken-and-egg problem where migration patches trigger validating webhooks
	// that aren't serving yet.
	// migrationBackoff allows enough time for Service endpoints to propagate
	// after the webhook server starts.
	migrationBackoff := wait.Backoff{
		Duration: 2 * time.Second,
		Factor:   1.5,
		Jitter:   0.1,
		Steps:    10,
	}
	if err := mgr.Add(leaderRunnable(func(ctx context.Context) error {
		setupLog.Info("running storage version migration")
		migrator := storageversion.NewMigrator(dynamic.NewForConfigOrDie(cfg), apixclient.NewForConfigOrDie(cfg))
		for _, gr := range []schema.GroupResource{
			{Group: v1alpha2.SchemeGroupVersion.Group, Resource: "llminferenceservices"},
			{Group: v1alpha2.SchemeGroupVersion.Group, Resource: "llminferenceserviceconfigs"},
		} {
			var lastErr error
			if err := wait.ExponentialBackoffWithContext(ctx, migrationBackoff, func(ctx context.Context) (bool, error) {
				if err := migrator.Migrate(ctx, gr); err != nil {
					lastErr = err
					if apierrors.IsForbidden(err) || apierrors.IsUnauthorized(err) || apierrors.IsNotFound(err) {
						return false, err
					}
					setupLog.Error(err, "storage version migration attempt failed, retrying", "resource", gr)
					return false, nil
				}
				return true, nil
			}); err != nil {
				if lastErr != nil && wait.Interrupted(err) {
					return fmt.Errorf("storage version migration for %s timed out: %w", gr, lastErr)
				}
				return fmt.Errorf("storage version migration for %s failed: %w", gr, err)
			}
		}
		setupLog.Info("storage version migration completed")
		return nil
	})); err != nil {
		setupLog.Error(err, "unable to register storage version migration")
		os.Exit(1)
	}

	setupLog.Info("starting manager")
	if err := mgr.Start(ctx); err != nil {
		setupLog.Error(err, "unable to run the manager")
		os.Exit(1)
	}
}

// wellKnownConfigChecker returns true if the given config name is a well-known config.
func wellKnownConfigChecker(name string) bool {
	return llmisvc.WellKnownDefaultConfigs.Has(name)
}

// validateLLMISVCConfig validates a v1alpha2 LLMInferenceServiceConfig by loading the controller
// config and validating the template variables.
func validateLLMISVCConfig(ctx context.Context, clientSet kubernetes.Interface, config *v1alpha2.LLMInferenceServiceConfig) error {
	cfg, err := llmisvc.LoadConfig(ctx, clientSet)
	if err != nil {
		return err
	}
	_, err = llmisvc.ReplaceVariables(llmisvc.LLMInferenceServiceSample(), config, cfg)
	return err
}

// createV1Alpha1ConfigValidationFunc creates a validation function for v1alpha1 LLMInferenceServiceConfig.
// It converts the config to v1alpha2 and validates using the v1alpha2 llmisvc package.
func createV1Alpha1ConfigValidationFunc(clientSet kubernetes.Interface) func(ctx context.Context, config *v1alpha1.LLMInferenceServiceConfig) error {
	return func(ctx context.Context, config *v1alpha1.LLMInferenceServiceConfig) error {
		v2Config := &v1alpha2.LLMInferenceServiceConfig{}
		if err := config.ConvertTo(v2Config); err != nil {
			return err
		}
		return validateLLMISVCConfig(ctx, clientSet, v2Config)
	}
}

// createV1Alpha2ConfigValidationFunc creates a validation function for v1alpha2 LLMInferenceServiceConfig.
func createV1Alpha2ConfigValidationFunc(clientSet kubernetes.Interface) func(ctx context.Context, config *v1alpha2.LLMInferenceServiceConfig) error {
	return func(ctx context.Context, config *v1alpha2.LLMInferenceServiceConfig) error {
		return validateLLMISVCConfig(ctx, clientSet, config)
	}
}
