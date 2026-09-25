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
	"errors"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	"github.com/kserve/kserve/pkg/constants"
	localmodelcontroller "github.com/kserve/kserve/pkg/controller/v1alpha1/localmodel"
	localmodelnodecontroller "github.com/kserve/kserve/pkg/controller/v1alpha1/localmodelnode"
	llmisvccontroller "github.com/kserve/kserve/pkg/controller/v1alpha2/llmisvc"
	kservescheme "github.com/kserve/kserve/pkg/scheme"
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

	It("allows KernelCache prefetch ServiceAccount reads without an informer", func(ctx SpecContext) {
		mgr := startCachePolicyManager(ctx, localmodelcontroller.NewCacheOptions(), localmodelcontroller.NewClientOptions())
		key := client.ObjectKey{Namespace: "default", Name: "kernel-cache-prefetcher"}
		Expect(apierrors.IsNotFound(mgr.GetClient().Get(ctx, key, &corev1.ServiceAccount{}))).To(BeTrue())
		var notCached *cache.ErrResourceNotCached
		Expect(errors.As(mgr.GetCache().Get(ctx, key, &corev1.ServiceAccount{}), &notCached)).To(BeTrue())
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

	It("rejects undeclared informers and permits explicit direct reads in all four binaries", func(ctx SpecContext) {
		mainOptions, err := NewCacheOptions()
		Expect(err).NotTo(HaveOccurred())
		nodeOptions, err := localmodelnodecontroller.NewCacheOptions("test-node")
		Expect(err).NotTo(HaveOccurred())
		for _, policy := range []struct {
			name   string
			cache  cache.Options
			client client.Options
			scheme func(*runtime.Scheme) error
		}{
			{"manager", mainOptions, NewClientOptions(), kservescheme.AddAll},
			{"localmodel", localmodelcontroller.NewCacheOptions(), localmodelcontroller.NewClientOptions(), kservescheme.AddControllerAPIs},
			{"localmodelnode", nodeOptions, localmodelnodecontroller.NewClientOptions(), kservescheme.AddControllerAPIs},
			{"llmisvc", llmisvccontroller.NewCacheOptions(), llmisvccontroller.NewClientOptions(), kservescheme.AddLLMISVCAPIs},
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
