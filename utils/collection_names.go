// FILE: utils/collection_names.go
// Utilities for generating standardized collection and table names

package utils

import (
	"fmt"
	"strings"
)

// SanitizeInterfaceID removes hyphens from UUID for use in collection/table names
// Converts: "762aebb9-0408-4a42-82c5-202f13f28315"
// To:       "762aebb9_0408_4a42_82c5_202f13f28315"
func SanitizeInterfaceID(interfaceID string) string {
	return strings.ReplaceAll(interfaceID, "-", "_")
}

// GetRawMessagesCollection returns standardized MongoDB collection name for raw messages
func GetRawMessagesCollection(interfaceID string) string {
	return fmt.Sprintf("raw_messages_%s", SanitizeInterfaceID(interfaceID))
}

// GetTransformedMessagesCollection returns standardized MongoDB collection name for transformed messages
func GetTransformedMessagesCollection(interfaceID string) string {
	return fmt.Sprintf("transformed_messages_intf_%s", SanitizeInterfaceID(interfaceID))
}

// GetProcessingLogsCollection returns standardized MongoDB collection name for processing logs
func GetProcessingLogsCollection(interfaceID string) string {
	return fmt.Sprintf("processing_logs_intf_%s", SanitizeInterfaceID(interfaceID))
}

// GetMessagesTable returns standardized PostgreSQL table name for interface messages
func GetMessagesTable(interfaceID string) string {
	return fmt.Sprintf("messages_intf_%s", SanitizeInterfaceID(interfaceID))
}

// GetArchivedMessagesCollection returns standardized MongoDB collection name for archived messages
func GetArchivedMessagesCollection(interfaceID string) string {
	return fmt.Sprintf("archived_messages_%s", SanitizeInterfaceID(interfaceID))
}
