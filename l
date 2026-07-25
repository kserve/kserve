# values.yaml
kserve:
  storage:
    image: kserve/storage-initializer:latest
    tag: latest
    resources:
      requests:
        memory: 100Mi
        cpu: 100m
      limits:
        memory: 1Gi
        cpu: 1