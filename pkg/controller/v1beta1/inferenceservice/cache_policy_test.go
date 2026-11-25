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

package inferenceservice

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	admissionv1 "k8s.io/api/admission/v1"
	batchv1 "k8s.io/api/batch/v1"
	coordinationv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	coordinationv1client "k8s.io/client-go/kubernetes/typed/coordination/v1"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/config"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	"github.com/kserve/kserve/pkg/constants"
	localmodelcontroller "github.com/kserve/kserve/pkg/controller/v1alpha1/localmodel"
	localmodelnodecontroller "github.com/kserve/kserve/pkg/controller/v1alpha1/localmodelnode"
	llmisvccontroller "github.com/kserve/kserve/pkg/controller/v1alpha2/llmisvc"
	"github.com/kserve/kserve/pkg/kernelcache/registryauth"
	kservescheme "github.com/kserve/kserve/pkg/scheme"
	servingruntime "github.com/kserve/kserve/pkg/webhook/admission/servingruntime"
)

var _ = Describe("controller cache policies", func() {
	It("defaults InferenceServices with local model caching enabled using a private scheme", func(ctx SpecContext) {
		kubeconfig := clientcmdapi.Config{
			Clusters: map[string]*clientcmdapi.Cluster{"envtest": {
				Server: cfg.Host, CertificateAuthorityData: cfg.CAData,
			}},
			AuthInfos: map[string]*clientcmdapi.AuthInfo{"envtest": {
				ClientCertificateData: cfg.CertData, ClientKeyData: cfg.KeyData,
			}},
			Contexts: map[string]*clientcmdapi.Context{"envtest": {
				Cluster: "envtest", AuthInfo: "envtest",
			}},
			CurrentContext: "envtest",
		}
		path := filepath.Join(GinkgoT().TempDir(), "kubeconfig")
		Expect(clientcmd.WriteToFile(kubeconfig, path)).To(Succeed())
		GinkgoT().Setenv("KUBECONFIG", path)
		configMap := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: constants.InferenceServiceConfigMapName, Namespace: constants.KServeNamespace},
			Data:       getBaseTestConfigs(),
		}
		configMap.Data["localModel"] = `{"enabled":true}`
		Expect(k8sClient.Create(ctx, configMap)).To(Succeed())
		DeferCleanup(func(ctx SpecContext) { Expect(k8sClient.Delete(ctx, configMap)).To(Succeed()) })
		isvc := &v1beta1.InferenceService{
			ObjectMeta: metav1.ObjectMeta{Name: "local-model-defaulting", Namespace: "default"},
			Spec: v1beta1.InferenceServiceSpec{Predictor: v1beta1.PredictorSpec{
				Model: &v1beta1.ModelSpec{ModelFormat: v1beta1.ModelFormat{Name: "sklearn"}},
			}},
		}
		Expect((&v1beta1.InferenceServiceDefaulter{}).Default(ctx, isvc)).To(Succeed())
	})

	It("keeps unlabeled Job conflicts visible through direct local-model reads", func(ctx SpecContext) {
		mgr := startCachePolicyManager(ctx, localmodelcontroller.NewCacheOptions(), localmodelcontroller.NewClientOptions(), &batchv1.Job{})
		ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{GenerateName: "import-cache-"}}
		Expect(k8sClient.Create(ctx, ns)).To(Succeed())
		DeferCleanup(func(ctx SpecContext) { Expect(k8sClient.Delete(ctx, ns)).To(Succeed()) })
		for _, name := range []string{"managed", "unrelated"} {
			job := &batchv1.Job{
				ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns.Name},
				Spec: batchv1.JobSpec{Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyNever,
					Containers:    []corev1.Container{{Name: "download", Image: "example.com/download:test"}},
				}}},
			}
			if name == "managed" {
				job.Labels = map[string]string{"model": "model", "modelNamespace": ns.Name}
			}
			Expect(k8sClient.Create(ctx, job)).To(Succeed())
		}
		Eventually(func(g Gomega) {
			jobs := &batchv1.JobList{}
			g.Expect(mgr.GetCache().List(ctx, jobs, client.InNamespace(ns.Name))).To(Succeed())
			g.Expect(jobs.Items).To(HaveLen(1))
			g.Expect(jobs.Items[0].Name).To(Equal("managed"))
		}, 10*time.Second).Should(Succeed())
		key := client.ObjectKey{Namespace: ns.Name, Name: "unrelated"}
		Expect(apierrors.IsNotFound(mgr.GetCache().Get(ctx, key, &batchv1.Job{}))).To(BeTrue())
		Expect(mgr.GetClient().Get(ctx, key, &batchv1.Job{})).To(Succeed())
	})

	It("limits each node agent to its own Jobs and LocalModelNode", func(ctx SpecContext) {
		const node = "cache-policy-node-a"
		opts, err := localmodelnodecontroller.NewCacheOptions(node)
		Expect(err).NotTo(HaveOccurred())
		mgr := startCachePolicyManager(ctx, opts, localmodelnodecontroller.NewClientOptions(), &batchv1.Job{}, &v1alpha1.LocalModelNode{})
		ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{GenerateName: "agent-cache-"}}
		Expect(k8sClient.Create(ctx, ns)).To(Succeed())
		DeferCleanup(func(ctx SpecContext) { Expect(k8sClient.Delete(ctx, ns)).To(Succeed()) })
		for _, name := range []string{node, "cache-policy-node-b"} {
			modelNode := &v1alpha1.LocalModelNode{
				ObjectMeta: metav1.ObjectMeta{Name: name},
				Spec:       v1alpha1.LocalModelNodeSpec{LocalModels: []v1alpha1.LocalModelInfo{}},
			}
			Expect(k8sClient.Create(ctx, modelNode)).To(Succeed())
			DeferCleanup(func(ctx SpecContext) { Expect(k8sClient.Delete(ctx, modelNode)).To(Succeed()) })
			Expect(k8sClient.Create(ctx, &batchv1.Job{
				ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns.Name, Labels: map[string]string{"node": name, "model": "model"}},
				Spec: batchv1.JobSpec{Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyNever,
					Containers:    []corev1.Container{{Name: "download", Image: "example.com/download:test"}},
				}}},
			})).To(Succeed())
		}
		Eventually(func(g Gomega) {
			jobs := &batchv1.JobList{}
			g.Expect(mgr.GetClient().List(ctx, jobs, client.InNamespace(ns.Name))).To(Succeed())
			g.Expect(jobs.Items).To(HaveLen(1))
			g.Expect(jobs.Items[0].Name).To(Equal(node))
			nodes := &v1alpha1.LocalModelNodeList{}
			g.Expect(mgr.GetClient().List(ctx, nodes)).To(Succeed())
			g.Expect(nodes.Items).To(HaveLen(1))
			g.Expect(nodes.Items[0].Name).To(Equal(node))
		}, 10*time.Second).Should(Succeed())
	})

	It("keeps user PVC metadata visible without starting a full PVC informer", func(ctx SpecContext) {
		metadata := &metav1.PartialObjectMetadata{TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "PersistentVolumeClaim"}}
		mgr := startCachePolicyManager(ctx, localmodelcontroller.NewCacheOptions(), localmodelcontroller.NewClientOptions(), metadata)
		ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{GenerateName: "volume-cache-"}}
		Expect(k8sClient.Create(ctx, ns)).To(Succeed())
		DeferCleanup(func(ctx SpecContext) { Expect(k8sClient.Delete(ctx, ns)).To(Succeed()) })
		pvc := &corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{Name: "user-claim", Namespace: ns.Name},
			Spec: corev1.PersistentVolumeClaimSpec{
				AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteMany},
				Resources:   corev1.VolumeResourceRequirements{Requests: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("1Gi")}},
			},
		}
		Expect(k8sClient.Create(ctx, pvc)).To(Succeed())
		key := client.ObjectKeyFromObject(pvc)
		Eventually(func() error { return mgr.GetCache().Get(ctx, key, metadata) }, 10*time.Second).Should(Succeed())
		actual := &corev1.PersistentVolumeClaim{}
		Expect(mgr.GetClient().Get(ctx, key, actual)).To(Succeed())
		Expect(actual.Spec.AccessModes).To(Equal(pvc.Spec.AccessModes))
		var notCached *cache.ErrResourceNotCached
		Expect(errors.As(mgr.GetCache().Get(ctx, key, actual), &notCached)).To(BeTrue())
	})

	It("keeps annotation-selected InferenceServices visible to KernelCache capture controllers", func(ctx SpecContext) {
		mgr := startCachePolicyManager(ctx, localmodelcontroller.NewCacheOptions(), localmodelcontroller.NewClientOptions(), &v1beta1.InferenceService{})
		ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{GenerateName: "isvc-cache-"}}
		Expect(k8sClient.Create(ctx, ns)).To(Succeed())
		DeferCleanup(func(ctx SpecContext) { Expect(k8sClient.Delete(ctx, ns)).To(Succeed()) })
		for _, name := range []string{"local-model-managed", "kernel-cache-managed"} {
			isvc := &v1beta1.InferenceService{
				ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns.Name},
				Spec: v1beta1.InferenceServiceSpec{Predictor: v1beta1.PredictorSpec{
					Model: &v1beta1.ModelSpec{
						ModelFormat: v1beta1.ModelFormat{Name: "sklearn"},
						PredictorExtensionSpec: v1beta1.PredictorExtensionSpec{
							StorageURI: ptr.To("s3://example.com/" + name),
						},
					},
				}},
			}
			if name == "local-model-managed" {
				isvc.Labels = map[string]string{constants.LocalModelLabel: "isvc-cache-model"}
			} else {
				// KernelCache capture controllers select workloads through this
				// annotation, not labels.
				isvc.Annotations = map[string]string{constants.KernelCacheSidecarInjectionAnnotationKey: "true"}
			}
			Expect(k8sClient.Create(ctx, isvc)).To(Succeed())
		}
		// hasCaptureWorkload lists InferenceServices without a label selector,
		// so the shared informer must serve annotation-selected services too.
		Eventually(func(g Gomega) {
			services := &v1beta1.InferenceServiceList{}
			g.Expect(mgr.GetCache().List(ctx, services, client.InNamespace(ns.Name))).To(Succeed())
			g.Expect(services.Items).To(HaveLen(2))
		}, 10*time.Second).Should(Succeed())
		key := client.ObjectKey{Namespace: ns.Name, Name: "kernel-cache-managed"}
		Expect(mgr.GetCache().Get(ctx, key, &v1beta1.InferenceService{})).To(Succeed())
	})

	It("revokes ephemeral registry access Secrets through direct reads", func(ctx SpecContext) {
		mgr := startCachePolicyManager(ctx, localmodelcontroller.NewCacheOptions(), localmodelcontroller.NewClientOptions())
		ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{GenerateName: "registry-cache-"}}
		Expect(k8sClient.Create(ctx, ns)).To(Succeed())
		DeferCleanup(func(ctx SpecContext) { Expect(k8sClient.Delete(ctx, ns)).To(Succeed()) })
		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "kernel-cache-source-",
				Namespace:    ns.Name,
				Annotations:  map[string]string{registryauth.AccessSecretAnnotation: "kernel-cache-access"},
			},
			Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "workload", Image: "example.com/workload:test"}}},
		}
		Expect(k8sClient.Create(ctx, pod)).To(Succeed())
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      pod.Annotations[registryauth.AccessSecretAnnotation],
				Namespace: ns.Name,
				Labels:    map[string]string{registryauth.ManagedLabel: "true"},
				OwnerReferences: []metav1.OwnerReference{{
					APIVersion: "v1", Kind: "Pod", Name: pod.Name, UID: pod.UID, Controller: ptr.To(true),
				}},
			},
			StringData: map[string]string{"token": "ephemeral"},
		}
		Expect(k8sClient.Create(ctx, secret)).To(Succeed())
		// Nothing watches Secrets in the localmodel binary, so Revoke's
		// Get+Delete must go through instead of failing strict-cache reads.
		Expect(registryauth.Revoke(ctx, mgr.GetClient(), pod)).To(Succeed())
		Eventually(func(g Gomega) {
			g.Expect(apierrors.IsNotFound(k8sClient.Get(ctx, client.ObjectKeyFromObject(secret), &corev1.Secret{}))).To(BeTrue())
		}, 10*time.Second).Should(Succeed())
	})

	It("keeps the ServingRuntime webhook readable before leadership is acquired", func(ctx SpecContext) {
		// Another replica holds leadership: the InferenceService controller
		// never starts on this manager, so controller watches cannot register
		// the ServingRuntime informer. The webhook runs on every replica.
		leaseName := "isvc-nonleader-webhook"
		lease := &coordinationv1.Lease{
			ObjectMeta: metav1.ObjectMeta{Name: leaseName, Namespace: "default"},
			Spec: coordinationv1.LeaseSpec{
				HolderIdentity:       ptr.To("leader-replica"),
				LeaseDurationSeconds: ptr.To(int32(30)),
				RenewTime:            &metav1.MicroTime{Time: time.Now().Add(10 * time.Minute)},
				LeaseTransitions:     ptr.To(int32(1)),
			},
		}
		// The suite scheme does not register the coordination API; leader
		// election itself uses a typed client, so do the same for setup.
		leaseClient, err := coordinationv1client.NewForConfig(cfg)
		Expect(err).NotTo(HaveOccurred())
		_, err = leaseClient.Leases("default").Create(ctx, lease, metav1.CreateOptions{})
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func(ctx SpecContext) {
			Expect(leaseClient.Leases("default").Delete(ctx, leaseName, metav1.DeleteOptions{})).To(Succeed())
		})

		cacheOpts, err := NewCacheOptions()
		Expect(err).NotTo(HaveOccurred())
		clientOpts, err := NewClientOptions(cfg)
		Expect(err).NotTo(HaveOccurred())
		mgr, err := ctrl.NewManager(cfg, ctrl.Options{
			Scheme: k8sClient.Scheme(), Cache: cacheOpts, Client: clientOpts,
			LeaderElection: true, LeaderElectionID: lease.Name, LeaderElectionNamespace: "default",
			// The suite manager already owns the "inferenceservice" controller name.
			Controller: config.Controller{SkipNameValidation: ptr.To(true)},
			Metrics:    metricsserver.Options{BindAddress: "0"},
		})
		Expect(err).NotTo(HaveOccurred())
		// Production wiring from suite_test.go: SetupWithManager registers the
		// controller (leader-election gated) exactly like cmd/manager.
		deployConfig := &v1beta1.DeployConfig{DefaultDeploymentMode: "Knative"}
		ingressConfig := &v1beta1.IngressConfig{
			IngressGateway:          constants.KnativeIngressGateway,
			LocalGateway:            constants.KnativeLocalGateway,
			LocalGatewayServiceName: "knative-local-gateway.istio-system.svc.cluster.local",
			DisableIstioVirtualHost: false,
		}
		Expect((&InferenceServiceReconciler{
			Client:    mgr.GetClient(),
			Scheme:    mgr.GetScheme(),
			Log:       ctrl.Log.WithName("NonLeaderWebhookTest"),
			Recorder:  record.NewFakeRecorder(10),
			Clientset: clientset,
		}).SetupWithManager(mgr, deployConfig, ingressConfig)).To(Succeed())

		validator := &servingruntime.ServingRuntimeValidator{
			Client: mgr.GetClient(), Decoder: admission.NewDecoder(mgr.GetScheme()),
		}
		nonLeaderCtx, cancelNonLeader := context.WithCancel(context.WithoutCancel(ctx))
		done := make(chan error, 1)
		go func() { done <- mgr.Start(nonLeaderCtx) }()
		DeferCleanup(func() {
			cancelNonLeader()
			Eventually(done, 10*time.Second).Should(Receive())
		})
		Expect(mgr.GetCache().WaitForCacheSync(ctx)).To(BeTrue())

		sr := &v1alpha1.ServingRuntime{
			TypeMeta:   metav1.TypeMeta{APIVersion: v1alpha1.SchemeGroupVersion.String(), Kind: "ServingRuntime"},
			ObjectMeta: metav1.ObjectMeta{Name: "nonleader-runtime", Namespace: "default"},
			Spec: v1alpha1.ServingRuntimeSpec{
				SupportedModelFormats: []v1alpha1.SupportedModelFormat{{Name: "sklearn", Priority: ptr.To(int32(1))}},
			},
		}
		raw, err := json.Marshal(sr)
		Expect(err).NotTo(HaveOccurred())
		response := validator.Handle(ctx, admission.Request{AdmissionRequest: admissionv1.AdmissionRequest{
			UID:       "isvc-nonleader-webhook-request",
			Namespace: "default",
			Operation: admissionv1.Create,
			Object:    runtime.RawExtension{Raw: raw},
		}})
		Expect(response.Result.Code).ToNot(BeEquivalentTo(http.StatusInternalServerError), "%v", response.Result.Message)
		Expect(response.Allowed).To(BeTrue(), "%v", response.Result.Message)
	})

	It("rejects undeclared informers and permit explicit direct reads in all four binaries", func(ctx SpecContext) {
		mainOptions, err := NewCacheOptions()
		Expect(err).NotTo(HaveOccurred())
		nodeOptions, err := localmodelnodecontroller.NewCacheOptions("test-node")
		Expect(err).NotTo(HaveOccurred())
		mainClient, err := NewClientOptions(cfg)
		Expect(err).NotTo(HaveOccurred())
		llmisvcClient, err := llmisvccontroller.NewClientOptions(cfg)
		Expect(err).NotTo(HaveOccurred())
		for _, policy := range []struct {
			name   string
			cache  cache.Options
			client client.Options
			scheme func(*runtime.Scheme) error
		}{
			{"manager", mainOptions, mainClient, kservescheme.AddAll},
			{"localmodel", localmodelcontroller.NewCacheOptions(), localmodelcontroller.NewClientOptions(), kservescheme.AddControllerAPIs},
			{"localmodelnode", nodeOptions, localmodelnodecontroller.NewClientOptions(), kservescheme.AddControllerAPIs},
			{"llmisvc", llmisvccontroller.NewCacheOptions(), llmisvcClient, kservescheme.AddLLMISVCAPIs},
		} {
			By(policy.name)
			Expect(policy.cache.ReaderFailOnMissingInformer).To(BeTrue())
			// Use each binary's scheme before constructing the manager, rather
			// than the envtest scheme that already registers every resource.
			scheme := runtime.NewScheme()
			Expect(policy.scheme(scheme)).To(Succeed())
			mgr, err := ctrl.NewManager(cfg, ctrl.Options{
				Scheme: scheme, Cache: policy.cache, Client: policy.client,
				Metrics: metricsserver.Options{BindAddress: "0"},
			})
			Expect(err).NotTo(HaveOccurred())
			cl := mgr.GetClient()
			// No informer was registered: this must fail immediately instead of
			// starting a cluster-wide Namespace informer on the first read.
			err = cl.Get(ctx, client.ObjectKey{Name: "default"}, &corev1.Namespace{})
			var notCached *cache.ErrResourceNotCached
			Expect(errors.As(err, &notCached)).To(BeTrue(), "%s: %v", policy.name, err)
			for _, object := range policy.client.Cache.DisableFor {
				err = cl.Get(ctx, client.ObjectKey{Name: "missing-cache-policy-object", Namespace: "default"}, object.DeepCopyObject().(client.Object))
				Expect(apierrors.IsNotFound(err)).To(BeTrue(), "%s direct read of %T: %v", policy.name, object, err)
			}
		}
	})
})

func startCachePolicyManager(ctx SpecContext, opts cache.Options, clientOpts client.Options, objects ...client.Object) ctrl.Manager {
	mgr, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme: k8sClient.Scheme(), Cache: opts, Client: clientOpts,
		Metrics: metricsserver.Options{BindAddress: "0"},
	})
	Expect(err).NotTo(HaveOccurred())
	for _, object := range objects {
		_, err = mgr.GetCache().GetInformer(ctx, object, cache.BlockUntilSynced(false))
		Expect(err).NotTo(HaveOccurred())
	}
	managerCtx, cancel := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() { done <- mgr.Start(managerCtx) }()
	DeferCleanup(func() {
		cancel()
		Eventually(done, 10*time.Second).Should(Receive(Succeed()))
	})
	Expect(mgr.GetCache().WaitForCacheSync(ctx)).To(BeTrue())
	return mgr
}
