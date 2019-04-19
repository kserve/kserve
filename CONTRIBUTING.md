# Contributor Guide

## Setting up a KFServing Environment
TODO flesh this out
1. make all
2. make docker-build
3. make docker-push
4. make deploy
5. make install
4. kubectl apply -f docs/samples/tensorflow.yaml

## Updating Dependencies
Go projects follow the pattern of committing vendor code to the repo. We will update dependencies frequently to solve upgrade conflicts early.

1. dep ensure
2. git commit -am "Updates vendor dependencies"

## Submitting a PR
The following should be viewed as Best Practices unless you know better ones (please submit a guidelines PR).

| Practice | Rationale |
| ----- | --------- |
| Keep the code clean | The health of the codebase is imperative to the success of the project. Files should be under 500 lines long in most cases, which may mean a refactor is necessary before adding changes. |
| Squash your commits | A clean codebase needs a clean commit log. Aim to submit one commit per PR, which may mean squashing your changes. You'll thank yourself later if you ever need to revert changes you've made and you'll simplify the lives of anyone looking into the history of the project. |
| Limit your scope | No one wants to review a 1000 line PR. Try to keep your changes focused to ease reviewability. This may mean separating a large feature into several smaller milestones.  |
| Refine commit messages | Your commit messages should be in the imperative tense and clearly describe your feature upon first glance. See [this article](https://chris.beams.io/posts/git-commit/) for guidelines.
| Reference an issue | Issues are a great way to gather design feedback from the community. To save yourself time on a controversial PR, first cut an issue for any major feature work. |
