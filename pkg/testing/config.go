/*
Copyright 2023 The KServe Authors.

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

package testing

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"time"

	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	"sigs.k8s.io/controller-runtime/pkg/webhook"

	"google.golang.org/protobuf/proto"

	utilruntime "k8s.io/apimachinery/pkg/util/runtime"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	"go.uber.org/zap/zapcore"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

type Config struct {
	ctrlSetupFuncs     []SetupFunc
	webhooksSetupFuncs []SetupFunc
	envTestOptions     []Option
	mgrOptions         []ManagerOption
}

// Client acts as a facade for setting up k8s envtest. It allows to wire controllers under tests through
// a simple builder funcs and configure underlying test environment through Option functions.
// It's composed of k8s client.Client and Cleaner to provide unified way of manipulating resources it the env test cluster.
type Client struct {
	client.Client
	*envtest.Environment
	*Cleaner
}

// Configure creates a new configuration for the Kubernetes EnvTest.
func Configure(options ...Option) *Config {
	return &Config{
		envTestOptions: options,
	}
}

func (c *Client) UsingExistingCluster() bool {
	envValue, exists := os.LookupEnv("USE_EXISTING_CLUSTER")
	if exists {
		return strings.EqualFold(envValue, "true")
	}

	return ptr.Deref(c.UseExistingCluster, false)
}

// WithControllers register controllers under tests required for the test suite.
func (e *Config) WithControllers(setupFunc ...SetupFunc) *Config {
	e.ctrlSetupFuncs = append(e.ctrlSetupFuncs, setupFunc...)

	return e
}

// WithWebhooks register webhooks under tests required for the test suite.
func (e *Config) WithWebhooks(setupFunc ...SetupFunc) *Config {
	e.webhooksSetupFuncs = append(e.webhooksSetupFuncs, setupFunc...)

	return e
}

// WithManagerOptions allows customizing the controller-runtime manager options (e.g., cache configuration).
func (e *Config) WithManagerOptions(opts ...ManagerOption) *Config {
	e.mgrOptions = append(e.mgrOptions, opts...)

	return e
}

// Start wires controller-runtime manager with controllers which are subject of the tests
// and starts Kubernetes EnvTest to verify their behavior.
func (e *Config) Start(ctx context.Context) *Client {
	ctx, cancel := context.WithCancel(ctx)

	opts := zap.Options{
		Development: true,
		TimeEncoder: zapcore.TimeEncoderOfLayout(time.RFC3339),
	}
	logf.SetLogger(zap.New(zap.WriteTo(ginkgo.GinkgoWriter), zap.UseFlagOptions(&opts)))

	envTest := &envtest.Environment{
		CRDInstallOptions: envtest.CRDInstallOptions{
			ErrorIfPathMissing: true,
			CleanUpAfterUse:    true,
			// The controller-runtime default is 10s, which is too short under
			// cross-suite parallel load in this repository.
			MaxTime:      2 * time.Minute,
			PollInterval: 250 * time.Millisecond,
		},
		UseExistingCluster: proto.Bool(false), // TODO(testenv): make it configurable
	}

	for _, opt := range e.envTestOptions {
		opt(envTest)
	}

	// Take exclusive ownership of SIGINT/SIGTERM so that envtest child
	// processes (etcd, kube-apiserver) are force-killed before the test
	// process exits. signal.Reset deregisters Ginkgo's handler — this is
	// intentional: if Ginkgo receives a second interrupt it may os.Exit()
	// before our cleanup goroutine finishes, leaving orphaned processes.
	//
	// The watcher must be registered before envTest.Start() so interrupts arriving
	// mid-startup (e.g. etcd already up, kube-apiserver not yet) are handled.
	sigCh := make(chan os.Signal, 1)
	stopSignalWatcher := make(chan struct{})
	signal.Reset(os.Interrupt, syscall.SIGTERM)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		select {
		case <-stopSignalWatcher:
			return
		case sig := <-sigCh:
			cancel()
			stopControlPlane(&envTest.ControlPlane)
			// Re-raise with default behavior so test process exits with the right status.
			signal.Reset(sig)
			p, _ := os.FindProcess(os.Getpid())
			_ = p.Signal(sig)
		}
	}()

	cleanupOnce := &sync.Once{}
	cleanup := func(assertStopSuccess bool) {
		cleanupOnce.Do(func() {
			// Stop receiving process signals and release this setup context.
			signal.Stop(sigCh)
			close(stopSignalWatcher)
			cancel()
			if assertStopSuccess {
				gomega.Expect(envTest.Stop()).To(gomega.Succeed())
				return
			}
			// Start() can fail after partially launching the control plane.
			// Best-effort force cleanup avoids orphaned etcd/apiserver.
			stopControlPlane(&envTest.ControlPlane)
		})
	}
	ginkgo.DeferCleanup(func() {
		cleanup(true)
	})

	cfg, errStart := envTest.Start()
	if errStart != nil {
		cleanup(false)
	}
	gomega.Expect(errStart).NotTo(gomega.HaveOccurred())
	gomega.Expect(cfg).NotTo(gomega.BeNil())

	cli, errClient := client.New(cfg, client.Options{Scheme: envTest.Scheme})
	gomega.Expect(errClient).NotTo(gomega.HaveOccurred())
	gomega.Expect(cli).NotTo(gomega.BeNil())

	mgrOptions := ctrl.Options{
		Scheme: envTest.Scheme,
		Metrics: metricsserver.Options{
			BindAddress: "0",
		},
		LeaderElection: false,
	}

	for _, opt := range e.mgrOptions {
		opt(&mgrOptions)
	}

	if len(e.webhooksSetupFuncs) > 0 {
		webhookOptions := webhook.Options{
			Port:    envTest.WebhookInstallOptions.LocalServingPort,
			Host:    envTest.WebhookInstallOptions.LocalServingHost,
			CertDir: envTest.WebhookInstallOptions.LocalServingCertDir,
		}
		mgrOptions.WebhookServer = webhook.NewServer(webhookOptions)
	}

	mgr, errMgr := ctrl.NewManager(cfg, mgrOptions)
	gomega.Expect(errMgr).NotTo(gomega.HaveOccurred())

	for _, setupFunc := range e.ctrlSetupFuncs {
		errSetup := setupFunc(cfg, mgr)
		gomega.Expect(errSetup).NotTo(gomega.HaveOccurred())
	}

	for _, webhookSetupFunc := range e.webhooksSetupFuncs {
		errSetup := webhookSetupFunc(cfg, mgr)
		gomega.Expect(errSetup).NotTo(gomega.HaveOccurred())
	}

	go func() {
		defer ginkgo.GinkgoRecover()
		gomega.Expect(mgr.Start(ctx)).To(gomega.Succeed(), "Failed to start manager")
	}()

	if len(e.webhooksSetupFuncs) > 0 {
		// wait for the webhook server to get ready
		dialer := &net.Dialer{Timeout: time.Second}
		addrPort := fmt.Sprintf("%s:%d", envTest.WebhookInstallOptions.LocalServingHost, envTest.WebhookInstallOptions.LocalServingPort)
		gomega.Eventually(func() error {
			conn, err := tls.DialWithDialer(dialer, "tcp", addrPort, &tls.Config{InsecureSkipVerify: true}) //nolint:gosec //reason testing infra code.
			if err != nil {
				return err
			}

			return conn.Close()
		}).Should(gomega.Succeed())
	}

	return &Client{
		Client:      cli,
		Cleaner:     CreateCleaner(cli, cfg, 10*time.Second, 250*time.Millisecond),
		Environment: envTest,
	}
}

// stopControlPlane performs best-effort cleanup of the envtest control plane.
// It first attempts to kill processes by command-line pattern as a defense against
// interrupts arriving mid-startup, when controller-runtime's internal process
// handles may not yet be populated (processState is unexported).
// ControlPlane.Stop() is called afterwards to clean up temporary directories.
func stopControlPlane(controlPlane *envtest.ControlPlane) {
	if controlPlane.APIServer != nil {
		forceKillByPattern(controlPlane.APIServer.Path, controlPlane.APIServer.CertDir)
	}
	if controlPlane.Etcd != nil {
		forceKillByPattern(controlPlane.Etcd.Path, controlPlane.Etcd.DataDir)
	}

	_ = controlPlane.Stop()
}

// forceKillByPattern kills processes matching the given binary path and unique argument.
// This is a Linux-specific fallback for envtest cleanup when process handles are unavailable.
func forceKillByPattern(binaryPath, uniqueArg string) {
	if binaryPath == "" {
		return
	}

	pattern := regexp.QuoteMeta(binaryPath)
	if uniqueArg != "" {
		pattern = pattern + ".*" + regexp.QuoteMeta(uniqueArg)
	}

	_ = exec.Command("pkill", "-9", "-f", pattern).Run() //nolint:gosec // test infrastructure only.
}

type Option func(target *envtest.Environment)

// WithCRDs adds CRDs to the test environment using paths.
func WithCRDs(paths ...string) Option {
	return func(target *envtest.Environment) {
		target.CRDInstallOptions.Paths = append(target.CRDInstallOptions.Paths, paths...)
	}
}

// WithWebhookManifests adds CRDs to the test environment using paths.
func WithWebhookManifests(paths ...string) Option {
	return func(target *envtest.Environment) {
		seen := make(map[string]bool)
		for _, p := range target.WebhookInstallOptions.Paths {
			seen[p] = true
		}
		for _, p := range paths {
			if !seen[p] {
				target.WebhookInstallOptions.Paths = append(target.WebhookInstallOptions.Paths, p)
			}
		}
	}
}

// WithScheme sets the scheme for the test environment.
func WithScheme(addToScheme ...AddToSchemeFunc) Option {
	return func(target *envtest.Environment) {
		testScheme := runtime.NewScheme()
		for _, add := range addToScheme {
			utilruntime.Must(add(testScheme))
		}
		target.Scheme = testScheme
		target.CRDInstallOptions.Scheme = testScheme
	}
}
