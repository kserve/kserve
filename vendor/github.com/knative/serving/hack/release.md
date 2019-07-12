# Creating a new Knative Serving release

The `release.sh` script automates the creation of Knative Serving releases,
either nightly or versioned ones.

By default, the script creates a nightly release but does not publish it
anywhere.

## Common flags for cutting releases

The following flags affect the behavior of the script, no matter the type of the
release.

- `--skip-tests` Do not run tests before building the release. Otherwise, build,
  unit and end-to-end tests are run and they all must pass for the release to be
  built.
- `--tag-release`, `--notag-release` Tag (or not) the generated images with
  either `vYYYYMMDD-<commit_short_hash>` (for nightly releases) or `vX.Y.Z` for
  versioned releases. _For versioned releases, a tag is always added._
- `--release-gcs` Defines the GCS bucket where the manifests will be stored. By
  default, this is `knative-nightly/serving`. This flag is ignored if the
  release is not being published.
- `--release-gcr` Defines the GCR where the images will be stored. By default,
  this is `gcr.io/knative-nightly`. This flag is ignored if the release is not
  being published.
- `--publish`, `--nopublish` Whether the generated images should be published to
  a GCR, and the generated manifests written to a GCS bucket or not. If yes, the
  `--release-gcs` and `--release-gcr` flags can be used to change the default
  values of the GCR/GCS used. If no, the images will be pushed to the `ko.local`
  registry, and the manifests written to the local disk only (in the repository
  root directory).

## Creating nightly releases

Nightly releases are built against the current git tree. The behavior of the
script is defined by the common flags. You must have write access to the GCR and
GCS bucket the release will be pushed to, unless `--nopublish` is used.

Examples:

```bash
# Create and publish a nightly, tagged release.
./hack/release.sh --publish --tag-release

# Create release, but don't test, publish or tag it.
./hack/release.sh --skip-tests --nopublish --notag-release
```

## Creating versioned releases

To specify a versioned release to be cut, you must use the `--version` flag.
Versioned releases are usually built against a branch in the Knative Serving
repository, specified by the `--branch` flag.

- `--version` Defines the version of the release, and must be in the form
  `X.Y.Z`, where X, Y and Z are numbers.
- `--branch` Defines the branch in Knative Serving repository from which the
  release will be built. If not passed, the `master` branch at HEAD will be
  used. This branch must be created before the script is executed, and must be
  in the form `release-X.Y`, where X and Y must match the numbers used in the
  version passed in the `--version` flag. This flag has no effect unless
  `--version` is also passed.
- `--release-notes` Points to a markdown file containing a description of the
  release. This is optional but highly recommended. It has no effect unless
  `--version` is also passed.
- `--github-token` Points to a text file containing the GitHub token to be used
  for authentication when publishing the release to GitHub. If this flag is not
  used and this is the first time you're publishing a versioned release, you'll
  be prompted for your GitHub username, password, and possibly 2-factor
  authentication challenge (you must be a Knative admin to have the required
  publishing permissions).

The release will be published in the _Releases_ page of the Knative Serving
repository, with the title _Knative Serving release vX.Y.Z_ and the given
release notes. It will also be tagged _vX.Y.Z_ (both on GitHub and as a git
annotated tag).

Example:

```bash
# Create and publish a versioned release.
./hack/release.sh --publish --tag-release \
  --release-gcr gcr.io/knative-releases \
  --release-gcs knative-releases/serving \
  --version 0.3.0 \
  --branch release-0.3 \
  --release-notes $HOME/docs/release-notes-0.3.md
```

## Creating incremental build releases ("dot releases")

An incremental build release (aka "dot release") is a versioned release built
automatically based on changes in the latest release branch, with the build
number increased.

For example, if the latest release on release branch `release-0.2` is `v0.2.1`,
creating an incremental build release will result in `v0.2.2`.

To specify an incremental build release to be cut, you must use the
`--dot-release` flag. The latest branch and release version will be
automatically detected and used.

_Note 1: when using the `--dot-release` flag, the flags `--nopublish` and
`--notag-release` have no effect. The release is always tagged and published._

_Note 2: if the release branch has no new commits since its last release was
cut, the script successfully exits with a warning, and no release will be
created._

The following flags are useful when creating incremental build releases:

- `--branch` Restricts the incremental build release to the given branch. If not
  passed, the latest branch will be automatically detected and used.
- `--release-notes` Points to a markdown file containing a description of the
  release. If not passed, the notes will be copied from the previous release.
- `--github-token` Points to a text file containing the GitHub token to be used
  for authentication when publishing the release to GitHub. If this flag is not
  used and this is the first time you're publishing a versioned release, you'll
  be prompted for your GitHub username, password, and possibly 2-factor
  authentication challenge (you must be a Knative admin to have the required
  publishing permissions).

Like any regular versioned release, an incremental build release is published in
the _Releases_ page of the Knative Serving repository.

Example:

```bash
# Create and publish a new dot release.
./hack/release.sh \
  --dot-release \
  --release-gcr gcr.io/knative-releases \
  --release-gcs knative-releases/serving
```
