package controllers

import (
	"database/sql"

	"github.com/gin-gonic/gin"
)

// Stub for Phase 1A/1B - Temporarily disabled

type MLLPInterfaceController struct {
	db *sql.DB
}

func NewMLLPInterfaceController(db *sql.DB) *MLLPInterfaceController {
	return &MLLPInterfaceController{db: db}
}

func (c *MLLPInterfaceController) StartInterface(ctx *gin.Context) {
	ctx.JSON(503, gin.H{"error": "MLLP interface temporarily disabled"})
}

func (c *MLLPInterfaceController) StopInterface(ctx *gin.Context) {
	ctx.JSON(503, gin.H{"error": "MLLP interface temporarily disabled"})
}

func (c *MLLPInterfaceController) GetStatus(ctx *gin.Context) {
	ctx.JSON(503, gin.H{"error": "MLLP interface temporarily disabled"})
}

func (c *MLLPInterfaceController) SendTestMessage(ctx *gin.Context) {
	ctx.JSON(503, gin.H{"error": "MLLP interface temporarily disabled"})
}

func (c *MLLPInterfaceController) RegisterRoutes(router *gin.RouterGroup) {
	// Stub - no routes registered
}
