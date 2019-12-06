# Architecture Overview
The InferenceService Data Plane architecture consists of a static graph of components which coordinate requests for a single model. Advanced features such as Ensembling, A/B testing, and Multi-Arm-Bandits should compose InferenceServices together.

![Data Plane](./diagrams/dataplane.jpg)

# Concepts
**Endpoint**: InferenceServers are divided into two endpoints: "default" and "canary". The endpoints allow users to safely make changes using the Pinned and Canary rollout strategies. Canarying is completely optional enabling users to simply deploy with a BlueGreen deployment strategy on the "default" endpoint.

**Component**: Each endpoint is composed of multiple components: "predictor", "explainer", and "transformer". The only required component is the predictor, which is the core of the system. As KFServing evolves, we plan to increase the number of supported components to enable use cases like Outlier Detection.

**Predictor**: The predictor is the workhorse of the InferenceService. It is simply a model and a model server that makes it available at a network endpoint.

**Explainer**: The explainer enables an optional alternate data plane that provides model explanations in addition to predictions. Users may define their own explanation container, which KFServing configures with relevant environment variables like prediction endpoint. For common use cases, KFServing provides out-of-the-box explainers like Alibi.

**Transformer**: The transformer enables users to define a pre and post processing step before the prediction and explanation workflows. Like the explainer, it is configured with relevant environment variables too. For common use cases, KFServing provides out-of-the-box transformers like Feast.

# Data Plane (V1)
KFServing has a standardized prediction workflow across all model frameworks. 

Note: We are actively developing a V2 data plane protocol to improve performance (i.e. GRPC).

## Predict
All InferenceServices speak the Tensorflow V1 HTTP API: https://www.tensorflow.org/tfx/serving/api_rest#predict_api. 

Note: Only Tensorflow models support the fields "signature_name" and "inputs".

## Explain
All InferenceServices that are deployed with an Explainer support a standardized explanation API. This interface is identical to the Tensorflow V1 HTTP API with the addition of an ":explain" verb.
