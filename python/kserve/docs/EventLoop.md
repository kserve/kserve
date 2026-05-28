# HTTP Server Event Loop Configuration

KServe allows configuring the event loop implementation used by the HTTP server.
This can be useful for performance tuning or for ensuring compatibility with
specific runtime environments.

## Configuration

The event loop is configured using the `--event-loop` command-line argument.

Supported values:

- `auto` (default): Automatically select the event loop. If `uvloop` is installed,
  it will be used; otherwise, the standard `asyncio` event loop is used.
- `asyncio`: Force the use of Python’s built-in `asyncio` event loop.
- `uvloop`: Force the use of `uvloop` (requires `uvloop` to be installed).

## Example

```bash
kserve start \
  --event-loop uvloop \
  --http_port 8080
```

```python
from kserve import ModelServer

server = ModelServer(
    http_port=8080,
    event_loop="uvloop",  # "auto", "asyncio", or "uvloop"
)

# Register models and start the server
server.start(models=[])
```