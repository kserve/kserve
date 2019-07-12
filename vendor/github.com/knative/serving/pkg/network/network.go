/*
Copyright 2018 The Knative Authors

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    https://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package network

import (
	"bytes"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"text/template"
	"time"

	corev1 "k8s.io/api/core/v1"
)

const (
	// ProbeHeaderName is the name of a header that can be added to
	// requests to probe the knative networking layer.  Requests
	// with this header will not be passed to the user container or
	// included in request metrics.
	ProbeHeaderName = "k-network-probe"

	// ProxyHeaderName is the name of an internal header that activator
	// uses to mark requests going through it.
	ProxyHeaderName = "k-proxy-request"

	// ConfigName is the name of the configmap containing all
	// customizations for networking features.
	ConfigName = "config-network"

	// IstioOutboundIPRangesKey is the name of the configuration entry
	// that specifies Istio outbound ip ranges.
	IstioOutboundIPRangesKey = "istio.sidecar.includeOutboundIPRanges"

	// DefaultClusterIngressClassKey is the name of the configuration entry
	// that specifies the default ClusterIngress.
	DefaultClusterIngressClassKey = "clusteringress.class"

	// IstioIngressClassName value for specifying knative's Istio
	// ClusterIngress reconciler.
	IstioIngressClassName = "istio.ingress.networking.knative.dev"

	// DomainTemplateKey is the name of the configuration entry that
	// specifies the golang template string to use to construct the
	// Knative service's DNS name.
	DomainTemplateKey = "domainTemplate"

	// Since K8s 1.8, prober requests have
	//   User-Agent = "kube-probe/{major-version}.{minor-version}".
	kubeProbeUAPrefix = "kube-probe/"

	// DefaultConnTimeout specifies a short default connection timeout
	// to avoid hitting the issue fixed in
	// https://github.com/kubernetes/kubernetes/pull/72534 but only
	// avalailable after Kubernetes 1.14.
	//
	// Our connections are usually between pods in the same cluster
	// like activator <-> queue-proxy, or even between containers
	// within the same pod queue-proxy <-> user-container, so a
	// smaller connect timeout would be justifiable.
	//
	// We should consider exposing this as a configuration.
	DefaultConnTimeout = 200 * time.Millisecond
)

var (
	// DefaultDomainTemplate is the default golang template to use when
	// constructing the Knative Route's Domain(host)
	DefaultDomainTemplate = "{{.Name}}.{{.Namespace}}.{{.Domain}}"
)

// DomainTemplateValues are the available properties people can choose from
// in their Route's "DomainTemplate" golang template sting.
// We could add more over time - e.g. RevisionName if we thought that
// might be of interest to people.
type DomainTemplateValues struct {
	Name      string
	Namespace string
	Domain    string
}

// Config contains the networking configuration defined in the
// network config map.
type Config struct {
	// IstioOutboundIPRange specifies the IP ranges to intercept
	// by Istio sidecar.
	IstioOutboundIPRanges string

	// DefaultClusterIngressClass specifies the default ClusterIngress class.
	DefaultClusterIngressClass string

	// DomainTemplate is the golang text template to use to generate the
	// Route's domain (host) for the Service.
	DomainTemplate string
}

func validateAndNormalizeOutboundIPRanges(s string) (string, error) {
	s = strings.TrimSpace(s)

	// * is a valid value
	if s == "*" {
		return s, nil
	}

	cidrs := strings.Split(s, ",")
	var normalized []string
	for _, cidr := range cidrs {
		cidr = strings.TrimSpace(cidr)
		if len(cidr) == 0 {
			continue
		}
		if _, _, err := net.ParseCIDR(cidr); err != nil {
			return "", err
		}

		normalized = append(normalized, cidr)
	}

	return strings.Join(normalized, ","), nil
}

// NewConfigFromConfigMap creates a Config from the supplied ConfigMap
func NewConfigFromConfigMap(configMap *corev1.ConfigMap) (*Config, error) {
	nc := &Config{}
	if ipr, ok := configMap.Data[IstioOutboundIPRangesKey]; !ok {
		// It is OK for this to be absent, we will elide the annotation.
		nc.IstioOutboundIPRanges = "*"
	} else if normalizedIpr, err := validateAndNormalizeOutboundIPRanges(ipr); err != nil {
		return nil, err
	} else {
		nc.IstioOutboundIPRanges = normalizedIpr
	}

	if ingressClass, ok := configMap.Data[DefaultClusterIngressClassKey]; !ok {
		nc.DefaultClusterIngressClass = IstioIngressClassName
	} else {
		nc.DefaultClusterIngressClass = ingressClass
	}

	// Blank DomainTemplate makes no sense so use our default
	if dt, ok := configMap.Data[DomainTemplateKey]; !ok {
		nc.DomainTemplate = DefaultDomainTemplate
	} else {
		t, err := template.New("domain-template").Parse(dt)
		if err != nil {
			return nil, err
		}
		if err := checkTemplate(t); err != nil {
			return nil, err
		}

		nc.DomainTemplate = dt
	}

	return nc, nil
}

func (c *Config) GetDomainTemplate() *template.Template {
	return template.Must(template.New("domain-template").Parse(
		c.DomainTemplate))
}

func checkTemplate(t *template.Template) error {
	// To a test run of applying the template, and see if the
	// result is a valid URL.
	data := DomainTemplateValues{
		Name:      "foo",
		Namespace: "bar",
		Domain:    "baz.com",
	}
	buf := bytes.Buffer{}
	if err := t.Execute(&buf, data); err != nil {
		return err
	}
	u, err := url.Parse("https://" + buf.String())
	if err != nil {
		return err
	}

	// TODO(mattmoor): Consider validating things like changing
	// Name / Namespace changes the resulting hostname.
	if u.Hostname() == "" {
		return errors.New("empty hostname")
	}
	if u.RequestURI() != "/" {
		return fmt.Errorf("domain template has url path: %s", u.RequestURI())
	}

	return nil
}

// IsKubeletProbe returns true if the request is a kubernetes probe.
func IsKubeletProbe(r *http.Request) bool {
	return strings.HasPrefix(r.Header.Get("User-Agent"), kubeProbeUAPrefix)
}
