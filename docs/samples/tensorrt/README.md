
# Predict on a KFService using TensorRT Inference Server
## Setup
1. Your ~/.kube/config should point to a cluster with [KFServing installed](https://github.com/kubeflow/kfserving/blob/master/docs/DEVELOPER_GUIDE.md#deploy-kfserving).
2. Your cluster's Istio Ingress gateway must be network accessible.
3. Your cluster's Istio Egresss gateway must [allow Google Cloud Storage](https://knative.dev/docs/serving/outbound-network-access/)

## Create the KFService
Apply the CRD
```
kubectl apply -f tensorrt.yaml 
```

Expected Output
```
kfservice.serving.kubeflow.org/simple-string created
```

## Run a prediction
Uses the client at: https://docs.nvidia.com/deeplearning/sdk/tensorrt-inference-server-guide/docs/client.html#section-client-api


1. get the cluster IP
```
echo $(kubectl -n istio-system get service istio-ingressgateway -o jsonpath='{.status.loadBalancer.ingress[0].ip}')
```

2. edit /etc/hosts to map the CLUSTER IP to simple-string.default.example.com
3. run the client
```
docker run -it --rm --net=host kcorer/tensorrtserver_client:19.05
./build/simple_string_client -u simple-string.default.example.com
```
