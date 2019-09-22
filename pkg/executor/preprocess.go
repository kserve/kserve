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
	"io/ioutil"
	"net/http"
	"net/url"
)

// Call preprocess
func (eh *executorHandler) preprocess(r *http.Request) error {
	b, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return err
	}
	b, err = eh.post(&url.URL{
		Scheme: "http",
		Host:   eh.preprocessHost,
		Path:   r.URL.Path,
	}, b, r.Header.Get("Content-Type"))
	if err != nil {
		return err
	} else {
		r.Body = ioutil.NopCloser(bytes.NewReader(b))
		r.ContentLength = int64(len(b))
	}
	return nil
}
