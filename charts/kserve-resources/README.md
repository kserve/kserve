# kserve

## Using this chart

First install the `kserve-crd` chart

```console
helm install kserve-crd oci://ghcr.io/kserve/charts/kserve-crd --version 0.11.2
```

Then install this chart

```console
helm install kserve oci://ghcr.io/kserve/charts/kserve --version 0.11.2
```
