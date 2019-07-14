/*
Copyright 2019 The Knative Authors

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

package main

import (
	"context"
	"flag"
	"fmt"
	"strings"
	"time"

	"go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/knative/pkg/logging"
	"github.com/knative/pkg/system"
	cicfg "github.com/knative/serving/pkg/reconciler/v1alpha1/clusteringress/config"
	routecfg "github.com/knative/serving/pkg/reconciler/v1alpha1/route/config"
	corev1 "k8s.io/api/core/v1"
)

var (
	masterURL  = flag.String("master", "", "The address of the Kubernetes API server. Overrides any value in kubeconfig. Only required if out-of-cluster.")
	kubeconfig = flag.String("kubeconfig", "", "Path to a kubeconfig. Only required if out-of-cluster.")
	magicDNS   = flag.String("magic-dns", "", "The hostname for the magic DNS service, e.g. xip.io or nip.io")
)

const (
	// Interval to poll for objects.
	pollInterval = 10 * time.Second
	// How long to wait for objects.
	waitTimeout = 20 * time.Minute
	appName     = "default-domain"
)

func kubeClientFromFlags() (*kubernetes.Clientset, error) {
	cfg, err := clientcmd.BuildConfigFromFlags(*masterURL, *kubeconfig)
	if err != nil {
		return nil, fmt.Errorf("error building kubeconfig: %v", err)
	}
	kubeClient, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("error building kube clientset: %v", err)
	}
	return kubeClient, nil
}

func lookupConfigMap(kubeClient *kubernetes.Clientset, name string) (*corev1.ConfigMap, error) {
	return kubeClient.CoreV1().ConfigMaps(system.Namespace()).Get(name, metav1.GetOptions{})
}

func lookupIngressGateway(kubeClient *kubernetes.Clientset) (*corev1.Service, error) {
	// Fetch and parse the Istio-ClusterIngress ConfigMap from the system namespace.
	istioCM, err := lookupConfigMap(kubeClient, cicfg.IstioConfigName)
	if err != nil {
		return nil, err
	}
	istioConfig, err := cicfg.NewIstioFromConfigMap(istioCM)
	if err != nil {
		return nil, fmt.Errorf("error parsing ConfigMap: %v", err)
	}
	// Parse the service name to determine which Kubernetes Service resource to fetch.
	serviceURL := istioConfig.IngressGateways[0].ServiceURL
	// serviceURL should be of the form serviceName.namespace.<domain>, for example
	// serviceName.namespace.svc.cluster.local.
	parts := strings.SplitN(serviceURL, ".", 3)
	if len(parts) != 3 {
		return nil, fmt.Errorf("unexpected service URL form: %s", serviceURL)
	}

	// Use the first two name parts to lookup the Kubernetes Service resource.
	name, ns := parts[0], parts[1]
	return kubeClient.CoreV1().Services(ns).Get(name, metav1.GetOptions{})
}

func lookupIngressGatewayAddress(kubeClient *kubernetes.Clientset) (*corev1.LoadBalancerIngress, error) {
	svc, err := lookupIngressGateway(kubeClient)
	if err != nil {
		return nil, fmt.Errorf("error looking up IngressGateway: %v", err)
	}
	// Walk the list of Ingress entries in the Service's LoadBalancer status.
	// If one of IP address is found, return it.  Otherwise, keep one with
	// a hostname assigned, if any.
	var hostname *corev1.LoadBalancerIngress
	for i, ing := range svc.Status.LoadBalancer.Ingress {
		if ing.IP != "" {
			return &svc.Status.LoadBalancer.Ingress[i], nil
		}
		if ing.Hostname != "" {
			hostname = &svc.Status.LoadBalancer.Ingress[i]
		}
	}
	if hostname != nil {
		return hostname, nil
	}
	return nil, fmt.Errorf("service %s/%s does not have an assigned external address",
		svc.Namespace, svc.Name)
}

func waitForIngressGatewayAddress(kubeclient *kubernetes.Clientset) (addr *corev1.LoadBalancerIngress, waitErr error) {
	logger := logging.FromContext(context.Background()).Named(appName)
	waitErr = wait.PollImmediate(pollInterval, waitTimeout, func() (done bool, err error) {
		addr, err = lookupIngressGatewayAddress(kubeclient)
		if err == nil {
			return true, nil
		}
		logger.Infof("Failed lookup IngressGateway, retrying: %v", err)
		return false, nil
	})
	return addr, waitErr
}

func main() {
	flag.Parse()
	logger := logging.FromContext(context.Background()).Named(appName)
	defer logger.Sync()

	kubeClient, err := kubeClientFromFlags()
	if err != nil {
		logger.Fatalw("Error building kube clientset", zap.Error(err))
	}

	// Fetch and parse the domain ConfigMap from the system namespace.
	domainCM, err := lookupConfigMap(kubeClient, routecfg.DomainConfigName)
	if err != nil {
		logger.Fatalw("Error getting ConfigMap", zap.Error(err))
	}
	domainConfig, err := routecfg.NewDomainFromConfigMap(domainCM)
	if err != nil {
		logger.Fatalw("Error parsing ConfigMap", zap.Error(err))
	}
	// If there is a catch-all domain configured, then bail out (successfully) here.
	defaultDomain := domainConfig.LookupDomainForLabels(map[string]string{})
	if defaultDomain != routecfg.DefaultDomain {
		logger.Infof("Domain is configured as: %v", defaultDomain)
		return
	}

	// Look up the address for IngressGateway.
	address, err := waitForIngressGatewayAddress(kubeClient)
	if err != nil {
		logger.Fatalw("Error waiting for IngressGateway address", zap.Error(err))
	}
	if address.IP == "" {
		logger.Info("IngressGateway has domain instead of IP address -- leaving default domain config intact")
		return
	}

	// Use the IP (assumes IPv4) to set up a magic DNS name under a top-level Magic
	// DNS service like xip.io or nip.io, where:
	//     1.2.3.4.xip.io  ===(magically resolves to)===> 1.2.3.4
	// Add this magic DNS name without a label selector to the ConfigMap,
	// and send it back to the API server.
	domain := fmt.Sprintf("%s.%s", address.IP, *magicDNS)
	domainCM.Data[domain] = ""
	if _, err = kubeClient.CoreV1().ConfigMaps(system.Namespace()).Update(domainCM); err != nil {
		logger.Fatalw("Error updating ConfigMap", zap.Error(err))
	}

	logger.Infof("Updated default domain to: %s", domain)
}
