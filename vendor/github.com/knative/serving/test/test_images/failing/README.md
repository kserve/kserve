# Failing test image

This directory contains the test image used to simulate a crashing image.

The image runs for 10 seconds and then exits. It is useful for testing readiness
probes.

## Trying out

To run the image as a Service outisde of the test suite:

`ko apply -f service.yaml`

## Building

For details about building and adding new images, see the
[section about test images](/test/README.md#test-images).
