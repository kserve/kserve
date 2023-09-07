FROM build-base-image as builder

COPY paddleserver/pyproject.toml paddleserver/poetry.lock paddleserver/
RUN cd paddleserver && poetry install --no-root --no-interaction --no-cache
COPY paddleserver paddleserver
RUN cd paddleserver && poetry install --no-interaction --no-cache


FROM prod-base-image as prod

USER root

RUN apt-get update && \
    apt-get install -y --no-install-recommends libgomp1 && \
    apt-get clean && \
    rm -rf /var/lib/apt/lists/*

USER 1000

COPY --from=builder --chown=kserve:kserve $VIRTUAL_ENV $VIRTUAL_ENV
COPY --from=builder kserve kserve
COPY --from=builder paddleserver paddleserver

ENTRYPOINT ["python", "-m", "paddleserver"]
