package handlers

import (
	"context"
	"encoding/base64"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/patrickdappollonio/mcp-kubernetes-ro/internal/connectivity"
	"github.com/patrickdappollonio/mcp-kubernetes-ro/internal/kubernetes"
	"github.com/patrickdappollonio/mcp-kubernetes-ro/internal/resourcefilter"
	"github.com/patrickdappollonio/mcp-kubernetes-ro/internal/response"
	"github.com/patrickdappollonio/mcp-kubernetes-ro/internal/secrets"
)

// SecretAccessMode selects how Secret values can be retrieved.
type SecretAccessMode int

const (
	// SecretAccessFile hides Secret values and offers save_secret_to_file.
	// Used with stdio, where the server runs on the user's machine.
	SecretAccessFile SecretAccessMode = iota

	// SecretAccessEncrypted hides Secret values and offers get_secret_encrypted.
	// Used when the server cannot reach the user's disk: remotely or in a container.
	SecretAccessEncrypted

	// SecretAccessInsecure returns Secret values as-is from get_resource and
	// offers no secret tools. Enabled by --insecure-secret-access.
	SecretAccessInsecure
)

// Names of the secret tools, one per mode that hides Secret values.
const (
	saveSecretToFileTool   = "save_secret_to_file"
	getSecretEncryptedTool = "get_secret_encrypted"
)

// toolName returns the tool that retrieves a Secret value in mode m, or ""
// when Secret values are not hidden.
func (m SecretAccessMode) toolName() string {
	switch m {
	case SecretAccessFile:
		return saveSecretToFileTool
	case SecretAccessEncrypted:
		return getSecretEncryptedTool
	case SecretAccessInsecure:
		return ""
	default:
		return ""
	}
}

// SecretAccessModeFor returns the mode for a transport. insecure selects
// SecretAccessInsecure and wins over encrypted, which selects SecretAccessEncrypted.
func SecretAccessModeFor(transport string, insecure, encrypted bool) SecretAccessMode {
	switch {
	case insecure:
		return SecretAccessInsecure
	case encrypted:
		return SecretAccessEncrypted
	case transport == "stdio":
		return SecretAccessFile
	default:
		return SecretAccessEncrypted
	}
}

// secretGetter fetches a Secret object by Kubernetes context, namespace and name.
type secretGetter func(ctx context.Context, kubeContext, namespace, name string) (*unstructured.Unstructured, error)

// SecretHandler provides the tools that retrieve a single Secret value without
// returning it to the model.
type SecretHandler struct {
	mode           SecretAccessMode
	getSecret      secretGetter
	resourceFilter *resourcefilter.Filter
	alwaysStart    bool
}

// NewSecretHandler creates a SecretHandler for the given mode. In
// SecretAccessInsecure mode it provides no tools.
func NewSecretHandler(client *kubernetes.Client, filter *resourcefilter.Filter, alwaysStart bool, mode SecretAccessMode) *SecretHandler {
	return &SecretHandler{
		mode:           mode,
		resourceFilter: filter,
		alwaysStart:    alwaysStart,
		getSecret: func(ctx context.Context, kubeContext, namespace, name string) (*unstructured.Unstructured, error) {
			c, err := client.ForContext(kubeContext)
			if err != nil {
				return nil, fmt.Errorf("failed to create client with context %q: %w", kubeContext, err)
			}
			return c.GetResource(ctx, secrets.GVR, namespace, name)
		},
	}
}

// secretRef identifies one key of one Secret.
type secretRef struct {
	Namespace string `json:"namespace,omitempty"`
	Name      string `json:"name"`
	Key       string `json:"key"`
	Context   string `json:"context,omitempty"`
}

// SaveSecretToFileParams defines the parameters for the save_secret_to_file MCP tool.
type SaveSecretToFileParams struct {
	secretRef

	// Path is the absolute path of the new file to write the value to. When
	// empty, the value goes to a new owner-only folder in the temp directory.
	Path string `json:"path,omitempty"`
}

// GetSecretEncryptedParams defines the parameters for the get_secret_encrypted MCP tool.
type GetSecretEncryptedParams struct {
	secretRef

	// PublicKey is the PEM-encoded RSA public key to encrypt the value to.
	PublicKey string `json:"public_key"`

	// PrivateKeyPath is where the matching private key lives on the user's
	// machine. It is only used to fill in the returned decrypt command.
	PrivateKeyPath string `json:"private_key_path"`

	// OutputPath is where the decrypt command writes the value on the user's
	// machine. It is only used to fill in the returned decrypt command.
	OutputPath string `json:"output_path"`
}

// SaveSecretToFile implements the save_secret_to_file MCP tool: it writes one
// Secret value to a new owner-only file and returns only the path and size.
func (h *SecretHandler) SaveSecretToFile(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var params SaveSecretToFileParams
	if err := request.BindArguments(&params); err != nil {
		return response.Errorf("failed to parse arguments: %s", err)
	}

	value, errResult := h.secretValue(ctx, &params.secretRef)
	if errResult != nil {
		return errResult, nil
	}

	path := params.Path
	var err error
	if path == "" {
		path, err = secrets.WriteTempFile(params.Key, value)
	} else {
		err = secrets.WriteFile(path, value)
	}
	if err != nil {
		return response.Errorf("failed to write secret value: %v", err)
	}

	return response.JSON(map[string]any{
		"path":  path,
		"bytes": len(value),
		"note":  "The value was written to the file and is not included here. Have scripts read the file, never print it, and delete it once it is no longer needed.",
	})
}

// GetSecretEncrypted implements the get_secret_encrypted MCP tool: it returns one
// Secret value encrypted to the caller's public key, as a command that decrypts it.
func (h *SecretHandler) GetSecretEncrypted(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var params GetSecretEncryptedParams
	if err := request.BindArguments(&params); err != nil {
		return response.Errorf("failed to parse arguments: %s", err)
	}

	switch {
	case params.PublicKey == "":
		return response.Error("public_key is required")
	case params.PrivateKeyPath == "":
		return response.Error("private_key_path is required")
	case params.OutputPath == "":
		return response.Error("output_path is required")
	}

	value, errResult := h.secretValue(ctx, &params.secretRef)
	if errResult != nil {
		return errResult, nil
	}

	blocks, err := secrets.Encrypt(params.PublicKey, value)
	if err != nil {
		return response.Errorf("failed to encrypt secret value: %v", err)
	}

	return response.JSON(map[string]any{
		"bytes":           len(value),
		"blocks":          len(blocks),
		"decrypt_command": secrets.DecryptCommand(blocks, params.PrivateKeyPath, params.OutputPath),
		"note":            "Run decrypt_command exactly as given, in a POSIX shell on the user's machine. It writes the value to output_path with owner-only permissions and deletes the private key. Never print the decrypted value, and delete the file when it is no longer needed.",
	})
}

// secretValue validates ref, fetches the Secret and returns the decoded value
// of ref.Key. On failure it returns a tool error result instead.
func (h *SecretHandler) secretValue(ctx context.Context, ref *secretRef) ([]byte, *mcp.CallToolResult) {
	switch {
	case ref.Name == "":
		return nil, mcp.NewToolResultError("name is required")
	case ref.Key == "":
		return nil, mcp.NewToolResultError("key is required")
	}

	if blocked := h.secretsBlocked(); blocked != nil {
		return nil, blocked
	}

	secret, err := h.getSecret(ctx, ref.Context, ref.Namespace, ref.Name)
	if err != nil {
		if h.alwaysStart && connectivity.IsError(err) {
			return nil, mcp.NewToolResultError(connectivity.ErrorMessage(err))
		}
		return nil, mcp.NewToolResultError(fmt.Sprintf("failed to get secret %q: %v", ref.Name, err))
	}

	value, err := secretDataValue(secret, ref.Name, ref.Key)
	if err != nil {
		return nil, mcp.NewToolResultError(err.Error())
	}

	return value, nil
}

// secretsBlocked returns a tool error when --disabled-resources blocks Secrets
// or the filter could not initialize, and nil when Secrets may be read.
func (h *SecretHandler) secretsBlocked() *mcp.CallToolResult {
	if h.resourceFilter == nil || !h.resourceFilter.IsDisabled(secrets.GVR) {
		return nil
	}

	if initErr := h.resourceFilter.InitError(); initErr != nil {
		if h.alwaysStart && connectivity.IsError(initErr) {
			return mcp.NewToolResultError(connectivity.ErrorMessage(initErr))
		}
		return mcp.NewToolResultError(fmt.Sprintf("resource filter could not be initialized: %v", initErr))
	}

	return mcp.NewToolResultError(fmt.Sprintf("access to secrets (%s) is disabled by configuration", resourcefilter.FormatGVR(secrets.GVR)))
}

// secretDataValue returns the decoded value of key in a Secret's data. The
// error for a missing key lists the available keys.
func secretDataValue(secret *unstructured.Unstructured, name, key string) ([]byte, error) {
	data, _ := secret.Object["data"].(map[string]any)
	encoded, ok := data[key].(string)
	if !ok {
		keys := slices.Sorted(maps.Keys(data))
		return nil, fmt.Errorf("secret %q has no key %q; available keys: %s", name, key, strings.Join(keys, ", "))
	}

	value, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("secret key %q is not valid base64: %w", key, err)
	}

	return value, nil
}

func secretRefOptions() []mcp.ToolOption {
	return []mcp.ToolOption{
		mcp.WithString("name",
			mcp.Required(),
			mcp.Description("Secret name"),
		),
		mcp.WithString("key",
			mcp.Required(),
			mcp.Description("Key inside the Secret's data to retrieve. Use get_resource on the Secret to see the available keys."),
		),
		mcp.WithString("namespace",
			mcp.Description("Secret namespace. Defaults to the server's default namespace."),
		),
		mcp.WithString("context",
			mcp.Description("Kubernetes context to use (defaults to current context from kubeconfig)"),
		),
	}
}

// GetTools returns the secret tool for the handler's mode, or none for
// SecretAccessInsecure.
func (h *SecretHandler) GetTools() []MCPTool {
	switch h.mode {
	case SecretAccessFile:
		opts := append([]mcp.ToolOption{
			mcp.WithDescription("Write one Secret value to a file on the user's machine without returning the value. " +
				"Use this whenever a Secret value is needed, for example by a script; get_resource hides Secret values. " +
				"By default the file goes in a new private folder in the system temp directory, outside any project, and the response gives its path and size. " +
				"Pass the file to a script, for example DB_PASSWORD=\"$(cat <path>)\" ./script.sh, and delete the file when the script no longer needs it. " +
				"Never print, cat or echo the value, even when a script fails."),
			mcp.WithString("path",
				mcp.Description("Optional absolute path for the new file. Leave it empty to use a private temp folder. "+
					"The file is created with owner-only permissions (0600) and an existing file is never overwritten."),
			),
		}, secretRefOptions()...)
		return []MCPTool{NewMCPTool(mcp.NewTool(saveSecretToFileTool, opts...), h.SaveSecretToFile)}

	case SecretAccessEncrypted:
		opts := append([]mcp.ToolOption{
			mcp.WithDescription("Retrieve one Secret value encrypted to a public key, so the value never appears in the conversation. " +
				"Use this whenever a Secret value is needed, for example by a script; get_resource hides Secret values. " +
				"Steps, all on the user's machine with the built-in openssl: " +
				"(1) create a new one-time key pair: (umask 077; openssl genrsa -out /tmp/k8s-secret-key.pem 4096) " +
				"(2) print its public key: openssl rsa -in /tmp/k8s-secret-key.pem -pubout " +
				"(3) call this tool with that public key, the private key path and the output path " +
				"(4) run the returned decrypt_command exactly as given. It writes the value to output_path with owner-only permissions and deletes the private key. " +
				"Delete the decrypted file when the script no longer needs it. Never send the private key, and never print the decrypted value, even when a script fails."),
			mcp.WithString("public_key",
				mcp.Required(),
				mcp.Description("PEM-encoded RSA public key (at least 2048 bits), including the -----BEGIN PUBLIC KEY----- and -----END PUBLIC KEY----- lines"),
			),
			mcp.WithString("private_key_path",
				mcp.Required(),
				mcp.Description("Path of the matching private key on the user's machine. Only used to fill in the decrypt command; the server never reads it."),
			),
			mcp.WithString("output_path",
				mcp.Required(),
				mcp.Description("Path on the user's machine where the decrypt command writes the value. Must not exist yet."),
			),
		}, secretRefOptions()...)
		return []MCPTool{NewMCPTool(mcp.NewTool(getSecretEncryptedTool, opts...), h.GetSecretEncrypted)}

	case SecretAccessInsecure:
		return nil

	default:
		return nil
	}
}

// SecretAccessInstructions returns the SECRETS section of the server
// instructions for mode.
func SecretAccessInstructions(mode SecretAccessMode) string {
	const why = "Secret values are hidden: get_resource shows each key with its size, such as \"[redacted: 16 bytes]\", " +
		"because anything returned by a tool stays readable in the conversation history and in client logs, " +
		"and base64 is an encoding, not encryption.\n"

	switch mode {
	case SecretAccessFile:
		return "SECRETS:\n" + why +
			"• When a Secret value is needed, for example by a script, call save_secret_to_file. " +
			"By default the server writes the value to a new owner-only folder in the system temp directory and returns only the file's path. " +
			"Pass an absolute path only if the file must go somewhere else.\n" +
			"• Have scripts read the file, for example DB_PASSWORD=\"$(cat <path>)\" ./script.sh, and delete the file when the script no longer needs it.\n" +
			"• Never print, cat or echo the value into the conversation, even when a script fails."

	case SecretAccessEncrypted:
		return "SECRETS:\n" + why +
			"• This server cannot write to the user's machine (it runs remotely or in a container), so values are delivered encrypted to a key only the user's machine holds.\n" +
			"• To retrieve a value, run these steps in a shell on the user's machine using the built-in openssl:\n" +
			"  1. Create a new one-time key pair: (umask 077; openssl genrsa -out /tmp/k8s-secret-key.pem 4096)\n" +
			"  2. Print its public key: openssl rsa -in /tmp/k8s-secret-key.pem -pubout\n" +
			"  3. Call get_secret_encrypted with that public key, private_key_path=/tmp/k8s-secret-key.pem and the output_path to write.\n" +
			"  4. Run the returned decrypt_command exactly as given. It writes the value to output_path with owner-only permissions and deletes the private key.\n" +
			"• Use a new key pair for every value, and delete the decrypted file when the script no longer needs it. " +
			"Never send the private key, and never print, cat or echo the decrypted value, even when a script fails."

	case SecretAccessInsecure:
		return "SECRETS:\n" +
			"• This server runs with --insecure-secret-access, so get_resource returns Secret values as stored (base64-encoded, which is not encryption). " +
			"Any value retrieved stays readable in the conversation history and client logs. " +
			"Only read Secret values when the user needs them, and avoid repeating them."

	default:
		return ""
	}
}
