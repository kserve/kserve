# Data Plane Specification

This document is a work in progress to disucss the data plane requirements for machine learning serving.

Its aims:

 * Provide a set of schemas for generic machine learning  components, such as model servers, explainers, outlier detectors.
 * Provide a set of proposals for components to advertise the schemas they support.

## Data Plane Schemas

There are various components that are useful for ML inference, these include

 * The core machine learning model
 * Model explanation
 * Outlier detection
 * Concept drift (skew) detection

Schemas will be needed for each.

### Models

Data planes for request/response to machine learning models are the most well defined in the ecosystem. Existing examples are:

 * [Tensorflow Serving](https://github.com/tensorflow/serving/blob/master/tensorflow_serving/apis/prediction_service.proto)
 * [Seldon Core](https://github.com/SeldonIO/seldon-core/blob/master/proto/prediction.proto)

It is suggested we don't define a new data plane for model input/output at present but allow models to publish the input/output schema they respect.


### Model Explanation

TODO

### Outlier Detection

TODO

### Concept Drift

TODO


## Combined Schema

There is an open question whether we define a combined schema to return the aggregation from the various components or we assume only the model response is returned and other components (model explanation etc) return their response asychronously to some metrics/logging channel.


## Metadata

  * There should be a unified prediction id so responses from varied components can be tied together for monitoring and auditing.

It unclear whether we should impose any other metadata.

## Schema Publishing

Components should be able to advertise what schemas they respect to allow the control plane to do static validation. Static validation will be important if we allow pipelines of components in future.

KNative has a [propsoal](https://github.com/knative/eventing/blob/6155ebb1f662e7d8930d27d3446e47103bde5c85/docs/registry/README.md) in the context of KNative eventing.

