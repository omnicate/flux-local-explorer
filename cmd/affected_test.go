package cmd

import (
	"bytes"
	"strings"
	"testing"

	"github.com/omnicate/flux-local-explorer/internal/affected"
)

func TestPrintAffectedTargets(t *testing.T) {
	targets := []affected.Target{{
		EntryPoint: "/repo/clusters/prod",
		Namespace:  "gitops",
		Name:       "prod-service",
	}}

	var buf bytes.Buffer
	if err := printAffectedTargets(&buf, targets, "tsv"); err != nil {
		t.Fatal(err)
	}
	if got, want := buf.String(), "/repo/clusters/prod\tgitops\tprod-service\n"; got != want {
		t.Fatalf("tsv = %q, want %q", got, want)
	}

	buf.Reset()
	if err := printAffectedTargets(&buf, targets, "yaml"); err != nil {
		t.Fatal(err)
	}
	if got := buf.String(); !strings.Contains(got, "entryPoint: /repo/clusters/prod") || !strings.Contains(got, "name: prod-service") {
		t.Fatalf("yaml = %q", got)
	}

	if err := printAffectedTargets(&buf, targets, "json"); err == nil || err.Error() != "unknown output format: json" {
		t.Fatalf("err = %v, want unknown output format", err)
	}
}
