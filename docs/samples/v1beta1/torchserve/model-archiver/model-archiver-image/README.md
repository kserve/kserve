# Model archiver for torchserve

**Steps:**

 1. Modify config in entrypoint for default config (optional)
 2. Build docker image
 3. Push docker image to repo

```bash
docker build --file Dockerfile -t margen:latest .

docker tag margen:latest {username}/margen:latest

docker push {username}/margen:latest
```
