// processing/fhir_output_connector.go
// FHIR output connector that uses REST implementation for FHIR server communication

package processing

import (
	"context"
)

// FHIROutputConnector implements OutputConnector for FHIR servers
// It uses the REST connector internally since FHIR over HTTP is REST-based
type FHIROutputConnector struct {
	restConnector *RESTOutputConnector
}

// NewFHIROutputConnector creates a new FHIR output connector
func NewFHIROutputConnector(config map[string]interface{}) (OutputConnector, error) {
	// Convert FHIR config to REST config
	restConfig := make(map[string]interface{})

	// Copy all config values
	for key, value := range config {
		restConfig[key] = value
	}

	// Map FHIR-specific fields to REST fields
	if baseUrl, exists := config["baseUrl"]; exists {
		restConfig["endpoint"] = baseUrl
	}

	// Set default FHIR-specific headers if not present
	if _, exists := restConfig["headers"]; !exists {
		restConfig["headers"] = map[string]interface{}{
			"Content-Type": "application/fhir+json",
			"Accept":       "application/fhir+json",
		}
	}

	// Create underlying REST connector
	restConnector, err := NewRESTOutputConnector(restConfig)
	if err != nil {
		return nil, err
	}

	return &FHIROutputConnector{
		restConnector: restConnector.(*RESTOutputConnector),
	}, nil
}

// Send sends a FHIR message via HTTP REST
func (f *FHIROutputConnector) Send(ctx context.Context, message ProcessedMessage) error {
	return f.restConnector.Send(ctx, message)
}

// TestConnection tests the FHIR server connection
func (f *FHIROutputConnector) TestConnection() error {
	return f.restConnector.TestConnection()
}

// Close cleans up the FHIR connector
func (f *FHIROutputConnector) Close() error {
	return f.restConnector.Close()
}

// GetStatus returns FHIR connector status
func (f *FHIROutputConnector) GetStatus() ConnectorStatus {
	status := f.restConnector.GetStatus()
	status.Type = "FHIR"
	return status
}