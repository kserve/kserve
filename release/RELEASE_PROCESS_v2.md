# Release Process

We are moving to a release cadence of 3 months. We will try out this cadence and see if it's too aggressive or too modest and iterate accordingly.
These are the timelines proposed for the process

| Week | Event (Ideal timelines with 3 months of cadence)                                                        |
| ---- | ------------------------------------------------------------------------------------------------------- |
| 1    | Development                                                                                             |
| 2    | Development                                                                                             |
| 3    | Development                                                                                             |
| 4    | Development                                                                                             |
| 5    | Development                                                                                             |
| 6    | Development                                                                                             |
| 7    | Development                                                                                             |
| 8    | Development                                                                                             |
| 9    | Development                                                                                             |
| 10   | Development                                                                                             |
| 11   | Development +<br>Start the prep i.e.<br>Announce/Reminder about upcoming feature freeze date in 2 weeks |
| 12   | Development (Last week of development)                                                                  |
| 13   | Feature Freeze + RC0 Released + Documentation update starts                                             |
| 14   | Testing                                                                                                 |
| 15   | RC1 Released if necessary                                                                               |
| 16   | Testing                                                                                                 |
| 17   | RC2 Released if necessary                                                                               |
| 18   | Testing + End of documentation                                                                          |
| 19   | Final release                                                                                           |




In 11th week,
- Create an issue of type feature in [kserve/kserve](https://github.com/kserve/kserve) to start tracking the release process
    - Copy paste the above timeline table in that issue and fill in the dates accordingly
- Announce the feature freeze and rest of the dates in the #kserve channel


## Process
### On feature freeze day
We will be creating the first release candidate (RC0) on the feature freeze day that could be and should be consumed by the community in their pre-production environments to test for any
bugs that might have been introduced in recent development cycles.</br></br>
Create a branch from the master and do the following:
1. Update the version number in following places:
    1. [VERSION](../python/VERSION) to `${MAJOR}.${MINOR}.${PATCH}rc${RELEASE_CANDIDATE_VERSION}` (note that the dash before "rc" is removed intentionally to meet [PyPI package version requirements](https://packaging.python.org/en/latest/specifications/version-specifiers/#public-version-identifiers))
    2. [quick_install.sh](../hack/quick_install.sh#L36) to `v${MAJOR}.${MINOR}.${PATCH}-rc${RELEASE_CANDIDATE_VERSION}`
    3. [Chart.yaml in kserve-crd](../charts/kserve-crd/Chart.yaml#L3) to `v${MAJOR}.${MINOR}.${PATCH}-rc${RELEASE_CANDIDATE_VERSION}`
    4. [Chart.yaml in kserve-crd-minimal](../charts/kserve-crd-minimal/Chart.yaml#L3) to `v${MAJOR}.${MINOR}.${PATCH}-rc${RELEASE_CANDIDATE_VERSION}`
    5. [Chart.yaml in kserve-resources](../charts/kserve-resources/Chart.yaml#L3) to `v${MAJOR}.${MINOR}.${PATCH}-rc${RELEASE_CANDIDATE_VERSION}`
    6. [values.yaml in kserve-resources](../charts/kserve-resources/values.yaml#L2) to `v${MAJOR}.${MINOR}.${PATCH}-rc${RELEASE_CANDIDATE_VERSION}`.
    8. The steps are automated in the script [prepare-for-release.sh](../hack/prepare-for-release.sh)
       9. To use it execute: `make bump-version NEW_VERSION=0.14.0-rc2 PRIOR_VERSION=0.14.0-rc1`
       10. Note the `-` in the version, keep it, the version will be updated accordingly and the dash removed when needed.
2. Add a new version `v${MAJOR}.${MINOR}.${PATCH}-rc${RELEASE_CANDIDATE_VERSION}` in the `RELEASES` array in [generate-install.sh](../hack/generate-install.sh). Example: Refer [this commit](https://github.com/rachitchauhan43/kserve/commit/6e9bd24ea137a3619da3297b4ff000379f7b2b38#diff-5f8f3e3a8ca601067664c7bf00c05aa2290a6ba625312754856ec873b840b6dbR42)
3. Generate install manifest `./hack/generate-install.sh $VERSION`.
4. Run `make poetry-lock` to update pyproject.toml files for all packages.
5. Run `make precommit` from the Kserve root directory. Address any lint errors if found.
6. Submit your PR and wait for it to merge.
7. After it is merged,
    1. Create a release branch of the form release-X.Y.Z from the master 
    2. Create a release candidate tag X.Y.Z-rc0 from that branch and do git push for both the branch and tag
    3. Now goto GITHUB and from the recently pushed X.Y.Z-rc0 tag, create a release-candidate (basically a pre-release) on GITHUB
    4. With this you are done with the creation of RC0 for upcoming release
8. Announce in the community about the availability of release-candidate so that community can start consuming and testing. And ask them to report bugs as soon as possible.
9. After feature freeze date, now only bug fixes will be merged into the release branch.

### 1 week after feature freeze:
After feature freeze, we will be merging only the bug fixes into the release branch and creating another release candidate (RC1) out of it.
This is only needed if any bugs have been fixed after feature freeze. Otherwise, it is not needed
Steps:
1. Create a PR following the `Step 1-3` from the above section and bump up the version to `rc1` in all the places and label it with `cherrypick-approved`
2. Get this PR reviewed and merged to the master
3. Now, cherry-pick the `merge commits` that have come out of PRs labeled with `cherrypick-approved` into the release branch (including the just created PR in step 1 in this section)
**Note:** Make sure merged commits are cherry-picked in the order they were merged. Cherry-picking should not result in any sort of merge conflicts since no one is working on release branch.
4. After all the commits have been cherry-picked into the release branch,
   1. Create a release candidate tag X.Y.Z-rc1 from that branch and do git push for both the branch and the tag
   2. Now goto GITHUB and from the recently pushed X.Y.Z-rc1 tag, create a release-candidate (basically a pre-release) on GITHUB
   3. With this you are done with the creation of RC1 for upcoming release
5. Announce in the community about the availability of release-candidate so that community can start consuming and testing. And ask them to report bugs as soon as possible.

You can repeat same steps for RC2 or other release candidates if needed

#### Instructions to Automatic Cherry-Pick:
We can use the GitHub action to automatically cherry-pick PRs. use the following comment

 `/cherry-pick release-branch`


### On the release day:

#### Updating the version in master 
This will be the last commit before the release and last one to be cherry-picked into release branch. So, we have to update the release version in master to reflect the latest release we are at.  
1. Create a PR with the following changes to update the version number in the following places in the master:
   1. [VERSION](../python/VERSION) to `${MAJOR}.${MINOR}.${PATCH}`
   2. [quick_install.sh](../hack/quick_install.sh#L36) to `v${MAJOR}.${MINOR}.${PATCH}`
   3. [Chart.yaml in kserve-crd](../charts/kserve-crd/Chart.yaml#L3) to `v${MAJOR}.${MINOR}.${PATCH}`
   4. [Chart.yaml in kserve-crd-minimal](../charts/kserve-crd-minimal/Chart.yaml#L3) to `v${MAJOR}.${MINOR}.${PATCH}`
   5. [Chart.yaml in kserve-resources](../charts/kserve-resources/Chart.yaml#L3) to `v${MAJOR}.${MINOR}.${PATCH}`
   6. [values.yaml in kserve-resources](../charts/kserve-resources/values.yaml#L2) to `v${MAJOR}.${MINOR}.${PATCH}`
2. Add a new version `v${MAJOR}.${MINOR}.${PATCH}` in the `RELEASES` array in [generate-install.sh](../hack/generate-install.sh). Example: Refer [this commit](https://github.com/rachitchauhan43/kserve/commit/6e9bd24ea137a3619da3297b4ff000379f7b2b38#diff-5f8f3e3a8ca601067664c7bf00c05aa2290a6ba625312754856ec873b840b6dbR42)
3. Generate install manifest `./hack/generate-install.sh $VERSION`.
4. Run `make poetry-lock` to update pyproject.toml files for all packages.
5. Run `make precommit` from the Kserve root directory. Address any lint errors if found.
6. Submit your PR and wait for it to merge.
7. Once merged, cherry-pick the `merge commits` that have come out of PRs labeled with `cherrypick-approved` to the release branch (including the just created PR in step 1 in this section)
8. Create a release tag X.Y.Z from the release branch and do git push for both the branch and tag
9. From that tag create the final release on GITHUB
10. With this you are done with the creation of final release
11. Announce in the community about the availability of release so that community can start consuming it



