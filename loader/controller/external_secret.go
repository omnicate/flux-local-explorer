package controller

import (
	"fmt"

	extv1b1 "github.com/external-secrets/external-secrets/apis/externalsecrets/v1beta1"
	"github.com/rs/zerolog"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/kustomize/api/resource"
	sigyaml "sigs.k8s.io/yaml"

	"github.com/omnicate/flx/loader"
)

type ExternalSecrets struct {
	logger zerolog.Logger
}

func NewExternalSecrets(logger zerolog.Logger) *ExternalSecrets {
	return &ExternalSecrets{logger: logger}
}

func (e *ExternalSecrets) Get(kind, namespace, name string) (any, error) {
	return nil, ErrNotFound
}

func (e *ExternalSecrets) Handle(res *loader.ResultSet) (*loader.ResultSet, error) {
	out := loader.EmptyResultSet()
	for _, r := range res.Resources {
		if r.GetKind() == "ExternalSecret" {
			secret, err := e.createExternalSecret(r)
			if err != nil {
				return nil, err
			}
			out.Merge(secret)
		}
	}
	return out, nil
}

func (e *ExternalSecrets) createExternalSecret(res *resource.Resource) (*loader.ResultSet, error) {
	var extSecret extv1b1.ExternalSecret
	data, err := res.AsYAML()
	if err != nil {
		return nil, err
	}
	if err := sigyaml.Unmarshal(data, &extSecret); err != nil {
		return nil, err
	}
	var secret = &corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Secret",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      extSecret.Spec.Target.Name,
			Namespace: extSecret.Namespace,
		},
		Data: map[string][]byte{},
	}

	for _, d := range extSecret.Spec.Data {
		secret.Data[d.SecretKey] = []byte(fmt.Sprintf(
			"externalSecret(%s.%s)",
			d.RemoteRef.Key,
			d.RemoteRef.Property,
		))
	}

	e.logger.Debug().
		Str("name", secret.Name).
		Str("namespace", secret.Namespace).
		Msg("creating external-secret")

	secretData, err := sigyaml.Marshal(secret)
	if err != nil {
		return nil, err
	}
	secretResource, err := loader.LoadBytes(secretData)
	if err != nil {
		return nil, err
	}
	return loader.NewResultSet(secretResource, extSecret.Namespace)
}
