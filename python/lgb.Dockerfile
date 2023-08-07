FROM build-base-image as builder

COPY lgbserver/pyproject.toml lgbserver/poetry.lock lgbserver/
RUN cd lgbserver && poetry install --no-root --no-interaction --no-cache
COPY lgbserver lgbserver
RUN cd lgbserver && poetry install --no-interaction --no-cache


FROM prod-base-image as prod

RUN apt-get update && apt-get install -y --no-install-recommends \
    libgomp1 && \
    apt-get clean && \
    rm -rf /var/lib/apt/lists/* /tmp/* /var/tmp/* \

COPY --from=builder --chown=kserve:kserve $VIRTUAL_ENV $VIRTUAL_ENV
COPY --from=builder kserve kserve
COPY --from=builder lgbserver lgbserver

ENTRYPOINT ["python", "-m", "lgbserver"]
