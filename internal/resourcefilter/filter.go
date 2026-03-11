package resourcefilter

import (
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/runtime/schema"
)

// ResourceResolver resolves a user-friendly resource name and optional API version
// to a canonical GroupVersionResource. This allows the filter to accept singular names,
// kind names, or short names and resolve them to the correct plural resource name.
type ResourceResolver interface {
	ResolveResourceType(resourceType, apiVersion string) (schema.GroupVersionResource, error)
}

// Filter handles checking if Kubernetes resources are disabled by configuration.
type Filter struct {
	disabled []schema.GroupVersionResource
	raw      []string
}

// NewFilter creates a new Filter from a comma/space-separated string of resource specs.
// Each spec can be either:
//   - A resource name only (e.g., "secrets", "secret", "Secret", "po") which will be
//     resolved using the provided resolver against all API versions.
//   - A full group/version/resource triple (e.g., "core/v1/secrets", "apps/v1/deployments")
//     where "core" is an alias for the empty core API group.
//
// The resolver is used to validate and canonicalize each spec (e.g., resolving singular
// names to plural, kind names to resource names). Returns an error if any spec cannot
// be parsed or resolved, since silently skipping a disabled resource is a security risk.
func NewFilter(value string, resolver ResourceResolver) (*Filter, error) {
	if value == "" {
		return &Filter{}, nil
	}

	tokens := strings.FieldsFunc(value, func(r rune) bool {
		return r == ',' || r == ' ' || r == '\t' || r == '\n' || r == '\r'
	})

	var disabled []schema.GroupVersionResource
	var raw []string

	for _, token := range tokens {
		parts := strings.Split(token, "/")

		var resourceType, apiVersion string

		switch len(parts) {
		case 1:
			// Just a resource name: "secrets", "secret", "po"
			resourceType = parts[0]
			apiVersion = ""
		case 3:
			// Full spec: "core/v1/secrets" or "apps/v1/deployments"
			group := parts[0]
			version := parts[1]
			resourceType = parts[2]

			if strings.EqualFold(group, "core") {
				// core/v1 → apiVersion "v1" (empty group)
				apiVersion = version
			} else {
				// apps/v1 → apiVersion "apps/v1"
				apiVersion = group + "/" + version
			}
		default:
			return nil, fmt.Errorf("invalid disabled resource spec %q: expected \"resource\" or \"group/version/resource\" format (e.g. secrets or core/v1/secrets)", token)
		}

		if resolver == nil {
			return nil, fmt.Errorf("cannot validate disabled resource spec %q: no resource resolver available", token)
		}

		gvr, err := resolver.ResolveResourceType(resourceType, apiVersion)
		if err != nil {
			return nil, fmt.Errorf("cannot resolve disabled resource spec %q: %w", token, err)
		}

		disabled = append(disabled, gvr)
		raw = append(raw, FormatGVR(gvr))
	}

	return &Filter{
		disabled: disabled,
		raw:      raw,
	}, nil
}

// IsDisabled checks if a given GroupVersionResource matches any disabled resource.
func (f *Filter) IsDisabled(gvr schema.GroupVersionResource) bool {
	for _, d := range f.disabled {
		if strings.EqualFold(d.Group, gvr.Group) &&
			strings.EqualFold(d.Version, gvr.Version) &&
			strings.EqualFold(d.Resource, gvr.Resource) {
			return true
		}
	}
	return false
}

// MatchesAPIResource checks if a discovered API resource should be filtered out.
// The groupVersion parameter is the API group version string (e.g., "v1" for core,
// "apps/v1" for apps group).
func (f *Filter) MatchesAPIResource(groupVersion string, resourceName string) bool {
	gv, err := schema.ParseGroupVersion(groupVersion)
	if err != nil {
		return false
	}

	for _, d := range f.disabled {
		if strings.EqualFold(d.Group, gv.Group) &&
			strings.EqualFold(d.Version, gv.Version) &&
			strings.EqualFold(d.Resource, resourceName) {
			return true
		}
	}
	return false
}

// HasDisabledResources returns true if any resources have been disabled.
func (f *Filter) HasDisabledResources() bool {
	return len(f.disabled) > 0
}

// GetDisabledResources returns the canonical string representations of disabled resources
// in group/version/resource format (using "core" for the empty core API group).
func (f *Filter) GetDisabledResources() []string {
	result := make([]string, len(f.raw))
	copy(result, f.raw)
	return result
}

// FormatGVR formats a GroupVersionResource as a human-readable string using the
// "core" alias for the empty API group.
func FormatGVR(gvr schema.GroupVersionResource) string {
	group := gvr.Group
	if group == "" {
		group = "core"
	}
	return group + "/" + gvr.Version + "/" + gvr.Resource
}
