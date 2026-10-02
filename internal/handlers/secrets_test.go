package handlers

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1" //nolint:gosec // matches the OAEP hash used by secrets.Encrypt
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/patrickdappollonio/mcp-kubernetes-ro/internal/resourcefilter"
	"github.com/patrickdappollonio/mcp-kubernetes-ro/internal/secrets"
)

const testSecretValue = "s3cr3t-p@ssw0rd"

// fakeSecret returns a getter serving one Secret named "db" in namespace
// "prod" with a "password" key and a "username" key.
func fakeSecret() secretGetter {
	return func(_ context.Context, _, namespace, name string) (*unstructured.Unstructured, error) {
		if namespace != "prod" || name != "db" {
			return nil, errors.New(`secrets "` + name + `" not found`)
		}
		return &unstructured.Unstructured{Object: map[string]any{
			"kind": "Secret",
			"data": map[string]any{
				"password": base64.StdEncoding.EncodeToString([]byte(testSecretValue)),
				"username": base64.StdEncoding.EncodeToString([]byte("admin")),
			},
		}}, nil
	}
}

func callTool(t *testing.T, handler func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error), args map[string]any) (string, bool) {
	t.Helper()
	var req mcp.CallToolRequest
	req.Params.Arguments = args
	result, err := handler(t.Context(), req)
	if err != nil {
		t.Fatalf("handler returned a protocol error: %v", err)
	}
	text, ok := result.Content[0].(mcp.TextContent)
	if !ok {
		t.Fatalf("result content is %T, want mcp.TextContent", result.Content[0])
	}
	return text.Text, result.IsError
}

func toolNames(tools []MCPTool) []string {
	names := make([]string, 0, len(tools))
	for _, tool := range tools {
		names = append(names, tool.Tool().Name)
	}
	return names
}

func TestSecretAccessModeFor(t *testing.T) {
	t.Parallel()

	tests := []struct {
		transport string
		insecure  bool
		want      SecretAccessMode
	}{
		{"stdio", false, SecretAccessFile},
		{"sse", false, SecretAccessEncrypted},
		{"streamable-http", false, SecretAccessEncrypted},
		{"stdio", true, SecretAccessInsecure},
		{"streamable-http", true, SecretAccessInsecure},
	}

	for _, tt := range tests {
		if got := SecretAccessModeFor(tt.transport, tt.insecure); got != tt.want {
			t.Errorf("SecretAccessModeFor(%q, %v) = %v, want %v", tt.transport, tt.insecure, got, tt.want)
		}
	}
}

func TestSecretHandlerToolsDependOnMode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		mode SecretAccessMode
		want []string
	}{
		{SecretAccessFile, []string{"save_secret_to_file"}},
		{SecretAccessEncrypted, []string{"get_secret_encrypted"}},
		{SecretAccessInsecure, []string{}},
	}

	for _, tt := range tests {
		h := &SecretHandler{mode: tt.mode, getSecret: fakeSecret()}
		if got := toolNames(h.GetTools()); !slices.Equal(got, tt.want) {
			t.Errorf("mode %v tools = %v, want %v", tt.mode, got, tt.want)
		}
	}
}

func TestSaveSecretToFile(t *testing.T) {
	t.Parallel()

	h := &SecretHandler{mode: SecretAccessFile, getSecret: fakeSecret()}
	path := filepath.Join(t.TempDir(), "db-password")

	text, isError := callTool(t, h.SaveSecretToFile, map[string]any{
		"namespace": "prod", "name": "db", "key": "password", "path": path,
	})
	if isError {
		t.Fatalf("tool returned an error: %s", text)
	}

	if strings.Contains(text, testSecretValue) {
		t.Fatalf("tool response contains the secret value: %s", text)
	}
	if !strings.Contains(text, path) {
		t.Fatalf("tool response does not mention the file path: %s", text)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != testSecretValue {
		t.Fatalf("file contents = %q, want %q", got, testSecretValue)
	}
}

func TestSaveSecretToFileReportsWriteErrors(t *testing.T) {
	t.Parallel()

	h := &SecretHandler{mode: SecretAccessFile, getSecret: fakeSecret()}

	text, isError := callTool(t, h.SaveSecretToFile, map[string]any{
		"namespace": "prod", "name": "db", "key": "password", "path": "relative/path",
	})
	if !isError || !strings.Contains(text, "absolute") {
		t.Fatalf("expected an 'absolute' error, got isError=%v text=%s", isError, text)
	}
}

func TestSecretToolsListAvailableKeysWhenKeyIsMissing(t *testing.T) {
	t.Parallel()

	h := &SecretHandler{mode: SecretAccessFile, getSecret: fakeSecret()}

	text, isError := callTool(t, h.SaveSecretToFile, map[string]any{
		"namespace": "prod", "name": "db", "key": "pasword", "path": filepath.Join(t.TempDir(), "x"),
	})
	if !isError {
		t.Fatalf("expected an error, got: %s", text)
	}
	if !strings.Contains(text, "password, username") {
		t.Fatalf("error does not list the available keys: %s", text)
	}
	if strings.Contains(text, testSecretValue) {
		t.Fatalf("error contains the secret value: %s", text)
	}
}

func TestSecretToolsRequireParameters(t *testing.T) {
	t.Parallel()

	h := &SecretHandler{mode: SecretAccessFile, getSecret: fakeSecret()}

	for _, missing := range []string{"name", "key", "path"} {
		args := map[string]any{"namespace": "prod", "name": "db", "key": "password", "path": "/tmp/x"}
		delete(args, missing)

		text, isError := callTool(t, h.SaveSecretToFile, args)
		if !isError || !strings.Contains(text, missing+" is required") {
			t.Errorf("without %q: isError=%v text=%s", missing, isError, text)
		}
	}
}

type staticResolver struct{}

func (staticResolver) ResolveResourceType(string, string) (schema.GroupVersionResource, error) {
	return secrets.GVR, nil
}

func TestSecretToolsHonorDisabledResources(t *testing.T) {
	t.Parallel()

	filter, err := resourcefilter.NewFilter("secrets", staticResolver{})
	if err != nil {
		t.Fatal(err)
	}
	h := &SecretHandler{mode: SecretAccessFile, getSecret: fakeSecret(), resourceFilter: filter}
	path := filepath.Join(t.TempDir(), "x")

	text, isError := callTool(t, h.SaveSecretToFile, map[string]any{
		"namespace": "prod", "name": "db", "key": "password", "path": path,
	})
	if !isError || !strings.Contains(text, "disabled") {
		t.Fatalf("expected a disabled error, got isError=%v text=%s", isError, text)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("file was written even though secrets are disabled")
	}
}

func TestGetSecretEncrypted(t *testing.T) {
	t.Parallel()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	pub := string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der}))

	h := &SecretHandler{mode: SecretAccessEncrypted, getSecret: fakeSecret()}

	text, isError := callTool(t, h.GetSecretEncrypted, map[string]any{
		"namespace":        "prod",
		"name":             "db",
		"key":              "password",
		"public_key":       pub,
		"private_key_path": "/tmp/k8s-secret-key.pem",
		"output_path":      "/home/user/db-password",
	})
	if isError {
		t.Fatalf("tool returned an error: %s", text)
	}
	if strings.Contains(text, testSecretValue) {
		t.Fatalf("tool response contains the secret value: %s", text)
	}

	var got struct {
		DecryptCommand string `json:"decrypt_command"`
	}
	if err := json.Unmarshal([]byte(text), &got); err != nil {
		t.Fatalf("response is not JSON: %v\n%s", err, text)
	}
	if !strings.Contains(got.DecryptCommand, "'/tmp/k8s-secret-key.pem'") ||
		!strings.Contains(got.DecryptCommand, "'/home/user/db-password'") {
		t.Fatalf("decrypt command does not use the given paths:\n%s", got.DecryptCommand)
	}

	// Every long line of the command is an encrypted block; decrypting them
	// in order must give back the value.
	var plain []byte
	for line := range strings.Lines(got.DecryptCommand) {
		ct, err := base64.StdEncoding.DecodeString(strings.TrimSpace(line))
		if err != nil || len(ct) != key.PublicKey.Size() {
			continue
		}
		part, err := rsa.DecryptOAEP(sha1.New(), nil, key, ct, nil)
		if err != nil {
			t.Fatalf("decrypting block: %v", err)
		}
		plain = append(plain, part...)
	}
	if !bytes.Equal(plain, []byte(testSecretValue)) {
		t.Fatalf("decrypted value = %q, want %q", plain, testSecretValue)
	}
}

func TestGetSecretEncryptedRejectsBadPublicKey(t *testing.T) {
	t.Parallel()

	h := &SecretHandler{mode: SecretAccessEncrypted, getSecret: fakeSecret()}

	text, isError := callTool(t, h.GetSecretEncrypted, map[string]any{
		"namespace":        "prod",
		"name":             "db",
		"key":              "password",
		"public_key":       "not a key",
		"private_key_path": "/tmp/k.pem",
		"output_path":      "/tmp/out",
	})
	if !isError || !strings.Contains(text, "PEM") {
		t.Fatalf("expected a PEM error, got isError=%v text=%s", isError, text)
	}
}

func TestSecretAccessInstructions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		mode        SecretAccessMode
		mustHave    []string
		mustNotHave []string
	}{
		{
			mode:        SecretAccessFile,
			mustHave:    []string{"save_secret_to_file", "hidden"},
			mustNotHave: []string{"get_secret_encrypted", "decode_base64"},
		},
		{
			mode:        SecretAccessEncrypted,
			mustHave:    []string{"get_secret_encrypted", "hidden", "openssl genrsa", "openssl rsa", "-pubout", "decrypt_command"},
			mustNotHave: []string{"save_secret_to_file", "decode_base64"},
		},
		{
			mode:        SecretAccessInsecure,
			mustHave:    []string{"--insecure-secret-access", "conversation"},
			mustNotHave: []string{"save_secret_to_file", "get_secret_encrypted"},
		},
	}

	for _, tt := range tests {
		got := SecretAccessInstructions(tt.mode)
		for _, s := range tt.mustHave {
			if !strings.Contains(got, s) {
				t.Errorf("mode %v instructions missing %q:\n%s", tt.mode, s, got)
			}
		}
		for _, s := range tt.mustNotHave {
			if strings.Contains(got, s) {
				t.Errorf("mode %v instructions should not mention %q:\n%s", tt.mode, s, got)
			}
		}
	}
}

func TestSecretToolsReportInvalidBase64(t *testing.T) {
	t.Parallel()

	getter := func(context.Context, string, string, string) (*unstructured.Unstructured, error) {
		return &unstructured.Unstructured{Object: map[string]any{
			"data": map[string]any{"password": "not base64!"},
		}}, nil
	}
	h := &SecretHandler{mode: SecretAccessFile, getSecret: getter}
	path := filepath.Join(t.TempDir(), "x")

	text, isError := callTool(t, h.SaveSecretToFile, map[string]any{
		"namespace": "prod", "name": "db", "key": "password", "path": path,
	})
	if !isError || !strings.Contains(text, "not valid base64") {
		t.Fatalf("expected a base64 error, got isError=%v text=%s", isError, text)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("file was written for an invalid value")
	}
}

func TestSecretDataValue(t *testing.T) {
	t.Parallel()

	secret := &unstructured.Unstructured{Object: map[string]any{
		"data": map[string]any{
			"password": "czNjcjN0", // "s3cr3t"
			"username": "YWRtaW4=", // "admin"
			"broken":   "not base64!",
		},
	}}

	tests := []struct {
		name    string
		secret  *unstructured.Unstructured
		key     string
		want    string
		wantErr string
	}{
		{name: "decodes the value", secret: secret, key: "password", want: "s3cr3t"},
		{name: "missing key lists the keys", secret: secret, key: "token", wantErr: `secret "db" has no key "token"; available keys: broken, password, username`},
		{name: "invalid base64", secret: secret, key: "broken", wantErr: `secret key "broken" is not valid base64`},
		{name: "secret without data", secret: &unstructured.Unstructured{Object: map[string]any{}}, key: "password", wantErr: `secret "db" has no key "password"; available keys: `},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := secretDataValue(tt.secret, "db", tt.key)
			if tt.wantErr != "" {
				if err == nil || !strings.HasPrefix(err.Error(), tt.wantErr) {
					t.Fatalf("error = %v, want prefix %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if string(got) != tt.want {
				t.Fatalf("value = %q, want %q", got, tt.want)
			}
		})
	}
}

type failingResolver struct{}

func (failingResolver) ResolveResourceType(string, string) (schema.GroupVersionResource, error) {
	return schema.GroupVersionResource{}, errors.New("discovery unavailable")
}

func TestSecretToolsFailClosedWhenFilterCannotInitialize(t *testing.T) {
	t.Parallel()

	filter, err := resourcefilter.NewLazyFilter("secrets", failingResolver{})
	if err != nil {
		t.Fatal(err)
	}
	h := &SecretHandler{mode: SecretAccessFile, getSecret: fakeSecret(), resourceFilter: filter}
	path := filepath.Join(t.TempDir(), "x")

	text, isError := callTool(t, h.SaveSecretToFile, map[string]any{
		"namespace": "prod", "name": "db", "key": "password", "path": path,
	})
	if !isError || !strings.Contains(text, "resource filter could not be initialized") || !strings.Contains(text, "discovery unavailable") {
		t.Fatalf("expected a filter initialization error, got isError=%v text=%s", isError, text)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("file was written while the filter could not initialize")
	}
}
