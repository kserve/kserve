/*
Copyright 2022 The KServe Authors.

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
	"strings"
	"text/template"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"knative.dev/pkg/network"

	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
)

type DomainTemplateValues struct {
	Name          string
	Namespace     string
	IngressDomain string
	Annotations   map[string]string
	Labels        map[string]string
}

// GenerateDomainName generate domain name using template configured in IngressConfig
func GenerateDomainName(name string, obj metav1.ObjectMeta, ingressConfig *v1beta1.IngressConfig) (string, error) {
	values := DomainTemplateValues{
		Name:          name,
		Namespace:     obj.Namespace,
		IngressDomain: ingressConfig.IngressDomain,
		Annotations:   obj.Annotations,
		Labels:        obj.Labels,
	}

	tpl, err := template.New("domain-template").Parse(ingressConfig.DomainTemplate)
	if err != nil {
		return "", err
	}

	buf := bytes.Buffer{}
	if err := tpl.Execute(&buf, values); err != nil {
		return "", fmt.Errorf("error rendering the domain template: %w", err)
	}

	urlErrs := validation.IsFullyQualifiedDomainName(field.NewPath("url"), buf.String())
	if urlErrs != nil {
		return "", fmt.Errorf("invalid domain name %q: %w", buf.String(), urlErrs.ToAggregate())
	}

	return buf.String(), nil
}

func GenerateInternalDomainName(name string, obj metav1.ObjectMeta, ingressConfig *v1beta1.IngressConfig) (string, error) {
	values := DomainTemplateValues{
		Name:          name,
		Namespace:     obj.Namespace,
		IngressDomain: network.GetClusterDomainName(),
		Annotations:   obj.Annotations,
		Labels:        obj.Labels,
	}

	tpl, err := template.New("domain-template").Parse(ingressConfig.DomainTemplate)
	if err != nil {
		return "", err
	}

	buf := bytes.Buffer{}
	if err := tpl.Execute(&buf, values); err != nil {
		return "", fmt.Errorf("error rendering the domain template: %w", err)
	}

	urlErrs := validation.IsFullyQualifiedDomainName(field.NewPath("url"), buf.String())
	if urlErrs != nil {
		return "", fmt.Errorf("invalid domain name %q: %w", buf.String(), urlErrs.ToAggregate())
	}

	return buf.String(), nil
}

// GetAdditionalHosts generates additional hostnames for an InferenceService based on the configured
// additionalIngressDomains in the IngressConfig.
//
// Parameters:
//   - domainList: A pointer to a slice of domain strings to search against when extracting the subdomain
//   - serviceHost: The main service hostname to extract the subdomain from
//   - config: The IngressConfig containing additionalIngressDomains configuration
//
// Returns:
//
//	A pointer to a slice of additional valid hostnames created by combining the extracted subdomain
//	with each domain in the additionalIngressDomains. Returns an empty slice if no additional hostnames
//	can be created or if additionalIngressDomains is not configured.
//
// Note:
//   - The function extracts a subdomain from the serviceHost by checking against domains in domainList
//   - The function validates each generated hostname using DNS validation rules
//   - Duplicate domains in the additionalIngressDomains are automatically deduplicated
func GetAdditionalHosts(domainList *[]string, serviceHost string, config *v1beta1.IngressConfig) *[]string {
	additionalHosts := &[]string{}
	// Include additional ingressDomain to the domains (both internal and external)
	subdomain := ""
	if domainList != nil && len(*domainList) != 0 {
		for _, domain := range *domainList {
			res, found := strings.CutSuffix(serviceHost, domain)
			if found {
				subdomain = res
				break
			}
		}
	}
	if len(subdomain) != 0 && config.AdditionalIngressDomains != nil && len(*config.AdditionalIngressDomains) > 0 {
		// len(subdomain) != 0 means we have found the subdomain.
		// If the list of the additionalIngressDomains is not empty, we will append the valid host created by the
		// additional ingress domain.
		// Deduplicate the domains in the additionalIngressDomains, making sure that the returned additionalHosts
		// do not have duplicate domains.
		deduplicateMap := make(map[string]bool, len(*config.AdditionalIngressDomains))
		for _, domain := range *config.AdditionalIngressDomains {
			// If the domain is redundant, go to the next element.
			if !deduplicateMap[domain] {
				host := fmt.Sprintf("%s%s", subdomain, domain)
				if err := validation.IsDNS1123Subdomain(host); len(err) > 0 {
					log.Error(fmt.Errorf("the domain name %s in the additionalIngressDomains is not valid", domain),
						"Failed to get the valid host name")
					continue
				}
				*additionalHosts = append(*additionalHosts, host)
				deduplicateMap[domain] = true
			}
		}
	}
	return additionalHosts
}
