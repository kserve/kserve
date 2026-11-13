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

package logger

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/onsi/gomega"
	"go.uber.org/zap/zaptest"
)

const testCertName = "ca.crt"

// newSelfSignedCert returns a certificate valid for 127.0.0.1 together with
// its PEM encoding, so a test can both serve it and trust it as a CA.
func newSelfSignedCert(t *testing.T, commonName string) (tls.Certificate, []byte) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generating key: %v", err)
	}
	template := &x509.Certificate{
		SerialNumber:          big.NewInt(time.Now().UnixNano()),
		Subject:               pkix.Name{CommonName: commonName},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
		IPAddresses:           []net.IP{net.IPv4(127, 0, 0, 1)},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("creating certificate: %v", err)
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key},
		pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
}

// rotatingTLSServer is an HTTPS server whose serving certificate can be
// swapped at runtime. It counts the events it received and the connections it
// saw open and close.
//
// Tests assert on received events rather than on sendHttpCloudEvent's error,
// because a failed TLS handshake is only logged, not returned.
type rotatingTLSServer struct {
	*httptest.Server
	cert     atomic.Pointer[tls.Certificate]
	received atomic.Int64
	opened   atomic.Int64
	closed   atomic.Int64
}

func newRotatingTLSServer(t *testing.T, initial tls.Certificate, keepAlives bool) *rotatingTLSServer {
	t.Helper()
	s := &rotatingTLSServer{}
	s.cert.Store(&initial)
	s.Server = httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		s.received.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	s.Config.SetKeepAlivesEnabled(keepAlives)
	s.Config.ConnState = func(_ net.Conn, state http.ConnState) {
		switch state {
		case http.StateNew:
			s.opened.Add(1)
		case http.StateClosed:
			s.closed.Add(1)
		default:
		}
	}
	// GetConfigForClient rather than GetCertificate: the latter is ignored
	// when the config has static certificates (httptest adds its own) and the
	// client sends no SNI, as is the case when dialing an IP.
	s.TLS = &tls.Config{
		MinVersion: tls.VersionTLS12,
		GetConfigForClient: func(*tls.ClientHelloInfo) (*tls.Config, error) {
			return &tls.Config{
				MinVersion:   tls.VersionTLS12,
				Certificates: []tls.Certificate{*s.cert.Load()},
			}, nil
		},
	}
	s.StartTLS()
	t.Cleanup(s.Close)
	return s
}

func newTLSTestWorker(t *testing.T, caDir string) *Worker {
	t.Helper()
	w := NewWorker(0, nil, zaptest.NewLogger(t).Sugar())
	w.caCertDir = caDir
	return &w
}

func newTLSLogRequest(t *testing.T, target string) LogRequest {
	t.Helper()
	u, err := url.Parse(target)
	if err != nil {
		t.Fatalf("parsing %q: %v", target, err)
	}
	body := []byte("{}")
	return LogRequest{
		Url:            u,
		SourceUri:      u,
		Bytes:          &body,
		ContentType:    "application/json",
		ReqType:        CEInferenceRequest,
		Id:             "test-id",
		CertName:       testCertName,
		OccurrenceTime: time.Now(),
	}
}

func writeCACert(t *testing.T, dir string, pemBytes []byte) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, testCertName), pemBytes, 0o600); err != nil {
		t.Fatalf("writing CA cert: %v", err)
	}
}

// TestNewWorkerKeepsBackwardsCompatibleSignature guards against making the
// HTTP client timeout a mandatory NewWorker argument: existing callers that
// pass only (id, queue, logger) must keep compiling and get the default.
func TestNewWorkerKeepsBackwardsCompatibleSignature(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	w := NewWorker(1, nil, zaptest.NewLogger(t).Sugar())

	g.Expect(w.httpClientTimeout).To(gomega.Equal(DefaultHTTPClientTimeout))
}

func TestNewWorkerAppliesHTTPClientTimeoutOption(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	w := NewWorker(1, nil, zaptest.NewLogger(t).Sugar(), WithHTTPClientTimeout(3*time.Second))

	g.Expect(w.httpClientTimeout).To(gomega.Equal(3 * time.Second))
}

func TestNewWorkerFallsBackToDefaultTimeoutForNonPositiveOption(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	w := NewWorker(1, nil, zaptest.NewLogger(t).Sugar(), WithHTTPClientTimeout(0))

	g.Expect(w.httpClientTimeout).To(gomega.Equal(DefaultHTTPClientTimeout))
}

// A CA file that is absent on the first event must not pin the worker to the
// default system roots: once the file shows up, delivery has to recover, as it
// did when the file was re-read for every event.
func TestWorkerRecoversWhenCACertAppearsAfterFirstEvent(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	cert, certPEM := newSelfSignedCert(t, "ca-a")
	srv := newRotatingTLSServer(t, cert, true)
	caDir := t.TempDir()
	worker := newTLSTestWorker(t, caDir)
	req := newTLSLogRequest(t, srv.URL)

	g.Expect(worker.sendHttpCloudEvent(req)).To(gomega.Succeed())
	g.Expect(srv.received.Load()).To(gomega.BeZero(),
		"the server's certificate is not trusted by the default roots")

	writeCACert(t, caDir, certPEM)

	g.Expect(worker.sendHttpCloudEvent(req)).To(gomega.Succeed())
	g.Expect(srv.received.Load()).To(gomega.BeEquivalentTo(1))
}

// A rotated CA file has to be picked up without restarting the worker.
// Keep-alives are off so every event performs a fresh TLS handshake against
// whatever roots the worker's client currently holds.
func TestWorkerPicksUpRotatedCACert(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	certA, pemA := newSelfSignedCert(t, "ca-a")
	certB, pemB := newSelfSignedCert(t, "ca-b")
	srv := newRotatingTLSServer(t, certA, false)
	caDir := t.TempDir()
	writeCACert(t, caDir, pemA)
	worker := newTLSTestWorker(t, caDir)
	req := newTLSLogRequest(t, srv.URL)

	g.Expect(worker.sendHttpCloudEvent(req)).To(gomega.Succeed())
	g.Expect(srv.received.Load()).To(gomega.BeEquivalentTo(1))

	srv.cert.Store(&certB)
	writeCACert(t, caDir, pemB)

	g.Expect(worker.sendHttpCloudEvent(req)).To(gomega.Succeed())
	g.Expect(srv.received.Load()).To(gomega.BeEquivalentTo(2), "the event after the rotation was not delivered")
}

// When the CA file changes, the connections of the client built from the old
// one must be closed instead of lingering in the idle pool.
func TestWorkerClosesConnectionsOfReplacedClient(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	certA, pemA := newSelfSignedCert(t, "ca-a")
	certB, pemB := newSelfSignedCert(t, "ca-b")
	srv := newRotatingTLSServer(t, certA, true)
	caDir := t.TempDir()
	writeCACert(t, caDir, pemA)
	worker := newTLSTestWorker(t, caDir)
	req := newTLSLogRequest(t, srv.URL)

	g.Expect(worker.sendHttpCloudEvent(req)).To(gomega.Succeed())
	g.Expect(srv.opened.Load()).To(gomega.BeEquivalentTo(1))

	srv.cert.Store(&certB)
	writeCACert(t, caDir, pemB)
	g.Expect(worker.sendHttpCloudEvent(req)).To(gomega.Succeed())

	g.Expect(srv.received.Load()).To(gomega.BeEquivalentTo(2))
	g.Expect(srv.opened.Load()).To(gomega.BeEquivalentTo(2), "the rotated CA needs a new connection")
	g.Eventually(srv.closed.Load, 2*time.Second, 10*time.Millisecond).Should(gomega.BeNumerically(">=", 1),
		"the replaced client's idle connection should have been closed")
}

// Re-reading the CA file for every event must not defeat connection reuse
// when its content has not changed.
func TestWorkerReusesTLSConnectionWhileCACertIsUnchanged(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	cert, certPEM := newSelfSignedCert(t, "ca-a")
	srv := newRotatingTLSServer(t, cert, true)
	caDir := t.TempDir()
	writeCACert(t, caDir, certPEM)
	worker := newTLSTestWorker(t, caDir)
	req := newTLSLogRequest(t, srv.URL)

	for range 10 {
		g.Expect(worker.sendHttpCloudEvent(req)).To(gomega.Succeed())
	}

	g.Expect(srv.received.Load()).To(gomega.BeEquivalentTo(10))
	g.Expect(srv.opened.Load()).To(gomega.BeEquivalentTo(1))
}
