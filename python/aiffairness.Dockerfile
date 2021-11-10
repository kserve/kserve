FROM python:3.7

COPY . .
RUN pip install --no-cache-dir --upgrade pip && pip install --no-cache-dir -e ./kserve
RUN pip install --no-cache-dir -e ./aiffairness
ENTRYPOINT ["python", "-m", "aifserver"]
