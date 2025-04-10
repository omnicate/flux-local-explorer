package resource

import (
	"fmt"

	helmv2 "github.com/fluxcd/helm-controller/api/v2"
	helmv2b1 "github.com/fluxcd/helm-controller/api/v2beta1"
	helmv2b2 "github.com/fluxcd/helm-controller/api/v2beta2"
	"github.com/fluxcd/pkg/apis/meta"
	"sigs.k8s.io/kustomize/api/resource"
)

const HelmReleaseKind = helmv2.HelmReleaseKind

type HelmRelease struct {
	ObjectMeta

	Chart      string
	Version    string
	SourceRef  CrossNamespaceObjectReference
	Values     map[string]any
	ValuesFrom []meta.ValuesReference

	Error error
}

// TODO: needs better error handling:

func NewHelmRelease(res *resource.Resource) (*HelmRelease, error) {
	if res.GetKind() != HelmReleaseKind {
		return nil, fmt.Errorf("kind != HelmRelease")
	}
	result := &HelmRelease{
		ObjectMeta: ObjectMeta{
			Name:      res.GetName(),
			Namespace: res.GetNamespace(),
		},
	}
	switch res.GetApiVersion() {
	case helmv2.GroupVersion.String():
		var hr helmv2.HelmRelease
		if err := unmarshalInto(res, &hr); err != nil {
			return nil, err
		}
		result.Chart = hr.Spec.Chart.Spec.Chart
		result.Version = hr.Spec.Chart.Spec.Version
		result.Values = hr.GetValues()
		result.ValuesFrom = hr.Spec.ValuesFrom
		result.SourceRef = CrossNamespaceObjectReference{
			APIVersion: hr.Spec.Chart.Spec.SourceRef.APIVersion,
			Kind:       hr.Spec.Chart.Spec.SourceRef.Kind,
			Name:       hr.Spec.Chart.Spec.SourceRef.Name,
			Namespace:  hr.Spec.Chart.Spec.SourceRef.Namespace,
		}
	case helmv2b1.GroupVersion.String():
		var hr helmv2b1.HelmRelease
		if err := unmarshalInto(res, &hr); err != nil {
			return nil, err
		}
		result.Chart = hr.Spec.Chart.Spec.Chart
		result.Version = hr.Spec.Chart.Spec.Version
		result.Values = hr.GetValues()
		result.ValuesFrom = helmv2b1ValuesReference(hr.Spec.ValuesFrom)
		result.SourceRef = CrossNamespaceObjectReference{
			APIVersion: hr.Spec.Chart.Spec.SourceRef.APIVersion,
			Kind:       hr.Spec.Chart.Spec.SourceRef.Kind,
			Name:       hr.Spec.Chart.Spec.SourceRef.Name,
			Namespace:  hr.Spec.Chart.Spec.SourceRef.Namespace,
		}
	case helmv2b2.GroupVersion.String():
		var hr helmv2b2.HelmRelease
		if err := unmarshalInto(res, &hr); err != nil {
			return nil, err
		}
		result.Chart = hr.Spec.Chart.Spec.Chart
		result.Version = hr.Spec.Chart.Spec.Version
		result.Values = hr.GetValues()
		result.ValuesFrom = helmv2b2ValuesReference(hr.Spec.ValuesFrom)
		result.SourceRef = CrossNamespaceObjectReference{
			APIVersion: hr.Spec.Chart.Spec.SourceRef.APIVersion,
			Kind:       hr.Spec.Chart.Spec.SourceRef.Kind,
			Name:       hr.Spec.Chart.Spec.SourceRef.Name,
			Namespace:  hr.Spec.Chart.Spec.SourceRef.Namespace,
		}
	}
	return result, nil
}

func helmv2b1ValuesReference(refs []helmv2b1.ValuesReference) []meta.ValuesReference {
	out := make([]meta.ValuesReference, len(refs))
	for i, ref := range refs {
		out[i] = meta.ValuesReference{
			Kind:       ref.Kind,
			Name:       ref.Name,
			ValuesKey:  ref.ValuesKey,
			TargetPath: ref.TargetPath,
			Optional:   ref.Optional,
		}
	}
	return out
}

func helmv2b2ValuesReference(refs []helmv2b2.ValuesReference) []meta.ValuesReference {
	out := make([]meta.ValuesReference, len(refs))
	for i, ref := range refs {
		out[i] = meta.ValuesReference{
			Kind:       ref.Kind,
			Name:       ref.Name,
			ValuesKey:  ref.ValuesKey,
			TargetPath: ref.TargetPath,
			Optional:   ref.Optional,
		}
	}
	return out
}
