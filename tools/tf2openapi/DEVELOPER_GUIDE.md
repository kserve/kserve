# Development
This doc explains how to setup your developer environment to contribute to
Tf2OpenAPI.

## Prerequisites
1. [`go`](https://golang.org/doc/install): KFServing controller is written in Go.
1. [`git`](https://help.github.com/articles/set-up-git/): For source control.
1. [`dep`](https://github.com/golang/dep): For managing external Go
   dependencies.
1. [`protoc`](http://google.github.io/proto-lens/installing-protoc.html): For
   compiling protobufs.
1. [`protoc-gen-go`](https://github.com/golang/protobuf): For using protobufs
   with Go.

## Potential Issues and Solutions
* When extending Tf2OpenAPI to support future versions of TensorFlow (> 1.13.1),
you may encounter a seemingly circular dependency in the compiled protos and/or the Go tool may complain that there are multiple Go packages in one directory. It's possible that certain TensorFlow protos aren't compiled correctly because they lack the field `option go_package`. File an issue in the TensorFlow repository to add that field.
* To make dependencies persist without importing them (e.g. TensorFlow when only using the protos), add them as required to the Gopkg.toml and add a [[prune.project]] section so they are not pruned.
