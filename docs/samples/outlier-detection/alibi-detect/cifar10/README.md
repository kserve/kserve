# CIFAR10 Outlier Detector

A [CIFAR10](https://www.cs.toronto.edu/~kriz/cifar.html) Outlier Detector. Run the [notebook demo](cifar10_outlier.ipynb) to test.

The architecture used is shown below and links the payload logging available within KFServing with asynchronous processing of those payloads in KNative to detect outliers.

![Architetcure](architecture.png)


## Dependencies

You will need a version of Knative that support V1 protocol of CloudEvents. Suggested **KNative Eventing >=0.14.2**
