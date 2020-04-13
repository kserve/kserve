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

package logger

import (
	"bytes"
	"github.com/kubeflow/kfserving/pkg/apis/serving/v1alpha2"
	"github.com/onsi/gomega"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"testing"
)

func TestLogger(t *testing.T) {

	g := gomega.NewGomegaWithT(t)

	predictorRequest := []byte(`{"instances":[[0,0,0]]}`)
	predictorResponse := []byte(`{"instances":[[4,5,6]]}`)

	// Start a local HTTP server
	logSvc := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		b, err := ioutil.ReadAll(req.Body)
		g.Expect(err).To(gomega.BeNil())
		g.Expect(b).To(gomega.Or(gomega.Equal(predictorRequest), gomega.Equal(predictorResponse)))
		_, err = rw.Write([]byte(`ok`))
		g.Expect(err).To(gomega.BeNil())
	}))
	// Close the server when test finishes
	defer logSvc.Close()

	// Start a local HTTP server
	predictor := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		b, err := ioutil.ReadAll(req.Body)
		g.Expect(err).To(gomega.BeNil())
		g.Expect(b).To(gomega.Or(gomega.Equal(predictorRequest), gomega.Equal(predictorResponse)))
		_, err = rw.Write(predictorResponse)
		g.Expect(err).To(gomega.BeNil())
	}))
	// Close the server when test finishes
	defer predictor.Close()

	reader := bytes.NewReader(predictorRequest)
	r := httptest.NewRequest("POST", "http://a", reader)
	w := httptest.NewRecorder()

	logf.SetLogger(logf.ZapLogger(false))
	log := logf.Log.WithName("entrypoint")

	predictorSvcUrl, err := url.Parse(predictor.URL)
	g.Expect(err).To(gomega.BeNil())
	logSvcUrl, err := url.Parse(logSvc.URL)
	g.Expect(err).To(gomega.BeNil())
	sourceUri, err := url.Parse("http://localhost:8080/")
	g.Expect(err).To(gomega.BeNil())
	oh := New(log, "0.0.0.0", predictorSvcUrl.Port(), logSvcUrl, sourceUri, v1alpha2.LogAll, "mymodel", "default", "default")

	oh.ServeHTTP(w, r)

	b2, _ := ioutil.ReadAll(w.Result().Body)
	g.Expect(b2).To(gomega.Equal(predictorResponse))

}
