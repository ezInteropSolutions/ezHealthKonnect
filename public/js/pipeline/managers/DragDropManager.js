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
     * Setup drop zones for each layer
     */
    setupDropZones() {
        const layers = ['pre', 'core', 'post'];
        layers.forEach(layerName => {
            const dropZone = document.querySelector(`.layer-drop-zone[data-layer="${layerName}"]`);
            if (dropZone) {
                this.dropZones.push({
                    layer: layerName,
                    element: dropZone
                });
                this.setupDropZone(dropZone);
            }
        });
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

        if (!this.draggedData) return;

        // Check if drop is allowed
        const targetLayer = dropZone.dataset.layer;
        if (this.canDropInLayer(this.draggedData, targetLayer)) {
            e.dataTransfer.dropEffect = this.dragSource === 'canvas' ? 'move' : 'copy';
            dropZone.classList.add('drag-over');
        } else {
            e.dataTransfer.dropEffect = 'none';
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

        dropZone.classList.remove('drag-over');

        if (!this.draggedData) return;

        const targetLayer = dropZone.dataset.layer;

        // Check if drop is allowed
        if (!this.canDropInLayer(this.draggedData, targetLayer)) {
            this.showNotification('Cannot drop this step in this layer', 'warning');
            return;
        }

        try {
            if (this.dragSource === 'toolbox') {
                this.handleToolboxDrop(this.draggedData, targetLayer);
            } else if (this.dragSource === 'canvas') {
                this.handleCanvasDrop(this.draggedData, targetLayer);
            }
        } catch (error) {
            console.error('Drop error:', error);
            this.showNotification('Failed to add step', 'error');
        }

        this.clearDragState();
    }

    /**
     * Handle drop from toolbox
     */
    handleToolboxDrop(data, targetLayer) {
        let step;

        if (data.type === 'template') {
            // Create step from template
            step = data.template.createStep();
            step.layer = targetLayer;
        } else if (data.type === 'step') {
            // Clone step data
            step = data.step.clone();
            step.layer = targetLayer;
        }

        // Add step to pipeline
        this.builder.addStepToLayer(step, targetLayer);
        this.showNotification(`Added ${step.stepName} to ${targetLayer} layer`, 'success');
    }

    /**
     * Handle drop from canvas (reordering)
     */
    handleCanvasDrop(data, targetLayer) {
        const { step, groupId, sourceLayer } = data;

        if (sourceLayer === targetLayer) {
            // Reordering within same layer
            this.showNotification('Reordering within layer', 'info');
        } else {
            // Moving to different layer
            this.builder.moveStepToLayer(step.id, groupId, sourceLayer, targetLayer);
            this.showNotification(`Moved ${step.stepName} to ${targetLayer} layer`, 'success');
        }
    }

    /**
     * Check if step can be dropped in layer
     */
    canDropInLayer(dragData, targetLayer) {
        if (!dragData) return false;

        // Templates can specify preferred layers
        if (dragData.type === 'template') {
            const template = dragData.template;
            // Allow drop if no layer preference or matches target
            return !template.layer || template.layer === targetLayer || template.layer === 'any';
        }

        // Steps can be moved between layers
        if (dragData.type === 'step' || dragData.type === 'step-node') {
            return true; // Allow moving steps between layers
        }

        return false;
    }

    /**
     * Clear drag state
     */
    clearDragState() {
        this.draggedElement = null;
        this.draggedData = null;
        this.dragSource = null;

        // Remove all drag-over classes
        document.querySelectorAll('.drag-over').forEach(el => {
            el.classList.remove('drag-over');
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
