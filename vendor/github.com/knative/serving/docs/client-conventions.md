# Client Conventions

This document describes conventions that Knative domain-specific clients can
follow to achieve specific end-user goals. It is intended as a set of best
practices for client implementers, and also as advice to direct users of the API
(for example, with kubectl).

These conventions are merely conventions:

- They are optional; you can use Knative entirely validly without them.
- They are designed to be useful even when some clients are not obeying the
  conventions. Each convention describes what happens in the presence of
  convention-unaware clients.

Some of the conventions involve the client setting labels or annotations; the
`client.knative.dev/*` label/annotation namespace is reserved for documented
Knative client conventions.

## Determine when an action is complete

As Knative is (like all of Kubernetes) a declarative API, the user expresses
their desire by changing some values in the Knative objects. Clients need not be
declarative, and might have expressions of user intent like "Deploy this code"
or "Change these environment variables". To tell when such an action is
complete, the client can look at the status conditions.

Each Knative object has a `Ready` status condition. When a change is initiated,
the controller flips this to `Unknown`. When the serving state again reflects
exactly what the spec of the object specifies, the `Ready` condition will flip
to `True`; this indicates the operation was a success. If reflecting the spec in
the serving state is impossible, the `Ready` condition will flip to `False`;
this indicates the operation was a failure, and the message of the status
condition should indicate something in English about why (and the Reason field
can indicate an enumeration suitable for i18n). Either `True` or `False`
indicates the operation is complete, for better or worse.

Note that someone else could start another operation while the client was
waiting for its operation. A conventional client still waits for the `Ready`
condition to land at `True` or `False`, and then describes to the user what
happened using logic based on the intended effect.

For example:

- Client A deploys image `gcr.io/foods/vegetables:eggplant`
- While that is not yet Ready, client B deploys `gcr.io/foods/vegetables:squash`
- The `eggplant` revision becomes Ready: True, and the service moves traffic to
  it. (NB: implementations may choose not to move traffic to any but the latest
  revision.)
- The `squash` revision fails to bind to a port, and becomes Ready: False
- The Service switches from Ready: Unknown to Ready: False because `squash`
  failed.

Both client A and B should wait for the last step in this procedure.

- Client A sees that `latestReadyRevisionName` is the revision with the
  [nonce](#associate-modifications-with-revisions) it specified, and that
  `latestCreatedRevisionName` is not. It tells the user that deploying was
  successful.
- Client B sees that `latestCreatedRevisionName` is the revision with the nonce
  it specified; it reports the failure with the appropriate message.

The rule is "Wait for `Ready` to become `True` or `False`, then report on
whether your intent was accomplished". The `Ready` success or failure can be
part of this report, but may be confusing (as in the example) if it's the only
thing you report.

## Associate modifications with Revisions

Every time the client changes a Service or Configuration in a way that results
in a new Revision, it should change the label `client.knative.dev/nonce` to a
new random value, chosen from a wide enough space to avoid collisions.

This way, the client can make a label selector query for Revisions with this
nonce value to find the Revision the particular change generated. The client can
use that revision to, for example, inform the user about the readiness of their
requested change.

### In the presence of non-conventional clients

If an unconventional client neglects to reset this value, the label selector
query could result in more than one revision; the client should pick the one
with the lowest `configurationGeneration` in this case.

## Force creation of a new Revision

The way to deploy new code with a previously-used tag is to make a new Revision,
which the Revision controller will re-pull and lock it to the current image at
that tag. Since Knative is a declarative API, it requires some change to the
desired state of the world (the spec) to trigger any change.

The same `client.knative.dev/nonce` label can help in forcing the creation of a
new Revision; if the nonce is changed, the Configuration controller must make a
new Revision even if nothing else has changed.

Example:

```yaml
apiVersion: serving.knative.dev/v1alpha1
kind: Configuration
metadata:
  name: my-service # Named the same as the Service
spec:
  revisionTemplate: # template for building Revision
    metadata:
      # In the spec of Configuration, but the metadata of the revision.
      labels:
        "client.knative.dev/nonce": dad00dab1de5
    spec:
      container:
        image: gcr.io/... # new image
```

## Change non-code attributes

When the user specifies they'd like to change an environment variable (or a
memory allocation, or a concurrency setting...), and does not specify that
they'd like to deploy a change in code, the user would be quite surprised to
find the newest image at their deployed tag running in the cloud.

### General idea

Since the Revision controller will resolve an image tag for every Revision
creation, we need a way to express a non-code change. Clients should do this by
changing the `image` field to be a digest-based image URL supplied by the
Revision `status.imageDigest` field, while marking the original tag-based user
intent in an annotation.

### Procedure

1.  Get the current state of the Service in question.
2.  Get a **base revision**, the Revision corresponding to the fetched state of
    the Service: Issue a label selector query for revisions with the
    `client.knative.dev/nonce` label listed in the `revisionTemplate.metadata`.
    If this yields exactly one Revision (after waiting a sensible timeout for a
    revision to appear with that nonce), that is the base revision. If not,
    fetch the `latestCreatedRevisionName` from the status, and uses that as the
    base revision.
3.  Copy the `status.imageDigest` field from the base revision into the `image`
    field of the Service. This ensures the running code stays the same.
4.  Make whatever other modifications to the Service.
5.  Add the `client.knative.dev/user-image` annotation to the Service,
    containing the original tag-based URL of the image.
6.  Add the nonce label, as usual.
7.  Post the resulting Service to create a new Revision.

### Changing code

When clients do want to change code, they can either require the user to specify
an image (which they put into the `image` field), or implement a "update the
code to whatever's at your previously-deployed tag" operation which copies the
`client.knative.dev/user-image` annotation back to `image`.

### Display images

Since we're now filling in the `image` field with a URL the user may never have
specified by hand, a client can display the image for human-readability as the
contents of the `client.knative.dev/user-image` annotation, combined with the
note that it is "at digest <digest>", fetched from the `imageDigest` of the
revision (or the `image` field itself of the Service).

For example, the displayed value for the image may be the same when:

- `container.image` is `gcr.io/veggies/eggplant:purple` and `status.imageDigest`
  of the relevant revision is `gcr.io/veggies/eggplant@sha256:45b23dee08af...`
- `container.image` is `gcr.io/veggies/eggplant@sha256:45b23dee08af...` and the
  `client.knative.dev/user-image` annotation is `gcr.io/veggies/eggplant:purple`

In both cases the client may tell the user the image is
"`gcr.io/veggies/eggplant:purple` at `sha256:45b23dee...`"

### In the presence of non-conventional clients

Non-convention-following clients can mess with this in the following ways:

- Not set a nonce (the label selector query returns 0 items), or fail to change
  the nonce (the label selector query returns >1 items)
  - In this case, we fall back to using the race-prone
    `latestCreatedRevisionName` field to determine the base revision. This will
    be almost-always correct, but may sometimes result in a situation where an
    unaware client changing code and a well-behaved client changing
    configuration race with each other, and the code change is not reflected in
    the revision that becomes live.
- Not set the user-image annotation.
  - Clients should display the contents of the `image` field if the `user-image`
    annotation is unspecified or implausible (an implausible value is one that
    does not share the same path prefix before the sha/tag).
- Attempt to deploy new code by changing something other than `image`. This will
  not work once a conventional client changes it to a digest. All clients should
  not assume that new code will be deployed unless they set the `image` field to
  their desired code _and_ change something about the `revisionTemplate`.

Furthermore, _before_ a user has used a well-behaved client to change an env var
or something, using an unaware client like kubectl to change an env var will
re-resolve the image if the user deployed an image by tag. (This would only be
avoidable if the server were to create new by-digest revisions for the user.)
After the user uses a well-behaved client, the image is by-digest anyway so
using kubectl won't mess anything up.
