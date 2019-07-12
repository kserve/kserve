# Helloworld test image

This directory contains the test image used in the helloworld e2e test.

The image contains a simple Go webserver, `helloworld.go`, that will, by
default, listen on port `8080` and expose a service at `/`.

When called, the server emits a "hello world" message.

## Trying out

To run the image as a Service outisde of the test suite:

`ko apply -f service.yaml`

To run this image as just a Route and Configuration:

`ko apply -f helloworld.yaml`

## Building

For details about building and adding new images, see the
[section about test images](/test/README.md#test-images).
