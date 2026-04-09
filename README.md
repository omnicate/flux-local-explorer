# flux-local-explorer

`flux-local-explorer` is an offline Flux evaluation CLI. The primary command
name remains `flx`. It resolves Flux sources and
Kustomizations from a local checkout, cloned Git repositories, and OCI
artifacts, then renders the resulting Kubernetes manifests without requiring a
running cluster.

The tool is intended for development and review workflows where you want to
inspect what Flux would build before a change is pushed or reconciled. It can
be used to:

- list Flux resources from a repository tree
- render a `Kustomization` and its dependencies locally
- diff the rendered output of local changes against the upstream Flux sources
- work across multiple repositories when a deployment depends on shared Git
  sources

`flux-local-explorer` currently focuses on the Flux resource types used by this repository and
its surrounding workflows. It is a companion CLI for evaluating Flux-managed
configurations, not a replacement for Flux controllers running in-cluster.

```shell
$ export FLX_DIR=../kubeconf/dub.dev.wgtwo.com/flux/flux-system
$ flx get ks -n appdynamics
```

`flx` starts looking for resources in `FLX_DIR`, clones referenced Git and OCI
repositories, checks out specific references (commits, branches, tags), runs
`kustomize`, and performs Flux post-build
[substitution](https://fluxcd.io/flux/components/kustomize/kustomizations/#post-build-variable-substitution).

## What It Does

At a high level, `flux-local-explorer` follows the same dependency chain a Flux installation
would follow:

1. Read Flux resources from the entrypoint repository under `FLX_DIR`
2. Resolve referenced `GitRepository`, `OCIRepository`, and Helm sources
3. Build `Kustomization` resources recursively
4. Apply post-build substitution
5. Print resources or compare rendered output with `flx diff`

## Installation

See [Releases](https://github.com/omnicate/flux-local-explorer/releases).

```shell
$ gh release download -R omnicate/flux-local-explorer v0.1.4 -p "flx_darwin_arm64.tar.gz"
$ tar -xvzf flx_darwin_arm64.tar.gz
$ cp flx /usr/bin/flx
```

Flx requires `helm` and `git` to be available in your path.
Optionally, [dyff](https://github.com/homeport/dyff) is recommended for diffing k8s resource sets,
unless you want to use some other diff utility.

## Supported Resources

FLX splits up functionality into [controllers](tree/main/internal/controller). Each controller handles certain resource
kinds (and versions). The following is an overview of the supported resources:

### Git controller:

- source.toolkit.fluxcd.io/v1/GitRepository

### OCI controller:

- source.toolkit.fluxcd.io/v1beta2/OCIRepository

### Kustomization controller:

- kustomize.toolkit.fluxcd.io/v1/Kustomization

### Helm controller:

- source.toolkit.fluxcd.io/v1/HelmRepository
- source.toolkit.fluxcd.io/v1beta1/HelmRepository
- source.toolkit.fluxcd.io/v1beta2/HelmRepository
- helm.toolkit.fluxcd.io/v2/HelmRelease
- helm.toolkit.fluxcd.io/v2beta1/HelmRelease
- helm.toolkit.fluxcd.io/v2beta2/HelmRelease

### External secrets controller:

- external-secrets.io/v1beta1/ExternalSecret

This controller creates secrets from `ExternalSecret` resources.


And possibly also other versions of the resources.

## Usage

```shell
$ flx version
Version: development

$ flx -h
  Offline Flux companion.

Usage:
  flx [command]

Available Commands:
  completion  Generate the autocompletion script for the specified shell
  diff        Diff two flux clusters
  get         Retrieve resources
  help        Help about any command
  stat        Flux Kustomization resources (ks)
  version     Prints the version of flx

Flags:
  --cache-dir string      cache location (default "/Users/juliusmh/.flx")
  --controllers strings   controllers to enable (default [ks,git,oci,helm,external-secrets])
  -C, --dir string            git repository tracked by flux
  --git-force-https       force git clone via https
  -h, --help                  help for flx
  -L, --local stringArray     paths to local git repository overrides
  --log-format string     log format to use (pretty, json) (default "pretty")
  -v, --verbose               verbose logging

  Use "flx [command] --help" for more information about a command.
```

## Examples

Get all Kustomizations
```shell
$ flx get ks -A
NAME  	        NAMESPACE   SOURCE                          RESOURCES	ERROR
cert-manager  	infra       git: flux-system/my-flux-repo	4
[..]
```

List all Kustomizations in namespace infra:
```shell
$ flx get ks -n infra
NAME  	        SOURCE                          RESOURCES	ERROR
cert-manager 	git: flux-system/my-flux-repo	4
[..]
```

List Kustomizations as yaml:
```shell
$ flx get ks -n infra -o yaml
---
apiVersion: kustomize.toolkit.fluxcd.io/v1
kind: Kustomization
[..]
```

Get a specific Kustomization as yaml:
```shell
$ flx get ks -n infra -o yaml
---
apiVersion: kustomize.toolkit.fluxcd.io/v1
kind: Kustomization
[..]
```

Get the result of building the kustomization (kustomize):
```shell
$ flx get ks -n infra -o kustomize
---
apiVersion: apps/v1
kind: Deployment
[..]
```

If the selected Kustomization has reconciliation errors, `-o kustomize` prints
the rendered resources collected so far and exits non-zero with a summary of the
errors.

Make some changes to the local repository, then run `flx diff` command to compare
the local file system against the remote repository:
```shell
$ flx diff ks -n infra yopass
# changed  networking.k8s.io/v1/Ingress/infra/yopass
spec.tls.0.hosts.0
  ± value change
    - pass.my.cluster.cisco.com
    + test.my.cluster.cisco.com
```

## Working Across Multiple Repositories

By default, flx only treats the git repo under "FLX_DIR" (entrypoint) as a *local* repository. Changes to other included
repositories are not computed correctly during building or diffing. To tell flx to use an additional local file systems
rather than cloned git repositories, use the `-L` or `--local` flag:

```shell
flx -L path/to/k8s-library -L path/to/other-library get ks -n test
```

This will determine default branch, top level repo path and remote URL from the path automatically.

Or, to specify includes with fine control (needed when working with a fork):

```shell
$ flx \
  -L "remote=ssh://git@github.com/test/k8s-library.git,branch=master,path=test/k8s-library" \
  get ks -n test -v

[..]
2025-04-30T12:45:54+02:00 DBG using local git repository branch=master pathpath=test/k8s-library remote=ssh://git@github.com/test/k8s-library.git
[..]
```

All references to git repositories `remote` on branch `master` will be replaced by content of `test/k8s-library`.
This is useful when working on a local fork that doesn't have the "correct" remote.

## Diff

The `flx diff ks ...` command works by building the flux project twice:

- with "original" Git repositories
- with local overrides

Flx, by default, calls `dyff --color on between -b -i ${base} ${against}` (which can be changed using `--diff-tool`
flag) where *base* is a file containing the "original" yaml and *against* the modified version of a single resource.
Hence, when five resources change, the diff tool is called five times.

## Development

```shell
$ git clone https://github.com/omnicate/flux-local-explorer
$ cd flux-local-explorer
$ bazel build //:flx
# Place the binary in your path
$ cp bazel-bin/flx_/flx /usr/bin/flx
```

## FAQ

- FLX asks for github username and password?

> By default flx clones via SSH. If you don't have git via SSH set up correctly, you can
> force flx to use HTTPS via `--git-force-https` flag.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for contribution guidelines,
[SECURITY.md](SECURITY.md) for vulnerability reporting, and
[CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md) for community expectations.
