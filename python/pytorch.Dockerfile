FROM ubuntu:18.04

ARG PYTORCH_VERSION=1.3.1

RUN apt-get update && apt-get install -y --no-install-recommends \
         build-essential \
         git \
         curl \
         ca-certificates \
         libjpeg-dev \
         libpng-dev && \
     rm -rf /var/lib/apt/lists/*

RUN curl -L -o ~/miniconda.sh -O  https://repo.continuum.io/miniconda/Miniconda3-4.2.12-Linux-x86_64.sh  && \
     chmod +x ~/miniconda.sh && \
     ~/miniconda.sh -b -p /opt/conda && \
     rm ~/miniconda.sh && \
     /opt/conda/bin/conda install conda-build && \
     /opt/conda/bin/conda create -y --name pytorch-py37 python=3.7.3 numpy pyyaml scipy ipython mkl && \
     /opt/conda/bin/conda clean -ya
ENV PATH /opt/conda/envs/pytorch-py37/bin:$PATH
RUN conda install --name pytorch-py37 pytorch==$PYTORCH_VERSION torchvision pillow==6.2.0 cpuonly -c pytorch && /opt/conda/bin/conda clean -ya

WORKDIR /workspace
RUN chmod -R a+w /workspace

COPY pytorchserver pytorchserver
COPY kserve kserve
COPY third_party third_party

RUN pip install --no-cache-dir --upgrade pip && pip install --no-cache-dir -e ./kserve
RUN pip install --no-cache-dir -e ./pytorchserver
ENTRYPOINT ["python", "-m", "pytorchserver"]
