### Build bgtest image
```
1. cd bgtest;go mod vendor; cd -
2. docker build -t {$your-dockerhub-name}/bgtest:latest .
3. docker push {$your-dockerhub-name}/bgtest:latest
```