This repository is a fork of [kserve/kserve](https://github.com/kserve/kserve)
repository to integrate with Open Data Hub platform. The releases on ODH
platform have a faster cadence than the upstream repository. Still, an upstream
stable release is used as a base for any releases of the ODH Project. We
accomplish this by creating a branch in ODH fork based from an upstream release
tag. Any integration changes to ODH Platform are added on top of the created
branch.

This guide outlines the process to fetch a stable upstream release to ODH fork.

# Prerequisites

As you will be creating a _new_ branch, the configured branch protections
may not apply to it. Despite this, you must still adhere to [GitHub
flow](https://docs.github.com/en/get-started/using-github/github-flow).

You will need:

* Push rights to
  [opendatahub-io/kserve](https://github.com/opendatahub-io/kserve/) repository.
  * This is to allow you doing the initial push of the release branch.
  * If you don't have push rights, you can ask somebody else to do the initial
    push.
* Your own fork of
  [opendatahub-io/kserve](https://github.com/opendatahub-io/kserve/).
  * This is to stay adhered to [GitHub
    flow](https://docs.github.com/en/get-started/using-github/github-flow).
  * You can create a fork on your account using this link:
    https://github.com/opendatahub-io/kserve/fork.
  * If you have push rights to
    [opendatahub-io/kserve](https://github.com/opendatahub-io/kserve/) the fork
    may be perceived as optional. Still, we prefer to keep the repository clean
    from working branches and open pull requests from contributor forks.
* Your own fork of
  [openshift/release](https://github.com/openshift/release) repository.
  * This is to configure CI for the new release branch.
  * You can create a fork on your account using this link:
    https://github.com/openshift/release/fork.
* CLI tools:
  * `jq` CLI as a helper tool [\[homepage\]](https://jqlang.org/).
  * `podman` because some operations are ran inside containers
  * `make` to run Makefiles
  * Of course: `git` command.
  * Other CLI tools: `curl`, `grep`, `sed`, `xdg-open`.

# Branch cut procedure

## Preparation of the working copy

This guide assumes the setup mentioned in
[CONTRIBUTING.md](../../CONTRIBUTING.md#working-copy-preparation-for-odh-development).

## Performing the branch cut

Fetch the tags of the community repository:

```sh
$ git fetch kserve --tags
```

Double check that the tag in your working copy is up to date with the tag in the
upstream repository

```sh
VERSION_TO_PULL="v0.15.0" # Replace with the kserve/kserve tag you want to push to ODH
LOCAL_SHA=$(git log -1 $VERSION_TO_PULL --pretty=format:"%H")
REMOTE_SHA=$(curl -s -H "Accept: application/vnd.github+json" https://api.github.com/repos/kserve/kserve/commits/$VERSION_TO_PULL | jq -r '.sha')
[[ "$LOCAL_SHA" == "$REMOTE_SHA" ]] && echo "OK! Commits match." || echo "Commits does not match."
```

From the previous script, ensure you get the "OK! Commits match." message.
Otherwise, stop and review your setup.

Fetch `master` branch from
[opendatahub-io/kserve](https://github.com/opendatahub-io/kserve/) repository:

```sh
$ git fetch odh master
```

Validate that you can proceed with the branch cut:

```sh
$ (git branch -a --contains tags/${VERSION_TO_PULL} | grep 'remotes/odh/master') && echo "Tag is available to fetch" || echo "Code sync is required"
```

If the output of the previous command is `Code sync is required`, a code sync is
must be done before the branch cut can be done. If the expected output `Tag is
available to fetch` is printed, the next step is to resolve what commit can be
used for the cut. 

Let's assume the following simplified graph of the `master` branches of ODH and
upstream may look like in the following graph:

    odh/master                *---*---*---A---*---*---*---B---*---*---*---*---C  (most recent commit)
                             /           /               /           /
    kserve/master   *---*---*---*---*---α---*---β---*---γ---*---*---*---*---δ  (most recent commit)
                                                |
                                            tag/release

The letters and the `*` character would represent commits on each fork's
`master` branches. The slash `/` characters would represent merges (i.e. code
syncs) to ODH fork that integrate code from upstream. Notice that the upstream
release `β` happened between the code sync `A` and `B`. It must be noted that
despite commit `β` can be reached from any commit after `B` (inclusive) on
odh/master, the ODH commits/customizations cannot be reached from the commit `β`
(i.e. the upstream release tag). This is because merges are one way: upstream ->
ODH. Should we generate a branch from commit `β`, we would not see any ODH
customizations. 

Given the previous graph, we would not want to cut directly from `odh/master`
(i.e. commit `C`), because ODH release branch must be based on a stable upstream
release. When cutting from latest commit `C`, although the additional ODH
commits between `B` and `C` may be worth to keep, notice there is an additional
code sync which would be unwanted (the situation may vary, this is for
illustrative purposes only).

The most appropriate cut points are `A` and `B`. In any case, there is
additional work to do after the cut:
* If doing the cut on `A`:
  * It is required to do a `git merge $release_tag` to replay upstream commits
    `α..β`.
  * It needs to be evaluated if ODH commits `A..C` should be replayed on the new
    release branch.
* If doing the cut on `B`:
  * Upstream commits `β..γ` must be reverted (excluding reverting `β`), because
    they are not part of the stable upstream release.
  * It needs to be evaluated if ODH commits `B..C` should be replayed on the new
    release branch.

To find the candidate cut points `A` and `B`, you can use the following script:

```sh
# Find commit that integrated the upstream release tag.
BASE_COMMIT_B=$(git rev-list ${VERSION_TO_PULL}..odh/master --ancestry-path | grep -f <(git rev-list ${VERSION_TO_PULL}..odh/master --first-parent) | tail -1)

# Find the earlier sync
BASE_COMMIT_A=$(git rev-list ${VERSION_TO_PULL}...${BASE_COMMIT_B}^ --ancestry-path | grep -f <(git rev-list ${VERSION_TO_PULL}..${BASE_COMMIT_B}^ --first-parent) | tail -1)

# Print results
echo "Base commit A: $BASE_COMMIT_A"
echo "Base commit B: $BASE_COMMIT_B"
```

Now you need to choose if you want to either use `A` or `B` for the branch cut.
A reasonable recommendation would be to pick the one that would lead to the
least work. Based on what is previously stated, to have an idea of the post-cut
work:

* If you pick `A`:
  * Check how much is remaining to merge to replay the stable release (this is
    `α..β`):
    * For a short summary of changes `git diff
      ${BASE_COMMIT_A}..${VERSION_TO_PULL} --shortstat`
    * List of commits to merge with stats: `git log
      ${BASE_COMMIT_A}..${VERSION_TO_PULL} --format=oneline --abbrev-commit
      --shortstat`
  * Check the potential ODH cherry-picks (this is `A..C`):
    * List of commits to evaluate for cherry-pick, with stats: `git log
      ${BASE_COMMIT_A}..odh/master --format=oneline --abbrev-commit
      --first-parent --shortstat`
* If you pick `B`:
  * Check how much needs to be reverted (this is `β..γ`):
    * List of commits to revert with stats: `git log
      ${VERSION_TO_PULL}..${BASE_COMMIT_B}^2 --format=oneline --abbrev-commit
      --shortstat --ancestry-path`.
      * NOTE: You need to evaluate if there are commits that should _not_ be
        reverted.
  * Check the potential ODH cherry-picks (this is `B..C`):
    * List of commits to evaluate for cherry-pick, with stats: `git log
      ${BASE_COMMIT_B}..odh/master --format=oneline --abbrev-commit
      --first-parent --shortstat`

Armed with the previous data, pick the commit for the cut, do the cut, and push
the new branch to odh/kserve repository:

```sh
# Pick one of the following two lines:
BASE_COMMIT=$BASE_COMMIT_A
BASE_COMMIT=$BASE_COMMIT_B

BRANCH_NAME="release-$(echo $VERSION_TO_PULL | sed 's/\.[[:digit:]]\+$//')"
echo "Branch name: $BRANCH_NAME"
git branch $BRANCH_NAME $BASE_COMMIT
git push odh $BRANCH_NAME
```

# Configuring openshift-ci

Now that the cut is done, and before doing any additional work on the new
branch, you should configure openshift-ci. This would help ensuring that any
post-cut work is verified.

Assuming `odh/master` branch is recent to the upstream release, it can be safe
to assume that its CI configurations work with the newly created release branch.
Thus, simply copy the CI configurations:

```sh
# Clone your fork of openshift-ci repository, and configure the upstream remote:
git clone git@github.com:{your-github-user}/openshift-release.git
cd openshift-release
git remote add upstream https://github.com/openshift/release.git
```

Make sure you have the latest code. Then, create the CI configurations of the
new release branch based on the ones for the `master` branch, commit and push
the changes to your fork, and create a pull request:

```sh
git fetch upstream master
git checkout -b ci-$BRANCH_NAME upstream/master
cp ci-operator/config/opendatahub-io/kserve/opendatahub-io-kserve-{master,$BRANCH_NAME}.yaml
sed -i "s/tag: latest/tag: $BRANCH_NAME/" ci-operator/config/opendatahub-io/kserve/opendatahub-io-kserve-$BRANCH_NAME.yaml

make ci-operator-config jobs
git add ci-operator/config/opendatahub-io/kserve/opendatahub-io-kserve-$BRANCH_NAME.yaml
git add ci-operator/jobs/opendatahub-io/kserve
git commit -m "Create CI configs for $BRANCH_NAME"
git push -u origin ci-$BRANCH_NAME

GH_USER={your-github-user}
xdg-open https://github.com/${GH_USER}/openshift-release/pull/new/ci-${BRANCH_NAME}
```

In this pull request, it is recommended that you run rehearsals of the
configured tests. At the time of the last update of this guide, the available
reharsals are run by commenting with the following slash command in your pull
request:

```
/pj-rehearse pull-ci-opendatahub-io-kserve-release-v0.15-e2e-path-based-routing pull-ci-opendatahub-io-kserve-release-v0.15-e2e-predictor pull-ci-opendatahub-io-kserve-release-v0.15-e2e-graph
```

The `openshift-ci-robot` would comment on your pull request with the current
list of tests. Ensure you run the current and relevant ones.

Ideally, the rehearsals of the repository tests should all succeed. If any does
not succeed, either:
* Check the history of the CI configuration for the `master` branch of the
  `openshift/release` repository for recent changes. It can be possible that a
  recent update is not applicable for the branch cut and the older configuration
  may work.
* The post-cut work mentioned in the following sections may bring CI of the new
  release branch to a succeeding state.

Thus, should rehearsals not succeed, evaluate if the CI configs can be merged
despite the failures. You will need to fix CI issues as part of post-cut work.
Otherwise, check if an older CI configuration works.

# Post-cut adaptation of the new release branch

As mentioned above, there is some additional work that needs to be done,
depending on the chosen commit for the branch cut. Please, follow only one of
the following two sub-sections.

## Post-cut work: cut on commit `A` (before integration of upstream release tag)

As mentioned above, and using the above sample graph, you need to replay
upstream commits `α..β`. Assuming you've been following the guide on the same
terminal and you are carrying the environment state, you replay upstream commits
`α..β` with a simple merge command:

```sh
git checkout -b ${BRANCH_NAME}-upstream-sync ${BRANCH_NAME}
git merge ${VERSION_TO_PULL}
```

After fixing any conflicts, push your changes to your own fork and create a pull
request:

```sh
git push -u origin ${BRANCH_NAME}-upstream-sync
xdg-open https://github.com/${GH_USER}/kserve/pull/new/${BRANCH_NAME}-upstream-sync
```

As normally, let CI to run and ask for reviews and approvals from repository
maintainers. Once the pull request is merged, and although there is still some
work to do, the release branch may now be promoted as the new ODH stable one and
is suitable for development, ODH builds and ODH releases.

The remaining work, as mentioned above, is to evaluate if any commits in ODH
fork should be cherry-picked to the new release branch. You can fetch the list
of commits with the following command:

```sh
git log ${BASE_COMMIT}..odh/master --format="%Credpick %Cgreen%h %Cblue[%an]%Creset %s" --first-parent --reverse
```

Sample output:

    aef1b4c5b [Vedant Mahabaleshwarkar] Add devtools and e2e teardown script  (#567)
    2544e3c3e [Vedant Mahabaleshwarkar] fix indent bug in e2e raw setup (#580)
    e909a9659 [Hannah DeFazio] Add optional go1.22 image and profile to run with it (#582)
    95620ce0e [Filippe Spolti] 250424 sync upstream (#574)
    64769a97d [Filippe Spolti] Revert "250424 sync upstream (#574)" (#583)
    93ef1a322 [openshift-merge-bot[bot]] Merge pull request #585 from spolti/250424_sync_upstream
    3356facb5 [openshift-merge-bot[bot]] Merge pull request #593 from spolti/250508_sync_master
    64db70210 [Andres Llausas] Merge pull request #573 from andresllh/fix-e2e-graph-tests-script
    b682f42d2 [openshift-merge-bot[bot]] Merge pull request #600 from spolti/250509_sync_master


Share the output with repository maintainers to discuss which commits should be
backported to the new release branch. Once it is decided which commits should be
backported, it is recommended to do the cherry-pick from top-to-bottom of the
list to minimize conflicts. The backport process is manual using `git
cherry-pick`.

When you finish backporting, proceed to create a pull request and follow the
usual review/approve workflow. Once your pull request is merged, the new release
branch is fully ready.

## Post-cut work: cut on commit `B` (after integration of upstream release tag)

As mentioned above, and using the above sample graph, you need to evaluate if
any commits in the range `β..γ` should be reverted. Assuming you've been
following the guide on the same terminal and you are carrying the environment
state, you can get the list of potential commits to revert with the following
command:

```sh
git log ${VERSION_TO_PULL}..${BASE_COMMIT}^2 --format="%Cgreen%h %Cblue[%an]%Creset %s" --ancestry-path
```

Sample output:

  3deee8d7a [Edgar Hernández] Re-sync rawkube_controller_test.go file
  fb678727d [Spolti] keep go1.23 for ODH
  7cafdf2a6 [Spolti] fix tests
  c9a506a94 [Spolti] pre-commit additions
  5f8024348 [Spolti] fix test and update auto generated files
  3c5ae8ddf [Spolti] recreate poetry.lock
  f90d3d400 [Spolti] Merge KServe into ODH
  b48c6ec85 [Sivanantham] Bump Go version to 1.24 (#4321)
  b4ddbd8c3 [Filippe Spolti] fix typo on inferenceservice-config (#4244)

Share the output with repository maintainers to discuss which commits should be
reverted from the new release branch. Once it is decided, it is recommended to
revert from top-to-bottom of the list to minimize conflicts. The process is
manual using `git revert`.

When you finish reverting, proceed to create a pull request and follow the usual
review/approve workflow. Once the pull request is merged, and although there is
still some work to do, the release branch may now be promoted as the new ODH
stable one and is suitable for development, ODH builds and ODH releases.

As mentioned above in previous sections, the remaining work is to evaluate if
any commits in ODH fork should be cherry-picked to the new release branch. You
can fetch the list of commits with the following command:

```sh
git log ${BASE_COMMIT}..odh/master --format="%Credpick %Cgreen%h %Cblue[%an]%Creset %s" --first-parent --reverse
```

Sample output:

    3356facb5 [openshift-merge-bot[bot]] Merge pull request #593 from spolti/250508_sync_master
    64db70210 [Andres Llausas] Merge pull request #573 from andresllh/fix-e2e-graph-tests-script
    b682f42d2 [openshift-merge-bot[bot]] Merge pull request #600 from spolti/250509_sync_master


Share the output with repository maintainers to discuss which commits should be
backported to the new release branch. Once it is decided which commits should be
backported, it is recommended to do the cherry-pick from top-to-bottom of the
list to minimize conflicts. The backport process is manual using `git
cherry-pick`.

> [!NOTE]  
> You can get an empty output. In that case, there is nothing to backport.

When you finish backporting, proceed to create a pull request and follow the
usual review/approve workflow. Once your pull request is merged, the new release
branch is fully ready.

# Enable branch protections

Given you have enough privileges on [opendatahub-io/kserve](https://github.com/opendatahub-io/kserve) repository, use the following link to configure branch protection rules, in case no entry is applicable to the new branch: https://github.com/opendatahub-io/kserve/settings/branches. 
