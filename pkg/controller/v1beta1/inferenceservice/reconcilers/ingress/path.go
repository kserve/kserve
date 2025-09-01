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

package ingress

import (
	"bytes"
	"fmt"
	"net/url"
	"text/template"

	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
)

type PathTemplateValues struct {
	Name      string
	Namespace string
}

// GenerateUrlPath generates the path using the pathTemplate configured in IngressConfig
func GenerateUrlPath(name string, namespace string, ingressConfig *v1beta1.IngressConfig) (string, error) {
	if ingressConfig.PathTemplate == "" {
		return "", nil
	}

	values := PathTemplateValues{
		Name:      name,
		Namespace: namespace,
	}
	tpl, err := template.New("url-template").Parse(ingressConfig.PathTemplate)
	if err != nil {
		return "", err
	}

	buf := bytes.Buffer{}
	if err := tpl.Execute(&buf, values); err != nil {
		return "", fmt.Errorf("error rendering the url template: %w", err)
	}

	// Validate generated URL. Use url.ParseRequestURI() instead of
	// apis.ParseURL(). The latter calls url.Parse() which allows pretty much anything.
	url, err := url.ParseRequestURI(buf.String())
	if err != nil {
		return "", fmt.Errorf("invalid url %q: %w", buf.String(), err)
	}

	if url.Scheme != "" || url.Host != "" {
		return "", fmt.Errorf("invalid url path %q: contains either a scheme or a host", buf.String())
	}

	return url.Path, nil
}
