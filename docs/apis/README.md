<p>Packages:</p>
<ul>
<li>
<a href="#serving.kubeflow.org%2fv1alpha2">serving.kubeflow.org/v1alpha2</a>
</li>
</ul>
<h2 id="serving.kubeflow.org/v1alpha2">serving.kubeflow.org/v1alpha2</h2>
<p>
<p>Package v1alpha2 contains API Schema definitions for the serving v1alpha2 API group</p>
</p>
Resource Types:
<ul></ul>
<h3 id="serving.kubeflow.org/v1alpha2.AlibiExplainerSpec">AlibiExplainerSpec
</h3>
<p>
(<em>Appears on:</em>
<a href="#serving.kubeflow.org/v1alpha2.ExplainerSpec">ExplainerSpec</a>)
</p>
<p>
<p>AlibiExplainerSpec defines the arguments for configuring an Alibi Explanation Server</p>
</p>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>type</code></br>
<em>
<a href="#serving.kubeflow.org/v1alpha2.AlibiExplainerType">
AlibiExplainerType
</a>
</em>
</td>
<td>
<p>The type of Alibi explainer</p>
</td>
</tr>
<tr>
<td>
<code>storageUri</code></br>
<em>
string
</em>
</td>
<td>
<p>The location of a trained explanation model</p>
</td>
</tr>
<tr>
<td>
<code>runtimeVersion</code></br>
<em>
string
</em>
</td>
<td>
<p>Alibi docker image version which defaults to latest release</p>
</td>
</tr>
<tr>
<td>
<code>resources</code></br>
<em>
<a href="https://v1-18.docs.kubernetes.io/docs/reference/generated/kubernetes-api/v1.18/#resourcerequirements-v1-core">
Kubernetes core/v1.ResourceRequirements
</a>
</em>
</td>
<td>
<p>Defaults to requests and limits of 1CPU, 2Gb MEM.</p>
</td>
</tr>
<tr>
<td>
<code>config</code></br>
<em>
map[string]string
</em>
</td>
<td>
<p>Inline custom parameter settings for explainer</p>
</td>
</tr>
</tbody>
</table>
<h3 id="serving.kubeflow.org/v1alpha2.AlibiExplainerType">AlibiExplainerType
(<code>string</code> alias)</p></h3>
<p>
(<em>Appears on:</em>
<a href="#serving.kubeflow.org/v1alpha2.AlibiExplainerSpec">AlibiExplainerSpec</a>)
</p>
<p>
</p>
<table>
<thead>
<tr>
<th>Value</th>
<th>Description</th>
</tr>
</thead>
<tbody><tr><td><p>&#34;AnchorImages&#34;</p></td>
<td></td>
</tr><tr><td><p>&#34;AnchorTabular&#34;</p></td>
<td></td>
</tr><tr><td><p>&#34;AnchorText&#34;</p></td>
<td></td>
</tr><tr><td><p>&#34;Contrastive&#34;</p></td>
<td></td>
</tr><tr><td><p>&#34;Counterfactuals&#34;</p></td>
<td></td>
</tr></tbody>
</table>
<h3 id="serving.kubeflow.org/v1alpha2.Batcher">Batcher
</h3>
<p>
(<em>Appears on:</em>
<a href="#serving.kubeflow.org/v1alpha2.DeploymentSpec">DeploymentSpec</a>)
</p>
<p>
<p>Batcher provides optional payload batcher for all endpoints</p>
</p>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>maxBatchSize</code></br>
<em>
int
</em>
</td>
<td>
<em>(Optional)</em>
<p>MaxBatchSize of batcher service</p>
</td>
</tr>
<tr>
<td>
<code>maxLatency</code></br>
<em>
int
</em>
</td>
<td>
<em>(Optional)</em>
<p>MaxLatency of batcher service</p>
</td>
</tr>
<tr>
<td>
<code>timeout</code></br>
<em>
int
</em>
</td>
<td>
<em>(Optional)</em>
<p>Timeout of batcher service</p>
</td>
</tr>
</tbody>
</table>
<h3 id="&lt;UNKNOWN_API_GROUP&gt;.ComponentStatusMap">ComponentStatusMap
</h3>
<p>
<p>EndpointStatusMap defines the observed state of InferenceService endpoints</p>
</p>
<h3 id="serving.kubeflow.org/v1alpha2.CustomSpec">CustomSpec
</h3>
<p>
(<em>Appears on:</em>
<a href="#serving.kubeflow.org/v1alpha2.ExplainerSpec">ExplainerSpec</a>, 
<a href="#serving.kubeflow.org/v1alpha2.PredictorSpec">PredictorSpec</a>, 
<a href="#serving.kubeflow.org/v1alpha2.TransformerSpec">TransformerSpec</a>)
</p>
<p>
<p>CustomSpec provides a hook for arbitrary container configuration.</p>
</p>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>container</code></br>
<em>
<a href="https://v1-18.docs.kubernetes.io/docs/reference/generated/kubernetes-api/v1.18/#container-v1-core">
Kubernetes core/v1.Container
</a>
</em>
</td>
<td>
</td>
</tr>
</tbody>
</table>
<h3 id="serving.kubeflow.org/v1alpha2.DeploymentSpec">DeploymentSpec
</h3>
<p>
(<em>Appears on:</em>
<a href="#serving.kubeflow.org/v1alpha2.ExplainerSpec">ExplainerSpec</a>, 
<a href="#serving.kubeflow.org/v1alpha2.PredictorSpec">PredictorSpec</a>, 
<a href="#serving.kubeflow.org/v1alpha2.TransformerSpec">TransformerSpec</a>)
</p>
<p>
<p>DeploymentSpec defines the configuration for a given InferenceService service component</p>
</p>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>serviceAccountName</code></br>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>ServiceAccountName is the name of the ServiceAccount to use to run the service</p>
</td>
</tr>
<tr>
<td>
<code>minReplicas</code></br>
<em>
int
</em>
</td>
<td>
<em>(Optional)</em>
<p>Minimum number of replicas which defaults to 1, when minReplicas = 0 pods scale down to 0 in case of no traffic</p>
</td>
</tr>
<tr>
<td>
<code>maxReplicas</code></br>
<em>
int
</em>
</td>
<td>
<em>(Optional)</em>
<p>This is the up bound for autoscaler to scale to</p>
</td>
</tr>
<tr>
<td>
<code>parallelism</code></br>
<em>
int
</em>
</td>
<td>
<p>Parallelism specifies how many requests can be processed concurrently, this sets the hard limit of the container
concurrency(<a href="https://knative.dev/docs/serving/autoscaling/concurrency">https://knative.dev/docs/serving/autoscaling/concurrency</a>).</p>
</td>
</tr>
<tr>
<td>
<code>logger</code></br>
<em>
<a href="#serving.kubeflow.org/v1alpha2.Logger">
Logger
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Activate request/response logging</p>
</td>
</tr>
<tr>
<td>
<code>batcher</code></br>
<em>
<a href="#serving.kubeflow.org/v1alpha2.Batcher">
Batcher
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Activate request batching</p>
</td>
</tr>
</tbody>
</table>
<h3 id="serving.kubeflow.org/v1alpha2.EndpointSpec">EndpointSpec
</h3>
<p>
(<em>Appears on:</em>
<a href="#serving.kubeflow.org/v1alpha2.InferenceServiceSpec">InferenceServiceSpec</a>)
</p>
<p>
</p>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>predictor</code></br>
<em>
<a href="#serving.kubeflow.org/v1alpha2.PredictorSpec">
PredictorSpec
</a>
</em>
</td>
<td>
<p>Predictor defines the model serving spec</p>
</td>
</tr>
<tr>
<td>
<code>explainer</code></br>
<em>
<a href="#serving.kubeflow.org/v1alpha2.ExplainerSpec">
ExplainerSpec
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Explainer defines the model explanation service spec,
explainer service calls to predictor or transformer if it is specified.</p>
</td>
</tr>
<tr>
<td>
<code>transformer</code></br>
<em>
<a href="#serving.kubeflow.org/v1alpha2.TransformerSpec">
TransformerSpec
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Transformer defines the pre/post processing before and after the predictor call,
transformer service calls to predictor service.</p>
</td>
</tr>
</tbody>
</table>
<h3 id="serving.kubeflow.org/v1alpha2.Explainer">Explainer
</h3>
<p>
</p>
<h3 id="serving.kubeflow.org/v1alpha2.ExplainerConfig">ExplainerConfig
</h3>
<p>
(<em>Appears on:</em>
<a href="#serving.kubeflow.org/v1alpha2.ExplainersConfig">ExplainersConfig</a>)
</p>
<p>
</p>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>image</code></br>
<em>
string
</em>
</td>
<td>
</td>
</tr>
<tr>
<td>
<code>defaultImageVersion</code></br>
<em>
string
</em>
</td>
<td>
</td>
</tr>
</tbody>
</table>
<h3 id="serving.kubeflow.org/v1alpha2.ExplainerSpec">ExplainerSpec
</h3>
<p>
(<em>Appears on:</em>
<a href="#serving.kubeflow.org/v1alpha2.EndpointSpec">EndpointSpec</a>)
</p>
<p>
<p>ExplainerSpec defines the arguments for a model explanation server,
The following fields follow a &ldquo;1-of&rdquo; semantic. Users must specify exactly one spec.</p>
</p>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>alibi</code></br>
<em>
<a href="#serving.kubeflow.org/v1alpha2.AlibiExplainerSpec">
AlibiExplainerSpec
</a>
</em>
</td>
<td>
<p>Spec for alibi explainer</p>
</td>
</tr>
<tr>
<td>
<code>custom</code></br>
<em>
<a href="#serving.kubeflow.org/v1alpha2.CustomSpec">
CustomSpec
</a>
</em>
</td>
<td>
<p>Spec for a custom explainer</p>
</td>
</tr>
<tr>
<td>
<code>DeploymentSpec</code></br>
<em>
<a href="#serving.kubeflow.org/v1alpha2.DeploymentSpec">
DeploymentSpec
</a>
</em>
</td>
<td>
<p>
(Members of <code>DeploymentSpec</code> are embedded into this type.)
</p>
</td>
</tr>
</tbody>
</table>
<h3 id="serving.kubeflow.org/v1alpha2.ExplainersConfig">ExplainersConfig
</h3>
<p>
(<em>Appears on:</em>
<a href="#serving.kubeflow.org/v1alpha2.InferenceServicesConfig">InferenceServicesConfig</a>)
</p>
<p>
</p>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>alibi</code></br>
<em>
<a href="#serving.kubeflow.org/v1alpha2.ExplainerConfig">
ExplainerConfig
</a>
</em>
</td>
<td>
</td>
</tr>
</tbody>
</table>
<h3 id="serving.kubeflow.org/v1alpha2.InferenceService">InferenceService
</h3>
<p>
<p>InferenceService is the Schema for the services API</p>
</p>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>metadata</code></br>
<em>
<a href="https://v1-18.docs.kubernetes.io/docs/reference/generated/kubernetes-api/v1.18/#objectmeta-v1-meta">
Kubernetes meta/v1.ObjectMeta
</a>
</em>
</td>
<td>
Refer to the Kubernetes API documentation for the fields of the
<code>metadata</code> field.
</td>
</tr>
<tr>
<td>
<code>spec</code></br>
<em>
<a href="#serving.kubeflow.org/v1alpha2.InferenceServiceSpec">
InferenceServiceSpec
</a>
</em>
</td>
<td>
<br/>
<br/>
<table>
<tr>
<td>
<code>default</code></br>
<em>
<a href="#serving.kubeflow.org/v1alpha2.EndpointSpec">
EndpointSpec
</a>
</em>
</td>
<td>
<p>Default defines default InferenceService endpoints</p>
</td>
</tr>
<tr>
<td>
<code>canary</code></br>
<em>
<a href="#serving.kubeflow.org/v1alpha2.EndpointSpec">
EndpointSpec
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Canary defines alternate endpoints to route a percentage of traffic.</p>
</td>
</tr>
<tr>
<td>
<code>canaryTrafficPercent</code></br>
<em>
int
</em>
</td>
<td>
<em>(Optional)</em>
<p>CanaryTrafficPercent defines the percentage of traffic going to canary InferenceService endpoints</p>
</td>
</tr>
</table>
</td>
</tr>
<tr>
<td>
<code>status</code></br>
<em>
<a href="#serving.kubeflow.org/v1alpha2.InferenceServiceStatus">
InferenceServiceStatus
</a>
</em>
</td>
<td>
</td>
</tr>
</tbody>
</table>
<h3 id="serving.kubeflow.org/v1alpha2.InferenceServiceSpec">InferenceServiceSpec
</h3>
<p>
(<em>Appears on:</em>
<a href="#serving.kubeflow.org/v1alpha2.InferenceService">InferenceService</a>)
</p>
<p>
<p>InferenceServiceSpec defines the desired state of InferenceService</p>
</p>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>default</code></br>
<em>
<a href="#serving.kubeflow.org/v1alpha2.EndpointSpec">
EndpointSpec
</a>
</em>
</td>
<td>
<p>Default defines default InferenceService endpoints</p>
</td>
</tr>
<tr>
<td>
<code>canary</code></br>
<em>
<a href="#serving.kubeflow.org/v1alpha2.EndpointSpec">
EndpointSpec
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Canary defines alternate endpoints to route a percentage of traffic.</p>
</td>
</tr>
<tr>
<td>
<code>canaryTrafficPercent</code></br>
<em>
int
</em>
</td>
<td>
<em>(Optional)</em>
<p>CanaryTrafficPercent defines the percentage of traffic going to canary InferenceService endpoints</p>
</td>
</tr>
</tbody>
</table>
<h3 id="serving.kubeflow.org/v1alpha2.InferenceServiceState">InferenceServiceState
(<code>string</code> alias)</p></h3>
<p>
<p>InferenceState describes the Readiness of the InferenceService</p>
</p>
<table>
<thead>
<tr>
<th>Value</th>
<th>Description</th>
</tr>
</thead>
<tbody><tr><td><p>&#34;InferenceServiceNotReady&#34;</p></td>
<td></td>
</tr><tr><td><p>&#34;InferenceServiceReady&#34;</p></td>
<td></td>
</tr></tbody>
</table>
<h3 id="serving.kubeflow.org/v1alpha2.InferenceServiceStatus">InferenceServiceStatus
</h3>
<p>
(<em>Appears on:</em>
<a href="#serving.kubeflow.org/v1alpha2.InferenceService">InferenceService</a>)
</p>
<p>
<p>InferenceServiceStatus defines the observed state of InferenceService</p>
</p>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>Status</code></br>
<em>
knative.dev/pkg/apis/duck/v1beta1.Status
</em>
</td>
<td>
<p>
(Members of <code>Status</code> are embedded into this type.)
</p>
</td>
</tr>
<tr>
<td>
<code>url</code></br>
<em>
string
</em>
</td>
<td>
<p>URL of the InferenceService</p>
</td>
</tr>
<tr>
<td>
<code>traffic</code></br>
<em>
int
</em>
</td>
<td>
<p>Traffic percentage that goes to default services</p>
</td>
</tr>
<tr>
<td>
<code>canaryTraffic</code></br>
<em>
int
</em>
</td>
<td>
<p>Traffic percentage that goes to canary services</p>
</td>
</tr>
<tr>
<td>
<code>default</code></br>
<em>
<a href="#serving.kubeflow.org/v1alpha2.map[github.com/kubeflow/kfserving/pkg/constants.InferenceServiceComponent]github.com/kubeflow/kfserving/pkg/apis/serving/v1alpha2.StatusConfigurationSpec">
map[github.com/kubeflow/kfserving/pkg/constants.InferenceServiceComponent]github.com/kubeflow/kfserving/pkg/apis/serving/v1alpha2.StatusConfigurationSpec
</a>
</em>
</td>
<td>
<p>Statuses for the default endpoints of the InferenceService</p>
</td>
</tr>
<tr>
<td>
<code>canary</code></br>
<em>
<a href="#serving.kubeflow.org/v1alpha2.map[github.com/kubeflow/kfserving/pkg/constants.InferenceServiceComponent]github.com/kubeflow/kfserving/pkg/apis/serving/v1alpha2.StatusConfigurationSpec">
map[github.com/kubeflow/kfserving/pkg/constants.InferenceServiceComponent]github.com/kubeflow/kfserving/pkg/apis/serving/v1alpha2.StatusConfigurationSpec
</a>
</em>
</td>
<td>
<p>Statuses for the canary endpoints of the InferenceService</p>
</td>
</tr>
<tr>
<td>
<code>address</code></br>
<em>
knative.dev/pkg/apis/duck/v1beta1.Addressable
</em>
</td>
<td>
<p>Addressable URL for eventing</p>
</td>
</tr>
</tbody>
</table>
<h3 id="serving.kubeflow.org/v1alpha2.InferenceServicesConfig">InferenceServicesConfig
</h3>
<p>
</p>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>transformers</code></br>
<em>
<a href="#serving.kubeflow.org/v1alpha2.TransformersConfig">
TransformersConfig
</a>
</em>
</td>
<td>
</td>
</tr>
<tr>
<td>
<code>predictors</code></br>
<em>
<a href="#serving.kubeflow.org/v1alpha2.PredictorsConfig">
PredictorsConfig
</a>
</em>
</td>
<td>
</td>
</tr>
<tr>
<td>
<code>explainers</code></br>
<em>
<a href="#serving.kubeflow.org/v1alpha2.ExplainersConfig">
ExplainersConfig
</a>
</em>
</td>
<td>
</td>
</tr>
</tbody>
</table>
<h3 id="serving.kubeflow.org/v1alpha2.Logger">Logger
</h3>
<p>
(<em>Appears on:</em>
<a href="#serving.kubeflow.org/v1alpha2.DeploymentSpec">DeploymentSpec</a>)
</p>
<p>
<p>Logger provides optional payload logging for all endpoints</p>
</p>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>url</code></br>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>URL to send request logging CloudEvents</p>
</td>
</tr>
<tr>
<td>
<code>mode</code></br>
<em>
<a href="#serving.kubeflow.org/v1alpha2.LoggerMode">
LoggerMode
</a>
</em>
</td>
<td>
<p>What payloads to log: [all, request, response]</p>
</td>
</tr>
</tbody>
</table>
<h3 id="serving.kubeflow.org/v1alpha2.LoggerMode">LoggerMode
(<code>string</code> alias)</p></h3>
<p>
(<em>Appears on:</em>
<a href="#serving.kubeflow.org/v1alpha2.Logger">Logger</a>)
</p>
<p>
</p>
<table>
<thead>
<tr>
<th>Value</th>
<th>Description</th>
</tr>
</thead>
<tbody><tr><td><p>&#34;all&#34;</p></td>
<td></td>
</tr><tr><td><p>&#34;request&#34;</p></td>
<td></td>
</tr><tr><td><p>&#34;response&#34;</p></td>
<td></td>
</tr></tbody>
</table>
<h3 id="serving.kubeflow.org/v1alpha2.ONNXSpec">ONNXSpec
</h3>
<p>
(<em>Appears on:</em>
<a href="#serving.kubeflow.org/v1alpha2.PredictorSpec">PredictorSpec</a>)
</p>
<p>
<p>ONNXSpec defines arguments for configuring ONNX model serving.</p>
</p>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>storageUri</code></br>
<em>
string
</em>
</td>
<td>
<p>The URI of the exported onnx model(model.onnx)</p>
</td>
</tr>
<tr>
<td>
<code>runtimeVersion</code></br>
<em>
string
</em>
</td>
<td>
<p>ONNXRuntime docker image versions, default version can be set in the inferenceservice configmap</p>
</td>
</tr>
<tr>
<td>
<code>resources</code></br>
<em>
<a href="https://v1-18.docs.kubernetes.io/docs/reference/generated/kubernetes-api/v1.18/#resourcerequirements-v1-core">
Kubernetes core/v1.ResourceRequirements
</a>
</em>
</td>
<td>
<p>Defaults to requests and limits of 1CPU, 2Gb MEM.</p>
</td>
</tr>
</tbody>
</table>
<h3 id="serving.kubeflow.org/v1alpha2.Predictor">Predictor
</h3>
<p>
</p>
<h3 id="serving.kubeflow.org/v1alpha2.PredictorConfig">PredictorConfig
</h3>
<p>
(<em>Appears on:</em>
<a href="#serving.kubeflow.org/v1alpha2.PredictorsConfig">PredictorsConfig</a>)
</p>
<p>
</p>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>image</code></br>
<em>
string
</em>
</td>
<td>
</td>
</tr>
<tr>
<td>
<code>defaultImageVersion</code></br>
<em>
string
</em>
</td>
<td>
</td>
</tr>
<tr>
<td>
<code>defaultGpuImageVersion</code></br>
<em>
string
</em>
</td>
<td>
</td>
</tr>
</tbody>
</table>
<h3 id="serving.kubeflow.org/v1alpha2.PredictorSpec">PredictorSpec
</h3>
<p>
(<em>Appears on:</em>
<a href="#serving.kubeflow.org/v1alpha2.EndpointSpec">EndpointSpec</a>)
</p>
<p>
<p>PredictorSpec defines the configuration for a predictor,
The following fields follow a &ldquo;1-of&rdquo; semantic. Users must specify exactly one spec.</p>
</p>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>custom</code></br>
<em>
<a href="#serving.kubeflow.org/v1alpha2.CustomSpec">
CustomSpec
</a>
</em>
</td>
<td>
<p>Spec for a custom predictor</p>
</td>
</tr>
<tr>
<td>
<code>tensorflow</code></br>
<em>
<a href="#serving.kubeflow.org/v1alpha2.TensorflowSpec">
TensorflowSpec
</a>
</em>
</td>
<td>
<p>Spec for Tensorflow Serving (<a href="https://github.com/tensorflow/serving">https://github.com/tensorflow/serving</a>)</p>
</td>
</tr>
<tr>
<td>
<code>triton</code></br>
<em>
<a href="#serving.kubeflow.org/v1alpha2.TritonSpec">
TritonSpec
</a>
</em>
</td>
<td>
<p>Spec for Triton Inference Server (<a href="https://github.com/NVIDIA/triton-inference-server">https://github.com/NVIDIA/triton-inference-server</a>)</p>
</td>
</tr>
<tr>
<td>
<code>xgboost</code></br>
<em>
<a href="#serving.kubeflow.org/v1alpha2.XGBoostSpec">
XGBoostSpec
</a>
</em>
</td>
<td>
<p>Spec for XGBoost predictor</p>
</td>
</tr>
<tr>
<td>
<code>sklearn</code></br>
<em>
<a href="#serving.kubeflow.org/v1alpha2.SKLearnSpec">
SKLearnSpec
</a>
</em>
</td>
<td>
<p>Spec for SKLearn predictor</p>
</td>
</tr>
<tr>
<td>
<code>onnx</code></br>
<em>
<a href="#serving.kubeflow.org/v1alpha2.ONNXSpec">
ONNXSpec
</a>
</em>
</td>
<td>
<p>Spec for ONNX runtime (<a href="https://github.com/microsoft/onnxruntime">https://github.com/microsoft/onnxruntime</a>)</p>
</td>
</tr>
<tr>
<td>
<code>pytorch</code></br>
<em>
<a href="#serving.kubeflow.org/v1alpha2.PyTorchSpec">
PyTorchSpec
</a>
</em>
</td>
<td>
<p>Spec for PyTorch predictor</p>
</td>
</tr>
<tr>
<td>
<code>DeploymentSpec</code></br>
<em>
<a href="#serving.kubeflow.org/v1alpha2.DeploymentSpec">
DeploymentSpec
</a>
</em>
</td>
<td>
<p>
(Members of <code>DeploymentSpec</code> are embedded into this type.)
</p>
</td>
</tr>
</tbody>
</table>
<h3 id="serving.kubeflow.org/v1alpha2.PredictorsConfig">PredictorsConfig
</h3>
<p>
(<em>Appears on:</em>
<a href="#serving.kubeflow.org/v1alpha2.InferenceServicesConfig">InferenceServicesConfig</a>)
</p>
<p>
</p>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>tensorflow</code></br>
<em>
<a href="#serving.kubeflow.org/v1alpha2.PredictorConfig">
PredictorConfig
</a>
</em>
</td>
<td>
</td>
</tr>
<tr>
<td>
<code>triton</code></br>
<em>
<a href="#serving.kubeflow.org/v1alpha2.PredictorConfig">
PredictorConfig
</a>
</em>
</td>
<td>
</td>
</tr>
<tr>
<td>
<code>xgboost</code></br>
<em>
<a href="#serving.kubeflow.org/v1alpha2.PredictorConfig">
PredictorConfig
</a>
</em>
</td>
<td>
</td>
</tr>
<tr>
<td>
<code>sklearn</code></br>
<em>
<a href="#serving.kubeflow.org/v1alpha2.PredictorConfig">
PredictorConfig
</a>
</em>
</td>
<td>
</td>
</tr>
<tr>
<td>
<code>pytorch</code></br>
<em>
<a href="#serving.kubeflow.org/v1alpha2.PredictorConfig">
PredictorConfig
</a>
</em>
</td>
<td>
</td>
</tr>
<tr>
<td>
<code>onnx</code></br>
<em>
<a href="#serving.kubeflow.org/v1alpha2.PredictorConfig">
PredictorConfig
</a>
</em>
</td>
<td>
</td>
</tr>
</tbody>
</table>
<h3 id="serving.kubeflow.org/v1alpha2.PyTorchSpec">PyTorchSpec
</h3>
<p>
(<em>Appears on:</em>
<a href="#serving.kubeflow.org/v1alpha2.PredictorSpec">PredictorSpec</a>)
</p>
<p>
<p>PyTorchSpec defines arguments for configuring PyTorch model serving.</p>
</p>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>storageUri</code></br>
<em>
string
</em>
</td>
<td>
<p>The URI of the trained model which contains model.pt</p>
</td>
</tr>
<tr>
<td>
<code>modelClassName</code></br>
<em>
string
</em>
</td>
<td>
<p>Defaults PyTorch model class name to &lsquo;PyTorchModel&rsquo;</p>
</td>
</tr>
<tr>
<td>
<code>runtimeVersion</code></br>
<em>
string
</em>
</td>
<td>
<p>PyTorch KFServer docker image version which defaults to latest release</p>
</td>
</tr>
<tr>
<td>
<code>resources</code></br>
<em>
<a href="https://v1-18.docs.kubernetes.io/docs/reference/generated/kubernetes-api/v1.18/#resourcerequirements-v1-core">
Kubernetes core/v1.ResourceRequirements
</a>
</em>
</td>
<td>
<p>Defaults to requests and limits of 1CPU, 2Gb MEM.</p>
</td>
</tr>
</tbody>
</table>
<h3 id="serving.kubeflow.org/v1alpha2.SKLearnSpec">SKLearnSpec
</h3>
<p>
(<em>Appears on:</em>
<a href="#serving.kubeflow.org/v1alpha2.PredictorSpec">PredictorSpec</a>)
</p>
<p>
<p>SKLearnSpec defines arguments for configuring SKLearn model serving.</p>
</p>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>storageUri</code></br>
<em>
string
</em>
</td>
<td>
<p>The URI of the trained model which contains model.pickle, model.pkl or model.joblib</p>
</td>
</tr>
<tr>
<td>
<code>runtimeVersion</code></br>
<em>
string
</em>
</td>
<td>
<p>SKLearn KFServer docker image version which defaults to latest release</p>
</td>
</tr>
<tr>
<td>
<code>resources</code></br>
<em>
<a href="https://v1-18.docs.kubernetes.io/docs/reference/generated/kubernetes-api/v1.18/#resourcerequirements-v1-core">
Kubernetes core/v1.ResourceRequirements
</a>
</em>
</td>
<td>
<p>Defaults to requests and limits of 1CPU, 2Gb MEM.</p>
</td>
</tr>
</tbody>
</table>
<h3 id="serving.kubeflow.org/v1alpha2.StatusConfigurationSpec">StatusConfigurationSpec
</h3>
<p>
<p>StatusConfigurationSpec describes the state of the configuration receiving traffic.</p>
</p>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>name</code></br>
<em>
string
</em>
</td>
<td>
<p>Latest revision name that is in ready state</p>
</td>
</tr>
<tr>
<td>
<code>host</code></br>
<em>
string
</em>
</td>
<td>
<p>Host name of the service</p>
</td>
</tr>
</tbody>
</table>
<h3 id="serving.kubeflow.org/v1alpha2.TensorflowSpec">TensorflowSpec
</h3>
<p>
(<em>Appears on:</em>
<a href="#serving.kubeflow.org/v1alpha2.PredictorSpec">PredictorSpec</a>)
</p>
<p>
<p>TensorflowSpec defines arguments for configuring Tensorflow model serving.</p>
</p>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>storageUri</code></br>
<em>
string
</em>
</td>
<td>
<p>The URI for the saved model(<a href="https://www.tensorflow.org/tutorials/keras/save_and_load">https://www.tensorflow.org/tutorials/keras/save_and_load</a>)</p>
</td>
</tr>
<tr>
<td>
<code>runtimeVersion</code></br>
<em>
string
</em>
</td>
<td>
<p>TFServing docker image version(<a href="https://hub.docker.com/r/tensorflow/serving">https://hub.docker.com/r/tensorflow/serving</a>), default version can be set in the inferenceservice configmap.</p>
</td>
</tr>
<tr>
<td>
<code>resources</code></br>
<em>
<a href="https://v1-18.docs.kubernetes.io/docs/reference/generated/kubernetes-api/v1.18/#resourcerequirements-v1-core">
Kubernetes core/v1.ResourceRequirements
</a>
</em>
</td>
<td>
<p>Defaults to requests and limits of 1CPU, 2Gb MEM.</p>
</td>
</tr>
</tbody>
</table>
<h3 id="serving.kubeflow.org/v1alpha2.Transformer">Transformer
</h3>
<p>
<p>Transformer interface is implemented by all Transformers</p>
</p>
<h3 id="serving.kubeflow.org/v1alpha2.TransformerConfig">TransformerConfig
</h3>
<p>
(<em>Appears on:</em>
<a href="#serving.kubeflow.org/v1alpha2.TransformersConfig">TransformersConfig</a>)
</p>
<p>
</p>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>image</code></br>
<em>
string
</em>
</td>
<td>
</td>
</tr>
<tr>
<td>
<code>defaultImageVersion</code></br>
<em>
string
</em>
</td>
<td>
</td>
</tr>
</tbody>
</table>
<h3 id="serving.kubeflow.org/v1alpha2.TransformerSpec">TransformerSpec
</h3>
<p>
(<em>Appears on:</em>
<a href="#serving.kubeflow.org/v1alpha2.EndpointSpec">EndpointSpec</a>)
</p>
<p>
<p>TransformerSpec defines transformer service for pre/post processing</p>
</p>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>custom</code></br>
<em>
<a href="#serving.kubeflow.org/v1alpha2.CustomSpec">
CustomSpec
</a>
</em>
</td>
<td>
<p>Spec for a custom transformer</p>
</td>
</tr>
<tr>
<td>
<code>DeploymentSpec</code></br>
<em>
<a href="#serving.kubeflow.org/v1alpha2.DeploymentSpec">
DeploymentSpec
</a>
</em>
</td>
<td>
<p>
(Members of <code>DeploymentSpec</code> are embedded into this type.)
</p>
</td>
</tr>
</tbody>
</table>
<h3 id="serving.kubeflow.org/v1alpha2.TransformersConfig">TransformersConfig
</h3>
<p>
(<em>Appears on:</em>
<a href="#serving.kubeflow.org/v1alpha2.InferenceServicesConfig">InferenceServicesConfig</a>)
</p>
<p>
</p>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>feast</code></br>
<em>
<a href="#serving.kubeflow.org/v1alpha2.TransformerConfig">
TransformerConfig
</a>
</em>
</td>
<td>
</td>
</tr>
</tbody>
</table>
<h3 id="serving.kubeflow.org/v1alpha2.TritonSpec">TritonSpec
</h3>
<p>
(<em>Appears on:</em>
<a href="#serving.kubeflow.org/v1alpha2.PredictorSpec">PredictorSpec</a>)
</p>
<p>
<p>TritonSpec defines arguments for configuring Triton Inference Server.</p>
</p>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>storageUri</code></br>
<em>
string
</em>
</td>
<td>
<p>The URI for the trained model repository(<a href="https://docs.nvidia.com/deeplearning/triton-inference-server/master-user-guide/docs/model_repository.html">https://docs.nvidia.com/deeplearning/triton-inference-server/master-user-guide/docs/model_repository.html</a>)</p>
</td>
</tr>
<tr>
<td>
<code>runtimeVersion</code></br>
<em>
string
</em>
</td>
<td>
<p>Triton Inference Server docker image version, default version can be set in the inferenceservice configmap</p>
</td>
</tr>
<tr>
<td>
<code>resources</code></br>
<em>
<a href="https://v1-18.docs.kubernetes.io/docs/reference/generated/kubernetes-api/v1.18/#resourcerequirements-v1-core">
Kubernetes core/v1.ResourceRequirements
</a>
</em>
</td>
<td>
<p>Defaults to requests and limits of 1CPU, 2Gb MEM.</p>
</td>
</tr>
</tbody>
</table>
<h3 id="serving.kubeflow.org/v1alpha2.VirtualServiceStatus">VirtualServiceStatus
</h3>
<p>
<p>VirtualServiceStatus captures the status of the virtual service</p>
</p>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>URL</code></br>
<em>
string
</em>
</td>
<td>
</td>
</tr>
<tr>
<td>
<code>CanaryWeight</code></br>
<em>
int
</em>
</td>
<td>
</td>
</tr>
<tr>
<td>
<code>DefaultWeight</code></br>
<em>
int
</em>
</td>
<td>
</td>
</tr>
<tr>
<td>
<code>address</code></br>
<em>
knative.dev/pkg/apis/duck/v1beta1.Addressable
</em>
</td>
<td>
<em>(Optional)</em>
<p>Address holds the information needed for a Route to be the target of an event.</p>
</td>
</tr>
<tr>
<td>
<code>Status</code></br>
<em>
knative.dev/pkg/apis/duck/v1beta1.Status
</em>
</td>
<td>
</td>
</tr>
</tbody>
</table>
<h3 id="serving.kubeflow.org/v1alpha2.XGBoostSpec">XGBoostSpec
</h3>
<p>
(<em>Appears on:</em>
<a href="#serving.kubeflow.org/v1alpha2.PredictorSpec">PredictorSpec</a>)
</p>
<p>
<p>XGBoostSpec defines arguments for configuring XGBoost model serving.</p>
</p>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>storageUri</code></br>
<em>
string
</em>
</td>
<td>
<p>The URI of the trained model which contains model.bst</p>
</td>
</tr>
<tr>
<td>
<code>nthread</code></br>
<em>
int
</em>
</td>
<td>
<p>Number of thread to be used by XGBoost</p>
</td>
</tr>
<tr>
<td>
<code>runtimeVersion</code></br>
<em>
string
</em>
</td>
<td>
<p>XGBoost KFServer docker image version which defaults to latest release</p>
</td>
</tr>
<tr>
<td>
<code>resources</code></br>
<em>
<a href="https://v1-18.docs.kubernetes.io/docs/reference/generated/kubernetes-api/v1.18/#resourcerequirements-v1-core">
Kubernetes core/v1.ResourceRequirements
</a>
</em>
</td>
<td>
<p>Defaults to requests and limits of 1CPU, 2Gb MEM.</p>
</td>
</tr>
</tbody>
</table>
<hr/>
<p><em>
Generated with <code>gen-crd-api-reference-docs</code>
on git commit <code>df869c1</code>.
</em></p>
