/**
 * Canvas Renderer
 * Handles SVG connection drawing and canvas visual management
 */

class CanvasRenderer {
    constructor(pipelineBuilder) {
        this.builder = pipelineBuilder;
        this.svg = document.getElementById('connectionsSvg');
        this.connections = [];
        this.zoomLevel = 1.0;
        this.panOffset = { x: 0, y: 0 };

        this.init();
    }

    init() {
        this.setupSVG();
        this.setupZoomControls();
    }

    /**
     * Setup SVG canvas
     */
    setupSVG() {
        if (!this.svg) return;

        // Set SVG attributes
        this.svg.setAttribute('width', '100%');
        this.svg.setAttribute('height', '100%');

        // Add marker definitions for arrow heads
        this.addArrowMarkers();
    }

    /**
     * Add arrow markers to SVG
     */
    addArrowMarkers() {
        const defs = document.createElementNS('http://www.w3.org/2000/svg', 'defs');

        // Regular arrow
        const marker = document.createElementNS('http://www.w3.org/2000/svg', 'marker');
        marker.setAttribute('id', 'arrowhead');
        marker.setAttribute('markerWidth', '10');
        marker.setAttribute('markerHeight', '10');
        marker.setAttribute('refX', '9');
        marker.setAttribute('refY', '3');
        marker.setAttribute('orient', 'auto');

        const polygon = document.createElementNS('http://www.w3.org/2000/svg', 'polygon');
        polygon.setAttribute('points', '0 0, 10 3, 0 6');
        polygon.setAttribute('fill', '#64748b');

        marker.appendChild(polygon);
        defs.appendChild(marker);

        // Active arrow (highlighted)
        const markerActive = document.createElementNS('http://www.w3.org/2000/svg', 'marker');
        markerActive.setAttribute('id', 'arrowhead-active');
        markerActive.setAttribute('markerWidth', '10');
        markerActive.setAttribute('markerHeight', '10');
        markerActive.setAttribute('refX', '9');
        markerActive.setAttribute('refY', '3');
        markerActive.setAttribute('orient', 'auto');

        const polygonActive = document.createElementNS('http://www.w3.org/2000/svg', 'polygon');
        polygonActive.setAttribute('points', '0 0, 10 3, 0 6');
        polygonActive.setAttribute('fill', '#2563eb');

        markerActive.appendChild(polygonActive);
        defs.appendChild(markerActive);

        this.svg.appendChild(defs);
    }

    /**
     * Setup zoom controls
     */
    setupZoomControls() {
        const zoomInBtn = document.getElementById('zoomInBtn');
        const zoomOutBtn = document.getElementById('zoomOutBtn');
        const zoomResetBtn = document.getElementById('zoomResetBtn');
        const zoomLevel = document.getElementById('zoomLevel');

        if (zoomInBtn) {
            zoomInBtn.addEventListener('click', () => this.zoom(0.1));
        }

        if (zoomOutBtn) {
            zoomOutBtn.addEventListener('click', () => this.zoom(-0.1));
        }

        if (zoomResetBtn) {
            zoomResetBtn.addEventListener('click', () => this.resetZoom());
        }

        this.zoomLevelDisplay = zoomLevel;
    }

    /**
     * Zoom canvas
     */
    zoom(delta) {
        this.zoomLevel = Math.max(0.5, Math.min(2.0, this.zoomLevel + delta));
        this.applyZoom();
    }

    /**
     * Reset zoom to 100%
     */
    resetZoom() {
        this.zoomLevel = 1.0;
        this.panOffset = { x: 0, y: 0 };
        this.applyZoom();
    }

    /**
     * Apply zoom transformation
     */
    applyZoom() {
        const canvasLayers = document.getElementById('canvasLayers');
        if (canvasLayers) {
            canvasLayers.style.transform = `scale(${this.zoomLevel}) translate(${this.panOffset.x}px, ${this.panOffset.y}px)`;
        }

        if (this.zoomLevelDisplay) {
            this.zoomLevelDisplay.textContent = `${Math.round(this.zoomLevel * 100)}%`;
        }

        // Redraw connections after zoom
        this.redrawAllConnections();
    }

    /**
     * Draw connection between two elements
     */
    drawConnection(fromElement, toElement, options = {}) {
        const fromRect = fromElement.getBoundingClientRect();
        const toRect = toElement.getBoundingClientRect();
        const svgRect = this.svg.getBoundingClientRect();

        // Calculate connection points
        const from = {
            x: fromRect.left + fromRect.width / 2 - svgRect.left,
            y: fromRect.bottom - svgRect.top
        };

        const to = {
            x: toRect.left + toRect.width / 2 - svgRect.left,
            y: toRect.top - svgRect.top
        };

        // Create path
        const path = this.createCurvedPath(from, to, options);

        // Store connection data
        this.connections.push({
            from: fromElement,
            to: toElement,
            path: path,
            options: options
        });

        this.svg.appendChild(path);

        return path;
    }

    /**
     * Create curved SVG path
     */
    createCurvedPath(from, to, options = {}) {
        const path = document.createElementNS('http://www.w3.org/2000/svg', 'path');

        // Calculate control points for smooth curve
        const midY = (from.y + to.y) / 2;
        const dx = to.x - from.x;
        const dy = to.y - from.y;

        // Curved path
        const pathData = `
            M ${from.x} ${from.y}
            C ${from.x} ${midY},
              ${to.x} ${midY},
              ${to.x} ${to.y}
        `;

        path.setAttribute('d', pathData);
        path.setAttribute('fill', 'none');
        path.setAttribute('stroke', options.color || '#64748b');
        path.setAttribute('stroke-width', options.width || '2');
        path.setAttribute('marker-end', options.active ? 'url(#arrowhead-active)' : 'url(#arrowhead)');

        if (options.dashed) {
            path.setAttribute('stroke-dasharray', '5,5');
        }

        // Add class for styling
        path.classList.add('connection-path');
        if (options.type) {
            path.classList.add(`connection-${options.type}`);
        }

        return path;
    }

    /**
     * Draw connection between execution groups
     */
    drawGroupConnection(fromGroupId, toGroupId, layer) {
        const fromGroup = document.querySelector(`[data-group-id="${fromGroupId}"]`);
        const toGroup = document.querySelector(`[data-group-id="${toGroupId}"]`);

        if (fromGroup && toGroup) {
            this.drawConnection(fromGroup, toGroup, {
                type: 'group-dependency',
                color: '#2563eb',
                width: 3,
                active: true
            });
        }
    }

    /**
     * Draw connections between steps in inline group
     */
    drawStepConnections(steps, groupElement) {
        if (!steps || steps.length < 2) return;

        for (let i = 0; i < steps.length - 1; i++) {
            const fromStep = groupElement.querySelector(`[data-step-id="${steps[i].id}"]`);
            const toStep = groupElement.querySelector(`[data-step-id="${steps[i + 1].id}"]`);

            if (fromStep && toStep) {
                this.drawConnection(fromStep, toStep, {
                    type: 'step-sequence',
                    color: '#94a3b8',
                    width: 2
                });
            }
        }
    }

    /**
     * Draw layer connections
     */
    drawLayerConnections() {
        const preLayer = document.querySelector('.layer-drop-zone[data-layer="pre"]');
        const coreLayer = document.querySelector('.layer-drop-zone[data-layer="core"]');
        const postLayer = document.querySelector('.layer-drop-zone[data-layer="post"]');

        // Draw connections between layers if they have content
        if (preLayer && coreLayer && preLayer.children.length > 0 && coreLayer.children.length > 0) {
            const lastPre = preLayer.lastElementChild;
            const firstCore = coreLayer.firstElementChild;
            if (lastPre && firstCore) {
                this.drawConnection(lastPre, firstCore, {
                    type: 'layer-boundary',
                    color: '#10b981',
                    width: 3,
                    dashed: true
                });
            }
        }

        if (coreLayer && postLayer && coreLayer.children.length > 0 && postLayer.children.length > 0) {
            const lastCore = coreLayer.lastElementChild;
            const firstPost = postLayer.firstElementChild;
            if (lastCore && firstPost) {
                this.drawConnection(lastCore, firstPost, {
                    type: 'layer-boundary',
                    color: '#10b981',
                    width: 3,
                    dashed: true
                });
            }
        }
    }

    /**
     * Clear all connections
     */
    clearConnections() {
        this.connections.forEach(conn => {
            if (conn.path && conn.path.parentNode) {
                conn.path.parentNode.removeChild(conn.path);
            }
        });
        this.connections = [];
    }

    /**
     * Redraw all connections
     */
    redrawAllConnections() {
        // Store connection data
        const connectionData = this.connections.map(conn => ({
            from: conn.from,
            to: conn.to,
            options: conn.options
        }));

        // Clear existing
        this.clearConnections();

        // Redraw
        connectionData.forEach(data => {
            if (data.from && data.to) {
                this.drawConnection(data.from, data.to, data.options);
            }
        });

        // Redraw layer connections
        this.drawLayerConnections();
    }

    /**
     * Auto-layout algorithm
     */
    autoLayout() {
        // Simple auto-layout: distribute groups evenly
        const layers = ['pre', 'core', 'post'];

        layers.forEach(layerName => {
            const dropZone = document.querySelector(`.layer-drop-zone[data-layer="${layerName}"]`);
            if (!dropZone) return;

            const groups = dropZone.querySelectorAll('.execution-group');
            const spacing = 20;

            groups.forEach((group, index) => {
                group.style.marginBottom = `${spacing}px`;
            });
        });

        // Redraw connections after layout
        setTimeout(() => this.redrawAllConnections(), 100);
    }

    /**
     * Highlight connection path
     */
    highlightConnection(fromElement, toElement) {
        const connection = this.connections.find(
            conn => conn.from === fromElement && conn.to === toElement
        );

        if (connection && connection.path) {
            connection.path.setAttribute('stroke', '#2563eb');
            connection.path.setAttribute('stroke-width', '3');
        }
    }

    /**
     * Unhighlight all connections
     */
    unhighlightConnections() {
        this.connections.forEach(conn => {
            if (conn.path) {
                conn.path.setAttribute('stroke', conn.options.color || '#64748b');
                conn.path.setAttribute('stroke-width', conn.options.width || '2');
            }
        });
    }

    /**
     * Destroy renderer
     */
    destroy() {
        this.clearConnections();
    }
}

// Export
if (typeof window !== 'undefined') {
    window.CanvasRenderer = CanvasRenderer;
}

if (typeof module !== 'undefined' && module.exports) {
    module.exports = CanvasRenderer;
}
