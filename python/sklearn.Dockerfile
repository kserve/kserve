FROM build-base-image as builder

COPY sklearnserver/pyproject.toml sklearnserver/poetry.lock sklearnserver/
RUN cd sklearnserver && poetry install --no-root --no-interaction --no-cache
COPY sklearnserver sklearnserver
RUN cd sklearnserver && poetry install --no-interaction --no-cache


FROM prod-base-image as prod

COPY --from=builder --chown=kserve:kserve $VIRTUAL_ENV $VIRTUAL_ENV
COPY --from=builder kserve kserve
COPY --from=builder sklearnserver sklearnserver

ENTRYPOINT ["python", "-m", "sklearnserver"]
