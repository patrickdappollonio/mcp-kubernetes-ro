package handlers

import (
	"reflect"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
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
