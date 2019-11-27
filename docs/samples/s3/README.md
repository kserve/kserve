
# Predict on a InferenceService with saved model on S3
## Setup
1. Your ~/.kube/config should point to a cluster with [KFServing installed](https://github.com/kubeflow/kfserving/blob/master/docs/DEVELOPER_GUIDE.md#deploy-kfserving).
2. Your cluster's Istio Ingress gateway must be network accessible.
3. The example uses the Kubeflow's Minio setup if you have [Kubeflow](https://www.kubeflow.org/docs/started/getting-started/) installed,
you can also setup your own [Minio server](https://docs.min.io/docs/deploy-minio-on-kubernetes.html) or use other S3 compatible cloud storage.
4. Install Kafka from [Confluent helm chart](https://www.confluent.io/blog/getting-started-apache-kafka-kubernetes/) if you do not have existing one
5. Install Knative [kafka event source](https://github.com/knative/eventing-contrib/tree/master/kafka/source)

## Deploy Kafka
If you do not have an existing kafka cluster, you can run the following command to install in-cluster kafka for testing purpose, here
we turn off the persistence.

```
helm repo add confluentinc https://confluentinc.github.io/cp-helm-charts/
helm repo update
helm install my-kafka -f values.yaml --set cp-schema-registry.enabled=false,cp-kafka-rest.enabled=false,cp-kafka-connect.enabled=false confluentinc/cp-helm-charts
```

after successful install you are expected to see running kafka cluster
```bash
kubectl get pods
NAME                      READY   STATUS    RESTARTS   AGE
my-kafka-cp-kafka-0       2/2     Running   0          126m
my-kafka-cp-kafka-1       2/2     Running   1          126m
my-kafka-cp-kafka-2       2/2     Running   0          126m
my-kafka-cp-zookeeper-0   2/2     Running   0          127m
```

## Deploy Minio
If you do not have Minio setup in your cluster, you can run following command to install Minio instance for testing purpose.
```bash
kubectl apply -f minio.yaml
```

Install [mc](https://docs.min.io/docs/minio-client-complete-guide) and create a bucket named `mnist`
```bash
kubectl port-forward --namespace default $(kubectl get pod --namespace default --selector="app=minio" --output jsonpath='{.items[0].metadata.name}') 9000:9000
mc config host add myminio http://127.0.0.1:9000 minio minio123
mc mb myminio/mnist
```

Setup event notification
```bash
 mc event add myminio/mnist arn:minio:sqs:us-east-1:1:kafka --suffix .png
```

you should expect a notification event sent to kafka topic `mnist` after uploading an image in `mnist` bucket
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
   "time":"2019-11-17T19:08:08Z"}
```

## Train TF mnist model and save on S3
Follow Kubeflow's [TF mnist example](https://github.com/kubeflow/examples/tree/master/mnist#using-s3) to train a TF mnist model and save on S3,
change following S3 access settings, `modelDir` and `exportDir` as needed. If you already have a mnist model saved on S3 you can skip this step.
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

## Create S3 Secret and attach to Service Account
If you already have a S3 secret created from last step you can skip this step, since `KFServing` is relying on secret annotations to setup proper
S3 environment variables you may still need to add following annotations to your secret to overwrite S3 endpoint or other S3 options.
```yaml
apiVersion: v1
kind: Secret
metadata:
  name: mysecret
  annotations:
     serving.kubeflow.org/s3-endpoint: minio-service:9000 # replace with your s3 endpoint
     serving.kubeflow.org/s3-usehttps: "0" # by default 1, for testing with minio you need to set to 0
type: Opaque
data:
  awsAccessKeyID: bWluaW8=
  awsSecretAccessKey: bWluaW8xMjM=
```

`KFServing` gets the secrets from your service account, you need to add the above created or existing secret to your service account's secret list. 
By default `KFServing` uses `default` service account, user can use own service account and overwrite on `InferenceService` CRD.

```yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: sa
secrets:
- name: mysecret
```

Apply the secret and service account
```bash
kubectl apply -f s3_secret.yaml
```

## Build mnist transformer image
```bash
docker build -t mnist-transformer:latest -f ./transformer.Dockerfile . --rm
```

## Create the InferenceService
Apply the CRD
```bash
kubectl apply -f mnist_kafka.yaml 
```

Expected Output
```
$ inferenceservice.serving.kubeflow.org/mnist_kafka created
```

## Create kafka event source
Set kafka event source sink with the inferenceservice deployed above 
```bash
kubectl apply -f kafka-source.yaml
```

## Upload a digit image to Minio mnist bucket
The uploaded image should then go to the classified bucket.



