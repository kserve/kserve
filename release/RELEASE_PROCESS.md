# Release Process
## Create an issue for release tracking

- Create an issue in [kserve/kserve](https://github.com/kserve/kserve)
- Label the issue with `priority p0`
- Label the issue with `kind process`
- Announce the release in the release channel

## Releasing KServe components
A release branch should be substantially _feature complete_ with respect to the intended release.
Code that is committed to `master` may be merged or cherry-picked on to a release branch, but code that is directly committed to the release branch should be solely applicable to that release (and should not be committed back to master).
In general, unless you're committing code that only applies to the release stream (for example, temporary hotfixes, backported security fixes, or image hashes), you should commit to `master` and then merge or cherry-pick to the release branch.

### List of Components

- [KServe](https://github.com/kserve/kserve)
- [ModelMesh](https://github.com/kserve/modelmesh-serving)
- [Website](https://github.com/kserve/website)

## Create a release branch
If you aren't already working on a release branch (of the form `release-${MAJOR}`, where `release-${MAJOR}` is a major-minor version number), then create one.
Release branches serve several purposes:

1.  They allow a release wrangler or other developers to focus on a release without interrupting development on `master`,
1.  they allow developers to track the development of a release before a release candidate is declared,
1.  they simplify back porting critical bug fixes to a patch level release for a particular release stream (e.g., producing a `v0.6.1` from `release-0.6`), when appropriate.

## Publish the release
It's generally a good idea to search the repo for control-f for strings of the old version number and replace them with the new, keeping in mind conflicts with other library version numbers.

1. Update configmap to point to $VERSION for all images in the release branch.
2. Update kserve and dependent python libraries to $VERSION in `setup.py`.
3. Generate install manifest `./hack/generate-install.sh $VERSION`.
4. Submit your PR and wait for it to merge.
5. Once everything has settled, tag and push the release with `git tag $VERSION` and `git push upstream $VERSION`.
6. KServe python sdk and images are published from github actions.
7. Upload kserve install manifests to github release artifacts.

## Cherry pick to release branch
After the release-X.Y release branch is cut, pull requests(PRs) merged to master will be only get merged in the next minor release X.(Y+1).0

If you want your PR released eariler in a patch release X.Y.(Z+1)
- The PR must be merged to master
- The PR should be a bug fix
- The PR should be cherry picked to corresponding release branch release-X.Y:
  Contributors should ask OWNERs who approved the PR to add a `cherrypick-approved` label if you want the PR cherry picked to release branch. Run `hack/cherry-pick.sh` script to cherry pick the
  PRs and it runs `git cherry-pick` for each merged commit and add `cherrypicked` label on the PR. Once PR is cherry picked push to remote branch to create a PR to release branch.

