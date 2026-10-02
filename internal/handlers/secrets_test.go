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
	"io/fs"
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
		encrypted bool
		want      SecretAccessMode
	}{
		{"stdio", false, false, SecretAccessFile},
		{"sse", false, false, SecretAccessEncrypted},
		{"streamable-http", false, false, SecretAccessEncrypted},
		{"stdio", false, true, SecretAccessEncrypted},
		{"streamable-http", false, true, SecretAccessEncrypted},
		{"stdio", true, false, SecretAccessInsecure},
		{"streamable-http", true, false, SecretAccessInsecure},
		{"stdio", true, true, SecretAccessInsecure},
	}

	for _, tt := range tests {
		if got := SecretAccessModeFor(tt.transport, tt.insecure, tt.encrypted); got != tt.want {
			t.Errorf("SecretAccessModeFor(%q, insecure=%v, encrypted=%v) = %v, want %v", tt.transport, tt.insecure, tt.encrypted, got, tt.want)
		}
	}
}

func TestSecretHandlerToolsDependOnMode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		mode     SecretAccessMode
		want     []string
		optional []string // arguments the tool must not require
	}{
		{SecretAccessFile, []string{"save_secret_to_file"}, []string{"path", "namespace", "context"}},
		{SecretAccessEncrypted, []string{"get_secret_encrypted"}, []string{"namespace", "context"}},
		{SecretAccessInsecure, []string{}, nil},
	}

	for _, tt := range tests {
		h := &SecretHandler{mode: tt.mode, getSecret: fakeSecret()}
		tools := h.GetTools()
		if got := toolNames(tools); !slices.Equal(got, tt.want) {
			t.Errorf("mode %v tools = %v, want %v", tt.mode, got, tt.want)
		}
		for _, tool := range tools {
			for _, arg := range tt.optional {
				if slices.Contains(tool.Tool().InputSchema.Required, arg) {
					t.Errorf("%s requires %q, want it optional", tool.Tool().Name, arg)
				}
			}
		}
	}
}

type staticResolver struct{}

func (staticResolver) ResolveResourceType(string, string) (schema.GroupVersionResource, error) {
	return secrets.GVR, nil
}

type failingResolver struct{}

func (failingResolver) ResolveResourceType(string, string) (schema.GroupVersionResource, error) {
	return schema.GroupVersionResource{}, errors.New("discovery unavailable")
}

func TestSaveSecretToFile(t *testing.T) {
	t.Parallel()

	blocked, err := resourcefilter.NewFilter("secrets", staticResolver{})
	if err != nil {
		t.Fatal(err)
	}
	broken, err := resourcefilter.NewLazyFilter("secrets", failingResolver{})
	if err != nil {
		t.Fatal(err)
	}
	invalidBase64 := func(context.Context, string, string, string) (*unstructured.Unstructured, error) {
		return &unstructured.Unstructured{Object: map[string]any{
			"data": map[string]any{"password": "not base64!"},
		}}, nil
	}

	tests := []struct {
		name      string
		getSecret secretGetter
		filter    *resourcefilter.Filter
		args      map[string]any // merged over a valid request; nil deletes a key
		wantErr   []string       // substrings of the tool error; empty means success
		wantTemp  bool           // the value lands in a new folder in the temp directory
	}{
		{name: "writes the value"},
		{name: "requires name", args: map[string]any{"name": nil}, wantErr: []string{"name is required"}},
		{name: "requires key", args: map[string]any{"key": nil}, wantErr: []string{"key is required"}},
		{name: "writes to a private temp folder when no path is given", args: map[string]any{"path": nil}, wantTemp: true},
		{name: "rejects a relative path", args: map[string]any{"path": "relative/path"}, wantErr: []string{"absolute"}},
		{name: "lists the keys when the key is missing", args: map[string]any{"key": "pasword"}, wantErr: []string{`no key "pasword"`, "password, username"}},
		{name: "reports a value that is not base64", getSecret: invalidBase64, wantErr: []string{"not valid base64"}},
		{name: "honors disabled resources", filter: blocked, wantErr: []string{"disabled by configuration"}},
		{name: "fails closed when the filter cannot initialize", filter: broken, wantErr: []string{"resource filter could not be initialized", "discovery unavailable"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			path := filepath.Join(t.TempDir(), "db-password")
			args := map[string]any{"namespace": "prod", "name": "db", "key": "password", "path": path}
			for k, v := range tt.args {
				if v == nil {
					delete(args, k)
				} else {
					args[k] = v
				}
			}
			getSecret := tt.getSecret
			if getSecret == nil {
				getSecret = fakeSecret()
			}
			h := &SecretHandler{mode: SecretAccessFile, getSecret: getSecret, resourceFilter: tt.filter}

			text, isError := callTool(t, h.SaveSecretToFile, args)

			if strings.Contains(text, testSecretValue) {
				t.Fatalf("SaveSecretToFile response contains the secret value: %s", text)
			}
			if len(tt.wantErr) > 0 {
				if !isError {
					t.Fatalf("SaveSecretToFile succeeded with %s, want an error", text)
				}
				for _, want := range tt.wantErr {
					if !strings.Contains(text, want) {
						t.Errorf("SaveSecretToFile error = %q, want it to contain %q", text, want)
					}
				}
				if _, err := os.Stat(path); !errors.Is(err, fs.ErrNotExist) {
					t.Errorf("SaveSecretToFile wrote %q despite failing (stat error: %v)", path, err)
				}
				return
			}

			if isError {
				t.Fatalf("SaveSecretToFile error = %s, want success", text)
			}
			var resp struct {
				Path string `json:"path"`
			}
			if err := json.Unmarshal([]byte(text), &resp); err != nil {
				t.Fatalf("SaveSecretToFile response is not JSON: %v\n%s", err, text)
			}
			if tt.wantTemp {
				dir := filepath.Dir(resp.Path)
				t.Cleanup(func() { _ = os.RemoveAll(dir) })
				if filepath.Dir(dir) != filepath.Clean(os.TempDir()) {
					t.Errorf("SaveSecretToFile wrote %q, want a new folder inside %q", resp.Path, os.TempDir())
				}
				if info, err := os.Stat(dir); err != nil || info.Mode().Perm() != 0o700 {
					t.Errorf("temp folder %q stat = %v, %v, want permissions 700", dir, info, err)
				}
				path = resp.Path
			} else if resp.Path != path {
				t.Errorf("SaveSecretToFile path = %q, want %q", resp.Path, path)
			}
			got, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != testSecretValue {
				t.Errorf("file contents = %q, want %q", got, testSecretValue)
			}
		})
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
	validKey := string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der}))

	tests := []struct {
		name      string
		publicKey string
		wantErr   string
	}{
		{name: "encrypts the value to the key", publicKey: validKey},
		{name: "rejects a key that is not PEM", publicKey: "not a key", wantErr: "PEM"},
		{name: "requires a public key", publicKey: "", wantErr: "public_key is required"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := &SecretHandler{mode: SecretAccessEncrypted, getSecret: fakeSecret()}

			text, isError := callTool(t, h.GetSecretEncrypted, map[string]any{
				"namespace":        "prod",
				"name":             "db",
				"key":              "password",
				"public_key":       tt.publicKey,
				"private_key_path": "/tmp/k8s-secret-key.pem",
				"output_path":      "/home/user/db-password",
			})

			if strings.Contains(text, testSecretValue) {
				t.Fatalf("GetSecretEncrypted response contains the secret value: %s", text)
			}
			if tt.wantErr != "" {
				if !isError || !strings.Contains(text, tt.wantErr) {
					t.Fatalf("GetSecretEncrypted = (isError=%v) %s, want an error containing %q", isError, text, tt.wantErr)
				}
				return
			}
			if isError {
				t.Fatalf("GetSecretEncrypted error = %s, want success", text)
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

			// Every line of the command that decodes to one RSA block is an
			// encrypted block; decrypting them in order must give back the value.
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
		})
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
			mustHave:    []string{"save_secret_to_file", "hidden", "temp directory", "delete the file", "even when a script fails"},
			mustNotHave: []string{"get_secret_encrypted", "decode_base64"},
		},
		{
			mode:        SecretAccessEncrypted,
			mustHave:    []string{"get_secret_encrypted", "hidden", "openssl genrsa", "openssl rsa", "-pubout", "decrypt_command", "delete the decrypted file", "even when a script fails"},
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
