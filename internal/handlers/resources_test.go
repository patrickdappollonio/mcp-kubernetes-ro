package handlers

import (
	"reflect"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestSanitizeMetadata(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                 string
		metadata             map[string]interface{}
		includeManagedFields bool
		want                 map[string]interface{}
	}{
		{
			name: "strips managed fields by default",
			metadata: map[string]interface{}{
				"name":      "demo-pod",
				"namespace": "default",
				"managedFields": []interface{}{
					map[string]interface{}{
						"manager": "kubectl-client-side-apply",
						"fieldsV1": map[string]interface{}{
							"f:metadata": map[string]interface{}{
								"f:labels": map[string]interface{}{
									"f:app": map[string]interface{}{},
								},
							},
						},
					},
				},
			},
			includeManagedFields: false,
			want: map[string]interface{}{
				"name":      "demo-pod",
				"namespace": "default",
			},
		},
		{
			name: "preserves managed fields when requested",
			metadata: map[string]interface{}{
				"name": "demo-pod",
				"managedFields": []interface{}{
					map[string]interface{}{
						"fieldsV1": map[string]interface{}{
							"f:spec": map[string]interface{}{},
						},
					},
				},
			},
			includeManagedFields: true,
			want: map[string]interface{}{
				"name": "demo-pod",
				"managedFields": []interface{}{
					map[string]interface{}{
						"fieldsV1": map[string]interface{}{
							"f:spec": map[string]interface{}{},
						},
					},
				},
			},
		},
		{
			name: "handles metadata without managed fields",
			metadata: map[string]interface{}{
				"name":              "demo-pod",
				"creationTimestamp": "2026-03-11T12:00:00Z",
			},
			includeManagedFields: false,
			want: map[string]interface{}{
				"name":              "demo-pod",
				"creationTimestamp": "2026-03-11T12:00:00Z",
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := sanitizeMetadata(tt.metadata, tt.includeManagedFields)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("sanitizeMetadata() mismatch\nwant: %#v\ngot:  %#v", tt.want, got)
			}
		})
	}
}

func TestSanitizeResourceObject(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                 string
		resource             map[string]interface{}
		includeManagedFields bool
		want                 map[string]interface{}
	}{
		{
			name: "strips managed fields from metadata by default",
			resource: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "Pod",
				"metadata": map[string]interface{}{
					"name": "demo-pod",
					"managedFields": []interface{}{
						map[string]interface{}{
							"fieldsV1": map[string]interface{}{
								"f:spec": map[string]interface{}{
									"f:containers": map[string]interface{}{},
								},
							},
						},
					},
				},
				"spec": map[string]interface{}{
					"restartPolicy": "Always",
				},
			},
			includeManagedFields: false,
			want: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "Pod",
				"metadata": map[string]interface{}{
					"name": "demo-pod",
				},
				"spec": map[string]interface{}{
					"restartPolicy": "Always",
				},
			},
		},
		{
			name: "preserves managed fields when requested",
			resource: map[string]interface{}{
				"apiVersion": "v1",
				"metadata": map[string]interface{}{
					"name": "demo-pod",
					"managedFields": []interface{}{
						map[string]interface{}{
							"fieldsV1": map[string]interface{}{
								"f:metadata": map[string]interface{}{},
							},
						},
					},
				},
			},
			includeManagedFields: true,
			want: map[string]interface{}{
				"apiVersion": "v1",
				"metadata": map[string]interface{}{
					"name": "demo-pod",
					"managedFields": []interface{}{
						map[string]interface{}{
							"fieldsV1": map[string]interface{}{
								"f:metadata": map[string]interface{}{},
							},
						},
					},
				},
			},
		},
		{
			name: "handles resource without metadata",
			resource: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "Namespace",
			},
			includeManagedFields: false,
			want: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "Namespace",
			},
		},
		{
			name: "preserves non-map metadata as-is",
			resource: map[string]interface{}{
				"apiVersion": "v1",
				"metadata":   "unexpected",
			},
			includeManagedFields: false,
			want: map[string]interface{}{
				"apiVersion": "v1",
				"metadata":   "unexpected",
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := sanitizeResourceObject(tt.resource, tt.includeManagedFields)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("sanitizeResourceObject() mismatch\nwant: %#v\ngot:  %#v", tt.want, got)
			}
		})
	}
}

func TestExtractResourceTitleIsUnchanged(t *testing.T) {
	t.Parallel()

	resource := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"metadata": map[string]interface{}{
				"name": "demo-pod",
				"managedFields": []interface{}{
					map[string]interface{}{"manager": "controller"},
				},
			},
		},
	}

	title := extractResourceTitle(resource)
	want := map[string]interface{}{"name": "demo-pod"}

	if !reflect.DeepEqual(title, want) {
		t.Fatalf("extractResourceTitle() mismatch\nwant: %#v\ngot:  %#v", want, title)
	}
}

func secretObject() map[string]any {
	return map[string]any{
		"apiVersion": "v1",
		"kind":       "Secret",
		"metadata": map[string]any{
			"name": "db",
			"annotations": map[string]any{
				"kubectl.kubernetes.io/last-applied-configuration": `{"data":{"password":"czNjcjN0"}}`,
			},
		},
		"data": map[string]any{"password": "czNjcjN0"},
	}
}

var (
	secretGVR = schema.GroupVersionResource{Version: "v1", Resource: "secrets"}
	podGVR    = schema.GroupVersionResource{Version: "v1", Resource: "pods"}
)

func TestPrepareResourceObjectHidesSecretValues(t *testing.T) {
	t.Parallel()

	got := prepareResourceObject(secretGVR, secretObject(), false, false)

	if v := got["data"].(map[string]any)["password"]; v != "[redacted: 6 bytes]" {
		t.Fatalf("data.password = %v, want it redacted", v)
	}
	annotations := got["metadata"].(map[string]any)["annotations"].(map[string]any)
	if _, ok := annotations["kubectl.kubernetes.io/last-applied-configuration"]; ok {
		t.Fatal("last-applied-configuration annotation was not removed")
	}
}

func TestPrepareResourceObjectExposesSecretValuesWhenInsecure(t *testing.T) {
	t.Parallel()

	got := prepareResourceObject(secretGVR, secretObject(), false, true)

	if v := got["data"].(map[string]any)["password"]; v != "czNjcjN0" {
		t.Fatalf("data.password = %v, want the original value", v)
	}
}

func TestPrepareResourceObjectLeavesOtherResourcesAlone(t *testing.T) {
	t.Parallel()

	obj := secretObject()
	obj["kind"] = "Pod"

	got := prepareResourceObject(podGVR, obj, false, false)

	if v := got["data"].(map[string]any)["password"]; v != "czNjcjN0" {
		t.Fatalf("data.password = %v, want non-Secret data untouched", v)
	}
}

func TestPrepareResourceSummaryHidesSecretLastApplied(t *testing.T) {
	t.Parallel()

	resource := &unstructured.Unstructured{Object: secretObject()}

	got := prepareResourceSummary(secretGVR, resource, false, false)

	metadata := got["metadata"].(map[string]any)
	if annotations, ok := metadata["annotations"].(map[string]any); ok {
		if _, found := annotations["kubectl.kubernetes.io/last-applied-configuration"]; found {
			t.Fatal("last-applied-configuration annotation was not removed from the summary")
		}
	}

	exposed := prepareResourceSummary(secretGVR, resource, false, true)
	annotations := exposed["metadata"].(map[string]any)["annotations"].(map[string]any)
	if _, found := annotations["kubectl.kubernetes.io/last-applied-configuration"]; !found {
		t.Fatal("last-applied-configuration annotation was removed even with insecure access")
	}
}

func TestGetResourceDescriptionPointsToTheSecretTool(t *testing.T) {
	t.Parallel()

	tests := []struct {
		mode        SecretAccessMode
		mustHave    []string
		mustNotHave []string
	}{
		{SecretAccessFile, []string{"Secret values are hidden", "save_secret_to_file"}, []string{"get_secret_encrypted"}},
		{SecretAccessEncrypted, []string{"Secret values are hidden", "get_secret_encrypted"}, []string{"save_secret_to_file"}},
		{SecretAccessInsecure, nil, []string{"Secret values are hidden", "save_secret_to_file", "get_secret_encrypted"}},
	}

	for _, tt := range tests {
		h := &ResourceHandler{secretAccess: tt.mode}
		var description string
		for _, tool := range h.GetTools() {
			if tool.Tool().Name == "get_resource" {
				description = tool.Tool().Description
			}
		}

		for _, want := range tt.mustHave {
			if !strings.Contains(description, want) {
				t.Errorf("mode %v get_resource description = %q, want it to contain %q", tt.mode, description, want)
			}
		}
		for _, unwanted := range tt.mustNotHave {
			if strings.Contains(description, unwanted) {
				t.Errorf("mode %v get_resource description = %q, want it without %q", tt.mode, description, unwanted)
			}
		}
	}
}
