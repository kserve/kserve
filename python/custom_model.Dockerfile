FROM build-base-image as builder

COPY custom_model/pyproject.toml custom_model/poetry.lock custom_model/
RUN cd custom_model && poetry install --no-root --no-interaction --no-cache
COPY custom_model custom_model
RUN cd custom_model && poetry install --no-interaction --no-cache


FROM prod-base-image as prod

COPY --from=builder --chown=kserve:kserve $VIRTUAL_ENV $VIRTUAL_ENV
COPY --from=builder kserve kserve
COPY --from=builder custom_model custom_model

ENTRYPOINT ["python", "-m", "custom_model.model"]
