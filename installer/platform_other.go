//go:build !windows && !linux

package main

import "errors"

// Standalone installation is supported on Windows and Linux only.

var errUnsupported = errors.New("ezHealthKonnect can be installed on Windows and Linux servers only")

type unsupportedPlatform struct{}

func newPlatform() Platform { return unsupportedPlatform{} }

func goosName() string { return "unsupported" }

func defaultInstallDir() string { return "/opt/ezhealthkonnect" }

func (unsupportedPlatform) Name() string              { return "unsupported" }
func (unsupportedPlatform) DefaultInstallDir() string { return defaultInstallDir() }
func (unsupportedPlatform) IsElevated() bool          { return false }
func (unsupportedPlatform) Prerequisites(*Config) []CheckResult {
	return []CheckResult{{Name: "Operating system", OK: false, Error: errUnsupported.Error()}}
}
func (unsupportedPlatform) EnsurePostgres(*Config, Reporter) (*PgSetup, error) {
	return nil, errUnsupported
}
func (unsupportedPlatform) PostgresAdmin(string, *Config) AdminSQL         { return nil }
func (unsupportedPlatform) GoAPIPath(dir string) string                    { return dir + "/go-api" }
func (unsupportedPlatform) ExtraEnv(string) map[string]string              { return nil }
func (unsupportedPlatform) PrepareInstall(string) error                    { return errUnsupported }
func (unsupportedPlatform) InstallService(string, *Config) error           { return errUnsupported }
func (unsupportedPlatform) StartService() error                            { return errUnsupported }
func (unsupportedPlatform) StopService() error                             { return nil }
func (unsupportedPlatform) RemoveService(string) error                     { return nil }
func (unsupportedPlatform) ServiceInstalled() bool                         { return false }
func (unsupportedPlatform) RegisterUninstall(string, string, string) error { return nil }
func (unsupportedPlatform) RemoveUninstall()                               {}
func (unsupportedPlatform) FreeBytes(string) (uint64, error)               { return 0, errUnsupported }

func embeddedBundle() []byte { return nil }

// PostgresMajorToInstall mirrors the supported platforms (shown in the UI only).
const PostgresMajorToInstall = 17
