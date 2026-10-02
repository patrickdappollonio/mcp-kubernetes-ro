// Package secrets keeps Kubernetes Secret values out of the conversation with
// the model: it hides values in Secret objects and provides the two safe ways
// to retrieve one, writing it to a local file or encrypting it to a public key
// held by the caller.
package secrets
