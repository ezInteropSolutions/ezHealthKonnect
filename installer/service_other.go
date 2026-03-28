//go:build !windows && !linux

package main

func registerService(_, _, _ string) error {
	emit("info", "Automatic service registration is not available on this platform.")
	emit("info", "Use Docker Desktop's startup settings to start containers on login.")
	return nil
}
