FROM python:3.9-slim
RUN pip install --upgrade pip && pip install "feast[gcp]~=0.30.0" "feast[redis]~=0.30.0" "feast[aws]~=0.30.0"
ENV FEAST_USAGE=False
EXPOSE 6566
ENTRYPOINT ["feast"]
CMD ["--help"]
