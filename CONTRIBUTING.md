# Contributing Guidelines

## Commit messages
Each PR should consist of a single commit with the following format:
```
Commit title no longer than 80 chars

Commit message can be as long as you want. It needs to be complete but minimal,
meaning it should contain all the information to understand the change but
exclude redundant information. Please keep the lines in the commit message up
to 80 chars as well.

[Jira/GH-issue number]
```

The reason for requesting to squash your PR into a single **logical** commit is because our merge policy will squash all the commits anyway before merging. When squashed by Github it will list all the commit titles and messages as bullets, therefore, it will be more informative if correctly written to begin with.

By using the `squash` merge policy on Github we ensure that each PR is treated as an "atomic" piece of code that can be easily monitored and reverted.

It is recommended to add the tracking Jira/GH-issue if such exist at the end of the commit message.

### Unit tests

We use the [Ginkgo](https://onsi.github.io/ginkgo/) and [Gomega](https://onsi.github.io/gomega/) frameworks for unit
tests.

Create one test file per Go source file in the project.
For example, if you are working on `source.go`, unit tests should be written in `source_test.go` in the same directory.

Add one `Describe` section per method or function that you are unit testing:
- `Describe` names for methods should be `MyType_MyMethod`;
- `Describe` names for functions should be `MyFunction`.

Within a `Describe`, use one `It` section per test case.
If several `It` sections are similar, consider refactoring them in a `DescribeTable` construct with varying inputs.

### Kubernetes resources

- [Contributor License Agreement](https://git.k8s.io/community/CLA.md) Kubernetes projects require that you sign a Contributor License Agreement (CLA) before we can accept your pull requests
- [Kubernetes Contributor Guide](https://git.k8s.io/community/contributors/guide) - Main contributor documentation, or you can just jump directly to the [contributing section](https://git.k8s.io/community/contributors/guide#contributing)
- [Contributor Cheat Sheet](https://git.k8s.io/community/contributors/guide/contributor-cheatsheet) - Common resources for existing developers

## Mentorship

- [Mentoring Initiatives](https://git.k8s.io/community/mentoring) - We have a diverse set of mentorship programs available that are always looking for volunteers!

## Contact Information

- [Slack channel](https://kubernetes.slack.com/messages/sig-node-kmm)
- [Mailing list](https://groups.google.com/g/kubernetes-kmm)
