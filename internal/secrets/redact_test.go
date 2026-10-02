package secrets

import (
	"fmt"
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
			before := fmt.Sprintf("%#v", tt.in)

			got := Redact(tt.in)

			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Redact() = %#v, want %#v", got, tt.want)
			}
			if after := fmt.Sprintf("%#v", tt.in); after != before {
				t.Errorf("Redact() modified its input: before %s, after %s", before, after)
			}
		})
	}
}
