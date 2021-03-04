FROM python:3.7

COPY . .
RUN pip install --upgrade pip && pip install -e ./kfserving
RUN pip install -e ./aiffairness
ENTRYPOINT ["python", "-m", "aifserver"]
