
# Predict on a InferenceService using Triton Inference Server
## Setup
1. Your ~/.kube/config should point to a cluster with [KFServing installed](https://github.com/kubeflow/kfserving/#install-kfserving).
2. Your cluster's Istio Ingress gateway must be network accessible.

## Create the InferenceService
Apply the CRD
```
kubectl apply -f triton.yaml 
```

Expected Output
```
inferenceservice.serving.kubeflow.org/triton-simple-string created
```

## Run a prediction
Uses the client at: https://docs.nvidia.com/deeplearning/triton-inference-server/user-guide/docs/client_example.html


1. setup vars

```
SERVICE_HOSTNAME=$(kubectl get ksvc triton-simple-string-predictor-default -o jsonpath='{.status.url}' | cut -d "/" -f 3)
INGRESS_GATEWAY=istio-ingressgateway
CLUSTER_IP=$(kubectl -n istio-system get service $INGRESS_GATEWAY -o jsonpath='{.status.loadBalancer.ingress[0].ip}')
echo $CLUSTER_IP
```
2. check server status
```
curl -H "Host: ${SERVICE_HOSTNAME}" http://${CLUSTER_IP}/api/status
```
3. edit /etc/hosts to map the CLUSTER IP to triton-simple-string-predictor-default.default.example.com
4. run the client
```
docker run -e SERVICE_HOSTNAME:$SERVICE_HOSTNAME -it --rm --net=host kcorer/tensorrtserver_client:19.05
./build/simple_string_client -u $SERVICE_HOSTNAME
```

You should see output like:
```
root@trantor:/workspace# ./build/simple_string_client -u triton-simple-string.default.example.com
0 + 1 = 1
0 - 1 = -1
1 + 1 = 2
1 - 1 = 0
2 + 1 = 3
2 - 1 = 1
3 + 1 = 4
3 - 1 = 2
4 + 1 = 5
4 - 1 = 3
5 + 1 = 6
5 - 1 = 4
6 + 1 = 7
6 - 1 = 5
7 + 1 = 8
7 - 1 = 6
8 + 1 = 9
8 - 1 = 7
9 + 1 = 10
9 - 1 = 8
```
