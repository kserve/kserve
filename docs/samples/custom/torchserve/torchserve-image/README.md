
# Docker image building

**Steps:**

 1. Copy marfiles to model-store folder
 2. Edit config.properties for requirement
 3. Run docker build

```bash
For CPU:
DOCKER_BUILDKIT=1 docker build --file Dockerfile -t torchserve:latest .

For GPU:
DOCKER_BUILDKIT=1 docker build --file Dockerfile --build-arg BASE_IMAGE=nvidia/cuda:10.1-cudnn7-runtime-ubuntu18.04 -t torchserve:latest .
```
