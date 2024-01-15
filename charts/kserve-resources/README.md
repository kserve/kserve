# kserve

## Using this chart

First install the `kserve-crd` chart

```console
helm install kserve-crd oci://ghcr.io/kserve/charts/kserve-crd --version v0.12.0-rc0
```

Then install this chart

```console
helm install kserve oci://ghcr.io/kserve/charts/kserve --version v0.12.0-rc0
```
