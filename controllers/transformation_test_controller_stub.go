package controllers

import (
	"database/sql"

	"github.com/gin-gonic/gin"
)

// Stub for Phase 1A/1B - Temporarily disabled

type TransformationTestController struct {
	db *sql.DB
}

func NewTransformationTestController(db *sql.DB) *TransformationTestController {
	return &TransformationTestController{db: db}
}

func (c *TransformationTestController) TestTransformation(ctx *gin.Context) {
	ctx.JSON(503, gin.H{"error": "Transformation test temporarily disabled"})
}

func (c *TransformationTestController) TestPipeline(ctx *gin.Context) {
	ctx.JSON(503, gin.H{"error": "Pipeline test temporarily disabled"})
}

func (c *TransformationTestController) GetPipeline(ctx *gin.Context) {
	ctx.JSON(503, gin.H{"error": "Pipeline test temporarily disabled"})
}
