package extsecret

import (
	"encoding/json"
	"fmt"

	extsecretv1b1 "github.com/external-secrets/external-secrets/apis/externalsecrets/v1beta1"
	"github.com/rs/zerolog"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/kustomize/api/resource"

	ctrl "github.com/omnicate/flx/loader/controller"
)

func init() {
	_ = extsecretv1b1.AddToScheme(ctrl.Scheme)
}

var _ ctrl.Controller = new(Controller)

type Controller struct {
	logger zerolog.Logger
}

func NewController(logger zerolog.Logger) *Controller {
	return &Controller{logger: logger}
}

func (r Controller) Kinds() []string {
	return []string{"ExternalSecret"}
}

func (r Controller) Reconcile(ctx ctrl.Context, req *ctrl.Resource) (*ctrl.Result, error) {
	var es extsecretv1b1.ExternalSecret
	if err := req.Unmarshal(&es); err != nil {
		return nil, err
	}

	logger := r.logger.With().
		Str("namespace", es.Namespace).
		Str("name", es.Name).
		Logger()

	var secret = &corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Secret",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      es.Spec.Target.Name,
			Namespace: es.Namespace,
		},
		Data: map[string][]byte{},
	}

	for _, d := range es.Spec.Data {
		secret.Data[d.SecretKey] = []byte(fmt.Sprintf(
			"externalSecret(%s.%s)",
			d.RemoteRef.Key,
			d.RemoteRef.Property,
		))
	}

	jsonSecret, err := json.Marshal(secret)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal secret: %w", err)
	}

	res := new(resource.Resource)
	if err := res.UnmarshalJSON(jsonSecret); err != nil {
		return nil, fmt.Errorf("failed to unmarshal secret: %w", err)
	}

	logger.Debug().Msg("creating secret")

	return &ctrl.Result{Resources: []*ctrl.Resource{
		ctrl.NewResource(res),
	}}, nil
}
