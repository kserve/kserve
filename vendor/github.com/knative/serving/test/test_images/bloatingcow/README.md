# Bloating cow test image

This directory contains the test image used in the resources e2e test.

The image contains a simple Go webserver, `bloatingcow.go`, that will, by
default, listen on port `8080` and expose a service at `/`.

When called, the server emits a "Moo!" message, if query parameter
`memory_in_mb` is specified, the app will allocated so many Mbs of memory.

## Trying out

To run the image as a Service outisde of the test suite:

`ko apply -f service.yaml`

## Building

For details about building and adding new images, see the
[section about test images](/test/README.md#test-images).
