package v1beta1

import (
	"testing"
	logger "log"
	"github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
)

func TestNewInferenceServiceConfig(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	cfg, err := config.GetConfig()
	if err != nil {
		logger.Fatalf("Failed to get config: %v", err)
	}
	client, err := client.New(cfg, client.Options{})
	if err != nil {
		logger.Fatalf("Unable to create client: %v", err)
	}
	isvcConfig, err := NewInferenceServicesConfig(client)
	g.Expect(err).Should(gomega.BeNil())
	g.Expect(isvcConfig).ShouldNot(gomega.BeNil())
}

func TestNewIngressConfig(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	cfg, err := config.GetConfig()
	if err != nil {
		logger.Fatalf("Failed to get config: %v", err)
	}
	client, err := client.New(cfg, client.Options{})
	if err != nil {
		logger.Fatalf("Unable to create client: %v", err)
	}
	ingressCfg, err := NewIngressConfig(client)
	g.Expect(err).Should(gomega.BeNil())
	g.Expect(ingressCfg).ShouldNot(gomega.BeNil())
}

func TestNewDeployConfig(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	cfg, err := config.GetConfig()
	if err != nil {
		logger.Fatalf("Failed to get config: %v", err)
	}
	client, err := client.New(cfg, client.Options{})
	if err != nil {
		logger.Fatalf("Unable to create client: %v", err)
	}
	deployConfig, err := NewDeployConfig(client)
	g.Expect(err).Should(gomega.BeNil())
	g.Expect(deployConfig).ShouldNot(gomega.BeNil())
}