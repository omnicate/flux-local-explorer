# flx

Recursively
evaluate [Kustomizations](https://fluxcd.io/flux/components/kustomize/kustomizations/), [GitRepositories](https://fluxcd.io/flux/components/source/gitrepositories/)
and [OCIRepositories](https://fluxcd.io/flux/components/source/ocirepositories/), without a cluster.

```shell
$ export FLX_DIR=../kubeconf/dub.dev.wgtwo.com/flux/flux-system 
$ flx get ks -n appdynamics
```

`flx` starts looking for resources in `FLX_DIR`, it clones git and oci repositories, checks
out specific references (commits, branches, tags), runs `kustomize` and performs post build
[substitution](https://fluxcd.io/flux/components/kustomize/kustomizations/#post-build-variable-substitution).

## Installation

See [Releases](https://github.com/omnicate/flx/releases).

Flx requires `helm` and `git` to be available in your path.
Optionally, [dyff](https://github.com/homeport/dyff) is recommended for diffing k8s resource sets,
unless you want to use some other diff utility.

## Supported resources

- kustomize.toolkit.fluxcd.io/v1/Kustomization
- source.toolkit.fluxcd.io/v1/GitRepository
- source.toolkit.fluxcd.io/v1beta2/OCIRepository
- source.toolkit.fluxcd.io/v1/HelmRepository
- helm.toolkit.fluxcd.io/v2/HelmRelease
- external-secrets.io/v1beta1/ExternalSecret

And possibly also other versions of the resources.

## Working across multiple repositories

By default, flx only treats the git repo under "FLX_DIR" (entrypoint) as a *local* repository. Changes to other included
repositories are not computed correctly during building or diffing. To tell flx to use a additional local file systems
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
$ git clone https://github.com/omnicate/flx
$ cd flx
$ bazel build //:flx
# Place the binary in your path
$ cp bazel-bin/flx_/flx /usr/bin/flx
```

## FAQ

- FLX asks for github username and password?

> By default flx clones via SSH. If you don't have git via SSH set up correctly, you can
> force flx to use HTTPS via `--git-force-https` flag.