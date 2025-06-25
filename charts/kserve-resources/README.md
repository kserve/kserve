# kserve

Helm chart for deploying kserve resources

![Version: v0.15.2](https://img.shields.io/badge/Version-v0.15.2-informational?style=flat-square)

## Installing the Chart

To install the chart, run the following:

```console
$ helm install kserve oci://ghcr.io/kserve/charts/kserve --version v0.15.2
```

## Values

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| kserve.agent.image | string | `"kserve/agent"` |  |
| kserve.agent.tag | string | `"v0.15.2"` |  |
| kserve.controller.affinity | object | `{}` | A Kubernetes Affinity, if required. For more information, see [Affinity v1 core](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.27/#affinity-v1-core).  For example:   affinity:     nodeAffinity:      requiredDuringSchedulingIgnoredDuringExecution:        nodeSelectorTerms:        - matchExpressions:          - key: foo.bar.com/role            operator: In            values:            - master |
| kserve.controller.annotations | object | `{}` | Optional additional annotations to add to the controller deployment. |
| kserve.controller.containerSecurityContext | object | `{"allowPrivilegeEscalation":false,"capabilities":{"drop":["ALL"]},"privileged":false,"readOnlyRootFilesystem":true,"runAsNonRoot":true}` | Container Security Context to be set on the controller component container. For more information, see [Configure a Security Context for a Pod or Container](https://kubernetes.io/docs/tasks/configure-pod-container/security-context/). |
| kserve.controller.deploymentMode | string | `"Serverless"` | KServe deployment mode: "Serverless", "RawDeployment". |
| kserve.controller.gateway.additionalIngressDomains | list | `[]` | Optional additional domains for ingress routing. |
| kserve.controller.gateway.disableIngressCreation | bool | `false` | Whether to disable ingress creation for RawDeployment mode. |
| kserve.controller.gateway.disableIstioVirtualHost | bool | `false` | DisableIstioVirtualHost controls whether to use istio as network layer for top level component routing or path based routing. This configuration is only applicable for Serverless mode, when disabled Istio is no longer required. |
| kserve.controller.gateway.domain | string | `"example.com"` | Ingress domain for RawDeployment mode, for Serverless it is configured in Knative. |
| kserve.controller.gateway.domainTemplate | string | `"{{ .Name }}-{{ .Namespace }}.{{ .IngressDomain }}"` | Ingress domain template for RawDeployment mode, for Serverless mode it is configured in Knative. |
| kserve.controller.gateway.ingressGateway | object | `{"className":"istio","createGateway":false,"enableGatewayApi":false,"gateway":"knative-serving/knative-ingress-gateway","kserveGateway":"kserve/kserve-ingress-gateway"}` | ingressGateway specifies the gateway which handles the network traffic from outside the cluster. |
| kserve.controller.gateway.ingressGateway.className | string | `"istio"` | class specifies the ingress class name. If Gateway API is enabled, this will not affect the ingress routing. |
| kserve.controller.gateway.ingressGateway.createGateway | bool | `false` | createGateway controls whether to create the default Gateway resource for ingress routing as part of the installation. This is only used when Gateway API is enabled. |
| kserve.controller.gateway.ingressGateway.enableGatewayApi | bool | `false` | enableGatewayApi controls whether to use the Gateway API for ingress routing instead of kuberetes Ingress. |
| kserve.controller.gateway.ingressGateway.gateway | string | `"knative-serving/knative-ingress-gateway"` | gateway specifies the name and namespace of the Knative's ingress gateway. |
| kserve.controller.gateway.ingressGateway.kserveGateway | string | `"kserve/kserve-ingress-gateway"` | kserveGateway specifies the name and namespace of the Gateway which handles the network traffic from outside the cluster. This is only used when Gateway API is enabled. The gateway should be specified in format <gateway namespace>/<gateway name> |
| kserve.controller.gateway.localGateway.gateway | string | `"knative-serving/knative-local-gateway"` | localGateway specifies the gateway which handles the network traffic within the cluster. |
| kserve.controller.gateway.localGateway.gatewayService | string | `"knative-local-gateway.istio-system.svc.cluster.local"` | localGatewayService specifies the hostname of the local gateway service. |
| kserve.controller.gateway.localGateway.knativeGatewayService | string | `""` | knativeLocalGatewayService specifies the hostname of the Knative's local gateway service. When unset, the value of "localGatewayService" will be used. When enabling strict mTLS in Istio, KServe local gateway should be created and pointed to the Knative local gateway. |
| kserve.controller.gateway.urlScheme | string | `"http"` | HTTP endpoint url scheme. |
| kserve.controller.image | string | `"kserve/kserve-controller"` | KServe controller container image name. |
| kserve.controller.imagePullSecrets | list | `[]` | Reference to one or more secrets to be used when pulling images. For more information, see [Pull an Image from a Private Registry](https://kubernetes.io/docs/tasks/configure-pod-container/pull-image-private-registry/).  For example:  imagePullSecrets:    - name: "image-pull-secret" |
| kserve.controller.knativeAddressableResolver | object | `{"enabled":false}` | Indicates whether to create an addressable resolver ClusterRole for Knative Eventing. This ClusterRole grants the necessary permissions for the Knative's DomainMapping reconciler to resolve InferenceService addressables. |
| kserve.controller.labels | object | `{}` | Optional additional labels to add to the controller deployment. |
| kserve.controller.metricsBindAddress | string | `"127.0.0.1"` | Metrics bind address |
| kserve.controller.metricsBindPort | string | `"8080"` | Metrics bind port |
| kserve.controller.nodeSelector | object | `{}` | The nodeSelector on Pods tells Kubernetes to schedule Pods on the nodes with matching labels. For more information, see [Assigning Pods to Nodes](https://kubernetes.io/docs/concepts/scheduling-eviction/assign-pod-node/).  |
| kserve.controller.podAnnotations | object | `{}` | Optional additional annotations to add to the controller Pods. |
| kserve.controller.podLabels | object | `{}` | Optional additional labels to add to the controller Pods. |
| kserve.controller.rbacProxy.resources.limits.cpu | string | `"100m"` |  |
| kserve.controller.rbacProxy.resources.limits.memory | string | `"300Mi"` |  |
| kserve.controller.rbacProxy.resources.requests.cpu | string | `"100m"` |  |
| kserve.controller.rbacProxy.resources.requests.memory | string | `"300Mi"` |  |
| kserve.controller.rbacProxy.securityContext.allowPrivilegeEscalation | bool | `false` |  |
| kserve.controller.rbacProxy.securityContext.capabilities.drop[0] | string | `"ALL"` |  |
| kserve.controller.rbacProxy.securityContext.privileged | bool | `false` |  |
| kserve.controller.rbacProxy.securityContext.readOnlyRootFilesystem | bool | `true` |  |
| kserve.controller.rbacProxy.securityContext.runAsNonRoot | bool | `true` |  |
| kserve.controller.rbacProxyImage | string | `"quay.io/brancz/kube-rbac-proxy:v0.18.0"` | KServe controller manager rbac proxy contrainer image |
| kserve.controller.resources | object | `{"limits":{"cpu":"100m","memory":"300Mi"},"requests":{"cpu":"100m","memory":"300Mi"}}` | Resources to provide to the kserve controller pod.  For example:  requests:    cpu: 10m    memory: 32Mi  For more information, see [Resource Management for Pods and Containers](https://kubernetes.io/docs/concepts/configuration/manage-resources-containers/). |
| kserve.controller.securityContext | object | `{"runAsNonRoot":true,"seccompProfile":{"type":"RuntimeDefault"}}` | Pod Security Context. For more information, see [Configure a Security Context for a Pod or Container](https://kubernetes.io/docs/tasks/configure-pod-container/security-context/). |
| kserve.controller.serviceAnnotations | object | `{}` | Optional additional annotations to add to the controller service. |
| kserve.controller.tag | string | `"v0.15.2"` | KServe controller contrainer image tag. |
| kserve.controller.tolerations | list | `[]` | A list of Kubernetes Tolerations, if required. For more information, see [Toleration v1 core](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.27/#toleration-v1-core).  For example:   tolerations:   - key: foo.bar.com/role     operator: Equal     value: master     effect: NoSchedule |
| kserve.controller.topologySpreadConstraints | list | `[]` | A list of Kubernetes TopologySpreadConstraints, if required. For more information, see [Topology spread constraint v1 core](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.27/#topologyspreadconstraint-v1-core  For example:   topologySpreadConstraints:   - maxSkew: 2     topologyKey: topology.kubernetes.io/zone     whenUnsatisfiable: ScheduleAnyway     labelSelector:       matchLabels:         app.kubernetes.io/instance: kserve-controller-manager         app.kubernetes.io/component: controller |
| kserve.controller.webhookServiceAnnotations | object | `{}` | Optional additional annotations to add to the webhook service. |
| kserve.inferenceservice.resources.limits.cpu | string | `"1"` |  |
| kserve.inferenceservice.resources.limits.memory | string | `"2Gi"` |  |
| kserve.inferenceservice.resources.requests.cpu | string | `"1"` |  |
| kserve.inferenceservice.resources.requests.memory | string | `"2Gi"` |  |
| kserve.localmodel.agent.affinity | object | `{}` |  |
| kserve.localmodel.agent.hostPath | string | `"/mnt/models"` |  |
| kserve.localmodel.agent.image | string | `"kserve/kserve-localmodelnode-agent"` |  |
| kserve.localmodel.agent.nodeSelector | object | `{}` |  |
| kserve.localmodel.agent.reconcilationFrequencyInSecs | int | `60` |  |
| kserve.localmodel.agent.securityContext.runAsNonRoot | bool | `true` |  |
| kserve.localmodel.agent.securityContext.runAsUser | int | `1000` |  |
| kserve.localmodel.agent.tag | string | `"v0.15.2"` |  |
| kserve.localmodel.agent.tolerations | list | `[]` |  |
| kserve.localmodel.controller.image | string | `"kserve/kserve-localmodel-controller"` |  |
| kserve.localmodel.controller.tag | string | `"v0.15.2"` |  |
| kserve.localmodel.disableVolumeManagement | bool | `false` |  |
| kserve.localmodel.enabled | bool | `false` |  |
| kserve.localmodel.jobNamespace | string | `"kserve-localmodel-jobs"` |  |
| kserve.localmodel.jobTTLSecondsAfterFinished | int | `3600` |  |
| kserve.localmodel.securityContext.fsGroup | int | `1000` |  |
| kserve.metricsaggregator.enableMetricAggregation | string | `"false"` | configures metric aggregation annotation. This adds the annotation serving.kserve.io/enable-metric-aggregation to every service with the specified boolean value. If true enables metric aggregation in queue-proxy by setting env vars in the queue proxy container to configure scraping ports. |
| kserve.metricsaggregator.enablePrometheusScraping | string | `"false"` | If true, prometheus annotations are added to the pod to scrape the metrics. If serving.kserve.io/enable-metric-aggregation is false, the prometheus port is set with the default prometheus scraping port 9090, otherwise the prometheus port annotation is set with the metric aggregation port. |
| kserve.router.image | string | `"kserve/router"` |  |
| kserve.router.imagePullPolicy | string | `"IfNotPresent"` | Specifies when to pull router image from registry. |
| kserve.router.imagePullSecrets | list | `[]` | specifies the list of secrets to be used for pulling the router image from registry. |
| kserve.router.tag | string | `"v0.15.2"` |  |
| kserve.security.autoMountServiceAccountToken | bool | `true` |  |
| kserve.service.serviceClusterIPNone | bool | `false` |  |
| kserve.servingruntime.art.defaultVersion | string | `"v0.15.2"` |  |
| kserve.servingruntime.art.image | string | `"kserve/art-explainer"` |  |
| kserve.servingruntime.art.imagePullSecrets | list | `[]` |  |
| kserve.servingruntime.huggingfaceserver.devShm.enabled | bool | `false` |  |
| kserve.servingruntime.huggingfaceserver.devShm.sizeLimit | string | `""` |  |
| kserve.servingruntime.huggingfaceserver.disabled | bool | `false` |  |
| kserve.servingruntime.huggingfaceserver.hostIPC.enabled | bool | `false` |  |
| kserve.servingruntime.huggingfaceserver.image | string | `"kserve/huggingfaceserver"` |  |
| kserve.servingruntime.huggingfaceserver.imagePullSecrets | list | `[]` |  |
| kserve.servingruntime.huggingfaceserver.lmcacheUseExperimental | string | `"True"` |  |
| kserve.servingruntime.huggingfaceserver.securityContext.allowPrivilegeEscalation | bool | `false` |  |
| kserve.servingruntime.huggingfaceserver.securityContext.capabilities.drop[0] | string | `"ALL"` |  |
| kserve.servingruntime.huggingfaceserver.securityContext.privileged | bool | `false` |  |
| kserve.servingruntime.huggingfaceserver.securityContext.runAsNonRoot | bool | `true` |  |
| kserve.servingruntime.huggingfaceserver.tag | string | `"v0.15.2"` |  |
| kserve.servingruntime.huggingfaceserver_multinode.disabled | bool | `false` |  |
| kserve.servingruntime.huggingfaceserver_multinode.imagePullSecrets | list | `[]` |  |
| kserve.servingruntime.huggingfaceserver_multinode.securityContext.allowPrivilegeEscalation | bool | `false` |  |
| kserve.servingruntime.huggingfaceserver_multinode.securityContext.capabilities.drop[0] | string | `"ALL"` |  |
| kserve.servingruntime.huggingfaceserver_multinode.securityContext.privileged | bool | `false` |  |
| kserve.servingruntime.huggingfaceserver_multinode.securityContext.runAsNonRoot | bool | `true` |  |
| kserve.servingruntime.huggingfaceserver_multinode.shm.enabled | bool | `true` |  |
| kserve.servingruntime.huggingfaceserver_multinode.shm.sizeLimit | string | `"3Gi"` |  |
| kserve.servingruntime.lgbserver.disabled | bool | `false` |  |
| kserve.servingruntime.lgbserver.image | string | `"kserve/lgbserver"` |  |
| kserve.servingruntime.lgbserver.imagePullSecrets | list | `[]` |  |
| kserve.servingruntime.lgbserver.securityContext.allowPrivilegeEscalation | bool | `false` |  |
| kserve.servingruntime.lgbserver.securityContext.capabilities.drop[0] | string | `"ALL"` |  |
| kserve.servingruntime.lgbserver.securityContext.privileged | bool | `false` |  |
| kserve.servingruntime.lgbserver.securityContext.runAsNonRoot | bool | `true` |  |
| kserve.servingruntime.lgbserver.tag | string | `"v0.15.2"` |  |
| kserve.servingruntime.mlserver.disabled | bool | `false` |  |
| kserve.servingruntime.mlserver.image | string | `"docker.io/seldonio/mlserver"` |  |
| kserve.servingruntime.mlserver.imagePullSecrets | list | `[]` |  |
| kserve.servingruntime.mlserver.modelClassPlaceholder | string | `"{{.Labels.modelClass}}"` |  |
| kserve.servingruntime.mlserver.securityContext.allowPrivilegeEscalation | bool | `false` |  |
| kserve.servingruntime.mlserver.securityContext.capabilities.drop[0] | string | `"ALL"` |  |
| kserve.servingruntime.mlserver.securityContext.privileged | bool | `false` |  |
| kserve.servingruntime.mlserver.securityContext.runAsNonRoot | bool | `true` |  |
| kserve.servingruntime.mlserver.tag | string | `"1.5.0"` |  |
| kserve.servingruntime.modelNamePlaceholder | string | `"{{.Name}}"` |  |
| kserve.servingruntime.paddleserver.disabled | bool | `false` |  |
| kserve.servingruntime.paddleserver.image | string | `"kserve/paddleserver"` |  |
| kserve.servingruntime.paddleserver.imagePullSecrets | list | `[]` |  |
| kserve.servingruntime.paddleserver.securityContext.allowPrivilegeEscalation | bool | `false` |  |
| kserve.servingruntime.paddleserver.securityContext.capabilities.drop[0] | string | `"ALL"` |  |
| kserve.servingruntime.paddleserver.securityContext.privileged | bool | `false` |  |
| kserve.servingruntime.paddleserver.securityContext.runAsNonRoot | bool | `true` |  |
| kserve.servingruntime.paddleserver.tag | string | `"v0.15.2"` |  |
| kserve.servingruntime.pmmlserver.disabled | bool | `false` |  |
| kserve.servingruntime.pmmlserver.image | string | `"kserve/pmmlserver"` |  |
| kserve.servingruntime.pmmlserver.imagePullSecrets | list | `[]` |  |
| kserve.servingruntime.pmmlserver.securityContext.allowPrivilegeEscalation | bool | `false` |  |
| kserve.servingruntime.pmmlserver.securityContext.capabilities.drop[0] | string | `"ALL"` |  |
| kserve.servingruntime.pmmlserver.securityContext.privileged | bool | `false` |  |
| kserve.servingruntime.pmmlserver.securityContext.runAsNonRoot | bool | `true` |  |
| kserve.servingruntime.pmmlserver.tag | string | `"v0.15.2"` |  |
| kserve.servingruntime.sklearnserver.disabled | bool | `false` |  |
| kserve.servingruntime.sklearnserver.image | string | `"kserve/sklearnserver"` |  |
| kserve.servingruntime.sklearnserver.imagePullSecrets | list | `[]` |  |
| kserve.servingruntime.sklearnserver.securityContext.allowPrivilegeEscalation | bool | `false` |  |
| kserve.servingruntime.sklearnserver.securityContext.capabilities.drop[0] | string | `"ALL"` |  |
| kserve.servingruntime.sklearnserver.securityContext.privileged | bool | `false` |  |
| kserve.servingruntime.sklearnserver.securityContext.runAsNonRoot | bool | `true` |  |
| kserve.servingruntime.sklearnserver.tag | string | `"v0.15.2"` |  |
| kserve.servingruntime.tensorflow.disabled | bool | `false` |  |
| kserve.servingruntime.tensorflow.image | string | `"tensorflow/serving"` |  |
| kserve.servingruntime.tensorflow.imagePullSecrets | list | `[]` |  |
| kserve.servingruntime.tensorflow.securityContext.allowPrivilegeEscalation | bool | `false` |  |
| kserve.servingruntime.tensorflow.securityContext.capabilities.drop[0] | string | `"ALL"` |  |
| kserve.servingruntime.tensorflow.securityContext.privileged | bool | `false` |  |
| kserve.servingruntime.tensorflow.securityContext.runAsNonRoot | bool | `true` |  |
| kserve.servingruntime.tensorflow.securityContext.runAsUser | int | `1000` |  |
| kserve.servingruntime.tensorflow.tag | string | `"2.6.2"` |  |
| kserve.servingruntime.torchserve.disabled | bool | `false` |  |
| kserve.servingruntime.torchserve.image | string | `"pytorch/torchserve-kfs"` |  |
| kserve.servingruntime.torchserve.imagePullSecrets | list | `[]` |  |
| kserve.servingruntime.torchserve.securityContext.allowPrivilegeEscalation | bool | `false` |  |
| kserve.servingruntime.torchserve.securityContext.capabilities.drop[0] | string | `"ALL"` |  |
| kserve.servingruntime.torchserve.securityContext.privileged | bool | `false` |  |
| kserve.servingruntime.torchserve.securityContext.runAsNonRoot | bool | `true` |  |
| kserve.servingruntime.torchserve.securityContext.runAsUser | int | `1000` |  |
| kserve.servingruntime.torchserve.serviceEnvelopePlaceholder | string | `"{{.Labels.serviceEnvelope}}"` |  |
| kserve.servingruntime.torchserve.tag | string | `"0.9.0"` |  |
| kserve.servingruntime.tritonserver.disabled | bool | `false` |  |
| kserve.servingruntime.tritonserver.image | string | `"nvcr.io/nvidia/tritonserver"` |  |
| kserve.servingruntime.tritonserver.imagePullSecrets | list | `[]` |  |
| kserve.servingruntime.tritonserver.securityContext.allowPrivilegeEscalation | bool | `false` |  |
| kserve.servingruntime.tritonserver.securityContext.capabilities.drop[0] | string | `"ALL"` |  |
| kserve.servingruntime.tritonserver.securityContext.privileged | bool | `false` |  |
| kserve.servingruntime.tritonserver.securityContext.runAsNonRoot | bool | `true` |  |
| kserve.servingruntime.tritonserver.securityContext.runAsUser | int | `1000` |  |
| kserve.servingruntime.tritonserver.tag | string | `"23.05-py3"` |  |
| kserve.servingruntime.xgbserver.disabled | bool | `false` |  |
| kserve.servingruntime.xgbserver.image | string | `"kserve/xgbserver"` |  |
| kserve.servingruntime.xgbserver.imagePullSecrets | list | `[]` |  |
| kserve.servingruntime.xgbserver.securityContext.allowPrivilegeEscalation | bool | `false` |  |
| kserve.servingruntime.xgbserver.securityContext.capabilities.drop[0] | string | `"ALL"` |  |
| kserve.servingruntime.xgbserver.securityContext.privileged | bool | `false` |  |
| kserve.servingruntime.xgbserver.securityContext.runAsNonRoot | bool | `true` |  |
| kserve.servingruntime.xgbserver.tag | string | `"v0.15.2"` |  |
| kserve.storage.caBundleConfigMapName | string | `""` | Mounted CA bundle config map name for storage initializer. |
| kserve.storage.caBundleVolumeMountPath | string | `"/etc/ssl/custom-certs"` | Mounted path for CA bundle config map. |
| kserve.storage.containerSecurityContext.allowPrivilegeEscalation | bool | `false` |  |
| kserve.storage.containerSecurityContext.capabilities.drop[0] | string | `"ALL"` |  |
| kserve.storage.containerSecurityContext.privileged | bool | `false` |  |
| kserve.storage.containerSecurityContext.runAsNonRoot | bool | `true` |  |
| kserve.storage.cpuModelcar | string | `"10m"` | Model sidecar cpu requirement. |
| kserve.storage.enableModelcar | bool | `true` | Flag for enabling model sidecar feature. |
| kserve.storage.image | string | `"kserve/storage-initializer"` |  |
| kserve.storage.memoryModelcar | string | `"15Mi"` | Model sidecar memory requirement. |
| kserve.storage.s3 | object | `{"CABundle":"","accessKeyIdName":"AWS_ACCESS_KEY_ID","endpoint":"","region":"","secretAccessKeyName":"AWS_SECRET_ACCESS_KEY","useAnonymousCredential":"","useHttps":"","useVirtualBucket":"","verifySSL":""}` | Configurations for S3 storage |
| kserve.storage.s3.CABundle | string | `""` | The path to the certificate bundle to use for HTTPS certificate validation. |
| kserve.storage.s3.accessKeyIdName | string | `"AWS_ACCESS_KEY_ID"` | AWS S3 static access key id. |
| kserve.storage.s3.endpoint | string | `""` | AWS S3 endpoint. |
| kserve.storage.s3.region | string | `""` | Default region name of AWS S3. |
| kserve.storage.s3.secretAccessKeyName | string | `"AWS_SECRET_ACCESS_KEY"` | AWS S3 static secret access key. |
| kserve.storage.s3.useAnonymousCredential | string | `""` | Whether to use anonymous credentials to download the model or not, default to false. |
| kserve.storage.s3.useHttps | string | `""` | Whether to use secured https or http to download models, allowed values are 0 and 1 and default to 1. |
| kserve.storage.s3.useVirtualBucket | string | `""` | Whether to use virtual bucket or not, default to false. |
| kserve.storage.s3.verifySSL | string | `""` | Whether to verify the tls/ssl certificate, default to true. |
| kserve.storage.storageSecretNameAnnotation | string | `"serving.kserve.io/secretName"` | Storage secret name reference for storage initializer. |
| kserve.storage.storageSpecSecretName | string | `"storage-config"` | Storage spec secret name. |
| kserve.storage.tag | string | `"v0.15.2"` |  |
| kserve.version | string | `"v0.15.2"` |  |
