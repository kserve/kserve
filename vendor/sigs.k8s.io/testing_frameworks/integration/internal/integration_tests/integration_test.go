package integration_tests

import (
	"fmt"
	"io/ioutil"
	"net"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/testing_frameworks/integration"
)

var _ = Describe("The Testing Framework", func() {
	var controlPlane *integration.ControlPlane

	AfterEach(func() {
		Expect(controlPlane.Stop()).To(Succeed())
	})

	It("Successfully manages the control plane lifecycle", func() {
		var err error

		controlPlane = &integration.ControlPlane{}

		By("Starting all the control plane processes")
		err = controlPlane.Start()
		Expect(err).NotTo(HaveOccurred(), "Expected controlPlane to start successfully")

		apiServerURL := controlPlane.APIURL()
		etcdClientURL := controlPlane.APIServer.EtcdURL

		isEtcdListeningForClients := isSomethingListeningOnPort(etcdClientURL.Host)
		isAPIServerListening := isSomethingListeningOnPort(apiServerURL.Host)

		By("Ensuring Etcd is listening")
		Expect(isEtcdListeningForClients()).To(BeTrue(),
			fmt.Sprintf("Expected Etcd to listen for clients on %s,", etcdClientURL.Host))

		By("Ensuring APIServer is listening")
		CheckAPIServerIsReady(controlPlane.KubeCtl())

		By("getting a kubectl & run it against the control plane")
		kubeCtl := controlPlane.KubeCtl()
		stdout, stderr, err := kubeCtl.Run("get", "pods")
		Expect(err).NotTo(HaveOccurred())
		bytes, err := ioutil.ReadAll(stdout)
		Expect(err).NotTo(HaveOccurred())
		Expect(bytes).To(BeEmpty())
		Expect(stderr).To(ContainSubstring("No resources found."))

		By("Stopping all the control plane processes")
		err = controlPlane.Stop()
		Expect(err).NotTo(HaveOccurred(), "Expected controlPlane to stop successfully")

		By("Ensuring Etcd is not listening anymore")
		Expect(isEtcdListeningForClients()).To(BeFalse(), "Expected Etcd not to listen for clients anymore")

		By("Ensuring APIServer is not listening anymore")
		Expect(isAPIServerListening()).To(BeFalse(), "Expected APIServer not to listen anymore")

		By("Not erroring when stopping a stopped ControlPlane")
		Expect(func() {
			Expect(controlPlane.Stop()).To(Succeed())
		}).NotTo(Panic())
	})

	Context("when Stop() is called on the control plane", func() {
		Context("but the control plane is not started yet", func() {
			It("does not error", func() {
				controlPlane = &integration.ControlPlane{}

				stoppingTheControlPlane := func() {
					Expect(controlPlane.Stop()).To(Succeed())
				}

				Expect(stoppingTheControlPlane).NotTo(Panic())
			})
		})
	})

	Context("when the control plane is configured with its components", func() {
		It("it does not default them", func() {
			myEtcd, myAPIServer :=
				&integration.Etcd{StartTimeout: 15 * time.Second},
				&integration.APIServer{StopTimeout: 16 * time.Second}

			controlPlane = &integration.ControlPlane{
				Etcd:      myEtcd,
				APIServer: myAPIServer,
			}

			Expect(controlPlane.Start()).To(Succeed())
			Expect(controlPlane.Etcd).To(BeIdenticalTo(myEtcd))
			Expect(controlPlane.APIServer).To(BeIdenticalTo(myAPIServer))
			Expect(controlPlane.Etcd.StartTimeout).To(Equal(15 * time.Second))
			Expect(controlPlane.APIServer.StopTimeout).To(Equal(16 * time.Second))
		})
	})

	Measure("It should be fast to bring up and tear down the control plane", func(b Benchmarker) {
		b.Time("lifecycle", func() {
			controlPlane = &integration.ControlPlane{}

			controlPlane.Start()
			controlPlane.Stop()
		})
	}, 10)
})

type portChecker func() bool

func isSomethingListeningOnPort(hostAndPort string) portChecker {
	return func() bool {
		conn, err := net.DialTimeout("tcp", hostAndPort, 1*time.Second)

		if err != nil {
			return false
		}
		conn.Close()
		return true
	}
}

// CheckAPIServerIsReady checks if the APIServer is really ready and not only
// listening.
//
// While porting some tests in k/k
// (https://github.com/hoegaarden/kubernetes/blob/287fdef1bd98646bc521f4433c1009936d5cf7a2/hack/make-rules/test-cmd-util.sh#L1524-L1535)
// we found, that the APIServer was
// listening but not serving certain APIs yet.
//
// We changed the readiness detection in the PR at
// https://github.com/kubernetes-sigs/testing_frameworks/pull/48. To confirm
// this changed behaviour does what it should do, we used the same test as in
// k/k's test-cmd (see link above) and test if certain well-known known APIs
// are actually available.
func CheckAPIServerIsReady(kubeCtl *integration.KubeCtl) {
	expectedAPIS := []string{
		"/api/v1/namespaces/default/pods 200 OK",
		"/api/v1/namespaces/default/replicationcontrollers 200 OK",
		"/api/v1/namespaces/default/services 200 OK",
		"/apis/apps/v1/namespaces/default/daemonsets 200 OK",
		"/apis/apps/v1/namespaces/default/deployments 200 OK",
		"/apis/apps/v1/namespaces/default/replicasets 200 OK",
		"/apis/apps/v1/namespaces/default/statefulsets 200 OK",
		"/apis/autoscaling/v1/namespaces/default/horizontalpodautoscalers 200",
		"/apis/batch/v1/namespaces/default/jobs 200 OK",
	}

	_, output, err := kubeCtl.Run("--v=6", "--namespace", "default", "get", "all", "--chunk-size=0")
	ExpectWithOffset(1, err).NotTo(HaveOccurred())

	stdoutBytes, err := ioutil.ReadAll(output)
	ExpectWithOffset(1, err).NotTo(HaveOccurred())

	for _, api := range expectedAPIS {
		ExpectWithOffset(1, string(stdoutBytes)).To(ContainSubstring(api))
	}
}
