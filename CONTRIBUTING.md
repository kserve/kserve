See [How to contribute](https://github.com/kserve/community#how-can-i-help-) in [KServe community repository](https://github.com/kserve/community).

## Working copy preparation for ODH development

To contribute, we recommend that you either [fork the
kserve/kserve](https://github.com/kserve/kserve/fork), or [fork the
opendatahub-io/kserve](https://github.com/opendatahub-io/kserve/fork)
repository. This is becase ODH contributors use [GitHub
flow](https://docs.github.com/en/get-started/using-github/github-flow).

The following steps outlines our recommended remotes setup. You will have three
remotes. When contributing, please be aware of using the right base branch,
depending on if you are contributing to kserve/kserve or opendatahub-io/kserve.

1. Clone your fork of the repository:
```sh
GH_USER={your-github-user}
$ git clone git@github.com:${GH_USER}/kserve.git 
```

2. Add [opendatahub-io/kserve](https://github.com/opendatahub-io/kserve/)
repository as a remote:
```sh
$ git remote add odh git@github.com:opendatahub-io/kserve.git
```

3. Add [kserve/kserve](https://github.com/kserve/kserve) repository as a remote:
```sh
$ git remote add kserve git@github.com:kserve/kserve.git
```

4. Your remotes setup would look similar to the following:
```sh
$ git remote -v
kserve  git@github.com:kserve/kserve.git (fetch)
kserve  git@github.com:kserve/kserve.git (push)
odh     git@github.com:opendatahub-io/kserve.git (fetch)
odh     git@github.com:opendatahub-io/kserve.git (push)
origin  git@github.com:${GH_USER}/kserve.git (fetch)
origin  git@github.com:${GH_USER}/kserve.git (push)
```
