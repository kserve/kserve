# Setting up custom domain name

## Motivation

A custom domain is useful in a microservice architecture when you want to expose kfserving outside of kubernetes to other compute resources under a semantic domain. This works especially well in cases where you are using a cloud-provided [Ingress Controller](https://kubernetes.io/docs/concepts/services-networking/ingress-controllers/) with [External DNS](https://github.com/kubernetes-sigs/external-dns) to dynamically create DNS records and route traffic to your models.

## Setup

1. Your ~/.kube/config should point to a cluster with [KFServing installed](https://github.com/kubeflow/kfserving/#install-kfserving).
2. Your cluster's Istio Ingress gateway must be network accessible.
3. You own a domain that can be configured to route incoming traffic to a cloud provided kubernetes `Ingress` or the istio-ingressgateway's IP address / Load Balancer.

## Modify the config-domain Configmap

Modify the config map to use your custom domain when assigning hostnames to Knative services. By default, kfserving uses `example.com`.

Open the `config-domain` configmap to start editing it.

```
 kubectl edit configmap config-domain -n knative-serving
```

Specify your custom domain in the `data` section of your configmap and remove the default domain that is set for your cluster.

```
apiVersion: v1
kind: ConfigMap
data:
  <custom_domain>: ""
metadata:
...
```

Save your changes. Expected Output

```
configmap/config-domain edited
```

## Create the Ingress resource

#### Note: This step is only necessary if you are configuring a domain to route incoming traffic to a Kubernetes Ingress. For example, many cloud platforms provide default domains which route to a Kubernetes Ingress. If you intend to route a domain directly to the `istio-ingressgateway`, you can skip this step.

Edit the `kfserving-ingress.yaml` file to add your custom wildcard domain to the `spec.rules.host` section, replacing `<*.custom_domain>` with your custom wildcard domain. This is so that all incoming network traffic from your custom domain and any subdomain is routed to the `istio-ingressgateway`.

```
apiVersion: networking.k8s.io/v1beta1
kind: Ingress
metadata:
  name: kfserving-ingress
  namespace: istio-system
spec:
  rules:
    - host: "<*.custom_domain>"
      http:
        paths:
          - backend:
              serviceName: istio-ingressgateway
              servicePort: 80
```

Apply the Ingress resource

```
kubectl apply -f kfserving-ingress.yaml
```

Expected Output

```
$ ingress.networking.k8s.io/kfserving-ingress created
```

## Verify 

You can call your models using your custom top-level domain with the correct subdomain. For example if you deploy the sample [sklearn model](https://github.com/kubeflow/kfserving/tree/master/docs/samples/sklearn) in the `default` namespace using `customdomain.com`, you can reach it in the following way

```
curl -v \
    http://sklearn-iris.default.customdomain.com/v1/models/sklearn-iris:predict \
    -d @./input.json
```

## AWS

If you are using the AWS's [ALB Ingress Controller](https://github.com/kubernetes-sigs/aws-alb-ingress-controller), you can set custom annotations in the `kfserving-ingress.yaml` to deploy an Application Load Balancer instead of the less-configurable, default Classic Load Balancer. For example, to create an internal Application Load Balancer, use the following annotations

```
metadata:
  annotations:
    kubernetes.io/ingress.class: alb
    alb.ingress.kubernetes.io/scheme: internal
```

If you are also using `external-dns` this configuration is enough to automatically create an entry in Route53 and expose your Knative applications to everything else that is inside your VPC.


## External Links

[Configure Ingress with TLS for https access](https://kubernetes.io/docs/concepts/services-networking/ingress/#tls)

[Setup custom domain names and certificates for IKS](https://cloud.ibm.com/docs/containers?topic=containers-serverless-apps-knative#knative-custom-domain-tls)
