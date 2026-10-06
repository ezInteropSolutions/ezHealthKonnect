// controllers/serial_port_controller.go
// SerialPortController exposes live COM-port enumeration — a genuinely new
// backend surface: before this feature, NOTHING in this codebase let a
// config field populate itself from a live backend call (ConnectorConfigBuilder.js
// only ever renders a <select> from a STATIC JSON-Schema enum baked into
// connectivity_types.config_schema at migration-authoring time). This only
// works under the confirmed "co-located process" deployment topology — the
// app process itself has direct OS access to the port list.
//
// Endpoint:
//
//	GET /api/connectivity/serial-ports → every COM port the OS currently sees
package controllers

import (
	"net/http"

	"go.bug.st/serial"

	"github.com/gin-gonic/gin"
)

// SerialPortController backs the "Connect a Device" wizard's and Pipeline
// Builder's own port-name picker for serial_inbound/serial_outbound.
type SerialPortController struct{}

// NewSerialPortController constructs the controller — stateless, no
// dependencies (serial.GetPortsList queries the OS directly each call).
func NewSerialPortController() *SerialPortController {
	return &SerialPortController{}
}

// ListSerialPorts returns every serial port name the OS currently exposes.
// An empty list is a normal, successful result (no error) — it just means
// no COM port is currently attached/visible to this process.
func (sc *SerialPortController) ListSerialPorts(c *gin.Context) {
	ports, err := serial.GetPortsList()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}
	if ports == nil {
		ports = []string{}
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"ports":   ports,
		"count":   len(ports),
	})
}
