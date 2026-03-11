package kubernetes

import (
	"context"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic/fake"
	kubefake "k8s.io/client-go/kubernetes/fake"
)

// newTestClient creates a Client with fake clientset and dynamic client seeded
// with the given objects. namespace sets the default namespace on the client.
func newTestClient(namespace string, objects ...runtime.Object) *Client {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	cs := kubefake.NewSimpleClientset(objects...)
	dynClient := fake.NewSimpleDynamicClient(scheme, objects...)

	return &Client{
		clientset:       cs,
		discoveryClient: cs.Discovery(),
		dynamicClient:   dynClient,
		namespace:       namespace,
	}
}

func TestTestConnectivity_WithNamespace(t *testing.T) {
	client := newTestClient("my-ns",
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "my-ns"}},
	)

	if err := client.TestConnectivity(context.Background()); err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

func TestTestConnectivity_WithNamespace_NotFound(t *testing.T) {
	client := newTestClient("nonexistent")

	err := client.TestConnectivity(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if !strings.Contains(err.Error(), `failed to get namespace "nonexistent"`) {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func TestTestConnectivity_WithoutNamespace(t *testing.T) {
	client := newTestClient("",
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
	)

	if err := client.TestConnectivity(context.Background()); err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

func TestTestConnectivity_WithoutNamespace_NoPermissions(t *testing.T) {
	client := newTestClient("")

	// fake clientset returns empty list (not an error) when no namespaces exist,
	// so this should still succeed — the real RBAC error would come from the API server
	if err := client.TestConnectivity(context.Background()); err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

var podGVR = schema.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"}

func TestTestConnectivity_WithNamespace_ThenListPods(t *testing.T) {
	pod1 := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "web-abc123", Namespace: "my-ns"},
	}
	pod2 := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "worker-def456", Namespace: "my-ns"},
	}
	pod3 := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "other-pod", Namespace: "other-ns"},
	}

	client := newTestClient("my-ns",
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "my-ns"}},
		pod1, pod2, pod3,
	)

	ctx := context.Background()

	if err := client.TestConnectivity(ctx); err != nil {
		t.Fatalf("connectivity check failed: %v", err)
	}

	// List pods — should only return pods in "my-ns" since namespace is set
	result, err := client.ListResources(ctx, podGVR, "", metav1.ListOptions{})
	if err != nil {
		t.Fatalf("failed to list pods: %v", err)
	}

	if len(result.Items) != 2 {
		t.Fatalf("expected 2 pods in my-ns, got %d", len(result.Items))
	}

	names := map[string]bool{}
	for _, item := range result.Items {
		names[item.GetName()] = true
	}

	if !names["web-abc123"] || !names["worker-def456"] {
		t.Fatalf("unexpected pod names: %v", names)
	}
}

func TestTestConnectivity_WithoutNamespace_ThenListAllPods(t *testing.T) {
	pod1 := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "pod-a", Namespace: "ns1"},
	}
	pod2 := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "pod-b", Namespace: "ns2"},
	}

	client := newTestClient("",
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "ns1"}},
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "ns2"}},
		pod1, pod2,
	)

	ctx := context.Background()

	if err := client.TestConnectivity(ctx); err != nil {
		t.Fatalf("connectivity check failed: %v", err)
	}

	// List pods with no namespace — should return pods across all namespaces
	result, err := client.ListResources(ctx, podGVR, "", metav1.ListOptions{})
	if err != nil {
		t.Fatalf("failed to list pods: %v", err)
	}

	if len(result.Items) != 2 {
		t.Fatalf("expected 2 pods across all namespaces, got %d", len(result.Items))
	}
}

func TestTestConnectivity_WithNamespace_ThenGetPod(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "my-pod", Namespace: "my-ns"},
	}

	client := newTestClient("my-ns",
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "my-ns"}},
		pod,
	)

	ctx := context.Background()

	if err := client.TestConnectivity(ctx); err != nil {
		t.Fatalf("connectivity check failed: %v", err)
	}

	result, err := client.GetResource(ctx, podGVR, "", "my-pod")
	if err != nil {
		t.Fatalf("failed to get pod: %v", err)
	}

	if result.GetName() != "my-pod" {
		t.Fatalf("expected pod name 'my-pod', got %q", result.GetName())
	}

	if result.GetNamespace() != "my-ns" {
		t.Fatalf("expected namespace 'my-ns', got %q", result.GetNamespace())
	}
}
