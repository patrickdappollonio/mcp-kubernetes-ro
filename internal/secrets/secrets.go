// Package secrets keeps Kubernetes Secret values out of the conversation with
// the model: it hides values in Secret objects, and retrieves one by writing it
// to a local file or by encrypting it to a public key held by the caller.
package secrets
