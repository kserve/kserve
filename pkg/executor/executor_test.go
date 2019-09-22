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

package executor

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
