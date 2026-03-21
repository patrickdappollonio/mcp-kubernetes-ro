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
	"strings"
	"testing"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	kubefake "k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

var (
	listAll = metav1.ListOptions{}
	getOpts = metav1.GetOptions{}
)

// podGR is a GroupResource used when constructing typed API errors.
var podGR = schema.GroupResource{Group: "", Resource: "pods"}

// injectError returns a fake clientset whose first reaction always returns
// the provided error. This proves that the apierrors predicates still work
// on *StatusError values after they travel through the fake client's reactor
// chain - just as they would from a real API server response.
func injectError(err error) *kubefake.Clientset {
	cs := kubefake.NewClientset()
	cs.PrependReactor("*", "*", func(action k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, err
	})
	return cs
}

// oidcExecErr simulates the error produced when an exec credential plugin
// (e.g. kubectl-oidc_login) fails during a token refresh. The HTTP transport
// always wraps such failures in a *url.Error, so IsTransportError must detect
// it via errors.As rather than string matching.
var oidcExecErr = &url.Error{
	Op:  "Get",
	URL: "https://cluster.example.com/api?timeout=32s",
	Err: errors.New(
		"getting credentials: exec: executable kubectl-oidc_login failed with exit code 1",
	),
}

// TestIsTransportError covers errors that are definitively pre-connection or
// transport-level failures. It also verifies that structured API-level errors
// (401, 403, 503 etc.) and normal end-of-stream signals are NOT caught here,
// so they can be handled (or passed through) accurately at the call site.
func TestIsTransportError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		// nil is never an error.
		{
			name: "nil",
			err:  nil,
			want: false,
		},

		// Plain application errors must not be misclassified.
		{
			name: "unrelated error",
			err:  fmt.Errorf("pod %q not found", "my-pod"),
			want: false,
		},

		// --- Structured Kubernetes API errors must NOT be caught ---
		// These are legitimate API responses; catching them as "connectivity
		// errors" would mislead the user into thinking they need to re-authenticate
		// when actually the cluster is reachable and the request itself failed.
		{
			name: "apierrors.IsUnauthorized - caught by IsAuthError, not here",
			err:  apierrors.NewUnauthorized("token has expired"),
			want: false,
		},
		{
			name: "apierrors.IsForbidden - valid RBAC denial, not a transport error",
			err:  apierrors.NewForbidden(podGR, "secrets", errors.New("not allowed")),
			want: false,
		},
		{
			name: "apierrors.IsServiceUnavailable - partial cluster failure, not transport",
			err:  apierrors.NewServiceUnavailable("metrics-server not ready"),
			want: false,
		},
		{
			name: "apierrors.IsNotFound - resource does not exist",
			err:  apierrors.NewNotFound(podGR, "my-pod"),
			want: false,
		},
		// Via fake client reactor - confirms the *StatusError survives the reactor chain.
		{
			name: "apierrors.IsForbidden via fake clientset reactor",
			err: func() error {
				cs := injectError(apierrors.NewForbidden(podGR, "secrets", errors.New("denied")))
				_, err := cs.CoreV1().Pods("default").List(context.Background(), listAll)
				return err
			}(),
			want: false,
		},
		{
			name: "apierrors.IsNotFound via fake clientset reactor",
			err: func() error {
				cs := injectError(apierrors.NewNotFound(podGR, "my-pod"))
				_, err := cs.CoreV1().Pods("default").Get(context.Background(), "my-pod", getOpts)
				return err
			}(),
			want: false,
		},

		// --- context.Canceled must NOT be caught ---
		// It also fires when the MCP client cancels a tool call (user presses stop),
		// so treating it as a connectivity failure would mislead the user.
		{
			name: "context.Canceled - user aborted, not a connectivity failure",
			err:  context.Canceled,
			want: false,
		},

		// --- io.EOF must NOT be caught ---
		// It is the normal end-of-stream signal for log streaming; treating it as
		// a transport error would misclassify a completed log stream.
		{
			name: "io.EOF - normal end of log stream",
			err:  io.EOF,
			want: false,
		},

		// --- Errors that ARE transport-level failures ---

		// Context deadline.
		{
			name: "context.DeadlineExceeded",
			err:  context.DeadlineExceeded,
			want: true,
		},
		{
			name: "context.DeadlineExceeded wrapped",
			err:  fmt.Errorf("operation failed: %w", context.DeadlineExceeded),
			want: true,
		},

		// Unexpected EOF: server dropped the connection mid-response.
		{
			name: "io.ErrUnexpectedEOF",
			err:  io.ErrUnexpectedEOF,
			want: true,
		},
		{
			name: "io.ErrUnexpectedEOF wrapped",
			err:  fmt.Errorf("reading response: %w", io.ErrUnexpectedEOF),
			want: true,
		},

		// Network-level errors.
		{
			name: "net.OpError - connection refused",
			err: &net.OpError{
				Op:  "dial",
				Net: "tcp",
				Err: errors.New("connection refused"),
			},
			want: true,
		},
		{
			name: "net.DNSError - no such host",
			err: &net.DNSError{
				Err:  "no such host",
				Name: "cluster.example.internal",
			},
			want: true,
		},
		{
			name: "url.Error - generic HTTP transport failure",
			err: &url.Error{
				Op:  "Get",
				URL: "https://cluster.example.com/api",
				Err: errors.New("connection refused"),
			},
			want: true,
		},
		{
			// Real-world OIDC exec-credential failure: surfaces as a *url.Error
			// wrapping a plain string from the exec plugin. Must be caught via
			// errors.As, not string matching.
			name: "url.Error - OIDC exec credential plugin failure",
			err:  oidcExecErr,
			want: true,
		},
		{
			name: "tls.RecordHeaderError - bad TLS record from server",
			err: tls.RecordHeaderError{
				Msg: "first record does not look like a TLS handshake",
			},
			want: true,
		},

		// x509 / certificate errors - can appear in OIDC scenarios when the
		// cluster CA or OIDC provider cert is invalid, expired, or self-signed.
		{
			name: "x509.UnknownAuthorityError - self-signed or unknown CA",
			err:  x509.UnknownAuthorityError{},
			want: true,
		},
		{
			name: "x509.CertificateInvalidError - expired certificate",
			err: x509.CertificateInvalidError{
				Reason: x509.Expired,
			},
			want: true,
		},
		{
			name: "x509.HostnameError - certificate hostname mismatch",
			err: x509.HostnameError{
				Host: "cluster.example.com",
			},
			want: true,
		},
		{
			name: "x509.UnknownAuthorityError wrapped in url.Error",
			err: &url.Error{
				Op:  "Get",
				URL: "https://cluster.example.com/api",
				Err: x509.UnknownAuthorityError{},
			},
			want: true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			got := IsTransportError(tt.err)
			if got != tt.want {
				t.Errorf("IsTransportError(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

// TestIsAuthError covers the single structured API error that IsAuthError
// recognises (HTTP 401). It also confirms that 403 Forbidden - a legitimate
// RBAC denial - is NOT treated as an auth error requiring re-authentication.
func TestIsAuthError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "nil",
			err:  nil,
			want: false,
		},
		{
			name: "unrelated error",
			err:  fmt.Errorf("some random error"),
			want: false,
		},
		// 401 Unauthorized always means credentials are invalid or expired.
		{
			name: "apierrors.IsUnauthorized - token expired",
			err:  apierrors.NewUnauthorized("token has expired"),
			want: true,
		},
		{
			// Via fake client reactor: confirms *StatusError survives the reactor chain.
			name: "apierrors.IsUnauthorized via fake clientset reactor",
			err: func() error {
				cs := injectError(apierrors.NewUnauthorized("bad token"))
				_, err := cs.CoreV1().Pods("default").List(context.Background(), listAll)
				return err
			}(),
			want: true,
		},
		// 403 Forbidden is a legitimate RBAC response, NOT an auth error.
		// The user is authenticated; they simply lack access to the resource.
		// Returning a connectivity/re-auth message here would mislead the user.
		{
			name: "apierrors.IsForbidden - valid RBAC denial, not an auth error",
			err:  apierrors.NewForbidden(podGR, "secrets", errors.New("not allowed")),
			want: false,
		},
		{
			name: "apierrors.IsForbidden via fake clientset reactor",
			err: func() error {
				cs := injectError(apierrors.NewForbidden(podGR, "secrets", errors.New("denied")))
				_, err := cs.CoreV1().Pods("default").List(context.Background(), listAll)
				return err
			}(),
			want: false,
		},
		// Other structured API errors are also not auth errors.
		{
			name: "apierrors.IsServiceUnavailable",
			err:  apierrors.NewServiceUnavailable("not ready"),
			want: false,
		},
		{
			name: "apierrors.IsNotFound",
			err:  apierrors.NewNotFound(podGR, "my-pod"),
			want: false,
		},
		// Transport errors are handled by IsTransportError, not here.
		{
			name: "net.OpError",
			err:  &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("refused")},
			want: false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			got := IsAuthError(tt.err)
			if got != tt.want {
				t.Errorf("IsAuthError(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

// TestIsError confirms that IsError is the union of IsTransportError and
// IsAuthError, and that errors which are false for both are also false for IsError.
func TestIsError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "nil", err: nil, want: false},
		{name: "unrelated error", err: fmt.Errorf("not found"), want: false},
		// Transport errors → true.
		{name: "net.OpError", err: &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("refused")}, want: true},
		{name: "context.DeadlineExceeded", err: context.DeadlineExceeded, want: true},
		{name: "io.ErrUnexpectedEOF", err: io.ErrUnexpectedEOF, want: true},
		{name: "url.Error (OIDC)", err: oidcExecErr, want: true},
		// Auth errors → true.
		{name: "apierrors.IsUnauthorized", err: apierrors.NewUnauthorized("expired"), want: true},
		// Errors that are neither → false, confirming no false positives.
		{name: "apierrors.IsForbidden - RBAC denial", err: apierrors.NewForbidden(podGR, "secrets", errors.New("no")), want: false},
		{name: "apierrors.IsServiceUnavailable", err: apierrors.NewServiceUnavailable("down"), want: false},
		{name: "apierrors.IsNotFound", err: apierrors.NewNotFound(podGR, "x"), want: false},
		{name: "context.Canceled - user abort", err: context.Canceled, want: false},
		{name: "io.EOF - normal log stream end", err: io.EOF, want: false},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			got := IsError(tt.err)
			if got != tt.want {
				t.Errorf("IsError(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

func TestErrorMessage_ContainsError(t *testing.T) {
	err := apierrors.NewUnauthorized("token has expired")
	msg := ErrorMessage(err)

	if msg == "" {
		t.Fatal("ErrorMessage returned empty string")
	}

	// The message must embed the underlying error text so the LLM has context.
	if !strings.Contains(msg, err.Error()) {
		t.Errorf("ErrorMessage does not contain underlying error text %q\ngot: %s", err.Error(), msg)
	}
}
