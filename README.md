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

## Installation

```shell
$ git clone https://github.com/omnicate/flx
$ cd flx
$ go install .
$ rm -rf flx
```

