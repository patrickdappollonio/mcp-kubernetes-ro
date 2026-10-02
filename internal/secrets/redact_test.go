package secrets

import (
	"reflect"
	"testing"

	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestIsSecret(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		gvr  schema.GroupVersionResource
		want bool
	}{
		{"core secrets", schema.GroupVersionResource{Version: "v1", Resource: "secrets"}, true},
		{"configmaps", schema.GroupVersionResource{Version: "v1", Resource: "configmaps"}, false},
		{"secrets in another group", schema.GroupVersionResource{Group: "example.com", Version: "v1", Resource: "secrets"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := IsSecret(tt.gvr); got != tt.want {
				t.Fatalf("IsSecret(%v) = %v, want %v", tt.gvr, got, tt.want)
			}
		})
	}
}

func TestRedact(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   map[string]any
		want map[string]any
	}{
		{
			name: "replaces data values with their decoded size",
			in: map[string]any{
				"kind": "Secret",
				"type": "Opaque",
				"data": map[string]any{
					"password": "czNjcjN0", // "s3cr3t"
					"empty":    "",
				},
			},
			want: map[string]any{
				"kind": "Secret",
				"type": "Opaque",
				"data": map[string]any{
					"password": "[redacted: 6 bytes]",
					"empty":    "[redacted: 0 bytes]",
				},
			},
		},
		{
			name: "replaces stringData values with their size",
			in: map[string]any{
				"stringData": map[string]any{"token": "abc"},
			},
			want: map[string]any{
				"stringData": map[string]any{"token": "[redacted: 3 bytes]"},
			},
		},
		{
			name: "hides values that are not valid base64 without a size",
			in: map[string]any{
				"data": map[string]any{"broken": "not base64!"},
			},
			want: map[string]any{
				"data": map[string]any{"broken": "[redacted]"},
			},
		},
		{
			name: "removes the last-applied-configuration annotation",
			in: map[string]any{
				"metadata": map[string]any{
					"name": "db",
					"annotations": map[string]any{
						"kubectl.kubernetes.io/last-applied-configuration": `{"data":{"password":"czNjcjN0"}}`,
						"team": "payments",
					},
				},
			},
			want: map[string]any{
				"metadata": map[string]any{
					"name":        "db",
					"annotations": map[string]any{"team": "payments"},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := Redact(tt.in); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("Redact() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestRedactDoesNotModifyInput(t *testing.T) {
	t.Parallel()

	in := map[string]any{
		"data": map[string]any{"password": "czNjcjN0"},
		"metadata": map[string]any{
			"annotations": map[string]any{
				"kubectl.kubernetes.io/last-applied-configuration": "{}",
			},
		},
	}

	Redact(in)

	if got := in["data"].(map[string]any)["password"]; got != "czNjcjN0" {
		t.Fatalf("input data was modified: %v", got)
	}
	annotations := in["metadata"].(map[string]any)["annotations"].(map[string]any)
	if _, ok := annotations["kubectl.kubernetes.io/last-applied-configuration"]; !ok {
		t.Fatal("input annotations were modified")
	}
}
