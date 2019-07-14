package internal_test

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"
	"github.com/onsi/gomega/ghttp"
	"sigs.k8s.io/testing_frameworks/integration/addr"
	. "sigs.k8s.io/testing_frameworks/integration/internal"
)

var _ = Describe("Start method", func() {
	var (
		processState *ProcessState
	)
	BeforeEach(func() {
		processState = &ProcessState{}
		processState.Path = "bash"
		processState.Args = simpleBashScript
	})

	It("can start a process", func() {
		processState.StartTimeout = 10 * time.Second
		processState.StartMessage = "loop 5"

		err := processState.Start(nil, nil)
		Expect(err).NotTo(HaveOccurred())

		Consistently(processState.Session.ExitCode).Should(BeNumerically("==", -1))
	})

	Context("when a health check endpoint is provided", func() {
		var server *ghttp.Server
		BeforeEach(func() {
			server = ghttp.NewServer()
		})
		AfterEach(func() {
			server.Close()
		})

		Context("when the healthcheck returns ok", func() {
			BeforeEach(func() {
				server.RouteToHandler("GET", "/healthz", ghttp.RespondWith(http.StatusOK, ""))
			})

			It("hits the endpoint, and successfully starts", func() {
				processState.HealthCheckEndpoint = "/healthz"
				processState.StartTimeout = 100 * time.Millisecond
				processState.URL = getServerURL(server)

				err := processState.Start(nil, nil)
				Expect(err).NotTo(HaveOccurred())
				Expect(server.ReceivedRequests()).To(HaveLen(1))
				Consistently(processState.Session.ExitCode).Should(BeNumerically("==", -1))
			})
		})

		Context("when the healthcheck always returns failure", func() {
			BeforeEach(func() {
				server.RouteToHandler("GET", "/healthz", ghttp.RespondWith(http.StatusInternalServerError, ""))
			})
			It("returns a timeout error and stops health API checker", func() {
				processState.HealthCheckEndpoint = "/healthz"
				processState.StartTimeout = 500 * time.Millisecond
				processState.URL = getServerURL(server)

				err := processState.Start(nil, nil)
				Expect(err).To(MatchError(ContainSubstring("timeout")))

				nrReceivedRequests := len(server.ReceivedRequests())
				Expect(nrReceivedRequests).To(Equal(5))
				time.Sleep(200 * time.Millisecond)
				Expect(nrReceivedRequests).To(Equal(5))
			})
		})

		Context("when the healthcheck isn't even listening", func() {
			BeforeEach(func() {
				server.Close()
			})

			It("returns a timeout error", func() {
				processState.HealthCheckEndpoint = "/healthz"
				processState.StartTimeout = 500 * time.Millisecond

				port, host, err := addr.Suggest()
				Expect(err).NotTo(HaveOccurred())

				processState.URL = url.URL{
					Scheme: "http",
					Host:   fmt.Sprintf("%s:%d", host, port),
				}

				err = processState.Start(nil, nil)
				Expect(err).To(MatchError(ContainSubstring("timeout")))
			})
		})

		Context("when the healthcheck fails initially but succeeds eventually", func() {
			BeforeEach(func() {
				server.AppendHandlers(
					ghttp.RespondWith(http.StatusInternalServerError, ""),
					ghttp.RespondWith(http.StatusInternalServerError, ""),
					ghttp.RespondWith(http.StatusInternalServerError, ""),
					ghttp.RespondWith(http.StatusOK, ""),
				)
			})

			It("hits the endpoint repeatedly, and successfully starts", func() {
				processState.HealthCheckEndpoint = "/healthz"
				processState.StartTimeout = 20 * time.Second
				processState.URL = getServerURL(server)

				err := processState.Start(nil, nil)
				Expect(err).NotTo(HaveOccurred())
				Expect(server.ReceivedRequests()).To(HaveLen(4))
				Consistently(processState.Session.ExitCode).Should(BeNumerically("==", -1))
			})

			Context("when the polling interval is not configured", func() {
				It("uses the default interval for polling", func() {
					processState.HealthCheckEndpoint = "/helathz"
					processState.StartTimeout = 300 * time.Millisecond
					processState.URL = getServerURL(server)

					Expect(processState.Start(nil, nil)).To(MatchError(ContainSubstring("timeout")))
					Expect(server.ReceivedRequests()).To(HaveLen(3))
				})
			})

			Context("when the polling interval is configured", func() {
				BeforeEach(func() {
					processState.HealthCheckPollInterval = time.Millisecond * 20
				})

				It("hits the endpoint in the configured interval", func() {
					processState.HealthCheckEndpoint = "/healthz"
					processState.StartTimeout = 3 * processState.HealthCheckPollInterval
					processState.URL = getServerURL(server)

					Expect(processState.Start(nil, nil)).To(MatchError(ContainSubstring("timeout")))
					Expect(server.ReceivedRequests()).To(HaveLen(3))
				})
			})
		})
	})

	Context("when a health check endpoint is not provided", func() {

		Context("when process takes too long to start", func() {
			It("returns a timeout error", func() {
				processState.StartTimeout = 200 * time.Millisecond
				processState.StartMessage = "loop 5000"

				err := processState.Start(nil, nil)
				Expect(err).To(MatchError(ContainSubstring("timeout")))

				Eventually(processState.Session.ExitCode).Should(Equal(143))
			})
		})

		Context("when the command cannot be started", func() {
			BeforeEach(func() {
				processState = &ProcessState{}
				processState.Path = "/nonexistent"
			})

			It("propagates the error", func() {
				err := processState.Start(nil, nil)

				Expect(os.IsNotExist(err)).To(BeTrue())
			})

			Context("but Stop() is called on it", func() {
				It("does not panic", func() {
					processState.Start(nil, nil)

					stoppingFailedProcess := func() {
						Expect(processState.Stop()).To(Succeed())
					}

					Expect(stoppingFailedProcess).NotTo(Panic())
				})
			})
		})
	})

	Context("when IO is configured", func() {
		It("can inspect stdout & stderr", func() {
			stdout := &bytes.Buffer{}
			stderr := &bytes.Buffer{}

			processState.Args = []string{
				"-c",
				`
					echo 'this is stderr' >&2
					echo 'that is stdout'
					echo 'i started' >&2
				`,
			}
			processState.StartMessage = "i started"
			processState.StartTimeout = 1 * time.Second

			Expect(processState.Start(stdout, stderr)).To(Succeed())

			Expect(stdout.String()).To(Equal("that is stdout\n"))
			Expect(stderr.String()).To(Equal("this is stderr\ni started\n"))
		})
	})
})

var _ = Describe("Stop method", func() {
	Context("when Stop() is called", func() {
		var (
			processState *ProcessState
		)
		BeforeEach(func() {
			var err error
			processState = &ProcessState{}
			processState.Session, err = gexec.Start(getSimpleCommand(), nil, nil)
			Expect(err).NotTo(HaveOccurred())
			processState.StopTimeout = 10 * time.Second
		})

		It("stops the process", func() {
			Expect(processState.Stop()).To(Succeed())
		})

		Context("multiple times", func() {
			It("does not error or panic on consecutive calls", func() {
				stoppingTheProcess := func() {
					Expect(processState.Stop()).To(Succeed())
				}
				Expect(stoppingTheProcess).NotTo(Panic())
				Expect(stoppingTheProcess).NotTo(Panic())
				Expect(stoppingTheProcess).NotTo(Panic())
			})
		})
	})

	Context("when the command cannot be stopped", func() {
		It("returns a timeout error", func() {
			var err error

			processState := &ProcessState{}
			processState.Session, err = gexec.Start(getSimpleCommand(), nil, nil)
			Expect(err).NotTo(HaveOccurred())
			processState.Session.Exited = make(chan struct{})
			processState.StopTimeout = 200 * time.Millisecond

			Expect(processState.Stop()).To(MatchError(ContainSubstring("timeout")))
		})
	})

	Context("when the directory needs to be cleaned up", func() {
		It("removes the directory", func() {
			var err error

			processState := &ProcessState{}
			processState.Session, err = gexec.Start(getSimpleCommand(), nil, nil)
			Expect(err).NotTo(HaveOccurred())
			processState.Dir, err = ioutil.TempDir("", "k8s_test_framework_")
			Expect(err).NotTo(HaveOccurred())
			processState.DirNeedsCleaning = true
			processState.StopTimeout = 400 * time.Millisecond

			Expect(processState.Stop()).To(Succeed())
			Expect(processState.Dir).NotTo(BeAnExistingFile())
		})
	})
})

var _ = Describe("DoDefaulting", func() {
	Context("when all inputs are provided", func() {
		It("passes them through", func() {
			defaults, err := DoDefaulting(
				"some name",
				&url.URL{Host: "some.host.to.listen.on"},
				"/some/dir",
				"/some/path/to/some/bin",
				20*time.Hour,
				65537*time.Millisecond,
			)
			Expect(err).NotTo(HaveOccurred())

			Expect(defaults.URL).To(Equal(url.URL{Host: "some.host.to.listen.on"}))
			Expect(defaults.Dir).To(Equal("/some/dir"))
			Expect(defaults.DirNeedsCleaning).To(BeFalse())
			Expect(defaults.Path).To(Equal("/some/path/to/some/bin"))
			Expect(defaults.StartTimeout).To(Equal(20 * time.Hour))
			Expect(defaults.StopTimeout).To(Equal(65537 * time.Millisecond))
		})
	})

	Context("when inputs are empty", func() {
		It("defaults them", func() {
			defaults, err := DoDefaulting(
				"some name",
				nil,
				"",
				"",
				0,
				0,
			)
			Expect(err).NotTo(HaveOccurred())

			Expect(defaults.Dir).To(BeADirectory())
			Expect(os.RemoveAll(defaults.Dir)).To(Succeed())
			Expect(defaults.DirNeedsCleaning).To(BeTrue())

			Expect(defaults.URL).NotTo(BeZero())
			Expect(defaults.URL.Scheme).To(Equal("http"))
			Expect(defaults.URL.Hostname()).NotTo(BeEmpty())
			Expect(defaults.URL.Port()).NotTo(BeEmpty())

			Expect(defaults.Path).NotTo(BeEmpty())

			Expect(defaults.StartTimeout).NotTo(BeZero())
			Expect(defaults.StopTimeout).NotTo(BeZero())
		})
	})

	Context("when neither name nor path are provided", func() {
		It("returns an error", func() {
			_, err := DoDefaulting(
				"",
				nil,
				"",
				"",
				0,
				0,
			)
			Expect(err).To(MatchError("must have at least one of name or path"))
		})
	})
})

var simpleBashScript = []string{
	"-c",
	`
		i=0
		while true
		do
			echo "loop $i" >&2
			let 'i += 1'
			sleep 0.2
		done
	`,
}

func getSimpleCommand() *exec.Cmd {
	return exec.Command("bash", simpleBashScript...)
}

func getServerURL(server *ghttp.Server) url.URL {
	url, err := url.Parse(server.URL())
	Expect(err).NotTo(HaveOccurred())
	return *url
}
