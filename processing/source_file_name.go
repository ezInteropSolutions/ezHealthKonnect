package processing

import (
	"path"
	"strings"

	"ezhealthkonnect/models"
)

// sourceFileName is the name of the file a message came from, for a pipeline
// that needs it (message._sourceFile): the file listener's File-Name, an S3
// object's key, or an SFTP file's name. Empty when the message is not a file.
func sourceFileName(msg *models.InboundMessage) string {
	if v := msg.Headers["File-Name"]; v != "" {
		return v
	}
	if key := msg.Headers["S3-Key"]; key != "" {
		return path.Base(key)
	}
	return strings.TrimSpace(msg.SourceMetadata["sftp_filename"])
}
