<p>Packages:</p>
<ul>
<li>
<a href="#serving.kubeflow.org%2fv1beta1">serving.kubeflow.org/v1beta1</a>
</li>
</ul>
<h2 id="serving.kubeflow.org/v1beta1">serving.kubeflow.org/v1beta1</h2>
<p>
<p>Package v1beta1 contains API Schema definitions for the serving v1beta1 API group</p>
</p>
Resource Types:
<ul></ul>
<h3 id="serving.kubeflow.org/v1beta1.AIXExplainerSpec">AIXExplainerSpec
</h3>
<p>
(<em>Appears on:</em>
<a href="#serving.kubeflow.org/v1beta1.ExplainerSpec">ExplainerSpec</a>)
</p>
<p>
<p>AIXExplainerSpec defines the arguments for configuring an AIX Explanation Server</p>
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
<a href="#serving.kubeflow.org/v1beta1.AIXExplainerType">
AIXExplainerType
</a>
</em>
</td>
<td>
<p>The type of AIX explainer</p>
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
<p>Defaults to latest AIX Version</p>
</td>
</tr>
<tr>
<td>
<code>Container</code></br>
<em>
<a href="https://v1-18.docs.kubernetes.io/docs/reference/generated/kubernetes-api/v1.18/#container-v1-core">
Kubernetes core/v1.Container
</a>
</em>
</td>
<td>
<p>
(Members of <code>Container</code> are embedded into this type.)
</p>
<em>(Optional)</em>
<p>Container enables overrides for the predictor.
Each framework will have different defaults that are populated in the underlying container spec.</p>
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
<h3 id="serving.kubeflow.org/v1beta1.AIXExplainerType">AIXExplainerType
(<code>string</code> alias)</p></h3>
<p>
(<em>Appears on:</em>
<a href="#serving.kubeflow.org/v1beta1.AIXExplainerSpec">AIXExplainerSpec</a>)
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
<tbody><tr><td><p>&#34;LimeImages&#34;</p></td>
<td></td>
</tr></tbody>
</table>
<h3 id="serving.kubeflow.org/v1beta1.AlibiExplainerSpec">AlibiExplainerSpec
</h3>
<p>
(<em>Appears on:</em>
<a href="#serving.kubeflow.org/v1beta1.ExplainerSpec">ExplainerSpec</a>)
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
<a href="#serving.kubeflow.org/v1beta1.AlibiExplainerType">
AlibiExplainerType
</a>
</em>
</td>
<td>
<p>The type of Alibi explainer <br />
Valid values are: <br />
- &ldquo;AnchorTabular&rdquo;; <br />
- &ldquo;AnchorImages&rdquo;; <br />
- &ldquo;AnchorText&rdquo;; <br />
- &ldquo;Counterfactuals&rdquo;; <br />
- &ldquo;Contrastive&rdquo;; <br /></p>
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
<em>(Optional)</em>
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
<em>(Optional)</em>
<p>Alibi docker image version, defaults to latest Alibi Version</p>
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
<em>(Optional)</em>
<p>Inline custom parameter settings for explainer</p>
</td>
</tr>
<tr>
<td>
<code>Container</code></br>
<em>
<a href="https://v1-18.docs.kubernetes.io/docs/reference/generated/kubernetes-api/v1.18/#container-v1-core">
Kubernetes core/v1.Container
</a>
</em>
</td>
<td>
<p>
(Members of <code>Container</code> are embedded into this type.)
</p>
<em>(Optional)</em>
<p>Container enables overrides for the predictor.
Each framework will have different defaults that are populated in the underlying container spec.</p>
</td>
</tr>
</tbody>
</table>
<h3 id="serving.kubeflow.org/v1beta1.AlibiExplainerType">AlibiExplainerType
(<code>string</code> alias)</p></h3>
<p>
(<em>Appears on:</em>
<a href="#serving.kubeflow.org/v1beta1.AlibiExplainerSpec">AlibiExplainerSpec</a>)
</p>
<p>
<p>AlibiExplainerType is the explanation method</p>
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
<h3 id="serving.kubeflow.org/v1beta1.Batcher">Batcher
</h3>
<p>
(<em>Appears on:</em>
<a href="#serving.kubeflow.org/v1beta1.ComponentExtensionSpec">ComponentExtensionSpec</a>)
</p>
<p>
<p>Batcher specifies optional payload batching available for all components</p>
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
<p>Specifies the max number of requests to trigger a batch</p>
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
<p>Specifies the max latency to trigger a batch</p>
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
<p>Specifies the timeout of a batch</p>
</td>
</tr>
</tbody>
</table>
<h3 id="serving.kubeflow.org/v1beta1.Component">Component
</h3>
<p>
<p>Component interface is implemented by all specs that contain component implementations, e.g. PredictorSpec, ExplainerSpec, TransformerSpec.</p>
</p>
<h3 id="serving.kubeflow.org/v1beta1.ComponentExtensionSpec">ComponentExtensionSpec
</h3>
<p>
(<em>Appears on:</em>
<a href="#serving.kubeflow.org/v1beta1.ExplainerSpec">ExplainerSpec</a>, 
<a href="#serving.kubeflow.org/v1beta1.PredictorSpec">PredictorSpec</a>, 
<a href="#serving.kubeflow.org/v1beta1.TransformerSpec">TransformerSpec</a>)
</p>
<p>
<p>ComponentExtensionSpec defines the deployment configuration for a given InferenceService component</p>
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
<code>minReplicas</code></br>
<em>
int
</em>
</td>
<td>
<em>(Optional)</em>
<p>Minimum number of replicas, defaults to 1 but can be set to 0 to enable scale-to-zero.</p>
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
<p>Maximum number of replicas for autoscaling.</p>
</td>
</tr>
<tr>
<td>
<code>containerConcurrency</code></br>
<em>
int64
</em>
</td>
<td>
<em>(Optional)</em>
<p>ContainerConcurrency specifies how many requests can be processed concurrently, this sets the hard limit of the container
concurrency(<a href="https://knative.dev/docs/serving/autoscaling/concurrency">https://knative.dev/docs/serving/autoscaling/concurrency</a>).</p>
</td>
</tr>
<tr>
<td>
<code>timeout</code></br>
<em>
int64
</em>
</td>
<td>
<em>(Optional)</em>
<p>TimeoutSeconds specifies the number of seconds to wait before timing out a request to the component.</p>
</td>
</tr>
<tr>
<td>
<code>canaryTrafficPercent</code></br>
<em>
int64
</em>
</td>
<td>
<em>(Optional)</em>
<p>CanaryTrafficPercent defines the traffic split percentage between the candidate revision and the last ready revision</p>
</td>
</tr>
<tr>
<td>
<code>logger</code></br>
<em>
<a href="#serving.kubeflow.org/v1beta1.LoggerSpec">
LoggerSpec
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Activate request/response logging and logger configurations</p>
</td>
</tr>
<tr>
<td>
<code>batcher</code></br>
<em>
<a href="#serving.kubeflow.org/v1beta1.Batcher">
Batcher
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Activate request batching and batching configurations</p>
</td>
</tr>
</tbody>
</table>
<h3 id="serving.kubeflow.org/v1beta1.ComponentImplementation">ComponentImplementation
</h3>
<p>
<p>ComponentImplementation interface is implemented by predictor, transformer, and explainer implementations</p>
</p>
<h3 id="serving.kubeflow.org/v1beta1.ComponentStatusSpec">ComponentStatusSpec
</h3>
<p>
(<em>Appears on:</em>
<a href="#serving.kubeflow.org/v1beta1.InferenceServiceStatus">InferenceServiceStatus</a>)
</p>
<p>
<p>ComponentStatusSpec describes the state of the component</p>
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
<code>latestReadyRevision</code></br>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>Latest revision name that is in ready state</p>
</td>
</tr>
<tr>
<td>
<code>previousReadyRevision</code></br>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>Previous revision name that is in ready state</p>
</td>
</tr>
<tr>
<td>
<code>latestCreatedRevision</code></br>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>Latest revision name that is in created</p>
</td>
</tr>
<tr>
<td>
<code>trafficPercent</code></br>
<em>
int64
</em>
</td>
<td>
<em>(Optional)</em>
<p>Traffic percent on the latest ready revision</p>
</td>
</tr>
<tr>
<td>
<code>url</code></br>
<em>
knative.dev/pkg/apis.URL
</em>
</td>
<td>
<em>(Optional)</em>
<p>URL holds the url that will distribute traffic over the provided traffic targets.
It generally has the form http[s]://{route-name}.{route-namespace}.{cluster-level-suffix}</p>
</td>
</tr>
<tr>
<td>
<code>address</code></br>
<em>
knative.dev/pkg/apis/duck/v1.Addressable
</em>
</td>
<td>
<em>(Optional)</em>
<p>Addressable endpoint for the InferenceService</p>
</td>
</tr>
</tbody>
</table>
<h3 id="serving.kubeflow.org/v1beta1.ComponentType">ComponentType
(<code>string</code> alias)</p></h3>
<p>
<p>ComponentType contains the different types of components of the service</p>
</p>
<table>
<thead>
<tr>
<th>Value</th>
<th>Description</th>
</tr>
</thead>
<tbody><tr><td><p>&#34;explainer&#34;</p></td>
<td></td>
</tr><tr><td><p>&#34;predictor&#34;</p></td>
<td></td>
</tr><tr><td><p>&#34;transformer&#34;</p></td>
<td></td>
</tr></tbody>
</table>
<h3 id="serving.kubeflow.org/v1beta1.CustomExplainer">CustomExplainer
</h3>
<p>
<p>CustomExplainer defines arguments for configuring a custom explainer.</p>
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
<code>PodSpec</code></br>
<em>
<a href="https://v1-18.docs.kubernetes.io/docs/reference/generated/kubernetes-api/v1.18/#podspec-v1-core">
Kubernetes core/v1.PodSpec
</a>
</em>
</td>
<td>
<p>
(Members of <code>PodSpec</code> are embedded into this type.)
</p>
</td>
</tr>
</tbody>
</table>
<h3 id="serving.kubeflow.org/v1beta1.CustomPredictor">CustomPredictor
</h3>
<p>
<p>CustomPredictor defines arguments for configuring a custom server.</p>
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
<code>PodSpec</code></br>
<em>
<a href="https://v1-18.docs.kubernetes.io/docs/reference/generated/kubernetes-api/v1.18/#podspec-v1-core">
Kubernetes core/v1.PodSpec
</a>
</em>
</td>
<td>
<p>
(Members of <code>PodSpec</code> are embedded into this type.)
</p>
</td>
</tr>
</tbody>
</table>
<h3 id="serving.kubeflow.org/v1beta1.CustomTransformer">CustomTransformer
</h3>
<p>
<p>CustomTransformer defines arguments for configuring a custom transformer.</p>
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
<code>PodSpec</code></br>
<em>
<a href="https://v1-18.docs.kubernetes.io/docs/reference/generated/kubernetes-api/v1.18/#podspec-v1-core">
Kubernetes core/v1.PodSpec
</a>
</em>
</td>
<td>
<p>
(Members of <code>PodSpec</code> are embedded into this type.)
</p>
</td>
</tr>
</tbody>
</table>
<h3 id="serving.kubeflow.org/v1beta1.ExplainerConfig">ExplainerConfig
</h3>
<p>
(<em>Appears on:</em>
<a href="#serving.kubeflow.org/v1beta1.ExplainersConfig">ExplainersConfig</a>)
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
<p>explainer docker image name</p>
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
<p>default explainer docker image version</p>
</td>
</tr>
</tbody>
</table>
<h3 id="serving.kubeflow.org/v1beta1.ExplainerSpec">ExplainerSpec
</h3>
<p>
(<em>Appears on:</em>
<a href="#serving.kubeflow.org/v1beta1.InferenceServiceSpec">InferenceServiceSpec</a>)
</p>
<p>
<p>ExplainerSpec defines the container spec for a model explanation server,
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
<a href="#serving.kubeflow.org/v1beta1.AlibiExplainerSpec">
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
<code>aix</code></br>
<em>
<a href="#serving.kubeflow.org/v1beta1.AIXExplainerSpec">
AIXExplainerSpec
</a>
</em>
</td>
<td>
<p>Spec for AIX explainer</p>
</td>
</tr>
<tr>
<td>
<code>PodSpec</code></br>
<em>
<a href="#serving.kubeflow.org/v1beta1.PodSpec">
PodSpec
</a>
</em>
</td>
<td>
<p>
(Members of <code>PodSpec</code> are embedded into this type.)
</p>
<p>This spec is dual purpose <br />
1) Provide a full PodSpec for custom explainer.
The field PodSpec.Containers is mutually exclusive with other explainers (i.e. Alibi). <br />
2) Provide a explainer (i.e. Alibi) and specify PodSpec
overrides, you must not provide PodSpec.Containers in this case. <br /></p>
</td>
</tr>
<tr>
<td>
<code>ComponentExtensionSpec</code></br>
<em>
<a href="#serving.kubeflow.org/v1beta1.ComponentExtensionSpec">
ComponentExtensionSpec
</a>
</em>
</td>
<td>
<p>
(Members of <code>ComponentExtensionSpec</code> are embedded into this type.)
</p>
<p>Component extension defines the deployment configurations for explainer</p>
</td>
</tr>
</tbody>
</table>
<h3 id="serving.kubeflow.org/v1beta1.ExplainersConfig">ExplainersConfig
</h3>
<p>
(<em>Appears on:</em>
<a href="#serving.kubeflow.org/v1beta1.InferenceServicesConfig">InferenceServicesConfig</a>)
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
<a href="#serving.kubeflow.org/v1beta1.ExplainerConfig">
ExplainerConfig
</a>
</em>
</td>
<td>
</td>
</tr>
<tr>
<td>
<code>aix</code></br>
<em>
<a href="#serving.kubeflow.org/v1beta1.ExplainerConfig">
ExplainerConfig
</a>
</em>
</td>
<td>
</td>
</tr>
</tbody>
</table>
<h3 id="serving.kubeflow.org/v1beta1.InferenceService">InferenceService
</h3>
<p>
<p>InferenceService is the Schema for the InferenceServices API</p>
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
<a href="#serving.kubeflow.org/v1beta1.InferenceServiceSpec">
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
<code>predictor</code></br>
<em>
<a href="#serving.kubeflow.org/v1beta1.PredictorSpec">
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
<a href="#serving.kubeflow.org/v1beta1.ExplainerSpec">
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
<a href="#serving.kubeflow.org/v1beta1.TransformerSpec">
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
</table>
</td>
</tr>
<tr>
<td>
<code>status</code></br>
<em>
<a href="#serving.kubeflow.org/v1beta1.InferenceServiceStatus">
InferenceServiceStatus
</a>
</em>
</td>
<td>
</td>
</tr>
</tbody>
</table>
<h3 id="serving.kubeflow.org/v1beta1.InferenceServiceSpec">InferenceServiceSpec
</h3>
<p>
(<em>Appears on:</em>
<a href="#serving.kubeflow.org/v1beta1.InferenceService">InferenceService</a>)
</p>
<p>
<p>InferenceServiceSpec is the top level type for this resource</p>
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
<a href="#serving.kubeflow.org/v1beta1.PredictorSpec">
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
<a href="#serving.kubeflow.org/v1beta1.ExplainerSpec">
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
<a href="#serving.kubeflow.org/v1beta1.TransformerSpec">
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
<h3 id="serving.kubeflow.org/v1beta1.InferenceServiceStatus">InferenceServiceStatus
</h3>
<p>
(<em>Appears on:</em>
<a href="#serving.kubeflow.org/v1beta1.InferenceService">InferenceService</a>)
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
knative.dev/pkg/apis/duck/v1.Status
</em>
</td>
<td>
<p>
(Members of <code>Status</code> are embedded into this type.)
</p>
<p>Conditions for the InferenceService <br/>
- PredictorReady: predictor readiness condition; <br/>
- TransformerReady: transformer readiness condition; <br/>
- ExplainerReady: explainer readiness condition; <br/>
- RoutesReady: aggregated routing condition; <br/>
- Ready: aggregated condition; <br/></p>
</td>
</tr>
<tr>
<td>
<code>address</code></br>
<em>
knative.dev/pkg/apis/duck/v1.Addressable
</em>
</td>
<td>
<em>(Optional)</em>
<p>Addressable endpoint for the InferenceService</p>
</td>
</tr>
<tr>
<td>
<code>url</code></br>
<em>
knative.dev/pkg/apis.URL
</em>
</td>
<td>
<em>(Optional)</em>
<p>URL holds the url that will distribute traffic over the provided traffic targets.
It generally has the form http[s]://{route-name}.{route-namespace}.{cluster-level-suffix}</p>
</td>
</tr>
<tr>
<td>
<code>components</code></br>
<em>
<a href="#serving.kubeflow.org/v1beta1.ComponentStatusSpec">
map[github.com/kubeflow/kfserving/pkg/apis/serving/v1beta1.ComponentType]github.com/kubeflow/kfserving/pkg/apis/serving/v1beta1.ComponentStatusSpec
</a>
</em>
</td>
<td>
<p>Statuses for the components of the InferenceService</p>
</td>
</tr>
</tbody>
</table>
<h3 id="serving.kubeflow.org/v1beta1.InferenceServicesConfig">InferenceServicesConfig
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
<a href="#serving.kubeflow.org/v1beta1.TransformersConfig">
TransformersConfig
</a>
</em>
</td>
<td>
<p>Transformer configurations</p>
</td>
</tr>
<tr>
<td>
<code>predictors</code></br>
<em>
<a href="#serving.kubeflow.org/v1beta1.PredictorsConfig">
PredictorsConfig
</a>
</em>
</td>
<td>
<p>Predictor configurations</p>
</td>
</tr>
<tr>
<td>
<code>explainers</code></br>
<em>
<a href="#serving.kubeflow.org/v1beta1.ExplainersConfig">
ExplainersConfig
</a>
</em>
</td>
<td>
<p>Explainer configurations</p>
</td>
</tr>
</tbody>
</table>
<h3 id="serving.kubeflow.org/v1beta1.IngressConfig">IngressConfig
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
<code>ingressGateway</code></br>
<em>
string
</em>
</td>
<td>
</td>
</tr>
<tr>
<td>
<code>ingressService</code></br>
<em>
string
</em>
</td>
<td>
</td>
</tr>
</tbody>
</table>
<h3 id="serving.kubeflow.org/v1beta1.LightGBMSpec">LightGBMSpec
</h3>
<p>
(<em>Appears on:</em>
<a href="#serving.kubeflow.org/v1beta1.PredictorSpec">PredictorSpec</a>)
</p>
<p>
<p>LightGBMSpec defines arguments for configuring LightGBMSpec model serving.</p>
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
<code>PredictorExtensionSpec</code></br>
<em>
<a href="#serving.kubeflow.org/v1beta1.PredictorExtensionSpec">
PredictorExtensionSpec
</a>
</em>
</td>
<td>
<p>
(Members of <code>PredictorExtensionSpec</code> are embedded into this type.)
</p>
<p>Contains fields shared across all predictors</p>
</td>
</tr>
</tbody>
</table>
<h3 id="serving.kubeflow.org/v1beta1.LoggerSpec">LoggerSpec
</h3>
<p>
(<em>Appears on:</em>
<a href="#serving.kubeflow.org/v1beta1.ComponentExtensionSpec">ComponentExtensionSpec</a>)
</p>
<p>
<p>LoggerSpec specifies optional payload logging available for all components</p>
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
<p>URL to send logging events</p>
</td>
</tr>
<tr>
<td>
<code>mode</code></br>
<em>
<a href="#serving.kubeflow.org/v1beta1.LoggerType">
LoggerType
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Specifies the scope of the loggers. <br />
Valid values are: <br />
- &ldquo;all&rdquo; (default): log both request and response; <br />
- &ldquo;request&rdquo;: log only request; <br />
- &ldquo;response&rdquo;: log only response <br /></p>
</td>
</tr>
</tbody>
</table>
<h3 id="serving.kubeflow.org/v1beta1.LoggerType">LoggerType
(<code>string</code> alias)</p></h3>
<p>
(<em>Appears on:</em>
<a href="#serving.kubeflow.org/v1beta1.LoggerSpec">LoggerSpec</a>)
</p>
<p>
<p>LoggerType controls the scope of log publishing</p>
</p>
<table>
<thead>
<tr>
<th>Value</th>
<th>Description</th>
</tr>
</thead>
<tbody><tr><td><p>&#34;all&#34;</p></td>
<td><p>Logger mode to log both request and response</p>
</td>
</tr><tr><td><p>&#34;request&#34;</p></td>
<td><p>Logger mode to log only request</p>
</td>
</tr><tr><td><p>&#34;response&#34;</p></td>
<td><p>Logger mode to log only response</p>
</td>
</tr></tbody>
</table>
<h3 id="serving.kubeflow.org/v1beta1.ONNXRuntimeSpec">ONNXRuntimeSpec
</h3>
<p>
(<em>Appears on:</em>
<a href="#serving.kubeflow.org/v1beta1.PredictorSpec">PredictorSpec</a>)
</p>
<p>
<p>ONNXRuntimeSpec defines arguments for configuring ONNX model serving.</p>
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
<code>PredictorExtensionSpec</code></br>
<em>
<a href="#serving.kubeflow.org/v1beta1.PredictorExtensionSpec">
PredictorExtensionSpec
</a>
</em>
</td>
<td>
<p>
(Members of <code>PredictorExtensionSpec</code> are embedded into this type.)
</p>
<p>Contains fields shared across all predictors</p>
</td>
</tr>
</tbody>
</table>
<h3 id="serving.kubeflow.org/v1beta1.PMMLSpec">PMMLSpec
</h3>
<p>
(<em>Appears on:</em>
<a href="#serving.kubeflow.org/v1beta1.PredictorSpec">PredictorSpec</a>)
</p>
<p>
<p>PMMLSpec defines arguments for configuring PMML model serving.</p>
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
<code>PredictorExtensionSpec</code></br>
<em>
<a href="#serving.kubeflow.org/v1beta1.PredictorExtensionSpec">
PredictorExtensionSpec
</a>
</em>
</td>
<td>
<p>
(Members of <code>PredictorExtensionSpec</code> are embedded into this type.)
</p>
<p>Contains fields shared across all predictors</p>
</td>
</tr>
</tbody>
</table>
<h3 id="serving.kubeflow.org/v1beta1.PodSpec">PodSpec
</h3>
<p>
(<em>Appears on:</em>
<a href="#serving.kubeflow.org/v1beta1.ExplainerSpec">ExplainerSpec</a>, 
<a href="#serving.kubeflow.org/v1beta1.PredictorSpec">PredictorSpec</a>, 
<a href="#serving.kubeflow.org/v1beta1.TransformerSpec">TransformerSpec</a>)
</p>
<p>
<p>PodSpec is a description of a pod.</p>
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
<code>volumes</code></br>
<em>
<a href="https://v1-18.docs.kubernetes.io/docs/reference/generated/kubernetes-api/v1.18/#volume-v1-core">
[]Kubernetes core/v1.Volume
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>List of volumes that can be mounted by containers belonging to the pod.
More info: <a href="https://kubernetes.io/docs/concepts/storage/volumes">https://kubernetes.io/docs/concepts/storage/volumes</a></p>
</td>
</tr>
<tr>
<td>
<code>initContainers</code></br>
<em>
<a href="https://v1-18.docs.kubernetes.io/docs/reference/generated/kubernetes-api/v1.18/#container-v1-core">
[]Kubernetes core/v1.Container
</a>
</em>
</td>
<td>
<p>List of initialization containers belonging to the pod.
Init containers are executed in order prior to containers being started. If any
init container fails, the pod is considered to have failed and is handled according
to its restartPolicy. The name for an init container or normal container must be
unique among all containers.
Init containers may not have Lifecycle actions, Readiness probes, Liveness probes, or Startup probes.
The resourceRequirements of an init container are taken into account during scheduling
by finding the highest request/limit for each resource type, and then using the max of
of that value or the sum of the normal containers. Limits are applied to init containers
in a similar fashion.
Init containers cannot currently be added or removed.
Cannot be updated.
More info: <a href="https://kubernetes.io/docs/concepts/workloads/pods/init-containers/">https://kubernetes.io/docs/concepts/workloads/pods/init-containers/</a></p>
</td>
</tr>
<tr>
<td>
<code>containers</code></br>
<em>
<a href="https://v1-18.docs.kubernetes.io/docs/reference/generated/kubernetes-api/v1.18/#container-v1-core">
[]Kubernetes core/v1.Container
</a>
</em>
</td>
<td>
<p>List of containers belonging to the pod.
Containers cannot currently be added or removed.
There must be at least one container in a Pod.
Cannot be updated.</p>
</td>
</tr>
<tr>
<td>
<code>ephemeralContainers</code></br>
<em>
<a href="https://v1-18.docs.kubernetes.io/docs/reference/generated/kubernetes-api/v1.18/#ephemeralcontainer-v1-core">
[]Kubernetes core/v1.EphemeralContainer
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>List of ephemeral containers run in this pod. Ephemeral containers may be run in an existing
pod to perform user-initiated actions such as debugging. This list cannot be specified when
creating a pod, and it cannot be modified by updating the pod spec. In order to add an
ephemeral container to an existing pod, use the pod&rsquo;s ephemeralcontainers subresource.
This field is alpha-level and is only honored by servers that enable the EphemeralContainers feature.</p>
</td>
</tr>
<tr>
<td>
<code>restartPolicy</code></br>
<em>
<a href="https://v1-18.docs.kubernetes.io/docs/reference/generated/kubernetes-api/v1.18/#restartpolicy-v1-core">
Kubernetes core/v1.RestartPolicy
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Restart policy for all containers within the pod.
One of Always, OnFailure, Never.
Default to Always.
More info: <a href="https://kubernetes.io/docs/concepts/workloads/pods/pod-lifecycle/#restart-policy">https://kubernetes.io/docs/concepts/workloads/pods/pod-lifecycle/#restart-policy</a></p>
</td>
</tr>
<tr>
<td>
<code>terminationGracePeriodSeconds</code></br>
<em>
int64
</em>
</td>
<td>
<em>(Optional)</em>
<p>Optional duration in seconds the pod needs to terminate gracefully. May be decreased in delete request.
Value must be non-negative integer. The value zero indicates delete immediately.
If this value is nil, the default grace period will be used instead.
The grace period is the duration in seconds after the processes running in the pod are sent
a termination signal and the time when the processes are forcibly halted with a kill signal.
Set this value longer than the expected cleanup time for your process.
Defaults to 30 seconds.</p>
</td>
</tr>
<tr>
<td>
<code>activeDeadlineSeconds</code></br>
<em>
int64
</em>
</td>
<td>
<em>(Optional)</em>
<p>Optional duration in seconds the pod may be active on the node relative to
StartTime before the system will actively try to mark it failed and kill associated containers.
Value must be a positive integer.</p>
</td>
</tr>
<tr>
<td>
<code>dnsPolicy</code></br>
<em>
<a href="https://v1-18.docs.kubernetes.io/docs/reference/generated/kubernetes-api/v1.18/#dnspolicy-v1-core">
Kubernetes core/v1.DNSPolicy
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Set DNS policy for the pod.
Defaults to &ldquo;ClusterFirst&rdquo;.
Valid values are &lsquo;ClusterFirstWithHostNet&rsquo;, &lsquo;ClusterFirst&rsquo;, &lsquo;Default&rsquo; or &lsquo;None&rsquo;.
DNS parameters given in DNSConfig will be merged with the policy selected with DNSPolicy.
To have DNS options set along with hostNetwork, you have to specify DNS policy
explicitly to &lsquo;ClusterFirstWithHostNet&rsquo;.</p>
</td>
</tr>
<tr>
<td>
<code>nodeSelector</code></br>
<em>
map[string]string
</em>
</td>
<td>
<em>(Optional)</em>
<p>NodeSelector is a selector which must be true for the pod to fit on a node.
Selector which must match a node&rsquo;s labels for the pod to be scheduled on that node.
More info: <a href="https://kubernetes.io/docs/concepts/configuration/assign-pod-node/">https://kubernetes.io/docs/concepts/configuration/assign-pod-node/</a></p>
</td>
</tr>
<tr>
<td>
<code>serviceAccountName</code></br>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>ServiceAccountName is the name of the ServiceAccount to use to run this pod.
More info: <a href="https://kubernetes.io/docs/tasks/configure-pod-container/configure-service-account/">https://kubernetes.io/docs/tasks/configure-pod-container/configure-service-account/</a></p>
</td>
</tr>
<tr>
<td>
<code>serviceAccount</code></br>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>DeprecatedServiceAccount is a depreciated alias for ServiceAccountName.
Deprecated: Use serviceAccountName instead.</p>
</td>
</tr>
<tr>
<td>
<code>automountServiceAccountToken</code></br>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
<p>AutomountServiceAccountToken indicates whether a service account token should be automatically mounted.</p>
</td>
</tr>
<tr>
<td>
<code>nodeName</code></br>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>NodeName is a request to schedule this pod onto a specific node. If it is non-empty,
the scheduler simply schedules this pod onto that node, assuming that it fits resource
requirements.</p>
</td>
</tr>
<tr>
<td>
<code>hostNetwork</code></br>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
<p>Host networking requested for this pod. Use the host&rsquo;s network namespace.
If this option is set, the ports that will be used must be specified.
Default to false.</p>
</td>
</tr>
<tr>
<td>
<code>hostPID</code></br>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
<p>Use the host&rsquo;s pid namespace.
Optional: Default to false.</p>
</td>
</tr>
<tr>
<td>
<code>hostIPC</code></br>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
<p>Use the host&rsquo;s ipc namespace.
Optional: Default to false.</p>
</td>
</tr>
<tr>
<td>
<code>shareProcessNamespace</code></br>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
<p>Share a single process namespace between all of the containers in a pod.
When this is set containers will be able to view and signal processes from other containers
in the same pod, and the first process in each container will not be assigned PID 1.
HostPID and ShareProcessNamespace cannot both be set.
Optional: Default to false.</p>
</td>
</tr>
<tr>
<td>
<code>securityContext</code></br>
<em>
<a href="https://v1-18.docs.kubernetes.io/docs/reference/generated/kubernetes-api/v1.18/#podsecuritycontext-v1-core">
Kubernetes core/v1.PodSecurityContext
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>SecurityContext holds pod-level security attributes and common container settings.
Optional: Defaults to empty.  See type description for default values of each field.</p>
</td>
</tr>
<tr>
<td>
<code>imagePullSecrets</code></br>
<em>
<a href="https://v1-18.docs.kubernetes.io/docs/reference/generated/kubernetes-api/v1.18/#localobjectreference-v1-core">
[]Kubernetes core/v1.LocalObjectReference
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>ImagePullSecrets is an optional list of references to secrets in the same namespace to use for pulling any of the images used by this PodSpec.
If specified, these secrets will be passed to individual puller implementations for them to use. For example,
in the case of docker, only DockerConfig type secrets are honored.
More info: <a href="https://kubernetes.io/docs/concepts/containers/images#specifying-imagepullsecrets-on-a-pod">https://kubernetes.io/docs/concepts/containers/images#specifying-imagepullsecrets-on-a-pod</a></p>
</td>
</tr>
<tr>
<td>
<code>hostname</code></br>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>Specifies the hostname of the Pod
If not specified, the pod&rsquo;s hostname will be set to a system-defined value.</p>
</td>
</tr>
<tr>
<td>
<code>subdomain</code></br>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>If specified, the fully qualified Pod hostname will be &ldquo;<hostname>.<subdomain>.<pod namespace>.svc.<cluster domain>&rdquo;.
If not specified, the pod will not have a domainname at all.</p>
</td>
</tr>
<tr>
<td>
<code>affinity</code></br>
<em>
<a href="https://v1-18.docs.kubernetes.io/docs/reference/generated/kubernetes-api/v1.18/#affinity-v1-core">
Kubernetes core/v1.Affinity
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>If specified, the pod&rsquo;s scheduling constraints</p>
</td>
</tr>
<tr>
<td>
<code>schedulerName</code></br>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>If specified, the pod will be dispatched by specified scheduler.
If not specified, the pod will be dispatched by default scheduler.</p>
</td>
</tr>
<tr>
<td>
<code>tolerations</code></br>
<em>
<a href="https://v1-18.docs.kubernetes.io/docs/reference/generated/kubernetes-api/v1.18/#toleration-v1-core">
[]Kubernetes core/v1.Toleration
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>If specified, the pod&rsquo;s tolerations.</p>
</td>
</tr>
<tr>
<td>
<code>hostAliases</code></br>
<em>
<a href="https://v1-18.docs.kubernetes.io/docs/reference/generated/kubernetes-api/v1.18/#hostalias-v1-core">
[]Kubernetes core/v1.HostAlias
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>HostAliases is an optional list of hosts and IPs that will be injected into the pod&rsquo;s hosts
file if specified. This is only valid for non-hostNetwork pods.</p>
</td>
</tr>
<tr>
<td>
<code>priorityClassName</code></br>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>If specified, indicates the pod&rsquo;s priority. &ldquo;system-node-critical&rdquo; and
&ldquo;system-cluster-critical&rdquo; are two special keywords which indicate the
highest priorities with the former being the highest priority. Any other
name must be defined by creating a PriorityClass object with that name.
If not specified, the pod priority will be default or zero if there is no
default.</p>
</td>
</tr>
<tr>
<td>
<code>priority</code></br>
<em>
int32
</em>
</td>
<td>
<em>(Optional)</em>
<p>The priority value. Various system components use this field to find the
priority of the pod. When Priority Admission Controller is enabled, it
prevents users from setting this field. The admission controller populates
this field from PriorityClassName.
The higher the value, the higher the priority.</p>
</td>
</tr>
<tr>
<td>
<code>dnsConfig</code></br>
<em>
<a href="https://v1-18.docs.kubernetes.io/docs/reference/generated/kubernetes-api/v1.18/#poddnsconfig-v1-core">
Kubernetes core/v1.PodDNSConfig
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Specifies the DNS parameters of a pod.
Parameters specified here will be merged to the generated DNS
configuration based on DNSPolicy.</p>
</td>
</tr>
<tr>
<td>
<code>readinessGates</code></br>
<em>
<a href="https://v1-18.docs.kubernetes.io/docs/reference/generated/kubernetes-api/v1.18/#podreadinessgate-v1-core">
[]Kubernetes core/v1.PodReadinessGate
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>If specified, all readiness gates will be evaluated for pod readiness.
A pod is ready when all its containers are ready AND
all conditions specified in the readiness gates have status equal to &ldquo;True&rdquo;
More info: <a href="https://git.k8s.io/enhancements/keps/sig-network/0007-pod-ready%2B%2B.md">https://git.k8s.io/enhancements/keps/sig-network/0007-pod-ready%2B%2B.md</a></p>
</td>
</tr>
<tr>
<td>
<code>runtimeClassName</code></br>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>RuntimeClassName refers to a RuntimeClass object in the node.k8s.io group, which should be used
to run this pod.  If no RuntimeClass resource matches the named class, the pod will not be run.
If unset or empty, the &ldquo;legacy&rdquo; RuntimeClass will be used, which is an implicit class with an
empty definition that uses the default runtime handler.
More info: <a href="https://git.k8s.io/enhancements/keps/sig-node/runtime-class.md">https://git.k8s.io/enhancements/keps/sig-node/runtime-class.md</a>
This is a beta feature as of Kubernetes v1.14.</p>
</td>
</tr>
<tr>
<td>
<code>enableServiceLinks</code></br>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
<p>EnableServiceLinks indicates whether information about services should be injected into pod&rsquo;s
environment variables, matching the syntax of Docker links.
Optional: Defaults to true.</p>
</td>
</tr>
<tr>
<td>
<code>preemptionPolicy</code></br>
<em>
<a href="https://v1-18.docs.kubernetes.io/docs/reference/generated/kubernetes-api/v1.18/#preemptionpolicy-v1-core">
Kubernetes core/v1.PreemptionPolicy
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>PreemptionPolicy is the Policy for preempting pods with lower priority.
One of Never, PreemptLowerPriority.
Defaults to PreemptLowerPriority if unset.
This field is alpha-level and is only honored by servers that enable the NonPreemptingPriority feature.</p>
</td>
</tr>
<tr>
<td>
<code>overhead</code></br>
<em>
<a href="https://v1-18.docs.kubernetes.io/docs/reference/generated/kubernetes-api/v1.18/#resourcelist-v1-core">
Kubernetes core/v1.ResourceList
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Overhead represents the resource overhead associated with running a pod for a given RuntimeClass.
This field will be autopopulated at admission time by the RuntimeClass admission controller. If
the RuntimeClass admission controller is enabled, overhead must not be set in Pod create requests.
The RuntimeClass admission controller will reject Pod create requests which have the overhead already
set. If RuntimeClass is configured and selected in the PodSpec, Overhead will be set to the value
defined in the corresponding RuntimeClass, otherwise it will remain unset and treated as zero.
More info: <a href="https://git.k8s.io/enhancements/keps/sig-node/20190226-pod-overhead.md">https://git.k8s.io/enhancements/keps/sig-node/20190226-pod-overhead.md</a>
This field is alpha-level as of Kubernetes v1.16, and is only honored by servers that enable the PodOverhead feature.</p>
</td>
</tr>
<tr>
<td>
<code>topologySpreadConstraints</code></br>
<em>
<a href="https://v1-18.docs.kubernetes.io/docs/reference/generated/kubernetes-api/v1.18/#topologyspreadconstraint-v1-core">
[]Kubernetes core/v1.TopologySpreadConstraint
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>TopologySpreadConstraints describes how a group of pods ought to spread across topology
domains. Scheduler will schedule pods in a way which abides by the constraints.
This field is alpha-level and is only honored by clusters that enables the EvenPodsSpread
feature.
All topologySpreadConstraints are ANDed.</p>
</td>
</tr>
</tbody>
</table>
<h3 id="serving.kubeflow.org/v1beta1.PredictorConfig">PredictorConfig
</h3>
<p>
(<em>Appears on:</em>
<a href="#serving.kubeflow.org/v1beta1.PredictorProtocols">PredictorProtocols</a>, 
<a href="#serving.kubeflow.org/v1beta1.PredictorsConfig">PredictorsConfig</a>)
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
<p>predictor docker image name</p>
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
<p>default predictor docker image version on cpu</p>
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
<p>default predictor docker image version on gpu</p>
</td>
</tr>
<tr>
<td>
<code>defaultTimeout,string</code></br>
<em>
int64
</em>
</td>
<td>
<p>Default timeout of predictor for serving a request, in seconds</p>
</td>
</tr>
</tbody>
</table>
<h3 id="serving.kubeflow.org/v1beta1.PredictorExtensionSpec">PredictorExtensionSpec
</h3>
<p>
(<em>Appears on:</em>
<a href="#serving.kubeflow.org/v1beta1.LightGBMSpec">LightGBMSpec</a>, 
<a href="#serving.kubeflow.org/v1beta1.ONNXRuntimeSpec">ONNXRuntimeSpec</a>, 
<a href="#serving.kubeflow.org/v1beta1.PMMLSpec">PMMLSpec</a>, 
<a href="#serving.kubeflow.org/v1beta1.SKLearnSpec">SKLearnSpec</a>, 
<a href="#serving.kubeflow.org/v1beta1.TFServingSpec">TFServingSpec</a>, 
<a href="#serving.kubeflow.org/v1beta1.TorchServeSpec">TorchServeSpec</a>, 
<a href="#serving.kubeflow.org/v1beta1.TritonSpec">TritonSpec</a>, 
<a href="#serving.kubeflow.org/v1beta1.XGBoostSpec">XGBoostSpec</a>)
</p>
<p>
<p>PredictorExtensionSpec defines configuration shared across all predictor frameworks</p>
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
<em>(Optional)</em>
<p>This field points to the location of the trained model which is mounted onto the pod.</p>
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
<em>(Optional)</em>
<p>Runtime version of the predictor docker image</p>
</td>
</tr>
<tr>
<td>
<code>protocolVersion</code></br>
<em>
invalid type
</em>
</td>
<td>
<em>(Optional)</em>
<p>Protocol version to use by the predictor (i.e. v1 or v2)</p>
</td>
</tr>
<tr>
<td>
<code>Container</code></br>
<em>
<a href="https://v1-18.docs.kubernetes.io/docs/reference/generated/kubernetes-api/v1.18/#container-v1-core">
Kubernetes core/v1.Container
</a>
</em>
</td>
<td>
<p>
(Members of <code>Container</code> are embedded into this type.)
</p>
<em>(Optional)</em>
<p>Container enables overrides for the predictor.
Each framework will have different defaults that are populated in the underlying container spec.</p>
</td>
</tr>
</tbody>
</table>
<h3 id="serving.kubeflow.org/v1beta1.PredictorProtocols">PredictorProtocols
</h3>
<p>
(<em>Appears on:</em>
<a href="#serving.kubeflow.org/v1beta1.PredictorsConfig">PredictorsConfig</a>)
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
<code>v1</code></br>
<em>
<a href="#serving.kubeflow.org/v1beta1.PredictorConfig">
PredictorConfig
</a>
</em>
</td>
<td>
</td>
</tr>
<tr>
<td>
<code>v2</code></br>
<em>
<a href="#serving.kubeflow.org/v1beta1.PredictorConfig">
PredictorConfig
</a>
</em>
</td>
<td>
</td>
</tr>
</tbody>
</table>
<h3 id="serving.kubeflow.org/v1beta1.PredictorSpec">PredictorSpec
</h3>
<p>
(<em>Appears on:</em>
<a href="#serving.kubeflow.org/v1beta1.InferenceServiceSpec">InferenceServiceSpec</a>)
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
<code>sklearn</code></br>
<em>
<a href="#serving.kubeflow.org/v1beta1.SKLearnSpec">
SKLearnSpec
</a>
</em>
</td>
<td>
<p>Spec for SKLearn model server</p>
</td>
</tr>
<tr>
<td>
<code>xgboost</code></br>
<em>
<a href="#serving.kubeflow.org/v1beta1.XGBoostSpec">
XGBoostSpec
</a>
</em>
</td>
<td>
<p>Spec for XGBoost model server</p>
</td>
</tr>
<tr>
<td>
<code>tensorflow</code></br>
<em>
<a href="#serving.kubeflow.org/v1beta1.TFServingSpec">
TFServingSpec
</a>
</em>
</td>
<td>
<p>Spec for TFServing (<a href="https://github.com/tensorflow/serving">https://github.com/tensorflow/serving</a>)</p>
</td>
</tr>
<tr>
<td>
<code>pytorch</code></br>
<em>
<a href="#serving.kubeflow.org/v1beta1.TorchServeSpec">
TorchServeSpec
</a>
</em>
</td>
<td>
<p>Spec for TorchServe (<a href="https://pytorch.org/serve">https://pytorch.org/serve</a>)</p>
</td>
</tr>
<tr>
<td>
<code>triton</code></br>
<em>
<a href="#serving.kubeflow.org/v1beta1.TritonSpec">
TritonSpec
</a>
</em>
</td>
<td>
<p>Spec for Triton Inference Server (<a href="https://github.com/triton-inference-server/server">https://github.com/triton-inference-server/server</a>)</p>
</td>
</tr>
<tr>
<td>
<code>onnx</code></br>
<em>
<a href="#serving.kubeflow.org/v1beta1.ONNXRuntimeSpec">
ONNXRuntimeSpec
</a>
</em>
</td>
<td>
<p>Spec for ONNX runtime (<a href="https://github.com/microsoft/onnxruntime">https://github.com/microsoft/onnxruntime</a>)</p>
</td>
</tr>
<tr>
<td>
<code>pmml</code></br>
<em>
<a href="#serving.kubeflow.org/v1beta1.PMMLSpec">
PMMLSpec
</a>
</em>
</td>
<td>
<p>Spec for PMML (<a href="http://dmg.org/pmml/v4-1/GeneralStructure.html">http://dmg.org/pmml/v4-1/GeneralStructure.html</a>)</p>
</td>
</tr>
<tr>
<td>
<code>lightgbm</code></br>
<em>
<a href="#serving.kubeflow.org/v1beta1.LightGBMSpec">
LightGBMSpec
</a>
</em>
</td>
<td>
<p>Spec for LightGBM model server</p>
</td>
</tr>
<tr>
<td>
<code>PodSpec</code></br>
<em>
<a href="#serving.kubeflow.org/v1beta1.PodSpec">
PodSpec
</a>
</em>
</td>
<td>
<p>
(Members of <code>PodSpec</code> are embedded into this type.)
</p>
<p>This spec is dual purpose. <br />
1) Provide a full PodSpec for custom predictor.
The field PodSpec.Containers is mutually exclusive with other predictors (i.e. TFServing). <br />
2) Provide a predictor (i.e. TFServing) and specify PodSpec
overrides, you must not provide PodSpec.Containers in this case. <br /></p>
</td>
</tr>
<tr>
<td>
<code>ComponentExtensionSpec</code></br>
<em>
<a href="#serving.kubeflow.org/v1beta1.ComponentExtensionSpec">
ComponentExtensionSpec
</a>
</em>
</td>
<td>
<p>
(Members of <code>ComponentExtensionSpec</code> are embedded into this type.)
</p>
<p>Component extension defines the deployment configurations for a predictor</p>
</td>
</tr>
</tbody>
</table>
<h3 id="serving.kubeflow.org/v1beta1.PredictorsConfig">PredictorsConfig
</h3>
<p>
(<em>Appears on:</em>
<a href="#serving.kubeflow.org/v1beta1.InferenceServicesConfig">InferenceServicesConfig</a>)
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
<a href="#serving.kubeflow.org/v1beta1.PredictorConfig">
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
<a href="#serving.kubeflow.org/v1beta1.PredictorConfig">
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
<a href="#serving.kubeflow.org/v1beta1.PredictorProtocols">
PredictorProtocols
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
<a href="#serving.kubeflow.org/v1beta1.PredictorProtocols">
PredictorProtocols
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
<a href="#serving.kubeflow.org/v1beta1.PredictorProtocols">
PredictorProtocols
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
<a href="#serving.kubeflow.org/v1beta1.PredictorConfig">
PredictorConfig
</a>
</em>
</td>
<td>
</td>
</tr>
<tr>
<td>
<code>pmml</code></br>
<em>
<a href="#serving.kubeflow.org/v1beta1.PredictorConfig">
PredictorConfig
</a>
</em>
</td>
<td>
</td>
</tr>
<tr>
<td>
<code>lightgbm</code></br>
<em>
<a href="#serving.kubeflow.org/v1beta1.PredictorConfig">
PredictorConfig
</a>
</em>
</td>
<td>
</td>
</tr>
</tbody>
</table>
<h3 id="serving.kubeflow.org/v1beta1.SKLearnSpec">SKLearnSpec
</h3>
<p>
(<em>Appears on:</em>
<a href="#serving.kubeflow.org/v1beta1.PredictorSpec">PredictorSpec</a>)
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
<code>PredictorExtensionSpec</code></br>
<em>
<a href="#serving.kubeflow.org/v1beta1.PredictorExtensionSpec">
PredictorExtensionSpec
</a>
</em>
</td>
<td>
<p>
(Members of <code>PredictorExtensionSpec</code> are embedded into this type.)
</p>
<p>Contains fields shared across all predictors</p>
</td>
</tr>
</tbody>
</table>
<h3 id="serving.kubeflow.org/v1beta1.TFServingSpec">TFServingSpec
</h3>
<p>
(<em>Appears on:</em>
<a href="#serving.kubeflow.org/v1beta1.PredictorSpec">PredictorSpec</a>)
</p>
<p>
<p>TFServingSpec defines arguments for configuring Tensorflow model serving.</p>
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
<code>PredictorExtensionSpec</code></br>
<em>
<a href="#serving.kubeflow.org/v1beta1.PredictorExtensionSpec">
PredictorExtensionSpec
</a>
</em>
</td>
<td>
<p>
(Members of <code>PredictorExtensionSpec</code> are embedded into this type.)
</p>
<p>Contains fields shared across all predictors</p>
</td>
</tr>
</tbody>
</table>
<h3 id="serving.kubeflow.org/v1beta1.TorchServeSpec">TorchServeSpec
</h3>
<p>
(<em>Appears on:</em>
<a href="#serving.kubeflow.org/v1beta1.PredictorSpec">PredictorSpec</a>)
</p>
<p>
<p>TorchServeSpec defines arguments for configuring PyTorch model serving.</p>
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
<code>modelClassName</code></br>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>When this field is specified KFS chooses the KFServer implementation, otherwise KFS uses the TorchServe implementation</p>
</td>
</tr>
<tr>
<td>
<code>PredictorExtensionSpec</code></br>
<em>
<a href="#serving.kubeflow.org/v1beta1.PredictorExtensionSpec">
PredictorExtensionSpec
</a>
</em>
</td>
<td>
<p>
(Members of <code>PredictorExtensionSpec</code> are embedded into this type.)
</p>
<p>Contains fields shared across all predictors</p>
</td>
</tr>
</tbody>
</table>
<h3 id="serving.kubeflow.org/v1beta1.TransformerConfig">TransformerConfig
</h3>
<p>
(<em>Appears on:</em>
<a href="#serving.kubeflow.org/v1beta1.TransformersConfig">TransformersConfig</a>)
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
<p>transformer docker image name</p>
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
<p>default transformer docker image version</p>
</td>
</tr>
</tbody>
</table>
<h3 id="serving.kubeflow.org/v1beta1.TransformerSpec">TransformerSpec
</h3>
<p>
(<em>Appears on:</em>
<a href="#serving.kubeflow.org/v1beta1.InferenceServiceSpec">InferenceServiceSpec</a>)
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
<code>PodSpec</code></br>
<em>
<a href="#serving.kubeflow.org/v1beta1.PodSpec">
PodSpec
</a>
</em>
</td>
<td>
<p>
(Members of <code>PodSpec</code> are embedded into this type.)
</p>
<p>This spec is dual purpose. <br />
1) Provide a full PodSpec for custom transformer.
The field PodSpec.Containers is mutually exclusive with other transformers. <br />
2) Provide a transformer and specify PodSpec
overrides, you must not provide PodSpec.Containers in this case. <br /></p>
</td>
</tr>
<tr>
<td>
<code>ComponentExtensionSpec</code></br>
<em>
<a href="#serving.kubeflow.org/v1beta1.ComponentExtensionSpec">
ComponentExtensionSpec
</a>
</em>
</td>
<td>
<p>
(Members of <code>ComponentExtensionSpec</code> are embedded into this type.)
</p>
<p>Component extension defines the deployment configurations for a transformer</p>
</td>
</tr>
</tbody>
</table>
<h3 id="serving.kubeflow.org/v1beta1.TransformersConfig">TransformersConfig
</h3>
<p>
(<em>Appears on:</em>
<a href="#serving.kubeflow.org/v1beta1.InferenceServicesConfig">InferenceServicesConfig</a>)
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
<a href="#serving.kubeflow.org/v1beta1.TransformerConfig">
TransformerConfig
</a>
</em>
</td>
<td>
</td>
</tr>
</tbody>
</table>
<h3 id="serving.kubeflow.org/v1beta1.TritonSpec">TritonSpec
</h3>
<p>
(<em>Appears on:</em>
<a href="#serving.kubeflow.org/v1beta1.PredictorSpec">PredictorSpec</a>)
</p>
<p>
<p>TritonSpec defines arguments for configuring Triton model serving.</p>
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
<code>PredictorExtensionSpec</code></br>
<em>
<a href="#serving.kubeflow.org/v1beta1.PredictorExtensionSpec">
PredictorExtensionSpec
</a>
</em>
</td>
<td>
<p>
(Members of <code>PredictorExtensionSpec</code> are embedded into this type.)
</p>
<p>Contains fields shared across all predictors</p>
</td>
</tr>
</tbody>
</table>
<h3 id="serving.kubeflow.org/v1beta1.XGBoostSpec">XGBoostSpec
</h3>
<p>
(<em>Appears on:</em>
<a href="#serving.kubeflow.org/v1beta1.PredictorSpec">PredictorSpec</a>)
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
<code>PredictorExtensionSpec</code></br>
<em>
<a href="#serving.kubeflow.org/v1beta1.PredictorExtensionSpec">
PredictorExtensionSpec
</a>
</em>
</td>
<td>
<p>
(Members of <code>PredictorExtensionSpec</code> are embedded into this type.)
</p>
<p>Contains fields shared across all predictors</p>
</td>
</tr>
</tbody>
</table>
<hr/>
<p><em>
Generated with <code>gen-crd-api-reference-docs</code>
on git commit <code>df869c1</code>.
</em></p>
