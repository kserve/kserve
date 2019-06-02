package handler

import (
	"bytes"
	"fmt"
	"github.com/onsi/gomega"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"testing"
)

func TestExecutor(t *testing.T) {

	g := gomega.NewGomegaWithT(t)

	predictorResponse := []byte(`{"instances":[[4,5,6]]}`)
	// Start a local HTTP server
	preprocessor := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		rw.Write([]byte(`{"instances":[[1,2,3]]}`))
	}))
	// Close the server when test finishes
	defer preprocessor.Close()

	// Start a local HTTP server
	predictor := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		rw.Write(predictorResponse)
	}))
	// Close the server when test finishes
	defer predictor.Close()

	b := []byte(`{"instances":[[0,0,0]]}`)
	reader := bytes.NewReader(b)
	r := httptest.NewRequest("POST", "http://a", reader)
	w := httptest.NewRecorder()

	logf.SetLogger(logf.ZapLogger(false))
	log := logf.Log.WithName("entrypoint")

	preprocessUrl, _ := url.Parse(preprocessor.URL)
	predictorUrl, _ := url.Parse(predictor.URL)

	oh := New(log, preprocessUrl.Host, predictorUrl.Host, "")

	oh.ServeHTTP(w, r)

	b2, _ := ioutil.ReadAll(w.Result().Body)
	fmt.Printf("%s", string(b2))

	g.Expect(b2).To(gomega.Equal(predictorResponse))

}
