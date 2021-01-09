/*
Copyright 2020 kubeflow.org.

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

package batcher

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/kubeflow/kfserving/pkg/constants"
	"github.com/onsi/gomega"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"testing"
)

func TestBatcher(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	predictorRequest := []byte(`{"instances":[[0,0,0]]}`)
	predictorResponse := []byte(`{"predictions":[[4,5,6]]}`)

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

	predictorSvcUrl, err := url.Parse(predictor.URL)
	reader := bytes.NewReader(predictorRequest)
	ip := "127.0.0.1"
	batcherUrl := fmt.Sprintf("http://%s:%s/", ip, constants.InferenceServiceDefaultBatcherPort)
	r := httptest.NewRequest("POST", batcherUrl, reader)
	w := httptest.NewRecorder()

	logf.SetLogger(logf.ZapLogger(false))
	log := logf.Log.WithName("entrypoint")

	g.Expect(err).To(gomega.BeNil())
	Config(constants.InferenceServiceDefaultBatcherPort, predictorSvcUrl.Hostname(),
		predictorSvcUrl.Port(), 32, 1.0, 60)
	println(constants.InferenceServiceDefaultBatcherPort, predictorSvcUrl.Hostname(),
		predictorSvcUrl.Port())

	log.Info("Starting", "port", constants.InferenceServiceDefaultBatcherPort)

	b2, _ := ioutil.ReadAll(w.Result().Body)
	var res Response
	var predictions Predictions
	_ = json.Unmarshal(b2, &res)
	predictions.Predictions = res.Predictions
	josnStr, _ := json.Marshal(predictions)
	fmt.Println(string(josnStr))
	g.Expect(josnStr).To(gomega.Equal(predictorResponse))
}
