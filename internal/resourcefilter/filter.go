package resourcefilter

import (
	"errors"
	"fmt"
	"strings"
	"sync"

	"k8s.io/apimachinery/pkg/runtime/schema"
)

// ResourceResolver resolves a user-friendly resource name and optional API version
// to a canonical GroupVersionResource. This allows the filter to accept singular names,
// kind names, or short names and resolve them to the correct plural resource name.
type ResourceResolver interface {
	ResolveResourceType(resourceType, apiVersion string) (schema.GroupVersionResource, error)
}

// Filter handles checking if Kubernetes resources are disabled by configuration.
// It can operate in two modes:
//
//   - Eager mode (NewFilter): names are resolved at construction time. Any error
//     (bad name, cluster unreachable) is returned immediately and the program
//     should fail fast.
//
//   - Lazy mode (NewLazyFilter): name resolution is deferred to the first call
//     to IsDisabled or MatchesAPIResource, using sync.Once. This is used with
//     --always-start so that the server can start without a live cluster
//     connection. If resolution fails at first use, InitError returns the error
//     and callers are expected to surface it to the LLM.
type Filter struct {
	// resolved state (populated at construction in eager mode, or on first use
	// in lazy mode)
	disabled []schema.GroupVersionResource
	raw      []string

	// lazy-init fields; all zero-valued in eager mode
	once     sync.Once
	rawInput string           // original comma-separated input, stored for deferred parsing
	resolver ResourceResolver // stored for deferred resolution
	initErr  error            // error from deferred resolution
}

// NewFilter creates a new Filter and eagerly resolves all resource specs against
// the cluster API. Any parse or resolution error is returned immediately.
//
// Each spec can be either:
//   - A resource name only (e.g., "secrets", "secret", "Secret", "po") which will be
//     resolved using the provided resolver against all API versions.
//   - A full group/version/resource triple (e.g., "core/v1/secrets", "apps/v1/deployments")
//     where "core" is an alias for the empty core API group.
//
// Returns an error if any spec cannot be parsed or resolved, since silently
// skipping a disabled resource is a security risk.
func NewFilter(value string, resolver ResourceResolver) (*Filter, error) {
	f := &Filter{}
	if err := f.resolve(value, resolver); err != nil {
		return nil, err
	}
	return f, nil
}

// NewLazyFilter creates a new Filter that defers name resolution to the first
// call to IsDisabled or MatchesAPIResource. This allows the server to start
// without a live cluster connection (--always-start mode).
//
// If the resolver is nil and value is non-empty, an error is returned
// immediately since there is no way to ever resolve the specs.
//
// Parse errors (malformed spec strings) are also returned immediately since
// they are not connectivity-dependent.
func NewLazyFilter(value string, resolver ResourceResolver) (*Filter, error) {
	if value == "" {
		return &Filter{}, nil
	}

	if resolver == nil {
		return nil, errors.New("cannot create lazy resource filter: no resource resolver provided")
	}

	// Validate token syntax eagerly (no API call needed) so malformed input
	// is caught at startup rather than silently at runtime.
	if err := validateTokenSyntax(value); err != nil {
		return nil, err
	}

	return &Filter{
		rawInput: value,
		resolver: resolver,
	}, nil
}

// validateTokenSyntax checks that every token in value is either a bare name
// or a "group/version/resource" triple. It does not make any API calls.
func validateTokenSyntax(value string) error {
	tokens := strings.FieldsFunc(value, isSeparator)
	for _, token := range tokens {
		parts := strings.Split(token, "/")
		switch len(parts) {
		case 1, 3:
			// valid formats
		default:
			return fmt.Errorf("invalid disabled resource spec %q: expected \"resource\" or \"group/version/resource\" format (e.g. secrets or core/v1/secrets)", token)
		}
	}
	return nil
}

// isSeparator reports whether r is a token separator character.
func isSeparator(r rune) bool {
	return r == ',' || r == ' ' || r == '\t' || r == '\n' || r == '\r'
}

// resolve parses value and calls the resolver for each token, populating
// f.disabled and f.raw. It is called directly by NewFilter (eager) and by
// sync.Once in lazy mode.
func (f *Filter) resolve(value string, resolver ResourceResolver) error {
	if value == "" {
		return nil
	}

	tokens := strings.FieldsFunc(value, isSeparator)

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
			return fmt.Errorf("invalid disabled resource spec %q: expected \"resource\" or \"group/version/resource\" format (e.g. secrets or core/v1/secrets)", token)
		}

		if resolver == nil {
			return fmt.Errorf("cannot validate disabled resource spec %q: no resource resolver available", token)
		}

		gvr, err := resolver.ResolveResourceType(resourceType, apiVersion)
		if err != nil {
			return fmt.Errorf("cannot resolve disabled resource spec %q: %w", token, err)
		}

		disabled = append(disabled, gvr)
		raw = append(raw, FormatGVR(gvr))
	}

	f.disabled = disabled
	f.raw = raw
	return nil
}

// ensureResolved triggers lazy resolution on first use. In eager mode the
// sync.Once func is never set, so this is a no-op there.
func (f *Filter) ensureResolved() {
	if f.rawInput == "" {
		// Either eager mode (already resolved) or empty input — nothing to do.
		return
	}
	f.once.Do(func() {
		f.initErr = f.resolve(f.rawInput, f.resolver)
	})
}

// InitError returns the error from deferred resolution, if any. Returns nil
// in eager mode or when lazy resolution has not yet been attempted or
// succeeded. Callers should check this after IsDisabled/MatchesAPIResource
// returns false to distinguish "not disabled" from "filter not yet ready".
//
// In practice, the ResourceHandler checks this at the start of each tool
// call so it can surface a meaningful connectivity error to the LLM.
func (f *Filter) InitError() error {
	if f == nil {
		return nil
	}
	// Don't trigger resolution just to check the error — only report what
	// has already been attempted.
	return f.initErr
}

// IsDisabled checks if a given GroupVersionResource matches any disabled resource.
// In lazy mode, this triggers resolution on first call.
// If resolution has failed, this returns true (fail-closed: block all access
// until the filter can be successfully initialized).
func (f *Filter) IsDisabled(gvr schema.GroupVersionResource) bool {
	f.ensureResolved()

	// Fail-closed: if we couldn't resolve the filter, block access.
	if f.initErr != nil {
		return true
	}

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
// "apps/v1" for apps group). In lazy mode, this triggers resolution on first call.
// If resolution has failed, this returns true (fail-closed).
func (f *Filter) MatchesAPIResource(groupVersion, resourceName string) bool {
	f.ensureResolved()

	// Fail-closed: if we couldn't resolve the filter, block access.
	if f.initErr != nil {
		return true
	}

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
// In lazy mode this is based on the raw input string so it works before
// resolution and without a cluster connection.
func (f *Filter) HasDisabledResources() bool {
	if f.rawInput != "" {
		// Lazy mode: count non-empty tokens without resolving
		return len(strings.FieldsFunc(f.rawInput, isSeparator)) > 0
	}
	return len(f.disabled) > 0
}

// GetDisabledResources returns the canonical string representations of disabled
// resources in group/version/resource format (using "core" for the empty core
// API group). In lazy mode, this triggers resolution — call only after
// IsDisabled/MatchesAPIResource has already been called, or after confirming
// InitError is nil.
func (f *Filter) GetDisabledResources() []string {
	f.ensureResolved()
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
