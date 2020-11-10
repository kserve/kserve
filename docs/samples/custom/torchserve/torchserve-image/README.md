
# Docker image building

**Steps:**

 1. Download model archive file from the [model-zoo](https://github.com/pytorch/serve/blob/master/docs/model_zoo.md) or create you own using the step provided [here](https://github.com/pytorch/serve/blob/master/model-archiver/README.md)
 2. Copy model archive files to model-store folder
 3. Edit config.properties for requirement
 4. Run docker build
 5. Publish the image to dockerhub repo

```bash
# For CPU:
DOCKER_BUILDKIT=1 docker build --file Dockerfile -t torchserve:latest .

# For GPU:
DOCKER_BUILDKIT=1 docker build --file Dockerfile --build-arg BASE_IMAGE=nvidia/cuda:10.1-cudnn7-runtime-ubuntu18.04 -t torchserve-gpu:latest .

docker push {username}/torchserve:latest
```
