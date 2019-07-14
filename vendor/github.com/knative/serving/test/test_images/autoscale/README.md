# Autoscale test image

This directory contains the test image used in the autoscale e2e tests.

The image contains a simple Go webserver, `autoscale.go`, that will, by default,
listen on port `8080` and expose a service at `/`.

When called, the server calculates the highest prime less than 40000000 and
emits a message.

## Trying out

To run the image as a Service outisde of the test suite:

`ko apply -f service.yaml`

## Building

For details about building and adding new images, see the
[section about test images](/test/README.md#test-images).
