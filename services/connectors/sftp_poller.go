// services/connectors/sftp_poller.go
// Shared SFTP polling mechanics — SSH dial, auth, directory listing +
// glob matching, download-with-timeout, and archive/delete-after-processing
// — used by every SFTP-based inbound connector (sftp_inbound.go,
// edi_x12_inbound.go, and any future one).
//
// Extracted after a real duplication was found and confirmed by direct file
// comparison: edi_x12_inbound.go was built by "structurally mirroring"
// sftp_inbound.go (its own prior header comment said exactly that) — every
// piece of the actual SFTP protocol handling (Initialize's SSH client
// build, Validate, TestConnection, Start's poll loop, pollOnce, postProcess,
// dialSSH, buildAuthMethods) was copy-pasted with renamed fields
// (remote_path vs remote_dir, polling_interval_seconds vs
// poll_interval_sec) rather than shared. Only two package-level helpers
// (sftpDownloadWithTimeout, generateSFTPMessageID) were ever actually
// shared. This file is the fix: the two connectors now differ only in
// their own config key names/defaults and the SourceType/MessageType/log
// prefix they stamp onto each InboundMessage — real behavioral differences
// (edi_x12_inbound also validates transport=="sftp" as a phase-1
// capability boundary) stay in each connector's own file.
package connectors

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"path"
	"sort"
	"time"

	"ezhealthkonnect/models"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

// sftpPollerConfig holds every SFTP polling parameter common across
// SFTP-based inbound connectors. Each connector's own Initialize() builds
// one of these from its own config keys/defaults and keeps its own
// SourceType/MessageType/LogPrefix tagging here rather than duplicating the
// SSH/SFTP mechanics themselves.
type sftpPollerConfig struct {
	Host           string
	Port           int
	SSHConfig      *ssh.ClientConfig
	RemoteDir      string
	FilePattern    string
	MaxFilesPerRun int
	ConnectTimeout time.Duration
	ReadTimeout    time.Duration

	AfterProcessing string // "delete" | "archive" | "none"
	ArchiveDir      string

	// SourceType/MessageType are stamped onto every models.InboundMessage
	// this poller produces — the one piece of per-connector-type tagging
	// that differs (e.g. "sftp"/"" vs "edi_x12_sftp"/"EDI").
	SourceType  string
	MessageType string
	LogPrefix   string
}

// buildSFTPAuthMethods builds the SSH auth method list shared by every
// SFTP-based connector — password or a PEM private key.
func buildSFTPAuthMethods(authType, password, keyContent string) ([]ssh.AuthMethod, error) {
	switch authType {
	case "key":
		if keyContent == "" {
			return nil, fmt.Errorf("key_content is required when auth_type is 'key'")
		}
		signer, err := ssh.ParsePrivateKey([]byte(keyContent))
		if err != nil {
			return nil, fmt.Errorf("parse private key: %w", err)
		}
		return []ssh.AuthMethod{ssh.PublicKeys(signer)}, nil
	default:
		return []ssh.AuthMethod{ssh.Password(password)}, nil
	}
}

// dialSFTPSSH opens the underlying SSH connection an SFTP session runs over.
func dialSFTPSSH(ctx context.Context, host string, port int, sshConfig *ssh.ClientConfig, connectTimeout time.Duration) (*ssh.Client, error) {
	addr := fmt.Sprintf("%s:%d", host, port)
	d := &net.Dialer{Timeout: connectTimeout}
	netConn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("ssh tcp dial: %w", err)
	}
	sshConn, chans, reqs, err := ssh.NewClientConn(netConn, addr, sshConfig)
	if err != nil {
		_ = netConn.Close()
		return nil, fmt.Errorf("ssh handshake: %w", err)
	}
	return ssh.NewClient(sshConn, chans, reqs), nil
}

// testSFTPDirectory dials, opens an SFTP session, and confirms
// cfg.RemoteDir is accessible — the shared body of every SFTP-based
// connector's own TestConnection(). retryable distinguishes a
// connection-level failure (true — dial/handshake/SFTP-subsystem, worth
// retrying) from a configuration-level one (false — the directory itself
// doesn't exist/isn't accessible), matching each connector's own prior
// NewConnectorError retryable flags exactly.
func testSFTPDirectory(ctx context.Context, cfg sftpPollerConfig) (retryable bool, err error) {
	conn, err := dialSFTPSSH(ctx, cfg.Host, cfg.Port, cfg.SSHConfig, cfg.ConnectTimeout)
	if err != nil {
		return true, err
	}
	defer conn.Close()

	sftpClient, err := sftp.NewClient(conn)
	if err != nil {
		return true, fmt.Errorf("SFTP subsystem unavailable: %w", err)
	}
	defer sftpClient.Close()

	if _, err := sftpClient.Stat(cfg.RemoteDir); err != nil {
		return false, fmt.Errorf("remote directory %q not accessible: %w", cfg.RemoteDir, err)
	}
	return false, nil
}

// pollSFTPOnce connects, lists cfg.RemoteDir via the real SFTP protocol,
// downloads and delivers every file matching cfg.FilePattern (deterministic
// order, capped at cfg.MaxFilesPerRun), then post-processes each one — the
// shared body of every SFTP-based connector's own pollOnce(). recordReceived
// is called once per poll cycle that found at least one file (mirrors each
// connector's own IncrementMessagesReceived() call).
func pollSFTPOnce(ctx context.Context, cfg sftpPollerConfig, messageChan chan<- *models.InboundMessage, recordReceived func()) error {
	conn, err := dialSFTPSSH(ctx, cfg.Host, cfg.Port, cfg.SSHConfig, cfg.ConnectTimeout)
	if err != nil {
		return fmt.Errorf("ssh dial: %w", err)
	}
	defer conn.Close()

	sftpClient, err := sftp.NewClient(conn)
	if err != nil {
		return fmt.Errorf("open SFTP session: %w", err)
	}
	defer sftpClient.Close()

	entries, err := sftpClient.ReadDir(cfg.RemoteDir)
	if err != nil {
		return fmt.Errorf("list %s: %w", cfg.RemoteDir, err)
	}

	var names []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		matched, _ := path.Match(cfg.FilePattern, entry.Name())
		if matched {
			names = append(names, entry.Name())
		}
	}
	if len(names) == 0 {
		return nil
	}
	sort.Strings(names) // deterministic processing order
	if len(names) > cfg.MaxFilesPerRun {
		names = names[:cfg.MaxFilesPerRun]
	}

	log.Printf("%s found %d file(s) to process", cfg.LogPrefix, len(names))
	recordReceived()

	for _, name := range names {
		filePath := path.Join(cfg.RemoteDir, name)

		content, err := sftpDownloadWithTimeout(sftpClient, filePath, cfg.ReadTimeout)
		if err != nil {
			log.Printf("%s download error for %s: %v", cfg.LogPrefix, filePath, err)
			continue
		}

		msg := &models.InboundMessage{
			MessageID:      generateSFTPMessageID(filePath),
			Content:        content,
			SourceType:     cfg.SourceType,
			SourceEndpoint: fmt.Sprintf("%s:%d", cfg.Host, cfg.Port),
			MessageType:    cfg.MessageType,
			ReceivedAt:     time.Now(),
			SourceMetadata: map[string]string{
				"sftp_host":     cfg.Host,
				"sftp_path":     filePath,
				"sftp_filename": name,
			},
		}

		select {
		case messageChan <- msg:
		case <-ctx.Done():
			return ctx.Err()
		}

		if err := sftpPostProcess(sftpClient, filePath, name, cfg.AfterProcessing, cfg.ArchiveDir); err != nil {
			log.Printf("%s post-process error for %s: %v", cfg.LogPrefix, filePath, err)
		}
	}

	return nil
}

// sftpPostProcess moves or deletes the remote file after successful
// delivery, via the real SFTP protocol (MkdirAll + PosixRename for archive,
// Remove for delete — PosixRename rather than Rename so re-archiving a
// same-named file on a retry doesn't hard-fail if the destination already
// exists).
func sftpPostProcess(client *sftp.Client, filePath, filename, afterProcessing, archiveDir string) error {
	switch afterProcessing {
	case "delete":
		return client.Remove(filePath)
	case "archive":
		if err := client.MkdirAll(archiveDir); err != nil {
			return fmt.Errorf("mkdir %s: %w", archiveDir, err)
		}
		destPath := path.Join(archiveDir, filename)
		if err := client.PosixRename(filePath, destPath); err != nil {
			return fmt.Errorf("move %s -> %s: %w", filePath, destPath, err)
		}
		return nil
	default: // "none"
		return nil
	}
}

// runSFTPPollLoop starts the background ticker loop every SFTP-based
// connector's own Start() ran (byte-for-byte identical between
// sftp_inbound.go and edi_x12_inbound.go before this refactor) — polls once
// per tick until ctx is done or stopCh fires.
func runSFTPPollLoop(ctx context.Context, stopCh <-chan struct{}, pollInterval time.Duration, logPrefix string, poll func(ctx context.Context) error, onError func(error)) {
	go func() {
		ticker := time.NewTicker(pollInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-stopCh:
				return
			case <-ticker.C:
				if err := poll(ctx); err != nil {
					log.Printf("%s poll error: %v", logPrefix, err)
					onError(err)
				}
			}
		}
	}()
}

// sftpDownloadWithTimeout reads a remote file's full content via the real
// SFTP protocol, bounded by timeout (the blocking read runs in a goroutine
// so a hung connection is reported as a timeout rather than blocking
// forever).
func sftpDownloadWithTimeout(client *sftp.Client, remotePath string, timeout time.Duration) (string, error) {
	type result struct {
		content string
		err     error
	}
	done := make(chan result, 1)
	go func() {
		f, err := client.Open(remotePath)
		if err != nil {
			done <- result{"", fmt.Errorf("open %s: %w", remotePath, err)}
			return
		}
		defer f.Close()
		b, err := io.ReadAll(f)
		if err != nil {
			done <- result{"", fmt.Errorf("read %s: %w", remotePath, err)}
			return
		}
		done <- result{string(b), nil}
	}()

	select {
	case r := <-done:
		return r.content, r.err
	case <-time.After(timeout):
		return "", fmt.Errorf("download of %s timed out after %s", remotePath, timeout)
	}
}

// generateSFTPMessageID produces a reproducible message ID from the file path.
func generateSFTPMessageID(filePath string) string {
	ts := time.Now().UTC().Format("20060102150405")
	base := path.Base(filePath)
	return fmt.Sprintf("sftp_%s_%s", ts, base)
}
