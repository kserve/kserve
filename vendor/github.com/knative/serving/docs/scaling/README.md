# Knative Serving Scaling

TODO: write developer/operator facing documentation.

## Scale Bounds

There are cases when Operators need to set lower and upper bounds on the number
of pods serving their apps (e.g. avoiding cold-start, control compute costs,
etc).

The following annotations can be used on `configuration.revisionTemplate` or
`revision` (propagated to `PodAutoscaler` objects) to do exactly that:

```yaml
# +optional
# When not specified, the revision can scale down to 0 pods
autoscaling.knative.dev/minScale: "2"
# +optional
# When not specified, there's no upper scale bound
autoscaling.knative.dev/maxScale: "10"
```

You can also use these annotations directly on `PodAutoscaler` objects.

**NOTE**: These annotations apply for the full lifetime of a `revision`. Even
when a `revision` is not referenced by any `route`, the minimal pod count
specified by `autoscaling.knative.dev/minScale` will still be provided. Keep in
mind that non-routeable `revisions` may be garbage collected, which enables
Knative to reclaim the resources. **These annotations are specific to Autoscaler
implementations but NOT subject to Conformance.**
