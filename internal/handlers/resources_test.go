package handlers

import (
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestSanitizeResourceObjectStripsManagedFieldsByDefault(t *testing.T) {
	resource := map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "Pod",
		"metadata": map[string]interface{}{
			"name":      "demo-pod",
			"namespace": "default",
			"managedFields": []interface{}{
				map[string]interface{}{
					"manager":   "kubectl-client-side-apply",
					"operation": "Update",
					"fieldsV1": map[string]interface{}{
						"f:metadata": map[string]interface{}{
							"f:labels": map[string]interface{}{
								"f:app": map[string]interface{}{},
							},
						},
						"f:spec": map[string]interface{}{
							"f:containers": map[string]interface{}{
								"k:{\"name\":\"demo\"}": map[string]interface{}{
									".":       map[string]interface{}{},
									"f:image": map[string]interface{}{},
								},
							},
						},
					},
				},
			},
		},
		"spec": map[string]interface{}{
			"containers": []interface{}{
				map[string]interface{}{"name": "demo", "image": "nginx"},
			},
		},
	}

	sanitized := sanitizeResourceObject(resource, false)
	metadata := sanitized["metadata"].(map[string]interface{})

	if _, ok := metadata["managedFields"]; ok {
		t.Fatal("expected managedFields to be removed")
	}

	if metadata["name"] != "demo-pod" {
		t.Fatalf("expected metadata.name to be preserved, got %v", metadata["name"])
	}

	if _, ok := resource["metadata"].(map[string]interface{})["managedFields"]; !ok {
		t.Fatal("expected original resource to remain unchanged")
	}
}

func TestSanitizeResourceObjectPreservesManagedFieldsWhenRequested(t *testing.T) {
	resource := map[string]interface{}{
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
	}

	sanitized := sanitizeResourceObject(resource, true)
	metadata := sanitized["metadata"].(map[string]interface{})

	if _, ok := metadata["managedFields"]; !ok {
		t.Fatal("expected managedFields to be preserved")
	}
}

func TestSanitizeResourceObjectHandlesMissingMetadata(t *testing.T) {
	resource := map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "Namespace",
	}

	sanitized := sanitizeResourceObject(resource, false)

	if sanitized["kind"] != "Namespace" {
		t.Fatalf("expected kind to be preserved, got %v", sanitized["kind"])
	}
}

func TestExtractResourceSummaryStripsManagedFieldsByDefault(t *testing.T) {
	resource := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Pod",
			"metadata": map[string]interface{}{
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
	}

	summary := extractResourceSummary(resource, false)
	metadata := summary["metadata"].(map[string]interface{})

	if _, ok := metadata["managedFields"]; ok {
		t.Fatal("expected managedFields to be removed from summary metadata")
	}

	if summary["apiVersion"] != "v1" {
		t.Fatalf("expected apiVersion to be preserved, got %v", summary["apiVersion"])
	}
}

func TestExtractResourceSummaryPreservesManagedFieldsWhenRequested(t *testing.T) {
	resource := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Pod",
			"metadata": map[string]interface{}{
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
	}

	summary := extractResourceSummary(resource, true)
	metadata := summary["metadata"].(map[string]interface{})

	if _, ok := metadata["managedFields"]; !ok {
		t.Fatal("expected managedFields to be preserved in summary metadata")
	}
}

func TestExtractResourceTitleIsUnchanged(t *testing.T) {
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

	if len(title) != 1 {
		t.Fatalf("expected title-only response to contain only one field, got %d", len(title))
	}

	if title["name"] != "demo-pod" {
		t.Fatalf("expected title name to be preserved, got %v", title["name"])
	}
}
