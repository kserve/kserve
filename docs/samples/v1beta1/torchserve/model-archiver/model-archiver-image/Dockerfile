FROM ubuntu:18.04

RUN apt-get update && \
    DEBIAN_FRONTEND=noninteractive apt-get install --no-install-recommends -y \
    ca-certificates \
    g++ \
    python3-dev \
    python3-distutils \
    python3-venv \
    curl \
    jq \
    && rm -rf /var/lib/apt/lists/* \
    && cd /tmp \
    && curl -O https://bootstrap.pypa.io/get-pip.py \
    && python3 get-pip.py

RUN python3 -m venv /home/venv

ENV PATH="/home/venv/bin:$PATH"

RUN update-alternatives --install /usr/bin/python python /usr/bin/python3 1
RUN update-alternatives --install /usr/local/bin/pip pip /usr/local/bin/pip3 1

RUN pip install --no-cache-dir torch-model-archiver

RUN useradd -m model-server

# ADD "https://www.random.org/cgi-bin/randbyte?nbytes=10&format=h" skipcache

COPY dockerd-entrypoint.sh /usr/local/bin/dockerd-entrypoint.sh

RUN chmod +x /usr/local/bin/dockerd-entrypoint.sh \
    && chown -R model-server /home/model-server

USER model-server
WORKDIR /home/model-server
ENTRYPOINT ["/usr/local/bin/dockerd-entrypoint.sh"]
