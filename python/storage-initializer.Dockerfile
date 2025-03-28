ARG VENV_PATH=/prod_venv

## Builder
FROM registry.access.redhat.com/ubi9/ubi-minimal:latest AS builder

# Install Python and dependencies
RUN microdnf install -y --setopt=ubi-9-appstream-rpms.module_hotfixes=1 --disablerepo=* \
    --enablerepo=ubi-9-baseos-rpms --enablerepo=ubi-9-appstream-rpms \
      python3.11-devel \
      python3.11 \
      gcc \
      libffi-devel \
      openssl-devel \
      krb5-workstation \
      krb5-libs  \
      krb5-devel  \
    && microdnf clean all \
    && alternatives --install /usr/bin/python python /usr/bin/python3.11 1


# Install Poetry
ARG POETRY_HOME=/opt/poetry
ARG POETRY_VERSION=1.8.3

RUN python -m venv ${POETRY_HOME} && ${POETRY_HOME}/bin/pip install poetry==${POETRY_VERSION}
ENV PATH="$PATH:${POETRY_HOME}/bin"

# Activate virtual env
ARG VENV_PATH
ENV VIRTUAL_ENV=${VENV_PATH}
RUN python -m venv $VIRTUAL_ENV
ENV PATH="$VIRTUAL_ENV/bin:$PATH"

COPY kserve/pyproject.toml kserve/poetry.lock kserve/
RUN cd kserve && poetry install --no-root --no-interaction --no-cache --extras "storage"
COPY kserve kserve
RUN cd kserve && poetry install --no-interaction --no-cache --extras "storage"

RUN pip install --no-cache-dir krbcontext==0.10 hdfs~=2.6.0 requests-kerberos==0.14.0

## Runtime
FROM registry.access.redhat.com/ubi9/ubi-minimal:latest AS prod

COPY third_party third_party

# Activate virtual env
ARG VENV_PATH
ENV VIRTUAL_ENV=${VENV_PATH}
ENV PATH="$VIRTUAL_ENV/bin:$PATH"

RUN microdnf install -y --setopt=ubi-9-appstream-rpms.module_hotfixes=1 --disablerepo=* \
    --enablerepo=ubi-9-baseos-rpms --enablerepo=ubi-9-appstream-rpms shadow-utils python3.11 python3.11-devel \
    && microdnf clean all \
    &&  alternatives --install /usr/bin/python python3 /usr/bin/python3.11 1
RUN useradd kserve -m -u 1000 -d /home/kserve

COPY --from=builder --chown=kserve:kserve $VIRTUAL_ENV $VIRTUAL_ENV
COPY --from=builder kserve kserve
COPY ./storage-initializer /storage-initializer

RUN chmod +x /storage-initializer/scripts/initializer-entrypoint
RUN mkdir /work
WORKDIR /work

# Set a writable /mnt folder to avoid permission issue on Huggingface download. See https://huggingface.co/docs/hub/spaces-sdks-docker#permissions
RUN chown -R kserve:kserve /mnt
USER 1000
ENTRYPOINT ["/storage-initializer/scripts/initializer-entrypoint"]
