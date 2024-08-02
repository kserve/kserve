ARG PYTHON_VERSION=3.9
ARG VENV_PATH=/prod_venv

FROM registry.access.redhat.com/ubi8/ubi-minimal:latest as builder

# Install Python and dependencies
RUN microdnf install -y --disablerepo=* --enablerepo=ubi-8-baseos-rpms --enablerepo=ubi-8-appstream-rpms python39 python39-devel gcc libffi-devel openssl-devel krb5-workstation krb5-libs && microdnf clean all

# Install Poetry
ARG POETRY_HOME=/opt/poetry
ARG POETRY_VERSION=1.7.1

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
# Fixes Quay alert GHSA-2jv5-9r88-3w3p https://github.com/Kludex/python-multipart/security/advisories/GHSA-2jv5-9r88-3w3p
RUN pip install --no-cache-dir starlette==0.36.2

FROM registry.access.redhat.com/ubi8/ubi-minimal:latest as prod

COPY third_party third_party

# Activate virtual env
ARG VENV_PATH
ENV VIRTUAL_ENV=${VENV_PATH}
ENV PATH="$VIRTUAL_ENV/bin:$PATH"

RUN microdnf install -y --disablerepo=* --enablerepo=ubi-8-baseos-rpms --enablerepo=ubi-8-appstream-rpms shadow-utils python39 python39-devel && \
    microdnf clean all
RUN useradd kserve -m -u 1000 -d /home/kserve

COPY --from=builder --chown=kserve:kserve $VIRTUAL_ENV $VIRTUAL_ENV
COPY --from=builder kserve kserve
COPY ./storage-initializer /storage-initializer

RUN chmod +x /storage-initializer/scripts/initializer-entrypoint
RUN mkdir /work
WORKDIR /work

USER 1000
ENTRYPOINT ["/storage-initializer/scripts/initializer-entrypoint"]
