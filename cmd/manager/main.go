/*

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

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	"github.com/kserve/kserve/pkg/constants"
	trainedmodelcontroller "github.com/kserve/kserve/pkg/controller/v1alpha1/trainedmodel"
	"github.com/kserve/kserve/pkg/controller/v1alpha1/trainedmodel/reconcilers/modelconfig"
	v1beta1controller "github.com/kserve/kserve/pkg/controller/v1beta1/inferenceservice"
	"github.com/kserve/kserve/pkg/webhook/admission/pod"
	istio_networking "istio.io/api/networking/v1alpha3"
	"istio.io/client-go/pkg/apis/networking/v1alpha3"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	"k8s.io/client-go/tools/record"
	knservingv1 "knative.dev/serving/pkg/apis/serving/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	client "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/manager/signals"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	// Allow unknown fields in Istio API client for backwards compatibility if cluster has existing vs with deprecated fields.
	istio_networking.VirtualServiceUnmarshaler.AllowUnknownFields = true
	istio_networking.GatewayUnmarshaler.AllowUnknownFields = true
}

func main() {
	var metricsAddr string
	flag.StringVar(&metricsAddr, "metrics-addr", ":8080", "The address the metric endpoint binds to.")
	flag.Parse()
	logf.SetLogger(zap.New())
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
	mgr, err := manager.New(cfg, manager.Options{MetricsBindAddress: metricsAddr, Port: 9443})
	if err != nil {
		log.Error(err, "unable to set up overall controller manager")
		os.Exit(1)
	}

	log.Info("Registering Components.")

	log.Info("Setting up KServe v1alpha1 scheme")
	if err := v1alpha1.AddToScheme(mgr.GetScheme()); err != nil {
		log.Error(err, "unable to add KServe v1alpha1 to scheme")
		os.Exit(1)
	}

	log.Info("Setting up KServe v1beta1 scheme")
	if err := v1beta1.AddToScheme(mgr.GetScheme()); err != nil {
		log.Error(err, "unable to add KServe v1beta1 to scheme")
		os.Exit(1)
	}

	client, err := client.New(mgr.GetConfig(), client.Options{Scheme: mgr.GetScheme()})
	if err != nil {
		log.Error(err, "unable to create new client.")
	}

	deployConfig, err := v1beta1.NewDeployConfig(client)
	if err != nil {
		log.Error(err, "unable to get deploy config.")
		os.Exit(1)
	}
	if deployConfig.DefaultDeploymentMode == string(constants.Serverless) {
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
	}

	log.Info("Setting up core scheme")
	if err := v1.AddToScheme(mgr.GetScheme()); err != nil {
		log.Error(err, "unable to add Core APIs to scheme")
		os.Exit(1)
	}

	// Setup all Controllers
	setupLog.Info("Setting up v1beta1 controller")
	eventBroadcaster := record.NewBroadcaster()
	clientSet, err := kubernetes.NewForConfig(mgr.GetConfig())
	if err != nil {
		setupLog.Error(err, "unable to create clientSet")
		os.Exit(1)
	}
	eventBroadcaster.StartRecordingToSink(&typedcorev1.EventSinkImpl{Interface: clientSet.CoreV1().Events("")})
	if err = (&v1beta1controller.InferenceServiceReconciler{
		Client: mgr.GetClient(),
		Log:    ctrl.Log.WithName("v1beta1Controllers").WithName("InferenceService"),
		Scheme: mgr.GetScheme(),
		Recorder: eventBroadcaster.NewRecorder(
			mgr.GetScheme(), v1.EventSource{Component: "v1beta1Controllers"}),
	}).SetupWithManager(mgr, deployConfig); err != nil {
		setupLog.Error(err, "unable to create controller", "v1beta1Controller", "InferenceService")
		os.Exit(1)
	}

	//Setup TrainedModel controller
	trainedModelEventBroadcaster := record.NewBroadcaster()
	setupLog.Info("Setting up v1beta1 TrainedModel controller")
	trainedModelEventBroadcaster.StartRecordingToSink(&typedcorev1.EventSinkImpl{Interface: clientSet.CoreV1().Events("")})
	if err = (&trainedmodelcontroller.TrainedModelReconciler{
		Client:                mgr.GetClient(),
		Log:                   ctrl.Log.WithName("v1beta1Controllers").WithName("TrainedModel"),
		Scheme:                mgr.GetScheme(),
		Recorder:              eventBroadcaster.NewRecorder(mgr.GetScheme(), v1.EventSource{Component: "v1beta1Controllers"}),
		ModelConfigReconciler: modelconfig.NewModelConfigReconciler(mgr.GetClient(), mgr.GetScheme()),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "v1beta1Controllers", "TrainedModel")
		os.Exit(1)
	}

	log.Info("setting up webhook server")
	hookServer := mgr.GetWebhookServer()

	log.Info("registering webhooks to the webhook server")
	hookServer.Register("/mutate-pods", &webhook.Admission{Handler: &pod.Mutator{}})

	if err = ctrl.NewWebhookManagedBy(mgr).
		For(&v1alpha1.TrainedModel{}).
		Complete(); err != nil {
		setupLog.Error(err, "unable to create webhook", "webhook", "v1alpha1")
		os.Exit(1)
	}
	if err = ctrl.NewWebhookManagedBy(mgr).
		For(&v1beta1.InferenceService{}).
		Complete(); err != nil {
		setupLog.Error(err, "unable to create webhook", "webhook", "v1beta1")
		os.Exit(1)
	}

	// Start the Cmd
	log.Info("Starting the Cmd.")
	if err := mgr.Start(signals.SetupSignalHandler()); err != nil {
		log.Error(err, "unable to run the manager")
		os.Exit(1)
	}
}
