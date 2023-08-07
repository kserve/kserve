FROM build-base-image as builder

COPY aiffairness/pyproject.toml aiffairness/poetry.lock aiffairness/
RUN cd aiffairness && poetry install --no-root --no-interaction --no-cache
COPY aiffairness aiffairness
RUN cd aiffairness && poetry install --no-interaction --no-cache


FROM prod-base-image as prod

COPY --from=builder --chown=kserve:kserve $VIRTUAL_ENV $VIRTUAL_ENV
COPY --from=builder kserve kserve
COPY --from=builder aiffairness aiffairness

ENTRYPOINT ["python", "-m", "aifserver"]
