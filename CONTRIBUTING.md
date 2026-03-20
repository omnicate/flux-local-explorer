# How to Contribute

Thanks for your interest in contributing to `flux-local-explorer`.

This project is maintained as an open source command-line tool for offline
evaluation of Flux-managed Kubernetes configuration. Contributions that improve
correctness, usability, documentation, and test coverage are welcome.

Please note that all interactions in this repository are subject to the
[Code of Conduct](CODE_OF_CONDUCT.md).

## Reporting Issues

Before opening a new issue, please check the existing
[issue tracker](https://github.com/omnicate/flux-local-explorer/issues) to avoid duplicates.

When filing an issue, include:

- a clear summary of the problem or request
- the commands you ran
- the relevant Flux resource types involved
- reproduction steps or example manifests when possible
- the expected result and the actual result

If you believe you have found a security issue, do not open a public GitHub
issue. Follow the instructions in [SECURITY.md](SECURITY.md) instead.

## Sending Pull Requests

Before opening a pull request:

- make sure the change is scoped to a clear problem
- update documentation if behavior or usage changed
- add or update tests when behavior changes

Pull requests should include enough context for review, including the motivation
for the change and any assumptions or limitations.

## Development

The repository uses Bazel for builds and includes Go tests for core packages.

```shell
git clone https://github.com/omnicate/flux-local-explorer.git
cd flux-local-explorer
bazel build //:flx
go test ./...
```

If you change user-facing behavior, update [README.md](README.md) accordingly.

## Other Ways to Contribute

You can also contribute by:

- improving documentation and examples
- reviewing pull requests
- reporting bugs with clear reproduction steps
- adding missing tests around Flux resource handling

Thanks for contributing to `flux-local-explorer`.
