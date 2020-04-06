# Setting up custom domain name

## Setup

1. Your ~/.kube/config should point to a cluster with [KFServing installed](https://github.com/kubeflow/kfserving/blob/master/docs/DEVELOPER_GUIDE.md#deploy-kfserving).
2. Your cluster's Istio Ingress gateway must be network accessible.
3. You have a custom domain configured to route incoming traffic either to the Cloud provided Kubernetes Ingress gateway or the istio-ingressgateway / kfserving-ingressgateway's IP address / Load Balancer.

## Create the Ingress resource

#### Note: This step is only necessary if using a domain that is configured to route incoming traffic to the cluster's Kubernetes Ingress. For example, many cloud platforms provide default domains which route to a Kuberenetes Ingress. If using a domain that is routed to the `istio-ingressgateway`, you can skip this step.

#### Note: Use `kfserving-ingressgateway` instead of `istio-ingressgateway` as your `INGRESS_GATEWAY` if you are deploying KFServing as part of Kubeflow install, and not independently.

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
            path: /
```

Apply the Ingress resource

```
kubectl apply -f kfserving-ingress.yaml
```

Expected Output

```
$ ingress.networking.k8s.io/kfserving-ingress created
```

## Modify the config-domain Configmap

Modify the config map to use your custom domain when assigning hostnames to Knative services.

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

With your Ingress routing rules and Knative configmaps set up, you can create Knative services that use your custom domain.

## Create a sample InferenceService

To create an InferenceService using Tensorflow, refer to the [guide](/docs/samples/tensorflow).

## Run a prediction

```
MODEL_NAME=flowers-sample
INPUT_PATH=@./input.json
SERVICE_HOSTNAME=$(kubectl get inferenceservice ${MODEL_NAME} -o jsonpath='{.status.url}')

curl -v $SERVICE_HOSTNAME:predict -d $INPUT_PATH
```

Expected Output

```
*   Trying 34.83.190.188...
* TCP_NODELAY set
* Connected to flowers-sample.default.<custom_domain> (34.83.190.188) port 80 (#0)
> POST /v1/models/flowers-sample:predict HTTP/1.1
> Host: flowers-sample.default.<custom_domain>
> User-Agent: curl/7.64.1
> Accept: */*
> Content-Length: 16169
> Content-Type: application/x-www-form-urlencoded
> Expect: 100-continue
>
< HTTP/1.1 100 Continue
* We are completely uploaded and fine
< HTTP/1.1 200 OK
< Date: Fri, 20 Mar 2020 17:25:49 GMT
< Content-Type: application/json
< Content-Length: 220
< Connection: keep-alive
< x-envoy-upstream-service-time: 18958
<
{
    "predictions": [
        {
            "scores": [0.999114931, 9.2098875e-05, 0.000136786344, 0.000337257865, 0.000300532876, 1.8481378e-05],
            "prediction": 0,
            "key": "   1"
        }
    ]
* Connection #0 to host flowers-sample.default.<custom_domain> left intact
}* Closing connection 0
```

## External Links

[Configure Ingress with TLS for https access](https://kubernetes.io/docs/concepts/services-networking/ingress/#tls)

[Setup custom domain names and certificates for IKS](https://cloud.ibm.com/docs/containers?topic=containers-serverless-apps-knative#knative-custom-domain-tls)
