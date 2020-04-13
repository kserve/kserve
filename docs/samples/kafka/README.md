
# End to end inference example with Minio and Kafka
## Setup
1. Your ~/.kube/config should point to a cluster with [KFServing installed](https://github.com/kubeflow/kfserving/blob/master/docs/DEVELOPER_GUIDE.md#deploy-kfserving).
2. Your cluster's Istio Ingress gateway must be network accessible.
3. Install Minio with following Minio deploy step.
4. Use existing Kafka cluster or install Kafka on your cluster with [Confluent helm chart](https://www.confluent.io/blog/getting-started-apache-kafka-kubernetes/).
5. Install [Kafka Event Source](https://github.com/knative/eventing-contrib/tree/master/kafka/source).
6. Install [Kustomize 3.0+](https://github.com/kubernetes-sigs/kustomize/blob/master/docs/INSTALL.md)

This example shows an end to end inference pipeline which processes an kafka event and invoke the inference service to get the prediction with provided
pre/post processing code.

![diagram](images/diagram.png)

## Deploy Kafka
If you do not have an existing kafka cluster, you can run the following commands to install in-cluster kafka using [helm3](https://helm.sh)
with persistence turned off.

```
helm repo add confluentinc https://confluentinc.github.io/cp-helm-charts/
helm repo update
helm install my-kafka -f values.yaml --set cp-schema-registry.enabled=false,cp-kafka-rest.enabled=false,cp-kafka-connect.enabled=false confluentinc/cp-helm-charts
```

after successful install you are expected to see the running kafka cluster
```bash
NAME                      READY   STATUS    RESTARTS   AGE
my-kafka-cp-kafka-0       2/2     Running   0          126m
my-kafka-cp-kafka-1       2/2     Running   1          126m
my-kafka-cp-kafka-2       2/2     Running   0          126m
my-kafka-cp-zookeeper-0   2/2     Running   0          127m
```

## Deploy Kafka Event Source
Install Knative Eventing and Kafka Event Source.
```bash
kubectl apply --selector knative.dev/crd-install=true \
  --filename https://github.com/knative/eventing/releases/download/v0.10.0/release.yaml
kubectl apply --filename https://github.com/knative/eventing/releases/download/v0.10.0/release.yaml
kubectl apply --filename https://github.com/knative/eventing-contrib/releases/download/v0.10.2/release.yaml
```
Apply the `InferenceService` addressable cluster role
```bash
kubectl apply -f addressable-resolver.yaml
```
## Deploy Minio
- If you do not have Minio setup in your cluster, you can run following command to install Minio test instance.
```bash
kubectl apply -f minio.yaml
```

- Install Minio client [mc](https://docs.min.io/docs/minio-client-complete-guide)
```bash
kubectl port-forward $(kubectl get pod --selector="app=minio" --output jsonpath='{.items[0].metadata.name}') 9000:9000
mc config host add myminio http://127.0.0.1:9000 minio minio123
```
- Create buckets `mnist` for uploading images and `digit-[0-9]` for classification.
```bash
mc mb myminio/mnist
```

- Setup event notification to publish events to kafka.
```bash
mc event add myminio/mnist arn:minio:sqs:us-east-1:1:kafka --suffix .png
```

you should expect a notification event like following sent to kafka topic `mnist` after uploading an image in `mnist` bucket
```json
{
   "EventType":"s3:ObjectCreated:Put",
   "Key":"mnist/0.png",
   "Records":[
      {"eventVersion":"2.0",
       "eventSource":"minio:s3",
       "awsRegion":"",
       "eventTime":"2019-11-17T19:08:08Z",
       "eventName":"s3:ObjectCreated:Put",
       "userIdentity":{"principalId":"minio"},
       "requestParameters":{"sourceIPAddress":"127.0.0.1:37830"},
       "responseElements":{"x-amz-request-id":"15D808BF706E0994",
       "x-minio-origin-endpoint":"http://10.244.0.71:9000"},
       "s3":{
          "s3SchemaVersion":"1.0",
          "configurationId":"Config",
          "bucket":{
               "name":"mnist",
               "ownerIdentity":{"principalId":"minio"},
               "arn":"arn:aws:s3:::mnist"},
          "object":{"key":"0.png","size":324,"eTag":"ebed21f6f77b0a64673a3c96b0c623ba","contentType":"image/png","userMetadata":{"content-type":"image/png"},"versionId":"1","sequencer":"15D808BF706E0994"}},
          "source":{"host":"","port":"","userAgent":""}}
   ],
   "level":"info",
   "msg":"",
   "time":"2019-11-17T19:08:08Z"
}
```

## Train TF mnist model and save on Minio
If you already have a mnist model saved on Minio or S3 you can skip this step, otherwise you can install [Kubeflow](https://www.kubeflow.org/docs/started/getting-started/)
and follow [TF mnist example](https://github.com/kubeflow/examples/tree/master/mnist#using-s3) to train a TF mnist model and save it on Minio.
Change following S3 credential settings to enable getting model from Minio.
```bash
export S3_USE_HTTPS=0 #set to 0 for default minio installs
export S3_ENDPOINT=minio-service:9000
export AWS_ENDPOINT_URL=http://${S3_ENDPOINT}

kustomize edit add configmap mnist-map-training --from-literal=S3_ENDPOINT=${S3_ENDPOINT}
kustomize edit add configmap mnist-map-training --from-literal=AWS_ENDPOINT_URL=${AWS_ENDPOINT_URL}
kustomize edit add configmap mnist-map-training --from-literal=S3_USE_HTTPS=${S3_USE_HTTPS}

kustomize edit add configmap mnist-map-training --from-literal=modelDir=s3://mnist/model-v1
kustomize edit add configmap mnist-map-training --from-literal=exportDir=s3://mnist/model-v1/export
```

## Create S3 Secret for Minio and attach to Service Account
`KFServing` gets the secrets from your service account, you need to add the created or existing secret to your service account's secret list. 
By default `KFServing` uses `default` service account, user can use own service account and overwrite on `InferenceService` CRD.

Apply the secret and attach the secret to the service account.
```bash
kubectl apply -f s3_secret.yaml
```

## Build mnist transformer image
The transformation image implements the preprocess handler to process the minio notification event to download the image from minio
and transform image bytes to tensors. The postprocess handler processes the prediction and upload the image to the classified minio
bucket `digit-[0-9]`.
```bash
docker build -t $USER/mnist-transformer:latest -f ./transformer.Dockerfile . --rm
docker push $USER/mnist-transformer:latest
```

## Create the InferenceService
Specify the built image on `Transformer` spec and apply the inference service CRD.
```bash
kubectl apply -f mnist_kafka.yaml 
```

This creates transformer and predictor pods, the request goes to transformer first where it invokes the preprocess handler, transformer
then calls out to predictor to get the prediction response which in turn invokes the postprocess handler. 
```
kubectl get pods -l serving.kubeflow.org/inferenceservice=mnist
mnist-predictor-default-9t5ms-deployment-74f5cd7767-khthf     2/2     Running       0          10s
mnist-transformer-default-jmf98-deployment-8585cbc748-ftfhd   2/2     Running       0          14m
```

## Create kafka event source
Apply kafka event source which creates the kafka consumer pod to pull the events from kafka and deliver to inference service.
```bash
kubectl apply -f kafka-source.yaml
```

This creates the kafka source pod which consumers the events from `mnist` topic
```bash
kafkasource-kafka-source-3d809fe2-1267-11ea-99d0-42010af00zbn5h   1/1     Running   0          8h
```

## Upload a digit image to Minio mnist bucket
The last step is to upload the image `images/0.png`, image then should be moved to the classified bucket based on the prediction response!



