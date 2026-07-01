package extsecret

import (
	"runtime/debug"
	"testing"

	"github.com/rs/zerolog"

	ctrl "github.com/omnicate/flux-local-explorer/internal/controller"
	"github.com/omnicate/flux-local-explorer/internal/loader"
)

func TestControllerDoesNotImportExternalSecretsModule(t *testing.T) {
	const vulnerableModule = "github.com/external-secrets/external-secrets"

	info, ok := debug.ReadBuildInfo()
	if !ok {
		t.Skip("build info unavailable")
	}
	for _, dep := range info.Deps {
		if dep.Path == vulnerableModule {
			t.Fatal("controller imports the External Secrets operator module; use a local manifest shape instead")
		}
	}
}

func TestReconcileCreatesSecretFromExternalSecretData(t *testing.T) {
	resources, err := loader.LoadBytes([]byte(`
apiVersion: external-secrets.io/v1beta1
kind: ExternalSecret
metadata:
  name: app-secrets
  namespace: apps
spec:
  target:
    name: rendered-secret
  data:
    - secretKey: username
      remoteRef:
        key: prod/db
        property: username
`))
	if err != nil {
		t.Fatal(err)
	}

	result, err := NewController(zerolog.Nop()).Reconcile(nil, ctrl.NewResource(resources[0]))
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Resources) != 1 {
		t.Fatalf("len(result.Resources) = %d, want 1", len(result.Resources))
	}

	var secret struct {
		Kind     string `json:"kind"`
		Metadata struct {
			Name      string `json:"name"`
			Namespace string `json:"namespace"`
		} `json:"metadata"`
		Data map[string][]byte `json:"data"`
	}
	if err := result.Resources[0].Unmarshal(&secret); err != nil {
		t.Fatal(err)
	}

	if secret.Kind != "Secret" || secret.Metadata.Name != "rendered-secret" || secret.Metadata.Namespace != "apps" {
		t.Fatalf("secret identity = %s/%s %s, want apps/rendered-secret Secret", secret.Metadata.Namespace, secret.Metadata.Name, secret.Kind)
	}
	if got := string(secret.Data["username"]); got != "externalSecret(prod/db.username)" {
		t.Fatalf("secret.Data[username] = %q, want rendered external secret marker", got)
	}
}
