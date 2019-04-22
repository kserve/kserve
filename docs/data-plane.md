# Data Plane Specification

This document is a work in progress to discuss the data plane requirements for machine learning inference.

Its aims:

 * Provide a set of schemas for machine learning inference including the predictor and associated components such as model explainers, outlier and skew detectors.
 * Provide a set of proposals for components to advertise the schemas they support.

## Data Plane Schemas

There are various components that are useful for machine learning inference, these include

 * The core predictor
 * Model explanation
 * Outlier detection
 * Concept drift (skew) detection

Schemas will be needed for each. The aim is to provide a set of schemas for the core predictive model along with associated schemas for the most common tasks in helping data scientists, users and devops teams monitor and understand the running model.

### Predictors

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

There is an open question whether we define a combined schema to return the aggregation from the various components or we assume only the model response is returned and other components (model explanation etc) return their response asynchronously to some metrics/logging channel.

The control plane will allow switching off/on of components so a synchronous response could provide some subset of all data payloads, e.g. prediction, explanation. In proto buffers representation a combined payload could look like:

```
message KFServing {
  KFPrediction prediction = 1;
  KFExplanation explanation = 2;
  KFOutlier outlier = 3;
  KFSkew skew = 4;
}
```

## Metadata

  * There should be a unified prediction id so responses from varied components can be tied together for monitoring and auditing.

It unclear whether we should impose any other metadata.

## Schema Publishing

Components should be able to advertise what schemas they respect to allow the control plane to do static validation. Static validation will be important if we allow pipelines of components in future.

Knative has a [proposal](https://github.com/knative/eventing/blob/6155ebb1f662e7d8930d27d3446e47103bde5c85/docs/registry/README.md) in the context of Knative eventing.

