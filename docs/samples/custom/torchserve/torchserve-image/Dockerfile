# syntax = docker/dockerfile:experimental
#
# This file can build images for cpu and gpu env. By default it builds image for CPU.
# Use following option to build image for cuda/GPU: --build-arg BASE_IMAGE=nvidia/cuda:10.1-cudnn7-runtime-ubuntu18.04
# Here is complete command for GPU/cuda - 
# $ DOCKER_BUILDKIT=1 docker build --file Dockerfile --build-arg BASE_IMAGE=nvidia/cuda:10.1-cudnn7-runtime-ubuntu18.04 -t torchserve:latest .
#
# Following comments have been shamelessly copied from https://github.com/pytorch/pytorch/blob/master/Dockerfile
# 
# NOTE: To build this you will need a docker version > 18.06 with
#       experimental enabled and DOCKER_BUILDKIT=1
#
#       If you do not use buildkit you are not going to have a good time
#
#       For reference: 
#           https://docs.docker.com/develop/develop-images/build_enhancements/


FROM pytorch/torchserve:latest

USER root

# Modify config.propeties path for external storage
COPY dockerd-entrypoint.sh /usr/local/bin/dockerd-entrypoint.sh
RUN chmod 755 /usr/local/bin/dockerd-entrypoint.sh 

# Copy model files and config properties
COPY config.properties /home/model-server/config.properties 
COPY model-store/* /home/model-server/model-store/

EXPOSE 8082 

USER model-server
