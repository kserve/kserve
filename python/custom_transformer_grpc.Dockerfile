FROM build-base-image as builder

COPY custom_transformer/pyproject.toml custom_transformer/poetry.lock custom_transformer/
RUN cd custom_transformer && poetry install --no-root --no-interaction --no-cache
COPY custom_transformer custom_transformer
RUN cd custom_transformer && poetry install --no-interaction --no-cache


FROM prod-base-image as prod

COPY --from=builder --chown=kserve:kserve $VIRTUAL_ENV $VIRTUAL_ENV
COPY --from=builder kserve kserve
COPY --from=builder custom_transformer custom_transformer

ENTRYPOINT ["python", "-m", "custom_transformer.model_grpc"]


