# Assorted scripts for development

This directory contains several scripts useful in the development process of
Knative Serving.

- `boilerplate/add-boilerplate.sh` Adds license boilerplate to _txt_ or _go_
  files in a directory, recursively.
- `deploy.sh` Deploys Knative Serving to an [environment](environments.md).
- `dev-patch-config-gke.sh` Patches the network configuration of a dev
  deployment in GKE.
- `diagnose-me.sh` Performs several diagnostic checks on the running Kubernetes
  cluster, for debugging.
- `generate-yamls.sh` Builds all the YAMLs that Knative Serving publishes.
- `release.sh` Creates a new [release](release.md) of Knative Serving.
- `update-codegen.sh` Updates auto-generated client libraries.
- `update-deps.sh` Updates Go dependencies.
- `verify-codegen.sh` Verifies that auto-generated client libraries are
  up-to-date.
