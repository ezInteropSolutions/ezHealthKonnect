/**
 * Drag & Drop Manager
 * Handles HTML5 drag-drop operations for pipeline builder
 */

class DragDropManager {
    constructor(pipelineBuilder) {
        this.builder = pipelineBuilder;
        this.draggedElement = null;
        this.draggedData = null;
        this.dragSource = null; // 'toolbox' or 'canvas'
        this.dropZones = [];

        this.init();
    }

    init() {
        this.setupDropZones();
    }

    /**
     * Reinitialize drop zones (called when view mode changes)
     */
    reinitialize() {
        // Clear existing drop zones
        this.dropZones = [];

        // Remove all existing drop zone listeners by cloning elements
        // (This is a simple way to remove all event listeners)
        const canvasWrapper = document.getElementById('canvasWrapper');
        if (canvasWrapper) {
            canvasWrapper.dataset.layer = '';
        }

        // Setup drop zones for current view mode
        this.setupDropZones();
    }

    /**
     * Setup drop zones (single canvas)
     */
    setupDropZones() {
        // Check if in flowchart mode
        const canvasWrapper = document.getElementById('canvasWrapper');
        if (canvasWrapper && this.builder.viewMode === 'flowchart') {
            console.log('📦 Setting up flowchart canvas as drop zone');
            canvasWrapper.dataset.layer = 'core';
            this.dropZones.push({ layer: 'core', element: canvasWrapper });
            this.setupDropZone(canvasWrapper);
            return;
        }

        // List view mode - single canvas drop zone
        const dropZone = document.getElementById('canvasDropZone');
        if (dropZone) {
            dropZone.dataset.layer = 'core';
            this.dropZones.push({ layer: 'core', element: dropZone });
            this.setupDropZone(dropZone);
        }
    }

    /**
     * Setup event listeners for drop zone
     */
    setupDropZone(dropZone) {
        dropZone.addEventListener('dragover', (e) => this.handleDragOver(e, dropZone));
        dropZone.addEventListener('dragleave', (e) => this.handleDragLeave(e, dropZone));
        dropZone.addEventListener('drop', (e) => this.handleDrop(e, dropZone));
    }

    /**
     * Make element draggable (from toolbox)
     */
    makeDraggable(element, data) {
        element.draggable = true;
        element.dataset.dragData = JSON.stringify(data);

        element.addEventListener('dragstart', (e) => {
            this.draggedElement = element;
            this.draggedData = data;
            this.dragSource = 'toolbox';

            element.classList.add('dragging');

            // Set drag data
            e.dataTransfer.effectAllowed = 'copy';
            e.dataTransfer.setData('application/json', JSON.stringify(data));

            // Create drag image
            if (data.type === 'template') {
                e.dataTransfer.setDragImage(element, 20, 20);
            }
        });

        element.addEventListener('dragend', (e) => {
            element.classList.remove('dragging');
            this.clearDragState();
        });
    }

    /**
     * Make step node draggable (in canvas)
     */
    makeStepNodeDraggable(element, step, groupId) {
        element.draggable = true;

        element.addEventListener('dragstart', (e) => {
            this.draggedElement = element;
            this.draggedData = {
                type: 'step-node',
                step: step,
                groupId: groupId,
                sourceLayer: step.layer
            };
            this.dragSource = 'canvas';

            element.classList.add('dragging');

            e.dataTransfer.effectAllowed = 'move';
            e.dataTransfer.setData('application/json', JSON.stringify(this.draggedData));
        });

        element.addEventListener('dragend', (e) => {
            element.classList.remove('dragging');
            this.clearDragState();
        });
    }

    /**
     * Handle drag over drop zone
     */
    handleDragOver(e, dropZone) {
        e.preventDefault();
        e.stopPropagation();

        if (!this.draggedData) {
            return;
        }

        // Check if drop is allowed
        const targetLayer = dropZone.dataset.layer;

        if (this.canDropInLayer(this.draggedData, targetLayer)) {
            e.dataTransfer.dropEffect = this.dragSource === 'canvas' ? 'move' : 'copy';
            dropZone.classList.add('drag-over');
        } else {
            e.dataTransfer.dropEffect = 'none';
        }

        // Container zone highlighting
        this.highlightContainerZone(e);
    }

    /**
     * Highlight container zone under cursor during drag
     */
    highlightContainerZone(e) {
        // Remove previous zone highlights
        document.querySelectorAll('.container-zone.drag-hover').forEach(z => {
            z.classList.remove('drag-hover');
        });

        // Find zone under cursor
        const zone = document.elementFromPoint(e.clientX, e.clientY)?.closest('.container-zone');
        if (zone) {
            zone.classList.add('drag-hover');
            this._hoveredZone = zone;
        } else {
            this._hoveredZone = null;
        }
    }

    /**
     * Handle drag leave drop zone
     */
    handleDragLeave(e, dropZone) {
        e.preventDefault();
        e.stopPropagation();

        // Only remove highlight if actually leaving (not entering child)
        const rect = dropZone.getBoundingClientRect();
        const x = e.clientX;
        const y = e.clientY;

        if (x < rect.left || x >= rect.right || y < rect.top || y >= rect.bottom) {
            dropZone.classList.remove('drag-over');
        }
    }

    /**
     * Handle drop in zone
     */
    handleDrop(e, dropZone) {
        e.preventDefault();
        e.stopPropagation();

        console.log('🎯 Drop event:', { draggedData: this.draggedData, dragSource: this.dragSource });

        dropZone.classList.remove('drag-over');

        if (!this.draggedData) {
            console.warn('⚠️ No dragged data on drop');
            return;
        }

        // Prevent double-processing if already handling a drop
        if (this.isProcessingDrop) {
            console.log('⏳ Already processing drop, ignoring duplicate event');
            return;
        }

        this.isProcessingDrop = true;

        const targetLayer = dropZone.dataset.layer;
        console.log('📍 Dropping in layer:', targetLayer);

        // Check if drop is allowed
        if (!this.canDropInLayer(this.draggedData, targetLayer)) {
            console.warn('❌ Drop not allowed in this layer');
            this.showNotification('Cannot drop this step in this layer', 'warning');
            this.isProcessingDrop = false;
            return;
        }

        // Check if dropping onto a container zone
        const containerZone = this._hoveredZone || document.elementFromPoint(e.clientX, e.clientY)?.closest('.container-zone');
        const containerInfo = containerZone ? {
            containerId: containerZone.dataset.containerId,
            zone: containerZone.dataset.zone
        } : null;

        // Clear zone highlights
        document.querySelectorAll('.container-zone.drag-hover').forEach(z => z.classList.remove('drag-hover'));
        this._hoveredZone = null;

        try {
            if (this.dragSource === 'toolbox') {
                console.log('📦 Handling toolbox drop', containerInfo ? `into container zone ${containerInfo.zone}` : '');
                this.handleToolboxDrop(this.draggedData, targetLayer, containerInfo);
            } else if (this.dragSource === 'canvas') {
                console.log('🎨 Handling canvas drop');
                this.handleCanvasDrop(this.draggedData, targetLayer);
            }
        } catch (error) {
            console.error('❌ Drop error:', error);
            this.showNotification('Failed to add step', 'error');
        }

        this.clearDragState();

        // Reset processing flag after a brief delay
        setTimeout(() => {
            this.isProcessingDrop = false;
        }, 100);
    }

    /**
     * Handle drop from toolbox
     */
    handleToolboxDrop(data, targetLayer, containerInfo = null) {
        let step;

        if (data.type === 'template') {
            // Create step from template
            step = data.template.createStep();
            step.layer = 'core';
        } else if (data.type === 'step') {
            // Clone step data
            step = data.step.clone();
            step.layer = 'core';
        }

        if (!step) return;

        // If dropping into a container zone, set parent relationship
        if (containerInfo && containerInfo.containerId && containerInfo.zone) {
            step.parentStepId = containerInfo.containerId;
            step.containerZone = containerInfo.zone;

            // Also update the container step's config arrays
            this.addStepToContainer(step, containerInfo);

            this.builder.addStep(step);
            this.showNotification(`Added ${step.stepName} to ${containerInfo.zone} block`, 'success');
        } else {
            // Normal drop (top-level step)
            this.builder.addStep(step);
            this.showNotification(`Added ${step.stepName}`, 'success');
        }
    }

    /**
     * Add a step to a container's config arrays
     */
    addStepToContainer(step, containerInfo) {
        const pipeline = this.builder.getPipeline();
        if (!pipeline) return;

        // Find the container step
        const allSteps = pipeline.getAllSteps ? pipeline.getAllSteps() : [];
        const containerStep = allSteps.find(s => s.id === containerInfo.containerId);
        if (!containerStep) return;

        if (!containerStep.config) containerStep.config = {};

        if (VisualStep.isLoopStep(containerStep)) {
            if (!containerStep.config.childStepIds) containerStep.config.childStepIds = [];
            if (!containerStep.config.childStepIds.includes(step.id)) {
                containerStep.config.childStepIds.push(step.id);
            }
        }

        console.log(`📦 Added step ${step.stepName} to container ${containerStep.stepName} zone=${containerInfo.zone}`);
    }

    /**
     * Handle drop from canvas (reordering)
     */
    handleCanvasDrop(data, targetLayer, dropPosition = null) {
        const { step } = data;
        this.builder.reorderSteps();
        this.showNotification(`Reordered ${step.stepName}`, 'success');
    }

    /**
     * Check if step can be dropped in layer
     */
    canDropInLayer(dragData, targetLayer) {
        if (!dragData) return false;

        // Layer constraints removed - any step type can go in any layer
        // Execution order is determined solely by sequence number
        return true;
    }

    /**
     * Clear drag state
     */
    clearDragState() {
        this.draggedElement = null;
        this.draggedData = null;
        this.dragSource = null;
        this._hoveredZone = null;

        // Remove all drag-over and container zone highlights
        document.querySelectorAll('.drag-over').forEach(el => {
            el.classList.remove('drag-over');
        });
        document.querySelectorAll('.container-zone.drag-hover').forEach(el => {
            el.classList.remove('drag-hover');
        });
    }

    /**
     * Show notification
     */
    showNotification(message, type = 'info') {
        // Simple notification - can be enhanced with a toast library
        const notification = document.createElement('div');
        notification.className = `notification notification-${type}`;
        notification.textContent = message;
        notification.style.cssText = `
            position: fixed;
            bottom: 20px;
            right: 20px;
            background: ${type === 'success' ? '#10b981' : type === 'error' ? '#ef4444' : type === 'warning' ? '#f59e0b' : '#06b6d4'};
            color: white;
            padding: 12px 20px;
            border-radius: 6px;
            box-shadow: 0 4px 6px rgba(0, 0, 0, 0.1);
            z-index: 10000;
            animation: slideIn 0.3s ease;
        `;

        document.body.appendChild(notification);

        setTimeout(() => {
            notification.style.animation = 'slideOut 0.3s ease';
            setTimeout(() => notification.remove(), 300);
        }, 3000);
    }

    /**
     * Show in-app confirmation dialog (replaces browser confirm)
     * @param {string} message - The message to display
     * @param {Object} options - Optional configuration
     * @param {string} options.title - Dialog title (default: 'Confirm')
     * @param {string} options.confirmText - Confirm button text (default: 'Yes')
     * @param {string} options.cancelText - Cancel button text (default: 'Cancel')
     * @param {string} options.type - Dialog type: 'warning', 'danger', 'info' (default: 'warning')
     * @returns {Promise<boolean>} - Resolves to true if confirmed, false if cancelled
     */
    showConfirmDialog(message, options = {}) {
        return new Promise((resolve) => {
            const {
                title = 'Confirm',
                confirmText = 'Yes',
                cancelText = 'Cancel',
                type = 'warning'
            } = options;

            // Color based on type
            const colors = {
                warning: { primary: '#f59e0b', bg: '#fef3c7', border: '#fcd34d' },
                danger: { primary: '#ef4444', bg: '#fee2e2', border: '#fca5a5' },
                info: { primary: '#3b82f6', bg: '#dbeafe', border: '#93c5fd' }
            };
            const color = colors[type] || colors.warning;

            // Create overlay
            const overlay = document.createElement('div');
            overlay.className = 'confirm-dialog-overlay';
            overlay.style.cssText = `
                position: fixed;
                top: 0;
                left: 0;
                right: 0;
                bottom: 0;
                background: rgba(0, 0, 0, 0.5);
                display: flex;
                align-items: center;
                justify-content: center;
                z-index: 100000;
                animation: fadeIn 0.2s ease;
            `;

            // Create dialog
            const dialog = document.createElement('div');
            dialog.className = 'confirm-dialog';
            dialog.style.cssText = `
                background: white;
                border-radius: 12px;
                box-shadow: 0 20px 25px -5px rgba(0, 0, 0, 0.1), 0 10px 10px -5px rgba(0, 0, 0, 0.04);
                max-width: 400px;
                width: 90%;
                padding: 0;
                overflow: hidden;
                animation: scaleIn 0.2s ease;
            `;

            dialog.innerHTML = `
                <div style="
                    padding: 16px 20px;
                    border-bottom: 1px solid ${color.border};
                    background: ${color.bg};
                    display: flex;
                    align-items: center;
                    gap: 10px;
                ">
                    <span style="font-size: 20px;">
                        ${type === 'danger' ? '⚠️' : type === 'info' ? 'ℹ️' : '❓'}
                    </span>
                    <span style="
                        font-weight: 600;
                        color: #1e293b;
                        font-size: 16px;
                    ">${this.escapeHtml(title)}</span>
                </div>
                <div style="
                    padding: 20px;
                    color: #475569;
                    font-size: 14px;
                    line-height: 1.6;
                ">${this.escapeHtml(message)}</div>
                <div style="
                    padding: 12px 20px;
                    background: #f8fafc;
                    border-top: 1px solid #e2e8f0;
                    display: flex;
                    justify-content: flex-end;
                    gap: 10px;
                ">
                    <button class="confirm-cancel-btn" style="
                        padding: 8px 16px;
                        border: 1px solid #e2e8f0;
                        background: white;
                        color: #64748b;
                        border-radius: 6px;
                        cursor: pointer;
                        font-size: 14px;
                        font-weight: 500;
                        transition: all 0.2s;
                    ">${this.escapeHtml(cancelText)}</button>
                    <button class="confirm-ok-btn" style="
                        padding: 8px 16px;
                        border: none;
                        background: ${color.primary};
                        color: white;
                        border-radius: 6px;
                        cursor: pointer;
                        font-size: 14px;
                        font-weight: 500;
                        transition: all 0.2s;
                    ">${this.escapeHtml(confirmText)}</button>
                </div>
            `;

            overlay.appendChild(dialog);
            document.body.appendChild(overlay);

            // Focus the confirm button
            const confirmBtn = dialog.querySelector('.confirm-ok-btn');
            const cancelBtn = dialog.querySelector('.confirm-cancel-btn');
            confirmBtn.focus();

            // Button hover effects
            confirmBtn.onmouseenter = () => confirmBtn.style.filter = 'brightness(1.1)';
            confirmBtn.onmouseleave = () => confirmBtn.style.filter = '';
            cancelBtn.onmouseenter = () => cancelBtn.style.background = '#f1f5f9';
            cancelBtn.onmouseleave = () => cancelBtn.style.background = 'white';

            // Cleanup function
            const cleanup = (result) => {
                overlay.style.animation = 'fadeOut 0.2s ease';
                dialog.style.animation = 'scaleOut 0.2s ease';
                setTimeout(() => {
                    overlay.remove();
                    resolve(result);
                }, 180);
            };

            // Event handlers
            confirmBtn.onclick = () => cleanup(true);
            cancelBtn.onclick = () => cleanup(false);

            // Close on overlay click
            overlay.onclick = (e) => {
                if (e.target === overlay) cleanup(false);
            };

            // Close on Escape key
            const handleKeydown = (e) => {
                if (e.key === 'Escape') {
                    cleanup(false);
                    document.removeEventListener('keydown', handleKeydown);
                } else if (e.key === 'Enter') {
                    cleanup(true);
                    document.removeEventListener('keydown', handleKeydown);
                }
            };
            document.addEventListener('keydown', handleKeydown);
        });
    }

    /**
     * Escape HTML to prevent XSS
     */
    escapeHtml(text) {
        const div = document.createElement('div');
        div.textContent = text;
        return div.innerHTML;
    }

    /**
     * Destroy manager and cleanup
     */
    destroy() {
        this.clearDragState();
        this.dropZones = [];
    }
}

// Add CSS animations
if (typeof document !== 'undefined') {
    const style = document.createElement('style');
    style.textContent = `
        @keyframes slideIn {
            from {
                transform: translateX(400px);
                opacity: 0;
            }
            to {
                transform: translateX(0);
                opacity: 1;
            }
        }

        @keyframes slideOut {
            from {
                transform: translateX(0);
                opacity: 1;
            }
            to {
                transform: translateX(400px);
                opacity: 0;
            }
        }

        @keyframes fadeIn {
            from { opacity: 0; }
            to { opacity: 1; }
        }

        @keyframes fadeOut {
            from { opacity: 1; }
            to { opacity: 0; }
        }

        @keyframes scaleIn {
            from {
                transform: scale(0.9);
                opacity: 0;
            }
            to {
                transform: scale(1);
                opacity: 1;
            }
        }

        @keyframes scaleOut {
            from {
                transform: scale(1);
                opacity: 1;
            }
            to {
                transform: scale(0.9);
                opacity: 0;
            }
        }
    `;
    document.head.appendChild(style);
}

// Export
if (typeof window !== 'undefined') {
    window.DragDropManager = DragDropManager;
}

if (typeof module !== 'undefined' && module.exports) {
    module.exports = DragDropManager;
}
