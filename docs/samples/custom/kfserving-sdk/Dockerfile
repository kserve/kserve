# Use the official lightweight Python image.
# https://hub.docker.com/_/python
FROM python:3.7-slim

# Copy local code to the container image.
ENV APP_HOME /app
WORKDIR $APP_HOME
COPY model.py requirements.txt imagenet_classes.txt ./

# Install production dependencies.
RUN pip install --no-cache-dir -r ./requirements.txt

CMD ["python", "model.py"]