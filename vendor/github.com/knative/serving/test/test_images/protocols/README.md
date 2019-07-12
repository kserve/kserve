# Protocols test image

This directory contains the test image used to test the
[supported network protocols](/docs/runtime-contract.md#protocols-and-ports).

The image contains a simple server that will serve HTTP/1.1 and HTTP/2.0
requests from the same port. The response will be a JSON with the following
format:

```
{
  'protoMajor': 1 # 2 if HTTP/2
  'protoMinor': 1 # 0 if HTTP/2
}
```

## Building

For details about building and adding new images, see the
[section about test images](/test/README.md#test-images).
