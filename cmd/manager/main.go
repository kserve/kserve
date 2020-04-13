/*
Copyright 2019 kubeflow.org.

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
	"github.com/kubeflow/kfserving/pkg/webhook/admission/inferenceservice"
	"github.com/kubeflow/kfserving/pkg/webhook/admission/pod"
	"istio.io/client-go/pkg/apis/networking/v1alpha3"
	"os"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	"github.com/kubeflow/kfserving/pkg/apis"
	"github.com/kubeflow/kfserving/pkg/controller"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	knservingv1 "knative.dev/serving/pkg/apis/serving/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/controller-runtime/pkg/runtime/signals"
)

func main() {
	var metricsAddr string
	flag.StringVar(&metricsAddr, "metrics-addr", ":8080", "The address the metric endpoint binds to.")
	flag.Parse()
	logf.SetLogger(logf.ZapLogger(false))
	log := logf.Log.WithName("entrypoint")

	// Get a config to talk to the apiserver
	log.Info("Setting up client for manager")
	cfg, err := config.GetConfig()
	if err != nil {
		log.Error(err, "unable to set up client config")
		os.Exit(1)
	}

	// Create a new Cmd to provide shared dependencies and start components
	log.Info("Setting up manager")
	mgr, err := manager.New(cfg, manager.Options{MetricsBindAddress: metricsAddr})
	if err != nil {
		log.Error(err, "unable to set up overall controller manager")
		os.Exit(1)
	}

	log.Info("Registering Components.")

	// Setup Scheme for all resources
	log.Info("Setting up scheme")
	if err := apis.AddToScheme(mgr.GetScheme()); err != nil {
		log.Error(err, "unable to add APIs to scheme")
		os.Exit(1)
	}

	// Setup Scheme for all resources
	log.Info("Setting up Knative scheme")
	if err := knservingv1.AddToScheme(mgr.GetScheme()); err != nil {
		log.Error(err, "unable to add Knative APIs to scheme")
		os.Exit(1)
	}

	log.Info("Setting up Istio schemes")
	if err := v1alpha3.AddToScheme(mgr.GetScheme()); err != nil {
		log.Error(err, "unable to add Istio v1alpha3 APIs to scheme")
		os.Exit(1)
	}

	// Setup all Controllers
	log.Info("Setting up controller")
	if err := controller.AddToManager(mgr); err != nil {
		log.Error(err, "unable to register controllers to the manager")
		os.Exit(1)
	}

	log.Info("setting up webhook server")
	hookServer := mgr.GetWebhookServer()

	log.Info("registering webhooks to the webhook server")
	hookServer.Register("/mutate-pods", &webhook.Admission{Handler: &pod.Mutator{}})
	hookServer.Register("/validate-inferenceservices", &webhook.Admission{Handler: &inferenceservice.Validator{}})
	hookServer.Register("/mutate-inferenceservices", &webhook.Admission{Handler: &inferenceservice.Defaulter{}})

	// Start the Cmd
	log.Info("Starting the Cmd.")
	if err := mgr.Start(signals.SetupSignalHandler()); err != nil {
		log.Error(err, "unable to run the manager")
		os.Exit(1)
	}
}
