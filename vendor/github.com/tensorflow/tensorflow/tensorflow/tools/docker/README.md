# WARNING: THESE IMAGES ARE DEPRECATED.

TensorFlow's Dockerfiles are now located in
[`tensorflow/tools/dockerfiles/`](https://github.com/tensorflow/tensorflow/blob/master/tensorflow/tools/dockerfiles).
However, these Dockerfiles are still used to build
[TensorFlow's official Docker images](https://hub.docker.com/r/tensorflow/tensorflow)
while the internal infrastructure for the newer Dockerfiles is being developed.

This directory will eventually be removed.

# Using TensorFlow via Docker

This directory contains `Dockerfile`s to make it easy to get up and running with
TensorFlow via [Docker](http://www.docker.com/).

## Installing Docker

General installation instructions are
[on the Docker site](https://docs.docker.com/installation/), but we give some
quick links here:

* [OSX](https://www.docker.com/products/docker#/mac)
* [Ubuntu](https://docs.docker.com/engine/installation/linux/ubuntulinux/)

## Which containers exist?

We currently maintain two Docker container images:

* `tensorflow/tensorflow` - TensorFlow with all dependencies - CPU only!

* `tensorflow/tensorflow:latest-gpu` - TensorFlow with all dependencies
  and support for NVidia CUDA

Note: We store all our containers on 
[Docker Hub](https://hub.docker.com/r/tensorflow/tensorflow/tags/).


## Running the container

Run non-GPU container using

    $ docker run -it -p 8888:8888 tensorflow/tensorflow

For GPU support install NVidia drivers (ideally latest) and
[nvidia-docker](https://github.com/NVIDIA/nvidia-docker). Run using

    $ nvidia-docker run -it -p 8888:8888 tensorflow/tensorflow:latest-gpu


Note: If you would have a problem running nvidia-docker you may try the old method
we have used. But it is not recommended. If you find a bug in nvidia-docker, please report
it there and try using nvidia-docker as described above.

    $ # The old, not recommended way to run docker with gpu support:
    $ export CUDA_SO=$(\ls /usr/lib/x86_64-linux-gnu/libcuda.* | xargs -I{} echo '-v {}:{}')
    $ export DEVICES=$(\ls /dev/nvidia* | xargs -I{} echo '--device {}:{}')
    $ docker run -it -p 8888:8888 $CUDA_SO $DEVICES tensorflow/tensorflow:latest-gpu


## More containers

See all available [tags](https://hub.docker.com/r/tensorflow/tensorflow/tags/)
for additional containers, such as release candidates or nightly builds.


## Rebuilding the containers

Building TensorFlow Docker containers should be done through the
[parameterized_docker_build.sh](https://github.com/tensorflow/tensorflow/blob/master/tensorflow/tools/docker/parameterized_docker_build.sh)
script. The raw Dockerfiles should not be used directly as they contain strings
to be replaced by the script during the build.

Attempting to run [parameterized_docker_build.sh](https://github.com/tensorflow/tensorflow/blob/master/tensorflow/tools/docker/parameterized_docker_build.sh)
from a binary docker image such as for example `tensorflow/tensorflow:latest` will
not work. One needs to execute the script from a developer docker image since by
contrast with a binary docker image it contains not only the compiled solution but
also the tensorflow source code. Please select the appropriate developer docker
image of tensorflow at `tensorflow/tensorflow:[.](https://hub.docker.com/r/tensorflow/tensorflow/tags/)`.

The smallest command line to generate a docker image will then be:
```docker run -it tensorflow/tensorflow:"right_tag"```

If you would like to start a jupyter notebook on your docker container, make sure
to map the port 8888 of your docker container by adding -p 8888:8888 to the above
command.

To use the script, specify the container type (`CPU` vs. `GPU`), the desired
Python version (`PYTHON2` vs. `PYTHON3`) and whether the developer Docker image
is to be built (`NO` vs. `YES`). In addition, you need to specify the central
location from where the pip package of TensorFlow will be downloaded.

For example, to build a CPU-only non-developer Docker image for Python 2, using
TensorFlow's nightly pip package:

``` bash
export TF_DOCKER_BUILD_IS_DEVEL=NO
export TF_DOCKER_BUILD_TYPE=CPU
export TF_DOCKER_BUILD_PYTHON_VERSION=PYTHON2

pip download --no-deps tf-nightly

export TF_DOCKER_BUILD_CENTRAL_PIP=$(ls tf_nightly*.whl)
export TF_DOCKER_BUILD_CENTRAL_PIP_IS_LOCAL=1

tensorflow/tools/docker/parameterized_docker_build.sh
```

If successful, the image will be tagged as `${USER}/tensorflow:latest` by default.

Rebuilding GPU images requires [nvidia-docker](https://github.com/NVIDIA/nvidia-docker).
