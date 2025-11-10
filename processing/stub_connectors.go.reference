// processing/stub_connectors.go
// Stub implementations for missing connectors

package processing

import (
	"context"
	"fmt"
)

// File Input Connector Stub
type FileInputConnector struct{}

func NewFileInputConnector(config map[string]interface{}) (InputConnector, error) {
	return &FileInputConnector{}, nil
}

func (f *FileInputConnector) Start(ctx context.Context, messageChan chan<- Message) error {
	return fmt.Errorf("file input connector not implemented")
}

func (f *FileInputConnector) TestConnection() error {
	return fmt.Errorf("file input connector not implemented")
}

func (f *FileInputConnector) Close() error {
	return nil
}

func (f *FileInputConnector) GetStatus() ConnectorStatus {
	return ConnectorStatus{Type: "FILE", Status: "not_implemented"}
}

// Database Input Connector Stub
type DatabaseInputConnector struct{}

func NewDatabaseInputConnector(config map[string]interface{}) (InputConnector, error) {
	return &DatabaseInputConnector{}, nil
}

func (d *DatabaseInputConnector) Start(ctx context.Context, messageChan chan<- Message) error {
	return fmt.Errorf("database input connector not implemented")
}

func (d *DatabaseInputConnector) TestConnection() error {
	return fmt.Errorf("database input connector not implemented")
}

func (d *DatabaseInputConnector) Close() error {
	return nil
}

func (d *DatabaseInputConnector) GetStatus() ConnectorStatus {
	return ConnectorStatus{Type: "DATABASE", Status: "not_implemented"}
}

// REST Input Connector Stub
type RESTInputConnector struct{}

func NewRESTInputConnector(config map[string]interface{}) (InputConnector, error) {
	return &RESTInputConnector{}, nil
}

func (r *RESTInputConnector) Start(ctx context.Context, messageChan chan<- Message) error {
	return fmt.Errorf("REST input connector not implemented")
}

func (r *RESTInputConnector) TestConnection() error {
	return fmt.Errorf("REST input connector not implemented")
}

func (r *RESTInputConnector) Close() error {
	return nil
}

func (r *RESTInputConnector) GetStatus() ConnectorStatus {
	return ConnectorStatus{Type: "REST", Status: "not_implemented"}
}

// TCP Output Connector Stub
type TCPOutputConnector struct{}

func NewTCPOutputConnector(config map[string]interface{}) (OutputConnector, error) {
	return &TCPOutputConnector{}, nil
}

func (t *TCPOutputConnector) Send(ctx context.Context, message ProcessedMessage) error {
	return fmt.Errorf("TCP output connector not implemented")
}

func (t *TCPOutputConnector) TestConnection() error {
	return fmt.Errorf("TCP output connector not implemented")
}

func (t *TCPOutputConnector) Close() error {
	return nil
}

func (t *TCPOutputConnector) GetStatus() ConnectorStatus {
	return ConnectorStatus{Type: "TCP", Status: "not_implemented"}
}

// File Output Connector Stub
type FileOutputConnector struct{}

func NewFileOutputConnector(config map[string]interface{}) (OutputConnector, error) {
	return &FileOutputConnector{}, nil
}

func (f *FileOutputConnector) Send(ctx context.Context, message ProcessedMessage) error {
	return fmt.Errorf("file output connector not implemented")
}

func (f *FileOutputConnector) TestConnection() error {
	return fmt.Errorf("file output connector not implemented")
}

func (f *FileOutputConnector) Close() error {
	return nil
}

func (f *FileOutputConnector) GetStatus() ConnectorStatus {
	return ConnectorStatus{Type: "FILE", Status: "not_implemented"}
}

// Database Output Connector Stub
type DatabaseOutputConnector struct{}

func NewDatabaseOutputConnector(config map[string]interface{}) (OutputConnector, error) {
	return &DatabaseOutputConnector{}, nil
}

func (d *DatabaseOutputConnector) Send(ctx context.Context, message ProcessedMessage) error {
	return fmt.Errorf("database output connector not implemented")
}

func (d *DatabaseOutputConnector) TestConnection() error {
	return fmt.Errorf("database output connector not implemented")
}

func (d *DatabaseOutputConnector) Close() error {
	return nil
}

func (d *DatabaseOutputConnector) GetStatus() ConnectorStatus {
	return ConnectorStatus{Type: "DATABASE", Status: "not_implemented"}
}

// FHIR Output Connector is now implemented in fhir_output_connector.go