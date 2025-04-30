# flx

Recursively evaluate [Kustomizations](https://fluxcd.io/flux/components/kustomize/kustomizations/), [GitRepositories](https://fluxcd.io/flux/components/source/gitrepositories/) and [OCIRepositories](https://fluxcd.io/flux/components/source/ocirepositories/), without a cluster.

```shell
$ export FLX_DIR=../kubeconf/dub.dev.wgtwo.com/flux/flux-system 
$ flx get ks -n appdynamics
```

`flx` starts looking for resources in `FLX_DIR`, it clones git and oci repositories, checks 
out specific references (commits, branches, tags), runs `kustomize` and performs post build
[substitution](https://fluxcd.io/flux/components/kustomize/kustomizations/#post-build-variable-substitution).
 
## Supported resources

- kustomize.toolkit.fluxcd.io/v1/Kustomization
- source.toolkit.fluxcd.io/v1/GitRepository
- source.toolkit.fluxcd.io/v1beta2/OCIRepository

## Working across multiple repositories

By default, flx only treats the git repo under "FLX_DIR" (entrypoint) as a local repository. Changes to other included
repositories are not computed correctly during building or diffing. To tell flx to use a additional local file systems 
rather than cloned git repositories, use the `-L` or `--local` flag:

```shell
flx -L path/to/k8s-library -L path/to/other-library get ks -n test
```

## Installation

```shell
$ git clone https://github.com/omnicate/flx
$ cd flx
$ go install .
$ rm -rf flx
```

## FAQ

- FLX asks for github username and password?
> By default flx clones via SSH. If you don't have git via SSH set up correctly, you can
> force flx to use HTTPS via `--git-force-https` flag.