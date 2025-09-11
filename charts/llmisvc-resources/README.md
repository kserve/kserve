# llmisvc-resources

Helm chart for deploying KServe LLMInferenceService resources

![Version: v0.16.0-rc0](https://img.shields.io/badge/Version-v0.16.0--rc0-informational?style=flat-square) ![Type: application](https://img.shields.io/badge/Type-application-informational?style=flat-square) ![AppVersion: v0.16.0-rc0](https://img.shields.io/badge/AppVersion-v0.16.0--rc0-informational?style=flat-square)

## Installing the Chart

To install the chart, run the following:

```console
$ helm install llmisvc oci://ghcr.io/kserve/charts/llmisvc --version v0.16.0-rc0
```

## Values

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| commonAnnotations | object | `{}` | Common annotations to add to all resources |
| commonLabels | object | `{}` | Common labels to add to all resources |
| kserve.llmisvc.controller.affinity | object | `{}` | A Kubernetes Affinity, if required For more information, see [Affinity v1 core](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.27/#affinity-v1-core)  For example:   affinity:     nodeAffinity:      requiredDuringSchedulingIgnoredDuringExecution:        nodeSelectorTerms:        - matchExpressions:          - key: foo.bar.com/role            operator: In            values:            - master |
| kserve.llmisvc.controller.annotations | object | `{}` | Optional additional annotations to add to the controller deployment |
| kserve.llmisvc.controller.containerSecurityContext | object | `{"allowPrivilegeEscalation":false,"capabilities":{"drop":["ALL"]},"privileged":false,"readOnlyRootFilesystem":true,"runAsNonRoot":true,"runAsUser":1000,"seccompProfile":{"type":"RuntimeDefault"}}` | Container Security Context to be set on the controller component container For more information, see [Configure a Security Context for a Pod or Container](https://kubernetes.io/docs/tasks/configure-pod-container/security-context/) |
| kserve.llmisvc.controller.env | list | `[]` | Environment variables to be set on the controller container |
| kserve.llmisvc.controller.extraArgs | list | `[]` | Additional command line arguments |
| kserve.llmisvc.controller.extraVolumeMounts | list | `[]` | Additional volume mounts |
| kserve.llmisvc.controller.extraVolumes | list | `[]` | Additional volumes to be mounted |
| kserve.llmisvc.controller.image | string | `"kserve/llmisvc-controller"` | KServe LLM ISVC controller manager container image |
| kserve.llmisvc.controller.imagePullPolicy | string | `"IfNotPresent"` | Specifies when to pull controller image from registry |
| kserve.llmisvc.controller.imagePullSecrets | list | `[]` | Reference to one or more secrets to be used when pulling images For more information, see [Pull an Image from a Private Registry](https://kubernetes.io/docs/tasks/configure-pod-container/pull-image-private-registry/)  For example:  imagePullSecrets:    - name: "image-pull-secret" |
| kserve.llmisvc.controller.labels | object | `{}` | Optional additional labels to add to the controller deployment |
| kserve.llmisvc.controller.livenessProbe | object | `{"enabled":true,"failureThreshold":5,"httpGet":{"path":"/healthz","port":8081},"initialDelaySeconds":30,"periodSeconds":10,"timeoutSeconds":5}` | Liveness probe configuration |
| kserve.llmisvc.controller.metricsBindAddress | string | `"127.0.0.1"` | Metrics bind address |
| kserve.llmisvc.controller.metricsBindPort | string | `"8443"` | Metrics bind port |
| kserve.llmisvc.controller.nodeSelector | object | `{}` | The nodeSelector on Pods tells Kubernetes to schedule Pods on the nodes with matching labels For more information, see [Assigning Pods to Nodes](https://kubernetes.io/docs/concepts/scheduling-eviction/assign-pod-node/)  For example:   nodeSelector:     kubernetes.io/arch: amd64 |
| kserve.llmisvc.controller.podAnnotations | object | `{}` | Optional additional annotations to add to the controller Pods |
| kserve.llmisvc.controller.podLabels | object | `{}` | Optional additional labels to add to the controller Pods |
| kserve.llmisvc.controller.readinessProbe | object | `{"enabled":true,"failureThreshold":5,"httpGet":{"path":"/readyz","port":8081},"initialDelaySeconds":30,"periodSeconds":5,"timeoutSeconds":5}` | Readiness probe configuration |
| kserve.llmisvc.controller.replicas | int | `1` | Number of replicas for the controller deployment |
| kserve.llmisvc.controller.resources | object | `{"limits":{"cpu":"100m","memory":"300Mi"},"requests":{"cpu":"100m","memory":"300Mi"}}` | Resources to provide to the llmisvc controller pod  For example:  resources:    limits:      cpu: 100m      memory: 300Mi    requests:      cpu: 100m      memory: 300Mi  For more information, see [Resource Management for Pods and Containers](https://kubernetes.io/docs/concepts/configuration/manage-resources-containers/) |
| kserve.llmisvc.controller.securityContext | object | `{"runAsNonRoot":true,"seccompProfile":{"type":"RuntimeDefault"}}` | Pod Security Context For more information, see [Configure a Security Context for a Pod or Container](https://kubernetes.io/docs/tasks/configure-pod-container/security-context/) |
| kserve.llmisvc.controller.service | object | `{"port":8443,"targetPort":"metrics","type":"ClusterIP"}` | Service configuration |
| kserve.llmisvc.controller.service.port | int | `8443` | Service port for metrics |
| kserve.llmisvc.controller.service.targetPort | string | `"metrics"` | Service target port |
| kserve.llmisvc.controller.service.type | string | `"ClusterIP"` | Service type |
| kserve.llmisvc.controller.serviceAccount | object | `{"name":""}` | Service account configuration |
| kserve.llmisvc.controller.serviceAccount.name | string | `""` | Name of the service account to use If not set, a name is generated using the deployment name |
| kserve.llmisvc.controller.serviceAnnotations | object | `{}` | Optional additional annotations to add to the controller service |
| kserve.llmisvc.controller.strategy | object | `{"rollingUpdate":{"maxSurge":1,"maxUnavailable":0},"type":"RollingUpdate"}` | Deployment strategy |
| kserve.llmisvc.controller.tag | string | `"v0.16.0-rc0"` | KServe LLM ISVC controller container image tag |
| kserve.llmisvc.controller.terminationGracePeriodSeconds | int | `10` | Termination grace period in seconds |
| kserve.llmisvc.controller.tolerations | list | `[]` | A list of Kubernetes Tolerations, if required For more information, see [Toleration v1 core](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.27/#toleration-v1-core)  For example:   tolerations:   - key: foo.bar.com/role     operator: Equal     value: master     effect: NoSchedule |
| kserve.llmisvc.controller.topologySpreadConstraints | list | `[]` | A list of Kubernetes TopologySpreadConstraints, if required For more information, see [Topology spread constraint v1 core](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.27/#topologyspreadconstraint-v1-core)  For example:   topologySpreadConstraints:   - maxSkew: 2     topologyKey: topology.kubernetes.io/zone     whenUnsatisfiable: ScheduleAnyway     labelSelector:       matchLabels:         app.kubernetes.io/instance: llmisvc-controller-manager         app.kubernetes.io/component: controller |
| kserve.version | string | `"v0.16.0-rc0"` | Version of KServe LLM ISVC components |
