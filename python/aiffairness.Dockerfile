FROM python:3.7

COPY kserve kserve
COPY aiffairness aiffairness
COPY third_party third_party
 
RUN pip install --no-cache-dir --upgrade pip && pip install --no-cache-dir -e ./kserve
RUN pip install --no-cache-dir -e ./aiffairness

RUN useradd kserve -m -u 1000 -d /home/kserve
USER 1000
ENTRYPOINT ["python", "-m", "aifserver"]
