package secrets

import (
	"encoding/base64"
	"fmt"
	"maps"

	"k8s.io/apimachinery/pkg/runtime/schema"
)

// lastAppliedAnnotation holds a full copy of the object as last applied by
// kubectl, which for a Secret includes every value.
const lastAppliedAnnotation = "kubectl.kubernetes.io/last-applied-configuration"

// GVR identifies core/v1 Secrets.
var GVR = schema.GroupVersionResource{Version: "v1", Resource: "secrets"}

// IsSecret reports whether gvr refers to core/v1 Secrets.
func IsSecret(gvr schema.GroupVersionResource) bool {
	return gvr == GVR
}

// Redact returns a copy of a Secret object with each data and stringData value
// replaced by its size and the last-applied-configuration annotation removed.
func Redact(obj map[string]any) map[string]any {
	out := maps.Clone(obj)

	if data, ok := obj["data"].(map[string]any); ok {
		out["data"] = redactValues(data, decodedSize)
	}

	if stringData, ok := obj["stringData"].(map[string]any); ok {
		out["stringData"] = redactValues(stringData, func(s string) (int, bool) { return len(s), true })
	}

	if metadata, ok := obj["metadata"].(map[string]any); ok {
		out["metadata"] = StripLastApplied(metadata)
	}

	return out
}

// StripLastApplied returns a copy of metadata without the
// last-applied-configuration annotation. The input is not modified.
func StripLastApplied(metadata map[string]any) map[string]any {
	annotations, ok := metadata["annotations"].(map[string]any)
	if !ok {
		return metadata
	}

	if _, found := annotations[lastAppliedAnnotation]; !found {
		return metadata
	}

	out := maps.Clone(metadata)
	stripped := maps.Clone(annotations)
	delete(stripped, lastAppliedAnnotation)
	out["annotations"] = stripped

	return out
}

func redactValues(values map[string]any, size func(string) (int, bool)) map[string]any {
	out := make(map[string]any, len(values))
	for key, value := range values {
		s, _ := value.(string)
		if n, ok := size(s); ok {
			out[key] = fmt.Sprintf("[redacted: %d bytes]", n)
		} else {
			out[key] = "[redacted]"
		}
	}
	return out
}

func decodedSize(s string) (int, bool) {
	decoded, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return 0, false
	}
	return len(decoded), true
}
