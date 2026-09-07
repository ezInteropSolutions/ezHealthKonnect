// services/connectors/sftp_uploader.go
// Shared SFTP upload mechanics — connection testing, dial+upload, and
// filename-pattern placeholder rendering — used by every SFTP-based
// outbound connector (sftp_outbound.go, edi_x12_outbound.go, and any
// future one). Reuses dialSFTPSSH/buildSFTPAuthMethods from sftp_poller.go
// (the inbound-side extraction) rather than re-duplicating SSH dial/auth a
// third time — those two functions have no inbound-specific knowledge at
// all, despite living in a file named for the poller.
//
// Extracted after the same duplication found on the inbound side turned
// out to exist here too — edi_x12_outbound.go's own
// ediSFTPUploadWithTimeout even said outright in its doc comment that it
// "mirrors sftp_outbound.go's own sftpUploadWithTimeout exactly", i.e. a
// knowing, deliberate copy kept only because both files share one package
// and the name would've collided.
package connectors

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"path"
	"strings"
	"time"

	"ezhealthkonnect/models"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

// testSFTPConnection dials and opens a real SFTP session — the shared body
// of every SFTP-based outbound connector's own TestConnection(). Unlike the
// inbound side's testSFTPDirectory, there's no directory to stat here (an
// outbound connector doesn't require the target directory to pre-exist —
// sftpUploadWithTimeout creates it via MkdirAll on send), so every failure
// here is connection-level and always retryable.
func testSFTPConnection(ctx context.Context, host string, port int, sshConfig *ssh.ClientConfig, connectTimeout time.Duration) error {
	conn, err := dialSFTPSSH(ctx, host, port, sshConfig, connectTimeout)
	if err != nil {
		return err
	}
	defer conn.Close()

	sftpClient, err := sftp.NewClient(conn)
	if err != nil {
		return fmt.Errorf("SFTP subsystem unavailable: %w", err)
	}
	defer sftpClient.Close()
	return nil
}

// sftpSendFile dials, opens an SFTP session, and uploads content to
// remotePath, bounded by writeTimeout — the shared body of every SFTP-based
// outbound connector's own Send().
func sftpSendFile(ctx context.Context, host string, port int, sshConfig *ssh.ClientConfig, connectTimeout time.Duration, remoteDir, remotePath string, content []byte, writeTimeout time.Duration) error {
	conn, err := dialSFTPSSH(ctx, host, port, sshConfig, connectTimeout)
	if err != nil {
		return fmt.Errorf("ssh dial: %w", err)
	}
	defer conn.Close()

	sftpClient, err := sftp.NewClient(conn)
	if err != nil {
		return fmt.Errorf("open SFTP session: %w", err)
	}
	defer sftpClient.Close()

	return sftpUploadWithTimeout(sftpClient, remoteDir, remotePath, content, writeTimeout)
}

// sftpUploadWithTimeout creates any missing subdirectories under remoteDir
// (filename_pattern can include them, e.g. "{interface_id}/{message_id}.hl7")
// and writes content to remotePath via the real SFTP protocol, bounded by
// timeout. The blocking SFTP calls run in a goroutine so a hung connection
// can still be reported as a timeout rather than blocking Send() forever.
func sftpUploadWithTimeout(client *sftp.Client, remoteDir, remotePath string, content []byte, timeout time.Duration) error {
	done := make(chan error, 1)
	go func() {
		if targetDir := path.Dir(remotePath); targetDir != "." && targetDir != remoteDir {
			if err := client.MkdirAll(targetDir); err != nil {
				done <- fmt.Errorf("mkdir -p %s: %w", targetDir, err)
				return
			}
		}

		f, err := client.Create(remotePath)
		if err != nil {
			done <- fmt.Errorf("create %s: %w", remotePath, err)
			return
		}
		defer f.Close()

		if _, err := io.Copy(f, bytes.NewReader(content)); err != nil {
			done <- fmt.Errorf("write %s: %w", remotePath, err)
			return
		}
		done <- nil
	}()

	select {
	case err := <-done:
		return err
	case <-time.After(timeout):
		return fmt.Errorf("SFTP upload timed out after %s", timeout)
	}
}

// renderFilenamePattern substitutes the {timestamp}/{date}/{time}/
// {message_id}/{interface_id} placeholders (same convention as
// aws_s3_outbound.go/azure_blob_outbound.go's key_pattern) and appends
// defaultExt when the rendered name has no extension of its own — shared by
// every SFTP-based outbound connector's own filename_pattern handling.
// defaultExt is a parameter, not hardcoded, because the two current callers
// genuinely differ here: sftp_outbound.go infers an extension from the
// message's own content type, while edi_x12_outbound.go always wants a
// fixed ".edi" regardless of content type — a real behavioral difference,
// not something to collapse into one shared default.
func renderFilenamePattern(pattern string, message *models.OutboundMessage, defaultExt string) string {
	name := pattern
	now := time.Now()

	replacements := map[string]string{
		"{timestamp}":    now.Format("20060102_150405"),
		"{date}":         now.Format("20060102"),
		"{time}":         now.Format("150405"),
		"{message_id}":   sanitizeObjectKeySegment(message.MessageID),
		"{interface_id}": sanitizeObjectKeySegment(message.InterfaceID),
	}
	for placeholder, value := range replacements {
		name = strings.ReplaceAll(name, placeholder, value)
	}

	if path.Ext(name) == "" {
		name += defaultExt
	}
	return name
}
