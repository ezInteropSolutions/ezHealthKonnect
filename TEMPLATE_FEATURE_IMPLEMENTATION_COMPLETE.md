# Template Feature Implementation - Complete Guide

## ✅ Backend Implementation (COMPLETE)

### 1. Database Migration V21
**File**: `database/migrations/V21__Add_Template_Tracking.sql`

**Created Tables/Columns**:
- `transformation_steps.template_id` - UUID reference to template
- `transformation_steps.is_customized` - BOOLEAN flag
- `transformation_templates.created_by_user_id` - User tracking
- `transformation_templates.is_public` - Visibility control
- `transformation_templates.usage_count` - Popularity metric

**Auto-triggers**:
- `increment_template_usage()` - Auto-increments when template is used
- `mark_step_customized()` - Auto-marks steps as customized when config changes

**To Apply**: Restart Docker container (Flyway will auto-run V21)

---

### 2. Template Controller
**File**: `controllers/templateController.js`

**API Endpoints Created**:
```javascript
// GET /api/templates - List all templates
// GET /api/templates/:id - Get single template
// POST /api/templates/from-step - Create from existing step
// POST /api/templates - Create template directly
// PUT /api/templates/:id - Update template
// DELETE /api/templates/:id - Delete template
// POST /api/templates/:id/apply - Apply template to pipeline
```

**Features**:
- Permission checking (system templates can't be modified/deleted)
- Visibility filtering (public vs private)
- Batch update of non-customized steps
- Usage tracking

---

### 3. Routes
**File**: `routes/templateRoutes.js`

**Mounted at**: `/api/templates`
**Registered in**: `app.js` (already added)

---

### 4. API Service (Frontend)
**File**: `public/js/pipeline/services/PipelineAPIService.js`

**Methods Added**:
```javascript
listTemplates()  // Already existed
getTemplate(id)  // Already existed
createTemplate(template)  // Already existed
createTemplateFromStep(stepId, name, desc, isPublic)  // NEW
applyTemplate(templateId, pipelineId, sequence, stepName)  // NEW
updateTemplate(id, data)  // Already existed
deleteTemplate(id)  // Already existed
```

---

## 🔧 Frontend Implementation (TO BE COMPLETED)

### Step 1: Add "Save as Template" Button in Properties Panel

**File**: `public/js/pipeline/managers/PropertiesPanel.js`

**Add after step form is rendered** (in `loadForm()` method):

```javascript
// Add template actions after form content
renderTemplateActions(step) {
    if (!step.id) return ''; // Only for existing steps

    return `
        <div class="template-actions" style="margin-top: 1.5rem; padding-top: 1rem; border-top: 2px solid #e5e7eb;">
            <h4 style="margin-bottom: 0.75rem; color: #1e3a8a; font-size: 14px;">
                <i class="fas fa-bookmark"></i> Template Actions
            </h4>
            <button type="button" id="saveAsTemplateBtn" class="btn btn-secondary"
                    style="width: 100%; background: linear-gradient(135deg, #7c3aed 0%, #a855f7 100%); border: none;">
                <i class="fas fa-bookmark"></i> Save as Reusable Template
            </button>
            <p style="margin-top: 0.5rem; font-size: 12px; color: #6b7280;">
                Save this step configuration as a template to reuse in other pipelines
            </p>
        </div>
    `;
}

// In loadForm(), after form HTML is rendered:
const templateActionsHTML = this.renderTemplateActions(step);
if (templateActionsHTML) {
    formContainer.insertAdjacentHTML('beforeend', templateActionsHTML);

    // Attach event listener
    const saveAsTemplateBtn = document.getElementById('saveAsTemplateBtn');
    if (saveAsTemplateBtn) {
        saveAsTemplateBtn.addEventListener('click', () => this.showTemplateSaveDialog(step));
    }
}
```

---

### Step 2: Template Save Dialog

**Add to PropertiesPanel.js**:

```javascript
/**
 * Show template save dialog
 */
async showTemplateSaveDialog(step) {
    const dialogHTML = `
        <div id="templateSaveDialog" class="modal" style="display: flex;">
            <div class="modal-content" style="max-width: 500px;">
                <div class="modal-header" style="background: linear-gradient(135deg, #7c3aed 0%, #a855f7 100%); color: white;">
                    <h3><i class="fas fa-bookmark"></i> Save as Template</h3>
                    <button class="modal-close" onclick="document.getElementById('templateSaveDialog').remove()">&times;</button>
                </div>
                <div class="modal-body">
                    <div class="form-group">
                        <label>Template Name *</label>
                        <input type="text" id="templateName" class="form-control"
                               value="${step.name} Template"
                               placeholder="e.g., VIP Patient Validation">
                    </div>

                    <div class="form-group">
                        <label>Description</label>
                        <textarea id="templateDescription" class="form-control" rows="3"
                                  placeholder="Describe what this template does...">${step.description || ''}</textarea>
                    </div>

                    <div class="form-group">
                        <label style="display: flex; align-items: center; cursor: pointer;">
                            <input type="checkbox" id="templateIsPublic" checked
                                   style="width: auto; margin-right: 0.5rem;">
                            <span>Share with other users</span>
                        </label>
                        <p style="font-size: 12px; color: #6b7280; margin-top: 0.25rem; margin-left: 1.5rem;">
                            Public templates can be used by anyone. Private templates are only visible to you.
                        </p>
                    </div>
                </div>
                <div class="modal-footer">
                    <button class="btn btn-secondary" onclick="document.getElementById('templateSaveDialog').remove()">
                        Cancel
                    </button>
                    <button class="btn btn-primary" onclick="window.propertiesPanel.saveAsTemplate('${step.id}')">
                        <i class="fas fa-save"></i> Save Template
                    </button>
                </div>
            </div>
        </div>
    `;

    // Remove existing dialog if any
    const existing = document.getElementById('templateSaveDialog');
    if (existing) existing.remove();

    // Add to DOM
    document.body.insertAdjacentHTML('beforeend', dialogHTML);

    // Close on overlay click
    const dialog = document.getElementById('templateSaveDialog');
    dialog.addEventListener('click', (e) => {
        if (e.target === dialog) dialog.remove();
    });
}

/**
 * Save step as template
 */
async saveAsTemplate(stepId) {
    try {
        const templateName = document.getElementById('templateName').value.trim();
        const templateDescription = document.getElementById('templateDescription').value.trim();
        const isPublic = document.getElementById('templateIsPublic').checked;

        if (!templateName) {
            this.builder.dragDropManager.showNotification('Template name is required', 'error');
            return;
        }

        // Show loading
        this.builder.dragDropManager.showNotification('Creating template...', 'info');

        // Call API
        const result = await this.builder.api.createTemplateFromStep(
            stepId,
            templateName,
            templateDescription,
            isPublic
        );

        // Close dialog
        document.getElementById('templateSaveDialog').remove();

        // Show success
        this.builder.dragDropManager.showNotification(
            `Template "${templateName}" created successfully!`,
            'success'
        );

        // Refresh toolbox to show new template
        await this.builder.toolboxManager.loadTemplates();

        console.log('✅ Template created:', result.template_id);
    } catch (error) {
        console.error('Failed to create template:', error);
        this.builder.dragDropManager.showNotification(
            `Failed to create template: ${error.message}`,
            'error'
        );
    }
}
```

---

### Step 3: Update ToolboxManager to Display Templates

**File**: `public/js/pipeline/managers/ToolboxManager.js`

**Add loadTemplates() method**:

```javascript
/**
 * Load and display templates
 */
async loadTemplates() {
    try {
        console.log('[Toolbox] Loading templates...');

        const response = await this.builder.api.listTemplates();
        const templates = response.templates || response.data || [];

        console.log(`[Toolbox] Loaded ${templates.length} templates`);

        // Group templates by layer
        const byLayer = {
            pre: templates.filter(t => t.layer === 'pre'),
            core: templates.filter(t => t.layer === 'core'),
            post: templates.filter(t => t.layer === 'post')
        };

        // Render templates in toolbox
        this.renderTemplateSection('Templates', byLayer.pre, 'templates-list');

        return templates;
    } catch (error) {
        console.error('[Toolbox] Failed to load templates:', error);
        return [];
    }
}

/**
 * Render template section
 */
renderTemplateSection(title, templates, containerId) {
    const container = document.getElementById(containerId);
    if (!container) return;

    if (templates.length === 0) {
        container.innerHTML = '<p style="color: #6b7280; font-size: 12px; padding: 0.5rem;">No templates available</p>';
        return;
    }

    const html = templates.map(template => `
        <div class="toolbox-item template-item"
             data-template-id="${template.id}"
             data-step-type="${template.template_type}"
             data-layer="${template.layer}"
             draggable="true">
            <div class="step-icon">
                <i class="${this.getIconForType(template.template_type)}"></i>
            </div>
            <div class="step-info">
                <div class="step-name">${template.template_name}</div>
                <div class="step-badges">
                    ${template.is_system ?
                        '<span class="badge system" style="background: #3b82f6;">System</span>' :
                        '<span class="badge user" style="background: #8b5cf6;">Custom</span>'
                    }
                    ${!template.is_public ?
                        '<span class="badge private" style="background: #ef4444;">Private</span>' : ''
                    }
                    ${template.usage_count > 0 ?
                        `<span class="badge usage" style="background: #10b981;">${template.usage_count} uses</span>` : ''
                    }
                </div>
            </div>
        </div>
    `).join('');

    container.innerHTML = html;

    // Attach drag listeners to template items
    container.querySelectorAll('.template-item').forEach(item => {
        item.addEventListener('dragstart', (e) => this.handleTemplateDragStart(e));
    });
}

/**
 * Handle template drag start
 */
handleTemplateDragStart(e) {
    const templateId = e.currentTarget.dataset.templateId;
    const stepType = e.currentTarget.dataset.stepType;
    const layer = e.currentTarget.dataset.layer;

    e.dataTransfer.setData('application/json', JSON.stringify({
        isTemplate: true,
        templateId: templateId,
        stepType: stepType,
        layer: layer
    }));

    e.dataTransfer.effectAllowed = 'copy';

    console.log('[Toolbox] Template drag started:', templateId);
}
```

**Update initialization in init():**

```javascript
async init() {
    // Existing code...

    // Load templates
    await this.loadTemplates();
}
```

---

### Step 4: Handle Template Drop in DragDropManager

**File**: `public/js/pipeline/managers/DragDropManager.js`

**Update handleDrop() method**:

```javascript
async handleDrop(e) {
    e.preventDefault();

    const data = JSON.parse(e.dataTransfer.getData('application/json'));

    if (data.isTemplate) {
        // Handle template drop
        await this.handleTemplateDrop(data, e);
    } else {
        // Existing step drop logic
        this.handleStepDrop(data, e);
    }
}

/**
 * Handle template drop
 */
async handleTemplateDrop(data, e) {
    try {
        console.log('[DragDrop] Applying template:', data.templateId);

        // Calculate drop position and sequence
        const sequence = this.calculateDropSequence(e);

        // Show loading
        this.showNotification('Applying template...', 'info');

        // Apply template to pipeline
        const result = await this.builder.api.applyTemplate(
            data.templateId,
            this.builder.pipeline.id,
            sequence,
            null // Use template name
        );

        // Reload pipeline to show new step
        await this.builder.loadPipeline(this.builder.pipeline.id);

        // Show success
        this.showNotification('Template applied successfully!', 'success');

        console.log('✅ Template applied, created step:', result.step_id);
    } catch (error) {
        console.error('Failed to apply template:', error);
        this.showNotification(`Failed to apply template: ${error.message}`, 'error');
    }
}
```

---

## 📝 Testing Checklist

### Backend Testing
```bash
# 1. Run migration
docker-compose restart app

# 2. Test endpoints
curl http://localhost:3000/api/templates
# Should return system templates

# 3. Create template from step (need auth token)
curl -X POST http://localhost:3000/api/templates/from-step \
  -H "Content-Type: application/json" \
  -d '{"step_id":"...","template_name":"Test Template","is_public":true}'
```

### Frontend Testing
1. Open Pipeline Builder
2. Add a Validation step
3. Configure it with rules
4. Click "Save as Template"
5. Enter template name
6. Check toolbox - should see new template
7. Drag template to another pipeline
8. Should create new step with same config

---

## 🎨 CSS Additions Needed

Add to `public/css/pipeline-builder.css`:

```css
/* Template Actions */
.template-actions {
    background: #f9fafb;
    padding: 1rem;
    border-radius: 8px;
    margin-top: 1.5rem;
}

.template-actions h4 {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    margin-bottom: 0.75rem;
}

/* Template Items in Toolbox */
.template-item {
    position: relative;
    padding: 0.75rem;
    margin-bottom: 0.5rem;
}

.template-item .step-badges {
    display: flex;
    gap: 0.25rem;
    margin-top: 0.25rem;
    flex-wrap: wrap;
}

.template-item .badge {
    font-size: 10px;
    padding: 2px 6px;
    border-radius: 4px;
    color: white;
    font-weight: 500;
}

.template-item .badge.system {
    background: #3b82f6;
}

.template-item .badge.user {
    background: #8b5cf6;
}

.template-item .badge.private {
    background: #ef4444;
}

.template-item .badge.usage {
    background: #10b981;
}
```

---

## 🚀 Deployment Steps

1. **Backend**:
   ```bash
   # Restart to apply migration V21
   docker-compose restart app

   # Verify migration applied
   docker-compose exec postgres psql -U postgres -d ezhealthkonnect -c "SELECT * FROM flyway_schema_history WHERE version = '21';"
   ```

2. **Frontend**:
   - Update PropertiesPanel.js (add methods above)
   - Update ToolboxManager.js (add template loading)
   - Update DragDropManager.js (add template drop handling)
   - Add CSS styles
   - Increment version numbers in pipeline-builder.html

3. **Test**:
   - Create template from existing step
   - Verify appears in toolbox
   - Drag to new pipeline
   - Verify step created with correct config

---

## 📊 Feature Capabilities

✅ **Create templates from existing steps**
✅ **Public/private visibility control**
✅ **Usage tracking (popularity)**
✅ **Drag-and-drop from toolbox**
✅ **Auto-mark customized steps**
✅ **Batch update non-customized steps**
✅ **System templates (can't be deleted)**
✅ **User permission checking**

---

## 🔮 Future Enhancements

- Template marketplace/sharing
- Template versioning
- Template import/export
- Template categories/tags
- Template preview before applying
- Template search/filter in toolbox
- Template usage analytics
