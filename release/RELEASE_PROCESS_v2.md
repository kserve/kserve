# Release Process

We are moving to a release cadence of 3 months. We will try out this cadence and see if it's too aggressive or too modest and iterate accordingly.
These are the timelines proposed for the process

| Week | Event (Ideal timelines with 3 months of cadence)                                                        |
| ---- | ------------------------------------------------------------------------------------------------------- |
| 0    | Gap week between releases. 0.9.0 release happened on July 22, 2022                                      |
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



In 8th week,
- Create an issue of type feature in [kserve/kserve](https://github.com/kserve/kserve) to start tracking the release process
    - Copy paste the above timeline table in that issue and fill in the dates accordingly
- Label the issue with `priority p0`
- Label the issue with `kind process`
- Announce the feature freeze and rest of the dates in the #kserve channel


## Process
### On feature freeze day
We will be creating first release candidate (RC0) on feature freeze day that could be and should be consumed by the community in their pre-production environments to test for any
bugs that might have been introduced in recent development lifecycle.</br></br>
Create a branch and do the following:
1. Update the version number in following places:
    1. [VERSION](../python/VERSION) to `${MAJOR}.${MINOR}.${PATCH}-rc${RELEASE_CANDIDATE_VERSION}`
    2. [quick_install.sh](../hack/quick_install.sh#L35) to `v${MAJOR}.${MINOR}.${PATCH}-rc${RELEASE_CANDIDATE_VERSION}`
    3. [values.yaml](../charts/kserve/values.yaml#L2) to `v${MAJOR}.${MINOR}.${PATCH}-rc${RELEASE_CANDIDATE_VERSION}`
    4. [Chart.yaml](../charts/kserve/Chart.yaml#L3) to `v${MAJOR}.${MINOR}.${PATCH}-rc${RELEASE_CANDIDATE_VERSION}`
2. Generate install manifest `./hack/generate-install.sh $VERSION`.
3. Submit your PR and wait for it to merge.
4. After it is merged,
    1. Create a release branch of the form release-X.Y.Z from the master // TODO add git commands
    2. Create a release candidate tag X.Y.Z-rc0 from that branch and do git push for both the branch and tag // TODO do mention only KServe owners can do it
    3. from that tag create a release-candidate (basically a pre-release) on github
    4. With this you are done with the creation of RC0 for upcoming release
5. Announce in the community about the availability of release-candidate so that community can start consuming and testing. And ask them to report bugs as soon as possible.
6. After feature freeze date, now only bug fixes will be merged into the release branch.

### 1 week after feature freeze:
After feature freeze,we will be merging only bug fixes into the release branch and creating another release candidate (RC1).
This is only needed if any bugs have been fixed after feature freeze. Otherwise, it is not needed
Process for getting those bug fixes is as follows:
1. Create a PR that fixes only the reported bugs.
2. Contributors should ask OWNERs who approved the PR to add a `cherrypick-approved` so that release manager/coordinator can cherry-pick them while creating next release candidate
3. Once all the PRs with `cherrypick-approved` labels are approved and merged, release manager should take following steps to create next release candidate:
    1. Cherry-pick the merged commits from master to the release branch. // TODO Do we have any specific process for cherry-picking merged commits from the master into release branch ? Add git commands
    2. Make sure merged commits are cherry-picked in the order they were merged. Cherry-picking should not result in any sort of merge conflicts since no one is working on release branch.  
    3. 

