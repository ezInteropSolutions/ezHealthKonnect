// services/connectors/connector_helpers.go
// Shared utility functions used across multiple connector implementations.

package connectors

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
)

// parseJSON unmarshals raw JSON bytes into dest, returning a descriptive error on failure.
func parseJSON(data []byte, dest interface{}) error {
	if err := json.Unmarshal(data, dest); err != nil {
		return fmt.Errorf("invalid JSON config: %w", err)
	}
	return nil
}

// tlsConfigSkipVerify returns a TLS config that skips certificate verification.
// ONLY use this in development / testing environments.
func tlsConfigSkipVerify() *tls.Config {
	return &tls.Config{InsecureSkipVerify: true} //nolint:gosec
}
