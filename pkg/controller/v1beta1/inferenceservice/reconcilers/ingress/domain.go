package ingress

import (
	"bytes"
	"fmt"
	"text/template"

	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/apimachinery/pkg/util/validation/field"
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
		return "", fmt.Errorf("error generating domain tempalte: %w", err)
	}

	urlErrs := validation.IsFullyQualifiedDomainName(field.NewPath("url"), buf.String())
	if urlErrs != nil {
		return "", fmt.Errorf("invalid domain name %q: %w", buf.String(), urlErrs.ToAggregate())
	}

	return buf.String(), nil
}
