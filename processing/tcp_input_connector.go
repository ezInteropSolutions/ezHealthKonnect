// processing/tcp_input_connector.go
// TCP input connector stub for Go processing engine

package processing

import (
	"context"
	"fmt"
	"time"
)

// TCPInputConnector implements InputConnector for TCP connections
type TCPInputConnector struct {
	host         string
	port         int
	messageCount int64
	errorCount   int64
	lastActivity time.Time
}

// NewTCPInputConnector creates a new TCP input connector
func NewTCPInputConnector(config map[string]interface{}) (InputConnector, error) {
	host, ok := config["host"].(string)
	if !ok {
		return nil, fmt.Errorf("TCP host not specified")
	}

	port, ok := config["port"].(float64) // JSON numbers come as float64
	if !ok {
		return nil, fmt.Errorf("TCP port not specified")
	}

	connector := &TCPInputConnector{
		host:         host,
		port:         int(port),
		lastActivity: time.Now(),
	}

	fmt.Printf("✅ TCP input connector initialized: %s:%d\n", host, int(port))
	return connector, nil
}

// Start begins listening for TCP messages
func (t *TCPInputConnector) Start(ctx context.Context, messageChan chan<- Message) error {
	fmt.Printf("🔌 Starting TCP input connector: %s:%d\n", t.host, t.port)
	// TODO: Implement TCP server listening logic
	return nil
}

// TestConnection tests the TCP connection
func (t *TCPInputConnector) TestConnection() error {
	fmt.Printf("🔍 Testing TCP connection: %s:%d\n", t.host, t.port)
	// TODO: Implement TCP connection test
	return nil
}

// Close cleans up the TCP connector
func (t *TCPInputConnector) Close() error {
	fmt.Printf("🔌 TCP input connector closed: %s:%d\n", t.host, t.port)
	return nil
}

// GetStatus returns connector status
func (t *TCPInputConnector) GetStatus() ConnectorStatus {
	return ConnectorStatus{
		Type:         "TCP",
		Status:       "ready",
		LastActivity: t.lastActivity.Format(time.RFC3339),
		MessageCount: t.messageCount,
		ErrorCount:   t.errorCount,
	}
}