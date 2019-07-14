# Conformance test image (v1)

This directory contains a test image used in the conformance tests.

The images contain a webserver that will by default listens on port `8080` and
expose a service at `/`.

When called, the server emits the message "What a spaceport!".

## Trying out

To run the image as a Service outisde of the test suite:

`ko apply -f service.yaml`

## Building

For details about building and adding new images, see the
[section about test images](/test/README.md#test-images).
