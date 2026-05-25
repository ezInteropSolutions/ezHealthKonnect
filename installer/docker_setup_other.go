//go:build !windows && !linux

package main

import "fmt"

func installDockerDesktop() error {
	emitDocker("info", "Automatic Docker installation is not supported on macOS.")
	emitDocker("info", "Please install Docker Desktop from: https://www.docker.com/products/docker-desktop/")
	emitDocker("info", "After installing, start Docker Desktop, then return here and click Retry.")
	return fmt.Errorf("manual installation required on macOS")
}
