# Conformance test image (single threaded)

This directory contains a test image used in the conformance tests.

The images contain a webserver that will by default listens on port `8080` and
expose a service at `/`.

When called, the server sleeps a short period and returns a 200 status if no
other request is running in the container concurrently. If it does detect
concurrent requests, it instead returns a 500 status.

## Trying out

To run the image as a Service outisde of the test suite:

`ko apply -f service.yaml`

## Building

For details about building and adding new images, see the
[section about test images](/test/README.md#test-images).
