# Echo WebSocket test image

A simple WebSocket server adapted from
https://github.com/gorilla/WebSocket/blob/master/examples/echo/server.go . The
server simply echoes messages sent to it. We use this server in testing that all
our proxies on request path can handle WebSocket upgrades.

## Building

For details about building and adding new images, see the
[section about test images](/test/README.md#test-images).
