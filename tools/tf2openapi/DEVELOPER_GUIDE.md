# Development
This doc explains how to setup your developer environment to contribute to
Tf2OpenAPI.

## Prerequisites
### Install requirements
1. [`go`](https://golang.org/doc/install): Tf2OpenAPI is written in Go.
1. [`git`](https://help.github.com/articles/set-up-git/): For source control.
1. [`dep`](https://github.com/golang/dep): For managing external Go
   dependencies.
1. [`protoc`](http://google.github.io/proto-lens/installing-protoc.html): For
   compiling protobufs.
1. [`protoc-gen-go`](https://github.com/golang/protobuf): For using protobufs
   with Go.
### Setup your environment
To start your environment you'll need to set these environment variables (we recommend adding them to your .bashrc):

1. `GOPATH`: If you don't have one, simply pick a directory (e.g. `$HOME/go`) and add `export GOPATH=...`
2. `$GOPATH/bin` on `PATH`: This is so that tooling installed via `go get` will work properly. For example, this allows `protoc` to use `protoc-gen-go`.


## Potential Issues and Solutions
* When extending Tf2OpenAPI to support future versions of TensorFlow (> 1.13.1),
you may encounter a seemingly circular dependency in the compiled protos and/or the Go tool may complain that there are multiple Go packages in one directory. It's possible that certain TensorFlow protos aren't compiled correctly because they lack the field `option go_package`. File an issue in the TensorFlow repository to add that field.
* To make dependencies persist without importing them (e.g. TensorFlow when only using the protos), add them as `required` to the `Gopkg.toml`. 
```
required = ["github.com/foo/bar/..] # this must be a package/subpackage containing Go code
```
See how TensorFlow is added as a dependency in `Gopkg.toml` in the top level for an example.
