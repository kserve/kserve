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
<ul><li>
<a href="#serving.kubeflow.org/v1alpha2.KFService">KFService</a>
</li></ul>
<h3 id="serving.kubeflow.org/v1alpha2.KFService">KFService
</h3>
<p>
<p>KFService is the Schema for the services API</p>
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
<code>apiVersion</code></br>
string</td>
<td>
<code>
serving.kubeflow.org/v1alpha2
</code>
</td>
</tr>
<tr>
<td>
<code>kind</code></br>
string
</td>
<td><code>KFService</code></td>
</tr>
<tr>
<td>
<code>metadata</code></br>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.13/#objectmeta-v1-meta">
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
<a href="#serving.kubeflow.org/v1alpha2.KFServiceSpec">
KFServiceSpec
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
<p>Default defines default KFService endpoints</p>
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
<p>Canary defines an alternate endpoints to route a percentage of traffic.</p>
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
<p>CanaryTrafficPercent defines the percentage of traffic going to canary KFService endpoints</p>
</td>
</tr>
</table>
</td>
</tr>
<tr>
<td>
<code>status</code></br>
<em>
<a href="#serving.kubeflow.org/v1alpha2.KFServiceStatus">
KFServiceStatus
</a>
</em>
</td>
<td>
</td>
</tr>
</tbody>
</table>
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
<p>Defaults to latest Alibi Version.</p>
</td>
</tr>
<tr>
<td>
<code>resources</code></br>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.13/#resourcerequirements-v1-core">
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
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.13/#container-v1-core">
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
<p>DeploymentSpec defines the configuration for a given KFService service component</p>
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
<p>Minimum number of replicas, pods won&rsquo;t scale down to 0 in case of no traffic</p>
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
</tbody>
</table>
<h3 id="serving.kubeflow.org/v1alpha2.EndpointSpec">EndpointSpec
</h3>
<p>
(<em>Appears on:</em>
<a href="#serving.kubeflow.org/v1alpha2.KFServiceSpec">KFServiceSpec</a>)
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
<p>Explainer defines the model explanation service spec
explainer service calls to transformer or predictor service</p>
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
<p>Transformer defines the transformer service spec for pre/post processing
transformer service calls to predictor service</p>
</td>
</tr>
</tbody>
</table>
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
</tbody>
</table>
<h3 id="serving.kubeflow.org/v1alpha2.ExplainerHandler">ExplainerHandler
</h3>
<p>
</p>
<h3 id="serving.kubeflow.org/v1alpha2.ExplainerSpec">ExplainerSpec
</h3>
<p>
(<em>Appears on:</em>
<a href="#serving.kubeflow.org/v1alpha2.EndpointSpec">EndpointSpec</a>)
</p>
<p>
<p>ExplainerSpec defines the arguments for a model explanation server</p>
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
<p>The following fields follow a &ldquo;1-of&rdquo; semantic. Users must specify exactly one spec.</p>
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
<h3 id="serving.kubeflow.org/v1alpha2.FrameworkConfig">FrameworkConfig
</h3>
<p>
(<em>Appears on:</em>
<a href="#serving.kubeflow.org/v1alpha2.FrameworksConfig">FrameworksConfig</a>)
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
</tbody>
</table>
<h3 id="serving.kubeflow.org/v1alpha2.FrameworkHandler">FrameworkHandler
</h3>
<p>
</p>
<h3 id="serving.kubeflow.org/v1alpha2.FrameworksConfig">FrameworksConfig
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
<code>tensorflow</code></br>
<em>
<a href="#serving.kubeflow.org/v1alpha2.FrameworkConfig">
FrameworkConfig
</a>
</em>
</td>
<td>
</td>
</tr>
<tr>
<td>
<code>tensorrt</code></br>
<em>
<a href="#serving.kubeflow.org/v1alpha2.FrameworkConfig">
FrameworkConfig
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
<a href="#serving.kubeflow.org/v1alpha2.FrameworkConfig">
FrameworkConfig
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
<a href="#serving.kubeflow.org/v1alpha2.FrameworkConfig">
FrameworkConfig
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
<a href="#serving.kubeflow.org/v1alpha2.FrameworkConfig">
FrameworkConfig
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
<a href="#serving.kubeflow.org/v1alpha2.FrameworkConfig">
FrameworkConfig
</a>
</em>
</td>
<td>
</td>
</tr>
</tbody>
</table>
<h3 id="serving.kubeflow.org/v1alpha2.KFServiceSpec">KFServiceSpec
</h3>
<p>
(<em>Appears on:</em>
<a href="#serving.kubeflow.org/v1alpha2.KFService">KFService</a>)
</p>
<p>
<p>KFServiceSpec defines the desired state of KFService</p>
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
<p>Default defines default KFService endpoints</p>
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
<p>Canary defines an alternate endpoints to route a percentage of traffic.</p>
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
<p>CanaryTrafficPercent defines the percentage of traffic going to canary KFService endpoints</p>
</td>
</tr>
</tbody>
</table>
<h3 id="serving.kubeflow.org/v1alpha2.KFServiceStatus">KFServiceStatus
</h3>
<p>
(<em>Appears on:</em>
<a href="#serving.kubeflow.org/v1alpha2.KFService">KFService</a>)
</p>
<p>
<p>KFServiceStatus defines the observed state of KFService</p>
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
</td>
</tr>
<tr>
<td>
<code>default</code></br>
<em>
<a href="#serving.kubeflow.org/v1alpha2.StatusConfigurationSpec">
StatusConfigurationSpec
</a>
</em>
</td>
<td>
</td>
</tr>
<tr>
<td>
<code>canary</code></br>
<em>
<a href="#serving.kubeflow.org/v1alpha2.StatusConfigurationSpec">
StatusConfigurationSpec
</a>
</em>
</td>
<td>
</td>
</tr>
</tbody>
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
<p>The location of the trained model</p>
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
<p>Defaults to latest ONNX Version.</p>
</td>
</tr>
<tr>
<td>
<code>resources</code></br>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.13/#resourcerequirements-v1-core">
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
<h3 id="serving.kubeflow.org/v1alpha2.PredictorSpec">PredictorSpec
</h3>
<p>
(<em>Appears on:</em>
<a href="#serving.kubeflow.org/v1alpha2.EndpointSpec">EndpointSpec</a>)
</p>
<p>
<p>PredictorSpec defines the configuration to route traffic to a predictor.</p>
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
<p>The following fields follow a &ldquo;1-of&rdquo; semantic. Users must specify exactly one spec.</p>
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
</td>
</tr>
<tr>
<td>
<code>tensorrt</code></br>
<em>
<a href="#serving.kubeflow.org/v1alpha2.TensorRTSpec">
TensorRTSpec
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
<a href="#serving.kubeflow.org/v1alpha2.XGBoostSpec">
XGBoostSpec
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
<a href="#serving.kubeflow.org/v1alpha2.SKLearnSpec">
SKLearnSpec
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
<a href="#serving.kubeflow.org/v1alpha2.ONNXSpec">
ONNXSpec
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
<a href="#serving.kubeflow.org/v1alpha2.PyTorchSpec">
PyTorchSpec
</a>
</em>
</td>
<td>
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
<p>The location of the trained model</p>
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
<p>Defaults to latest PyTorch Version</p>
</td>
</tr>
<tr>
<td>
<code>resources</code></br>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.13/#resourcerequirements-v1-core">
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
<p>The location of the trained model</p>
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
<p>Defaults to latest SKLearn Version.</p>
</td>
</tr>
<tr>
<td>
<code>resources</code></br>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.13/#resourcerequirements-v1-core">
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
(<em>Appears on:</em>
<a href="#serving.kubeflow.org/v1alpha2.KFServiceStatus">KFServiceStatus</a>)
</p>
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
</td>
</tr>
<tr>
<td>
<code>replicas</code></br>
<em>
int
</em>
</td>
<td>
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
</td>
</tr>
</tbody>
</table>
<h3 id="serving.kubeflow.org/v1alpha2.TensorRTSpec">TensorRTSpec
</h3>
<p>
(<em>Appears on:</em>
<a href="#serving.kubeflow.org/v1alpha2.PredictorSpec">PredictorSpec</a>)
</p>
<p>
<p>TensorRTSpec defines arguments for configuring TensorRT model serving.</p>
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
<p>The location of the trained model</p>
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
<p>Defaults to latest TensorRT Version.</p>
</td>
</tr>
<tr>
<td>
<code>resources</code></br>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.13/#resourcerequirements-v1-core">
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
<p>The location of the trained model</p>
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
<p>Defaults to latest TF Version.</p>
</td>
</tr>
<tr>
<td>
<code>resources</code></br>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.13/#resourcerequirements-v1-core">
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
<p>The location of the trained model</p>
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
<p>Defaults to latest XGBoost Version.</p>
</td>
</tr>
<tr>
<td>
<code>resources</code></br>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.13/#resourcerequirements-v1-core">
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
on git commit <code>52e137b</code>.
</em></p>
