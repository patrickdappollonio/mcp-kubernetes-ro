package resourcefilter

import (
	"fmt"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/runtime/schema"
)

// mockResolver simulates Kubernetes resource resolution for testing.
type mockResolver struct {
	resources map[string]schema.GroupVersionResource
}

func newMockResolver() *mockResolver {
	return &mockResolver{
		resources: map[string]schema.GroupVersionResource{
			// Core resources (empty group)
			"v1/secrets":      {Group: "", Version: "v1", Resource: "secrets"},
			"v1/secret":       {Group: "", Version: "v1", Resource: "secrets"},
			"v1/Secret":       {Group: "", Version: "v1", Resource: "secrets"},
			"/secrets":        {Group: "", Version: "v1", Resource: "secrets"},
			"/secret":         {Group: "", Version: "v1", Resource: "secrets"},
			"/Secret":         {Group: "", Version: "v1", Resource: "secrets"},
			"v1/pods":         {Group: "", Version: "v1", Resource: "pods"},
			"v1/pod":          {Group: "", Version: "v1", Resource: "pods"},
			"v1/po":           {Group: "", Version: "v1", Resource: "pods"},
			"/pods":           {Group: "", Version: "v1", Resource: "pods"},
			"/pod":            {Group: "", Version: "v1", Resource: "pods"},
			"/po":             {Group: "", Version: "v1", Resource: "pods"},
			"v1/services":     {Group: "", Version: "v1", Resource: "services"},
			"/services":       {Group: "", Version: "v1", Resource: "services"},
			"v1/configmaps":   {Group: "", Version: "v1", Resource: "configmaps"},
			"/configmaps":     {Group: "", Version: "v1", Resource: "configmaps"},
			"/cm":             {Group: "", Version: "v1", Resource: "configmaps"},
			// Non-core resources
			"apps/v1/deployments": {Group: "apps", Version: "v1", Resource: "deployments"},
			"apps/v1/deploy":      {Group: "apps", Version: "v1", Resource: "deployments"},
			"apps/v1/Deployment":  {Group: "apps", Version: "v1", Resource: "deployments"},
			"/deployments":        {Group: "apps", Version: "v1", Resource: "deployments"},
			"/deploy":             {Group: "apps", Version: "v1", Resource: "deployments"},
			"batch/v1/jobs":       {Group: "batch", Version: "v1", Resource: "jobs"},
			"/jobs":               {Group: "batch", Version: "v1", Resource: "jobs"},
		},
	}
}

func (m *mockResolver) ResolveResourceType(resourceType, apiVersion string) (schema.GroupVersionResource, error) {
	key := strings.ToLower(apiVersion + "/" + resourceType)
	for k, gvr := range m.resources {
		if strings.EqualFold(k, key) {
			return gvr, nil
		}
	}
	return schema.GroupVersionResource{}, fmt.Errorf("resource type %q not found", resourceType)
}

// mustNewFilter is a test helper that calls NewFilter and fails on error.
func mustNewFilter(t *testing.T, value string, resolver ResourceResolver) *Filter {
	t.Helper()
	f, err := NewFilter(value, resolver)
	if err != nil {
		t.Fatalf("NewFilter(%q) returned unexpected error: %v", value, err)
	}
	return f
}

func TestNewFilter_Empty(t *testing.T) {
	f := mustNewFilter(t, "", newMockResolver())
	if f.HasDisabledResources() {
		t.Error("expected no disabled resources for empty input")
	}
}

func TestNewFilter_SingleResource_FullSpec(t *testing.T) {
	f := mustNewFilter(t, "core/v1/secrets", newMockResolver())
	if !f.HasDisabledResources() {
		t.Fatal("expected disabled resources")
	}
	if got := f.GetDisabledResources(); len(got) != 1 || got[0] != "core/v1/secrets" {
		t.Errorf("unexpected disabled resources: %v", got)
	}
}

func TestNewFilter_SingleResource_NameOnly(t *testing.T) {
	f := mustNewFilter(t, "secrets", newMockResolver())
	if !f.HasDisabledResources() {
		t.Fatal("expected disabled resources")
	}
	if got := f.GetDisabledResources(); len(got) != 1 || got[0] != "core/v1/secrets" {
		t.Errorf("unexpected disabled resources: %v", got)
	}
}

func TestNewFilter_SingularName(t *testing.T) {
	f := mustNewFilter(t, "secret", newMockResolver())
	if !f.HasDisabledResources() {
		t.Fatal("expected singular name to resolve")
	}
	gvr := schema.GroupVersionResource{Group: "", Version: "v1", Resource: "secrets"}
	if !f.IsDisabled(gvr) {
		t.Error("expected 'secret' (singular) to disable secrets")
	}
}

func TestNewFilter_ShortName(t *testing.T) {
	f := mustNewFilter(t, "po", newMockResolver())
	if !f.HasDisabledResources() {
		t.Fatal("expected short name to resolve")
	}
	gvr := schema.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"}
	if !f.IsDisabled(gvr) {
		t.Error("expected 'po' (short name) to disable pods")
	}
}

func TestNewFilter_FullSpecSingular(t *testing.T) {
	f := mustNewFilter(t, "core/v1/secret", newMockResolver())
	if !f.HasDisabledResources() {
		t.Fatal("expected singular full spec to resolve")
	}
	gvr := schema.GroupVersionResource{Group: "", Version: "v1", Resource: "secrets"}
	if !f.IsDisabled(gvr) {
		t.Error("expected 'core/v1/secret' to disable secrets")
	}
}

func TestNewFilter_MultipleResources(t *testing.T) {
	f := mustNewFilter(t, "core/v1/secrets,apps/v1/deployments", newMockResolver())
	if got := len(f.GetDisabledResources()); got != 2 {
		t.Errorf("expected 2 disabled resources, got %d", got)
	}
}

func TestNewFilter_MixedSeparators(t *testing.T) {
	f := mustNewFilter(t, "secrets deployments\tjobs", newMockResolver())
	if got := len(f.GetDisabledResources()); got != 3 {
		t.Errorf("expected 3 disabled resources, got %d", got)
	}
}

func TestNewFilter_InvalidSpecReturnsError(t *testing.T) {
	_, err := NewFilter("also/invalid", newMockResolver())
	if err == nil {
		t.Error("expected error for invalid spec with 2 parts")
	}
}

func TestNewFilter_TooManyPartsReturnsError(t *testing.T) {
	_, err := NewFilter("too/many/parts/here", newMockResolver())
	if err == nil {
		t.Error("expected error for spec with 4 parts")
	}
}

func TestNewFilter_UnresolvableReturnsError(t *testing.T) {
	_, err := NewFilter("nonexistent", newMockResolver())
	if err == nil {
		t.Error("expected error for unresolvable resource")
	}
}

func TestIsDisabled_CoreAlias(t *testing.T) {
	f := mustNewFilter(t, "core/v1/secrets", newMockResolver())

	gvr := schema.GroupVersionResource{Group: "", Version: "v1", Resource: "secrets"}
	if !f.IsDisabled(gvr) {
		t.Error("expected core/v1/secrets to be disabled")
	}
}

func TestIsDisabled_CaseInsensitive(t *testing.T) {
	f := mustNewFilter(t, "Core/V1/Secret", newMockResolver())

	gvr := schema.GroupVersionResource{Group: "", Version: "v1", Resource: "secrets"}
	if !f.IsDisabled(gvr) {
		t.Error("expected case-insensitive match")
	}
}

func TestIsDisabled_NonCoreGroup(t *testing.T) {
	f := mustNewFilter(t, "apps/v1/deployments", newMockResolver())

	gvr := schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}
	if !f.IsDisabled(gvr) {
		t.Error("expected apps/v1/deployments to be disabled")
	}
}

func TestIsDisabled_NoFalsePositive(t *testing.T) {
	f := mustNewFilter(t, "core/v1/secrets", newMockResolver())

	gvr := schema.GroupVersionResource{Group: "", Version: "v1", Resource: "services"}
	if f.IsDisabled(gvr) {
		t.Error("services should not be disabled when only secrets is disabled")
	}
}

func TestIsDisabled_NoFalsePositiveGroup(t *testing.T) {
	f := mustNewFilter(t, "apps/v1/deployments", newMockResolver())

	gvr := schema.GroupVersionResource{Group: "extensions", Version: "v1", Resource: "deployments"}
	if f.IsDisabled(gvr) {
		t.Error("extensions/v1/deployments should not match apps/v1/deployments")
	}
}

func TestIsDisabled_NotDisabled(t *testing.T) {
	f := mustNewFilter(t, "", newMockResolver())

	gvr := schema.GroupVersionResource{Group: "", Version: "v1", Resource: "secrets"}
	if f.IsDisabled(gvr) {
		t.Error("nothing should be disabled with empty filter")
	}
}

func TestMatchesAPIResource_CoreGroup(t *testing.T) {
	f := mustNewFilter(t, "secrets", newMockResolver())

	if !f.MatchesAPIResource("v1", "secrets") {
		t.Error("expected v1/secrets to match")
	}
}

func TestMatchesAPIResource_NonCoreGroup(t *testing.T) {
	f := mustNewFilter(t, "apps/v1/deployments", newMockResolver())

	if !f.MatchesAPIResource("apps/v1", "deployments") {
		t.Error("expected apps/v1/deployments to match")
	}
}

func TestMatchesAPIResource_NoMatch(t *testing.T) {
	f := mustNewFilter(t, "core/v1/secrets", newMockResolver())

	if f.MatchesAPIResource("v1", "pods") {
		t.Error("pods should not match secrets filter")
	}

	if f.MatchesAPIResource("apps/v1", "secrets") {
		t.Error("apps/v1/secrets should not match core/v1/secrets")
	}
}

func TestGetDisabledResources_ReturnsCopy(t *testing.T) {
	f := mustNewFilter(t, "core/v1/secrets", newMockResolver())
	got := f.GetDisabledResources()
	got[0] = "modified"

	if f.GetDisabledResources()[0] == "modified" {
		t.Error("GetDisabledResources should return a copy")
	}
}

func TestNewFilter_NilResolverReturnsError(t *testing.T) {
	_, err := NewFilter("secrets", nil)
	if err == nil {
		t.Error("expected error when resolver is nil")
	}
}

func TestNewFilter_NonCoreShortName(t *testing.T) {
	f := mustNewFilter(t, "deploy", newMockResolver())
	if !f.HasDisabledResources() {
		t.Fatal("expected short name 'deploy' to resolve")
	}
	gvr := schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}
	if !f.IsDisabled(gvr) {
		t.Error("expected 'deploy' to disable deployments")
	}
}
