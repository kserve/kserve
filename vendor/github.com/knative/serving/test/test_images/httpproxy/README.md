# HTTPproxy test image

This directory contains the test image used in the e2e test to verify
service-to-service call within cluster.

The image contains a simple Go webserver, `httproxy.go`, that will, by default,
listen on port `8080` and expose a service at `/`.

When called, the proxy server redirects request to the target server.

To use this image, users need to first set the host of the target server that
the proxy redirects request to by setting environment variable `TARGET_HOST`.

## Trying out

To run the image as a Service outisde of the test suite:

`ko apply -f service.yaml`

## Building

For details about building and adding new images, see the
[section about test images](/test/README.md#test-images).
