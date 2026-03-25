package controllers

import (
	"database/sql"
	"net/http"
	"strconv"

	"ezhealthkonnect/models"
	"ezhealthkonnect/services"

	"github.com/gin-gonic/gin"
)

// GitController handles all /api/git/* routes.
// Pattern mirrors CodeTemplateController exactly.
type GitController struct {
	svc *services.GitRepoService
}

// NewGitController creates a GitController wired to the given DB and credential store.
func NewGitController(db *sql.DB, credStore *services.CredentialStore) *GitController {
	return &GitController{
		svc: services.NewGitRepoService(db, credStore),
	}
}

// RegisterRoutes mounts all /api/git/* routes.
// Static-prefix routes are registered BEFORE wildcards to avoid Gin radix-tree conflicts.
func (ctrl *GitController) RegisterRoutes(rg *gin.RouterGroup) {
	// Static-prefix routes — must come before /:id wildcard
	rg.GET("/export-preview/:interfaceId", ctrl.ExportPreview)

	// Repository CRUD
	rg.GET("", ctrl.List)
	rg.POST("", ctrl.Create)
	rg.GET("/:id", ctrl.Get)
	rg.PUT("/:id", ctrl.Update)
	rg.DELETE("/:id", ctrl.Delete)

	// Connection & sync
	rg.POST("/:id/test", ctrl.TestConnection)
	rg.POST("/:id/pull", ctrl.Pull)
	rg.POST("/:id/push", ctrl.Push)
	rg.GET("/:id/status", ctrl.Status)
	rg.GET("/:id/log", ctrl.Log)
	rg.GET("/:id/interface-diff", ctrl.InterfaceDiff)

	// Branch management
	rg.GET("/:id/branches", ctrl.ListBranches)
	rg.POST("/:id/branches", ctrl.CreateBranch)
	rg.POST("/:id/checkout", ctrl.Checkout)

	// Interface export
	rg.POST("/:id/commit-interface", ctrl.CommitInterface)
	rg.POST("/:id/commit-interfaces", ctrl.CommitInterfaces) // batch

	// Audit
	rg.GET("/:id/sync-log", ctrl.GetSyncLog)
}

// ── CRUD ──────────────────────────────────────────────────────────────────────

// List godoc
// GET /api/git
func (ctrl *GitController) List(c *gin.Context) {
	repos, err := ctrl.svc.List(c.Request.Context(), ctrl.userIDFromContext(c))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	if repos == nil {
		repos = []*models.GitRepository{} // never return null
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": repos, "count": len(repos)})
}

// Create godoc
// POST /api/git
func (ctrl *GitController) Create(c *gin.Context) {
	var req services.CreateGitRepoRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}
	if req.Name == "" || req.RemoteURL == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "name and remote_url are required"})
		return
	}

	createdBy := ctrl.userIDFromContext(c)
	repo, err := ctrl.svc.Create(c.Request.Context(), req, createdBy)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"success": true, "data": repo})
}

// Get godoc
// GET /api/git/:id
func (ctrl *GitController) Get(c *gin.Context) {
	repo, err := ctrl.svc.Get(c.Request.Context(), c.Param("id"), ctrl.userIDFromContext(c))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	if repo == nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": repo})
}

// Update godoc
// PUT /api/git/:id
func (ctrl *GitController) Update(c *gin.Context) {
	var req services.UpdateGitRepoRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}
	repo, err := ctrl.svc.Update(c.Request.Context(), c.Param("id"), req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": repo})
}

// Delete godoc
// DELETE /api/git/:id
func (ctrl *GitController) Delete(c *gin.Context) {
	if err := ctrl.svc.Delete(c.Request.Context(), c.Param("id")); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// ── Connection & sync ─────────────────────────────────────────────────────────

// TestConnection godoc
// POST /api/git/:id/test
func (ctrl *GitController) TestConnection(c *gin.Context) {
	if err := ctrl.svc.TestConnection(c.Request.Context(), c.Param("id")); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Connection successful"})
}

// Push godoc
// POST /api/git/:id/push
func (ctrl *GitController) Push(c *gin.Context) {
	pushed, err := ctrl.svc.Push(c.Request.Context(), c.Param("id"), ctrl.userIDFromContext(c))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	msg := "Already up to date"
	if pushed {
		msg = "Pushed to remote"
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "pushed": pushed, "message": msg})
}

// InterfaceDiff godoc
// GET /api/git/:id/interface-diff?interfaceId=xxx
func (ctrl *GitController) InterfaceDiff(c *gin.Context) {
	interfaceID := c.Query("interfaceId")
	if interfaceID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "interfaceId query param is required"})
		return
	}
	exportedBy := ""
	if uid := ctrl.userIDFromContext(c); uid != nil {
		exportedBy = *uid
	}
	result, err := ctrl.svc.InterfaceDiff(c.Request.Context(), c.Param("id"), interfaceID, exportedBy)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": result})
}

// Pull godoc
// POST /api/git/:id/pull
func (ctrl *GitController) Pull(c *gin.Context) {
	changed, err := ctrl.svc.Pull(c.Request.Context(), c.Param("id"), ctrl.userIDFromContext(c))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	msg := "Already up to date"
	if changed {
		msg = "Pulled new commits"
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "changed": changed, "message": msg})
}

// Status godoc
// GET /api/git/:id/status
func (ctrl *GitController) Status(c *gin.Context) {
	files, err := ctrl.svc.GetStatus(c.Request.Context(), c.Param("id"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	if files == nil {
		files = []models.GitStatusFile{}
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    files,
		"clean":   len(files) == 0,
	})
}

// Log godoc
// GET /api/git/:id/log?limit=20
func (ctrl *GitController) Log(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	commits, err := ctrl.svc.GetLog(c.Request.Context(), c.Param("id"), limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	if commits == nil {
		commits = []models.GitCommitSummary{}
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": commits, "count": len(commits)})
}

// ── Branch management ─────────────────────────────────────────────────────────

// ListBranches godoc
// GET /api/git/:id/branches
func (ctrl *GitController) ListBranches(c *gin.Context) {
	branches, err := ctrl.svc.ListBranches(c.Request.Context(), c.Param("id"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	current, _ := ctrl.svc.CurrentBranch(c.Request.Context(), c.Param("id"))
	c.JSON(http.StatusOK, gin.H{"success": true, "data": branches, "current": current})
}

// CreateBranch godoc
// POST /api/git/:id/branches   body: {"name":"feature/my-branch"}
func (ctrl *GitController) CreateBranch(c *gin.Context) {
	var body struct {
		Name string `json:"name"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || body.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "name is required"})
		return
	}
	if err := ctrl.svc.CreateBranch(c.Request.Context(), c.Param("id"), body.Name, ctrl.userIDFromContext(c)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Branch created: " + body.Name})
}

// Checkout godoc
// POST /api/git/:id/checkout   body: {"branch":"feature/my-branch"}
func (ctrl *GitController) Checkout(c *gin.Context) {
	var body struct {
		Branch string `json:"branch"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || body.Branch == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "branch is required"})
		return
	}
	if err := ctrl.svc.Checkout(c.Request.Context(), c.Param("id"), body.Branch, ctrl.userIDFromContext(c)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Checked out: " + body.Branch})
}

// ── Interface export ──────────────────────────────────────────────────────────

// CommitInterface godoc
// POST /api/git/:id/commit-interface
func (ctrl *GitController) CommitInterface(c *gin.Context) {
	var req services.CommitInterfaceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}
	if req.InterfaceID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "interface_id is required"})
		return
	}

	sha, err := ctrl.svc.CommitInterface(c.Request.Context(), c.Param("id"), req, ctrl.userIDFromContext(c))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success":    true,
		"commit_sha": sha,
		"message":    "Interface committed to git",
	})
}

// CommitInterfaces godoc
// POST /api/git/:id/commit-interfaces
// Commits multiple interfaces in a single git commit.
func (ctrl *GitController) CommitInterfaces(c *gin.Context) {
	var req services.CommitInterfacesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}
	if len(req.InterfaceIDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "interface_ids must not be empty"})
		return
	}

	sha, results, err := ctrl.svc.CommitInterfaces(c.Request.Context(), c.Param("id"), req, ctrl.userIDFromContext(c))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error(), "results": results})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success":    true,
		"commit_sha": sha,
		"results":    results,
		"message":    "Interfaces committed to git",
	})
}

// ExportPreview godoc
// GET /api/git/export-preview/:interfaceId
// Returns the bundle JSON without writing to git — useful for preview before committing.
func (ctrl *GitController) ExportPreview(c *gin.Context) {
	exportedBy := ""
	if uid := ctrl.userIDFromContext(c); uid != nil {
		exportedBy = *uid
	}
	bundle, err := ctrl.svc.ExportPreview(c.Request.Context(), c.Param("interfaceId"), exportedBy)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	if bundle == nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "interface not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": bundle})
}

// ── Audit ─────────────────────────────────────────────────────────────────────

// GetSyncLog godoc
// GET /api/git/:id/sync-log?limit=50
func (ctrl *GitController) GetSyncLog(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	entries, err := ctrl.svc.GetSyncLog(c.Request.Context(), c.Param("id"), limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	if entries == nil {
		entries = []*models.GitSyncLogEntry{}
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": entries, "count": len(entries)})
}

// ── Helpers ───────────────────────────────────────────────────────────────────

// userIDFromContext extracts the authenticated user ID from the Gin context.
// The Node.js proxy injects X-User-ID from the verified JWT session.
// Returns nil if not present (unauthenticated or not set by middleware).
func (ctrl *GitController) userIDFromContext(c *gin.Context) *string {
	// Primary: injected by Node.js proxy after JWT verification
	if h := c.GetHeader("X-User-ID"); h != "" {
		return &h
	}
	// Fallback: set directly by a Go middleware (future)
	if v, exists := c.Get("userID"); exists {
		if s, ok := v.(string); ok && s != "" {
			return &s
		}
	}
	return nil
}
