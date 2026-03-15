package connectivity

import (
	"fmt"
	"strings"
)

// IsError reports whether err looks like a transient connectivity
// or authentication failure rather than an application-level error. This is
// used by tool handlers to decide whether to emit a "do not retry" message
// to the LLM when --always-start is in use and the cluster is unreachable.
func IsError(err error) bool {
	if err == nil {
		return false
	}

	s := strings.ToLower(err.Error())

	patterns := []string{
		// Network-level failures
		"connection refused",
		"connection reset",
		"connection timed out",
		"no such host",
		"i/o timeout",
		"dial tcp",
		"no route to host",
		"network is unreachable",
		// TLS / certificate errors
		"tls handshake",
		"x509:",
		"certificate",
		// Auth / token errors
		"unauthorized",
		"401 unauthorized",
		"403 forbidden",
		"token has expired",
		"id token",
		"oidc",
		"credentials",
		// Context / timeout
		"context deadline exceeded",
		// Unexpected EOF often signals the server dropped the connection
		"unexpected eof",
	}

	for _, p := range patterns {
		if strings.Contains(s, p) {
			return true
		}
	}

	return false
}

// ErrorMessage returns a formatted message suitable for returning to an LLM
// via a tool error result. It includes the underlying error and explicit
// guidance not to retry the request automatically.
func ErrorMessage(err error) string {
	return fmt.Sprintf(
		"Failed to reach the Kubernetes cluster: %v\n\n"+
			"This appears to be a connectivity or authentication problem. "+
			"Do not retry this request automatically — instead, let the user know they may need to:\n"+
			"  • Refresh their credentials (e.g. re-run the OIDC browser login flow)\n"+
			"  • Verify the cluster endpoint is reachable from this machine\n"+
			"  • Check that their kubeconfig is valid and up to date",
		err,
	)
}
