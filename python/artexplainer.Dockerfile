FROM build-base-image as builder

COPY artexplainer/pyproject.toml artexplainer/poetry.lock artexplainer/
RUN cd artexplainer && poetry install --no-root --no-interaction --no-cache
COPY artexplainer artexplainer
RUN cd artexplainer && poetry install --no-interaction --no-cache


FROM prod-base-image as prod

COPY --from=builder --chown=kserve:kserve $VIRTUAL_ENV $VIRTUAL_ENV
COPY --from=builder kserve kserve
COPY --from=builder artexplainer artexplainer

ENTRYPOINT ["python", "-m", "artserver"]
