// Package portforward manages port-forwarding sessions to Kubernetes pods.
// It provides thread-safe lifecycle management for multiple concurrent port forwards,
// including automatic cleanup when connections drop or pods are deleted.
package portforward

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/portforward"
	"k8s.io/client-go/transport/spdy"
)

// PortMapping represents a single local-to-pod port mapping.
type PortMapping struct {
	// PodPort is the port on the pod to forward to.
	PodPort int `json:"pod_port"`

	// LocalPort is the local port to listen on. If 0, a free port is auto-assigned.
	LocalPort int `json:"local_port"`
}

// ForwardEntry holds metadata and control channels for one active port-forward session.
type ForwardEntry struct {
	// ID is the unique identifier for this port-forward session.
	ID string `json:"id"`

	// Namespace is the Kubernetes namespace of the target pod.
	Namespace string `json:"namespace"`

	// Pod is the name of the target pod.
	Pod string `json:"pod"`

	// Context is the Kubernetes context used for this session (empty means default).
	Context string `json:"context,omitempty"`

	// Ports contains the resolved port mappings (with actual local ports filled in).
	Ports []PortMapping `json:"ports"`

	// StartedAt is the time when this port-forward session was established.
	StartedAt time.Time `json:"started_at"`

	stopChan  chan struct{}
	forwarder *portforward.PortForwarder
}

// Manager tracks all active port-forward sessions with thread-safe access.
type Manager struct {
	mu       sync.Mutex
	forwards map[string]*ForwardEntry
	counter  atomic.Uint64
}

// NewManager creates a new empty Manager.
func NewManager() *Manager {
	return &Manager{
		forwards: make(map[string]*ForwardEntry),
	}
}

// Start establishes a port-forward session to a pod with the given port mappings.
// For any PortMapping with LocalPort == 0, a free port is automatically assigned.
// The returned ForwardEntry contains the resolved local ports.
func (m *Manager) Start(
	ctx context.Context,
	config *rest.Config,
	clientset kubernetes.Interface,
	namespace, pod string,
	ports []PortMapping,
	contextName string,
) (*ForwardEntry, error) {
	// Build the pod portforward subresource URL
	reqURL := clientset.CoreV1().RESTClient().Post().
		Resource("pods").
		Namespace(namespace).
		Name(pod).
		SubResource("portforward").
		URL()

	// Create SPDY transport and dialer
	transport, upgrader, err := spdy.RoundTripperFor(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create SPDY round tripper: %w", err)
	}

	dialer := spdy.NewDialer(upgrader, &http.Client{Transport: transport}, http.MethodPost, reqURL)

	// Resolve auto-assigned ports and build port strings
	resolvedPorts := make([]PortMapping, len(ports))
	portStrings := make([]string, len(ports))

	for i, p := range ports {
		localPort := p.LocalPort
		if localPort == 0 {
			assigned, err := findFreePort(ctx)
			if err != nil {
				return nil, fmt.Errorf("failed to find free port for pod port %d: %w", p.PodPort, err)
			}
			localPort = assigned
		}

		resolvedPorts[i] = PortMapping{
			PodPort:   p.PodPort,
			LocalPort: localPort,
		}
		portStrings[i] = fmt.Sprintf("%d:%d", localPort, p.PodPort)
	}

	// Create control channels
	stopChan := make(chan struct{})
	readyChan := make(chan struct{})

	// Create the port forwarder
	fw, err := portforward.New(dialer, portStrings, stopChan, readyChan, io.Discard, io.Discard)
	if err != nil {
		return nil, fmt.Errorf("failed to create port forwarder: %w", err)
	}

	// Run the forwarder in a background goroutine
	errChan := make(chan error, 1)
	go func() {
		errChan <- fw.ForwardPorts()
	}()

	// Wait for the forwarder to be ready or fail
	select {
	case <-readyChan:
		// Port forwarding is established
	case err := <-errChan:
		return nil, fmt.Errorf("port forwarding failed to start: %w", err)
	case <-time.After(10 * time.Second):
		close(stopChan)
		return nil, errors.New("port forwarding timed out waiting for ready signal")
	}

	// Retrieve the actual forwarded ports (handles :0 resolution by the forwarder)
	actualPorts, err := fw.GetPorts()
	if err == nil && len(actualPorts) == len(resolvedPorts) {
		for i, ap := range actualPorts {
			resolvedPorts[i].LocalPort = int(ap.Local)
			resolvedPorts[i].PodPort = int(ap.Remote)
		}
	}

	// Generate unique ID and store the entry
	id := fmt.Sprintf("pf-%d", m.counter.Add(1))
	entry := &ForwardEntry{
		ID:        id,
		Namespace: namespace,
		Pod:       pod,
		Context:   contextName,
		Ports:     resolvedPorts,
		StartedAt: time.Now(),
		stopChan:  stopChan,
		forwarder: fw,
	}

	m.mu.Lock()
	m.forwards[id] = entry
	m.mu.Unlock()

	// Background goroutine: auto-remove entry when the forwarder exits
	go func() {
		if err := <-errChan; err != nil {
			fmt.Fprintf(os.Stderr, "Port forward %s (%s/%s) terminated: %v\n", id, namespace, pod, err)
		} else {
			fmt.Fprintf(os.Stderr, "Port forward %s (%s/%s) terminated\n", id, namespace, pod)
		}
		m.mu.Lock()
		delete(m.forwards, id)
		m.mu.Unlock()
	}()

	return entry, nil
}

// Stop terminates a specific port-forward session by ID.
func (m *Manager) Stop(id string) error {
	m.mu.Lock()
	entry, ok := m.forwards[id]
	if !ok {
		m.mu.Unlock()
		return fmt.Errorf("port forward %q not found", id)
	}
	delete(m.forwards, id)
	m.mu.Unlock()

	close(entry.stopChan)
	return nil
}

// List returns a snapshot of all active port-forward sessions.
func (m *Manager) List() []*ForwardEntry {
	m.mu.Lock()
	defer m.mu.Unlock()

	entries := make([]*ForwardEntry, 0, len(m.forwards))
	for _, entry := range m.forwards {
		entries = append(entries, entry)
	}
	return entries
}

// StopAll terminates all active port-forward sessions.
// This is intended for graceful shutdown.
func (m *Manager) StopAll() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for id, entry := range m.forwards {
		close(entry.stopChan)
		delete(m.forwards, id)
	}
}

// findFreePort binds to :0 to discover a free port, then closes the listener.
func findFreePort(ctx context.Context) (int, error) {
	lc := net.ListenConfig{}
	listener, err := lc.Listen(ctx, "tcp", "127.0.0.1:0")
	if err != nil {
		return 0, fmt.Errorf("failed to bind to free port: %w", err)
	}
	defer func() { _ = listener.Close() }()

	addr, ok := listener.Addr().(*net.TCPAddr)
	if !ok {
		return 0, fmt.Errorf("unexpected address type: %T", listener.Addr())
	}
	return addr.Port, nil
}
