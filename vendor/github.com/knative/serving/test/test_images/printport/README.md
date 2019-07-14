# PrintPort Test Image

This directory contains the test image used in the user-port e2e test.

The image contains a simple Go webserver, `printport.go`, that will listen on
the port defined by environment variable `PORT` and expose a service at `/`.

When called, the server emits the port it was called on.

See the example below when the default `8080` port is used:

```
> curl -H "Host: printport.default.example.com"
http://$IP
8080
```

## Trying out

To run the image as a Service outisde of the test suite:

`ko apply -f service.yaml`

## Building

For details about building and adding new images, see the
[section about test images](/test/README.md#test-images).
