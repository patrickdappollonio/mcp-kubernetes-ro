package connectivity

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

// IsTransportError reports whether err is a pre-connection or transport-level
// failure: the request never reached the API server, or the TLS/network layer
// broke before a structured response was received. These errors are always
// unambiguously a connectivity or configuration problem.
//
// Use this at call sites that perform live API calls (list, get, logs, metrics,
// port-forward) where a structured API response like 403 Forbidden would be a
// legitimate application-level result, not a connectivity problem.
func IsTransportError(err error) bool {
	if err == nil {
		return false
	}

	// Context deadline - the request did not complete within its allowed time.
	// context.Canceled is intentionally excluded: it also fires when the MCP
	// client cancels a tool call (e.g. user presses stop), so treating it as a
	// connectivity failure would mislead the user.
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}

	// Unexpected EOF means the server dropped the connection mid-response
	// without sending a complete, parseable reply. Plain io.EOF is excluded
	// because it is the normal end-of-stream signal for log streaming.
	if errors.Is(err, io.ErrUnexpectedEOF) {
		return true
	}

	// Network-level errors. *url.Error is the outermost wrapper produced by
	// the HTTP transport and covers exec-credential failures (e.g. OIDC browser
	// flow errors), connection refused, DNS failures, and TLS handshake errors.
	var netErr *net.OpError
	var dnsErr *net.DNSError
	var urlErr *url.Error
	var tlsRecordErr tls.RecordHeaderError

	if errors.As(err, &netErr) || errors.As(err, &dnsErr) ||
		errors.As(err, &urlErr) || errors.As(err, &tlsRecordErr) {
		return true
	}

	// TLS / x509 certificate errors - can appear in OIDC scenarios when the
	// cluster CA or the OIDC provider certificate is invalid, expired, or
	// self-signed. These are caught separately because they may be unwrapped
	// from a *url.Error in some code paths.
	var certInvalidErr x509.CertificateInvalidError
	var hostnameErr x509.HostnameError
	var unknownAuthErr x509.UnknownAuthorityError

	if errors.As(err, &certInvalidErr) || errors.As(err, &hostnameErr) ||
		errors.As(err, &unknownAuthErr) {
		return true
	}

	return false
}

// IsAuthError reports whether err is a structured Kubernetes API-level
// authentication failure (HTTP 401). Unlike 403 Forbidden, which may mean the
// user is authenticated but simply lacks RBAC access to a specific resource, a
// 401 always means the credentials themselves are invalid or expired.
//
// Use this together with IsTransportError at call sites where hitting the API
// server at all requires valid credentials (e.g. discovery, resource filter
// initialisation) and a 401 is indistinguishable from a connectivity failure
// from the user's perspective.
func IsAuthError(err error) bool {
	return apierrors.IsUnauthorized(err)
}

// IsError reports whether err is either a transport-level connectivity failure
// or an API-level authentication error. It is a convenience combinator of
// IsTransportError and IsAuthError.
//
// Use this only at call sites where both kinds of error should be treated as
// "the cluster is unreachable or credentials are invalid", such as during API
// discovery or lazy resource filter initialisation. Prefer IsTransportError
// alone for call sites that perform ordinary resource reads, where a 401 or
// 403 may be a legitimate RBAC response rather than a connectivity problem.
func IsError(err error) bool {
	return IsTransportError(err) || IsAuthError(err)
}

// ErrorMessage returns a formatted message suitable for returning to an LLM
// via a tool error result. It includes the underlying error and explicit
// guidance not to retry the request automatically.
func ErrorMessage(err error) string {
	return fmt.Sprintf(
		"Failed to reach the Kubernetes cluster: %v\n\n"+
			"This appears to be a connectivity or authentication problem. "+
			"Do not retry this request automatically - instead, let the user know they may need to:\n"+
			"  • Refresh their credentials (e.g. re-run the OIDC browser login flow)\n"+
			"  • Verify the cluster endpoint is reachable from this machine\n"+
			"  • Check that their kubeconfig is valid and up to date",
		err,
	)
}
