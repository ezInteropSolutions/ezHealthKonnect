// controllers/mllp_interface_controller.go
// Controller for MLLP interface management and message processing

package controllers

import (
	"database/sql"
	"net/http"
	"strconv"
	"time"

	"ezhealthkonnect/services"

	"github.com/gin-gonic/gin"
)

// MLLPInterfaceController handles MLLP interface operations
type MLLPInterfaceController struct {
	mllpService *services.MLLPConnectivityService
	db          *sql.DB
}

// NewMLLPInterfaceController creates a new MLLP interface controller
func NewMLLPInterfaceController(db *sql.DB) *MLLPInterfaceController {
	return &MLLPInterfaceController{
		mllpService: services.NewMLLPConnectivityService(db),
		db:          db,
	}
}

// RegisterRoutes registers MLLP interface routes
func (controller *MLLPInterfaceController) RegisterRoutes(router *gin.RouterGroup) {
	mllp := router.Group("/mllp")
	{
		// Listener management
		mllp.POST("/listeners", controller.StartListener)
		mllp.GET("/listeners", controller.GetListeners)
		mllp.GET("/listeners/:id", controller.GetListener)
		mllp.DELETE("/listeners/:id", controller.StopListener)

		// Message handling
		mllp.POST("/send", controller.SendMessage)
		mllp.GET("/listeners/:id/messages", controller.GetMessages)

		// Health and status
		mllp.GET("/health", controller.GetHealth)
		mllp.GET("/stats", controller.GetStats)
	}
}

// StartListener starts a new MLLP listener
func (controller *MLLPInterfaceController) StartListener(c *gin.Context) {
	var request struct {
		Host              string `json:"host"`
		Port              int    `json:"port" binding:"required"`
		MaxConnections    int    `json:"maxConnections"`
		ReadTimeout       int    `json:"readTimeout"`       // seconds
		WriteTimeout      int    `json:"writeTimeout"`      // seconds
		MaxMessageSize    int    `json:"maxMessageSize"`
		EnableKeepAlive   bool   `json:"enableKeepAlive"`
		KeepAliveInterval int    `json:"keepAliveInterval"` // seconds
	}

	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   err.Error(),
			"message": "Invalid request payload",
		})
		return
	}

	// Create MLLP configuration
	config := &services.MLLPConfig{
		Host:           request.Host,
		Port:           request.Port,
		MaxConnections: request.MaxConnections,
		EnableKeepAlive: request.EnableKeepAlive,
	}

	// Set timeouts
	if request.ReadTimeout > 0 {
		config.ReadTimeout = time.Duration(request.ReadTimeout) * time.Second
	}
	if request.WriteTimeout > 0 {
		config.WriteTimeout = time.Duration(request.WriteTimeout) * time.Second
	}
	if request.MaxMessageSize > 0 {
		config.MaxMessageSize = request.MaxMessageSize
	}
	if request.KeepAliveInterval > 0 {
		config.KeepAliveInterval = time.Duration(request.KeepAliveInterval) * time.Second
	}

	// Start listener
	listener, err := controller.mllpService.StartListener(c.Request.Context(), config)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   err.Error(),
			"message": "Failed to start MLLP listener",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "MLLP listener started successfully",
		"listener": gin.H{
			"id":        listener.ID,
			"host":      listener.Host,
			"port":      listener.Port,
			"isActive":  listener.IsActive,
			"startTime": listener.StartTime,
		},
	})
}

// GetListeners returns all active MLLP listeners
func (controller *MLLPInterfaceController) GetListeners(c *gin.Context) {
	listeners := controller.mllpService.GetActiveListeners()

	response := make([]gin.H, len(listeners))
	for i, listener := range listeners {
		listener.ConnMutex.RLock()
		connectionCount := len(listener.Connections)
		listener.ConnMutex.RUnlock()

		response[i] = gin.H{
			"id":              listener.ID,
			"host":            listener.Host,
			"port":            listener.Port,
			"isActive":        listener.IsActive,
			"startTime":       listener.StartTime,
			"messageCount":    listener.MessageCount,
			"connectionCount": connectionCount,
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"success":   true,
		"listeners": response,
		"count":     len(listeners),
	})
}

// GetListener returns details for a specific MLLP listener
func (controller *MLLPInterfaceController) GetListener(c *gin.Context) {
	listenerID := c.Param("id")
	listeners := controller.mllpService.GetActiveListeners()

	for _, listener := range listeners {
		if listener.ID == listenerID {
			listener.ConnMutex.RLock()
			connections := make([]gin.H, 0, len(listener.Connections))
			for _, conn := range listener.Connections {
				connections = append(connections, gin.H{
					"id":           conn.ID,
					"remoteAddr":   conn.RemoteAddr,
					"startTime":    conn.StartTime,
					"lastActivity": conn.LastActivity,
					"messageCount": conn.MessageCount,
					"isActive":     conn.IsActive,
				})
			}
			listener.ConnMutex.RUnlock()

			c.JSON(http.StatusOK, gin.H{
				"success": true,
				"listener": gin.H{
					"id":           listener.ID,
					"host":         listener.Host,
					"port":         listener.Port,
					"isActive":     listener.IsActive,
					"startTime":    listener.StartTime,
					"messageCount": listener.MessageCount,
					"connections":  connections,
				},
			})
			return
		}
	}

	c.JSON(http.StatusNotFound, gin.H{
		"success": false,
		"error":   "Listener not found",
		"id":      listenerID,
	})
}

// StopListener stops a specific MLLP listener
func (controller *MLLPInterfaceController) StopListener(c *gin.Context) {
	listenerID := c.Param("id")

	if err := controller.mllpService.StopListener(listenerID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   err.Error(),
			"message": "Failed to stop MLLP listener",
			"id":      listenerID,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "MLLP listener stopped successfully",
		"id":      listenerID,
	})
}

// SendMessage sends an HL7 message via MLLP
func (controller *MLLPInterfaceController) SendMessage(c *gin.Context) {
	var request struct {
		Message        string `json:"message" binding:"required"`
		TargetEndpoint string `json:"targetEndpoint" binding:"required"`
		Config         struct {
			ReadTimeout    int `json:"readTimeout"`  // seconds
			WriteTimeout   int `json:"writeTimeout"` // seconds
			MaxMessageSize int `json:"maxMessageSize"`
		} `json:"config"`
	}

	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   err.Error(),
			"message": "Invalid request payload",
		})
		return
	}

	// Create HL7 message
	hl7Message := &services.HL7Message{
		ID:         "send_" + strconv.FormatInt(time.Now().UnixNano(), 10),
		Content:    request.Message,
		Source:     "api",
		ReceivedAt: time.Now(),
		Size:       len(request.Message),
	}

	// Create MLLP configuration
	config := &services.MLLPConfig{
		ReadTimeout:    30 * time.Second,
		WriteTimeout:   10 * time.Second,
		MaxMessageSize: 10 * 1024 * 1024, // 10MB
	}

	// Override with request config
	if request.Config.ReadTimeout > 0 {
		config.ReadTimeout = time.Duration(request.Config.ReadTimeout) * time.Second
	}
	if request.Config.WriteTimeout > 0 {
		config.WriteTimeout = time.Duration(request.Config.WriteTimeout) * time.Second
	}
	if request.Config.MaxMessageSize > 0 {
		config.MaxMessageSize = request.Config.MaxMessageSize
	}

	// Send message
	if err := controller.mllpService.SendMessage(c.Request.Context(), hl7Message, request.TargetEndpoint, config); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   err.Error(),
			"message": "Failed to send HL7 message",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":        true,
		"message":        "HL7 message sent successfully",
		"messageId":      hl7Message.ID,
		"targetEndpoint": request.TargetEndpoint,
		"sentAt":         time.Now(),
	})
}

// GetMessages returns messages received by a specific listener
func (controller *MLLPInterfaceController) GetMessages(c *gin.Context) {
	listenerID := c.Param("id")
	limitStr := c.DefaultQuery("limit", "100")

	limit, err := strconv.Atoi(limitStr)
	if err != nil || limit <= 0 {
		limit = 100
	}

	// Get message channel
	messageChan, err := controller.mllpService.GetListenerMessages(listenerID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   err.Error(),
			"message": "Listener not found",
		})
		return
	}

	// Collect messages from channel (non-blocking)
	messages := make([]*services.HL7Message, 0, limit)
	timeout := time.After(100 * time.Millisecond) // Short timeout for API response

collectLoop:
	for len(messages) < limit {
		select {
		case msg := <-messageChan:
			messages = append(messages, msg)
		case <-timeout:
			break collectLoop
		default:
			break collectLoop
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"success":    true,
		"listenerId": listenerID,
		"messages":   messages,
		"count":      len(messages),
		"limit":      limit,
	})
}

// GetHealth returns MLLP service health status
func (controller *MLLPInterfaceController) GetHealth(c *gin.Context) {
	listeners := controller.mllpService.GetActiveListeners()

	totalConnections := 0
	totalMessages := int64(0)

	for _, listener := range listeners {
		listener.ConnMutex.RLock()
		totalConnections += len(listener.Connections)
		listener.ConnMutex.RUnlock()
		totalMessages += listener.MessageCount
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"health": gin.H{
			"status":            "healthy",
			"activeListeners":   len(listeners),
			"totalConnections":  totalConnections,
			"totalMessages":     totalMessages,
			"timestamp":         time.Now(),
		},
	})
}

// GetStats returns detailed MLLP service statistics
func (controller *MLLPInterfaceController) GetStats(c *gin.Context) {
	listeners := controller.mllpService.GetActiveListeners()

	stats := gin.H{
		"listeners": gin.H{
			"total":  len(listeners),
			"active": 0,
		},
		"connections": gin.H{
			"total":  0,
			"active": 0,
		},
		"messages": gin.H{
			"total": int64(0),
		},
		"uptime": gin.H{},
	}

	var oldestStartTime *time.Time
	totalConnections := 0
	totalMessages := int64(0)

	for _, listener := range listeners {
		if listener.IsActive {
			stats["listeners"].(gin.H)["active"] = stats["listeners"].(gin.H)["active"].(int) + 1
		}

		listener.ConnMutex.RLock()
		connectionCount := len(listener.Connections)
		activeConnections := 0
		for _, conn := range listener.Connections {
			if conn.IsActive {
				activeConnections++
			}
		}
		listener.ConnMutex.RUnlock()

		totalConnections += connectionCount
		stats["connections"].(gin.H)["active"] = stats["connections"].(gin.H)["active"].(int) + activeConnections
		totalMessages += listener.MessageCount

		if oldestStartTime == nil || listener.StartTime.Before(*oldestStartTime) {
			oldestStartTime = &listener.StartTime
		}
	}

	stats["connections"].(gin.H)["total"] = totalConnections
	stats["messages"].(gin.H)["total"] = totalMessages

	if oldestStartTime != nil {
		stats["uptime"] = gin.H{
			"since":   *oldestStartTime,
			"seconds": int64(time.Since(*oldestStartTime).Seconds()),
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"stats":   stats,
	})
}