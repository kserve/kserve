# Timeout test image

The image contains a simple Go webserver, `timeout.go`, that will, by default,
listen on port `8080` and expose a service at `/`.

When called, the server sleeps for the amount of milliseconds passed in via the
query parameter `timeout` and responds with "Slept for X milliseconds` where X
is the amount of milliseconds passed in.

## Trying out

To run the image as a Service outisde of the test suite:

`ko apply -f service.yaml`

## Building

For details about building and adding new images, see the
[section about test images](/test/README.md#test-images).
