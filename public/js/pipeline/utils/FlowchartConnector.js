/**
 * FlowchartConnector - SVG Path Rendering for Pipeline Connections
 *
 * Features:
 * - Smooth Bezier curves for all connections
 * - Color-coded connection types (sequential, fork, merge, route)
 * - Animated arrows with proper markers
 * - Interactive hover effects with tooltips
 * - Smart path routing to avoid overlaps
 */
class FlowchartConnector {
    constructor(svgElement, layoutEngine) {
        this.svg = svgElement;
        this.layoutEngine = layoutEngine;
        this.connections = [];
        this.tooltipElement = null;

        this.initializeSVG();
        this.createTooltip();
    }

    /**
     * Initialize SVG with markers and definitions
     */
    initializeSVG() {
        console.log('🎯 [FlowchartConnector v1.3] Initializing SVG markers with ezHealthKonnect colors');

        // Create defs element for markers
        const defs = document.createElementNS('http://www.w3.org/2000/svg', 'defs');

        // Arrow markers for different connection types
        const markers = [
            { id: 'arrow-sequential', color: '#64748b' },
            { id: 'arrow-true', color: '#22c55e' },
            { id: 'arrow-false', color: '#ef4444' },
            { id: 'arrow-merge', color: '#64748b' },
            { id: 'arrow-route', color: '#f59e0b' },
            { id: 'arrow-conditional', color: '#8b5cf6' },
            { id: 'arrow-manual', color: '#ec4899' }  // Pink/magenta for user-created connections
        ];

        markers.forEach(marker => {
            const markerElement = document.createElementNS('http://www.w3.org/2000/svg', 'marker');
            markerElement.setAttribute('id', marker.id);
            markerElement.setAttribute('markerWidth', '10');
            markerElement.setAttribute('markerHeight', '10');
            markerElement.setAttribute('refX', '9');
            markerElement.setAttribute('refY', '3');
            markerElement.setAttribute('orient', 'auto');
            markerElement.setAttribute('markerUnits', 'strokeWidth');

            const path = document.createElementNS('http://www.w3.org/2000/svg', 'path');
            path.setAttribute('d', 'M0,0 L0,6 L9,3 z');
            path.setAttribute('fill', marker.color);
            path.classList.add('arrow-marker');

            markerElement.appendChild(path);
            defs.appendChild(markerElement);
        });

        this.svg.appendChild(defs);
    }

    /**
     * Create tooltip element for connection hover
     */
    createTooltip() {
        this.tooltipElement = document.createElement('div');
        this.tooltipElement.className = 'connection-tooltip';
        document.body.appendChild(this.tooltipElement);
    }

    /**
     * Render all connections
     */
    renderConnections(connections, steps) {
        // Clear existing connections
        this.clearConnections();
        this.connections = connections;

        connections.forEach(conn => {
            this.renderConnection(conn, steps);
        });
    }

    /**
     * Render a single connection
     */
    renderConnection(connection, steps) {
        const fromBox = this.layoutEngine.positions.get(connection.from);
        const toBox = this.layoutEngine.positions.get(connection.to);

        if (!fromBox || !toBox) return;

        // Get optimal connection points on box edges
        const fromPoint = this.getOptimalConnectionPoint(fromBox, toBox, true);
        const toPoint = this.getOptimalConnectionPoint(fromBox, toBox, false);

        console.log(`🔗 [v1.1] Connection ${connection.type}:`, {
            from: connection.from,
            to: connection.to,
            fromPoint,
            toPoint,
            fromBox,
            toBox
        });

        let path, label, markerEnd;

        switch (connection.type) {
            case 'sequential':
                path = this.createSequentialPath(fromPoint, toPoint);
                markerEnd = 'url(#arrow-sequential)';
                label = null;
                break;

            case 'cross-layer':
                // Cross-layer connections (between pre/core/post layers)
                path = this.createSequentialPath(fromPoint, toPoint);
                markerEnd = 'url(#arrow-sequential)';
                label = null;
                break;

            case 'manual':
                // User-created manual connections - distinct pink/magenta color
                path = this.createSequentialPath(fromPoint, toPoint);
                markerEnd = 'url(#arrow-manual)';
                label = null;
                break;

            case 'true-branch':
                path = this.createBranchPath(fromPoint, toPoint, 'left');
                markerEnd = 'url(#arrow-true)';
                label = { text: 'TRUE', color: '#22c55e', position: this.getMidPoint(fromPoint, toPoint) };
                break;

            case 'false-branch':
                path = this.createBranchPath(fromPoint, toPoint, 'right');
                markerEnd = 'url(#arrow-false)';
                label = { text: 'FALSE', color: '#ef4444', position: this.getMidPoint(fromPoint, toPoint) };
                break;

            case 'merge':
                path = this.createMergePath(fromPoint, toPoint);
                markerEnd = 'url(#arrow-merge)';
                label = null;
                break;

            case 'route-to':
                path = this.createRoutePath(fromPoint, toPoint);
                markerEnd = 'url(#arrow-route)';
                label = { text: 'ROUTE TO', color: '#f59e0b', position: this.getMidPoint(fromPoint, toPoint) };
                break;

            case 'conditional':
                path = this.createSequentialPath(fromPoint, toPoint);
                markerEnd = 'url(#arrow-conditional)';
                label = null;
                break;

            default:
                // Fallback for any other connection type
                console.warn(`⚠️ Unknown connection type: ${connection.type}, using sequential style`);
                path = this.createSequentialPath(fromPoint, toPoint);
                markerEnd = 'url(#arrow-sequential)';
                label = null;
                break;
        }

        if (path) {
            this.renderPath(path, connection.type, markerEnd, connection, steps);
        }

        if (label) {
            this.renderLabel(label.text, label.position, label.color);
        }
    }

    /**
     * Create sequential connection path (straight vertical)
     */
    createSequentialPath(from, to) {
        const midY = (from.y + to.y) / 2;

        // Smooth S-curve
        return `M ${from.x},${from.y}
                C ${from.x},${midY}
                  ${to.x},${midY}
                  ${to.x},${to.y}`;
    }

    /**
     * Create branch path (fork left or right)
     */
    createBranchPath(from, to, direction) {
        const forkY = from.y + 30; // Fork point below source
        const controlY = (forkY + to.y) / 2;

        // Start straight down, then curve to branch
        return `M ${from.x},${from.y}
                L ${from.x},${forkY}
                C ${from.x},${controlY}
                  ${to.x},${controlY}
                  ${to.x},${to.y}`;
    }

    /**
     * Create merge path (converge from branch)
     */
    createMergePath(from, to) {
        const mergeY = to.y - 30; // Merge point above target
        const controlY = (from.y + mergeY) / 2;

        // Curve from branch, then straight down
        return `M ${from.x},${from.y}
                C ${from.x},${controlY}
                  ${to.x},${controlY}
                  ${to.x},${mergeY}
                L ${to.x},${to.y}`;
    }

    /**
     * Create route-to path (horizontal skip with curve)
     */
    createRoutePath(from, to) {
        const offsetX = (to.x - from.x) / 2;
        const controlY1 = from.y - 20;
        const controlY2 = to.y - 20;

        // Curved path for visual distinction
        return `M ${from.x},${from.y}
                C ${from.x + offsetX},${controlY1}
                  ${from.x + offsetX},${controlY2}
                  ${to.x},${to.y}`;
    }

    /**
     * Render SVG path element
     */
    renderPath(pathData, type, markerEnd, connection, steps) {
        const path = document.createElementNS('http://www.w3.org/2000/svg', 'path');
        path.setAttribute('d', pathData);
        path.setAttribute('class', `connection-path ${type}`);
        path.setAttribute('marker-end', markerEnd);
        path.setAttribute('fill', 'none');
        path.setAttribute('data-from', connection.from);
        path.setAttribute('data-to', connection.to);

        // Add hover interaction
        path.addEventListener('mouseenter', (e) => this.showTooltip(e, connection, steps));
        path.addEventListener('mousemove', (e) => this.updateTooltipPosition(e));
        path.addEventListener('mouseleave', () => this.hideTooltip());

        this.svg.appendChild(path);
    }

    /**
     * Render label text on connection
     */
    renderLabel(text, position, color) {
        const label = document.createElementNS('http://www.w3.org/2000/svg', 'text');
        label.setAttribute('x', position.x);
        label.setAttribute('y', position.y - 5);
        label.setAttribute('class', 'connection-label');
        label.setAttribute('text-anchor', 'middle');
        label.style.fill = color;
        label.textContent = text;

        this.svg.appendChild(label);
    }

    /**
     * Get midpoint between two points
     */
    getMidPoint(from, to) {
        return {
            x: (from.x + to.x) / 2,
            y: (from.y + to.y) / 2
        };
    }

    /**
     * Get optimal connection point on box edge
     */
    getOptimalConnectionPoint(fromBox, toBox, isSource) {
        const fromCenter = { x: fromBox.x, y: fromBox.y + fromBox.height / 2 };
        const toCenter = { x: toBox.x, y: toBox.y + toBox.height / 2 };

        // Determine direction
        const dx = toCenter.x - fromCenter.x;
        const dy = toCenter.y - fromCenter.y;

        if (isSource) {
            // From source box
            if (Math.abs(dy) > Math.abs(dx)) {
                // Vertical connection
                if (dy > 0) {
                    // Going down - connect from bottom
                    return { x: fromBox.x, y: fromBox.y + fromBox.height };
                } else {
                    // Going up - connect from top
                    return { x: fromBox.x, y: fromBox.y };
                }
            } else {
                // Horizontal connection
                if (dx > 0) {
                    // Going right - connect from right
                    return { x: fromBox.x + fromBox.width / 2, y: fromBox.y + fromBox.height / 2 };
                } else {
                    // Going left - connect from left
                    return { x: fromBox.x - fromBox.width / 2, y: fromBox.y + fromBox.height / 2 };
                }
            }
        } else {
            // To target box
            if (Math.abs(dy) > Math.abs(dx)) {
                // Vertical connection
                if (dy > 0) {
                    // Coming from above - connect to top
                    return { x: toBox.x, y: toBox.y };
                } else {
                    // Coming from below - connect to bottom
                    return { x: toBox.x, y: toBox.y + toBox.height };
                }
            } else {
                // Horizontal connection
                if (dx > 0) {
                    // Coming from left - connect to left
                    return { x: toBox.x - toBox.width / 2, y: toBox.y + toBox.height / 2 };
                } else {
                    // Coming from right - connect to right
                    return { x: toBox.x + toBox.width / 2, y: toBox.y + toBox.height / 2 };
                }
            }
        }
    }

    /**
     * Show tooltip on connection hover
     */
    showTooltip(event, connection, steps) {
        const fromStep = steps.find(s => s.id === connection.from);
        const toStep = steps.find(s => s.id === connection.to);

        if (!fromStep || !toStep) return;

        let content = '';

        switch (connection.type) {
            case 'sequential':
                content = `
                    <div><strong>Sequential Flow</strong></div>
                    <div>From: ${fromStep.stepName}</div>
                    <div>To: ${toStep.stepName}</div>
                `;
                break;

            case 'true-branch':
                content = `
                    <div style="color: #22c55e;"><strong>✓ TRUE Branch</strong></div>
                    <div>Condition passed</div>
                    <div>→ ${toStep.stepName}</div>
                `;
                break;

            case 'false-branch':
                content = `
                    <div style="color: #ef4444;"><strong>✗ FALSE Branch</strong></div>
                    <div>Condition failed</div>
                    <div>→ ${toStep.stepName}</div>
                `;
                break;

            case 'merge':
                content = `
                    <div><strong>Merge Point</strong></div>
                    <div>Branches rejoin here</div>
                    <div>→ ${toStep.stepName}</div>
                `;
                break;

            case 'route-to':
                content = `
                    <div style="color: #f59e0b;"><strong>🔀 Route To</strong></div>
                    <div>Conditional skip</div>
                    <div>→ ${toStep.stepName}</div>
                `;
                break;
        }

        this.tooltipElement.innerHTML = content;
        this.tooltipElement.classList.add('visible');
        this.updateTooltipPosition(event);
    }

    /**
     * Update tooltip position
     */
    updateTooltipPosition(event) {
        const x = event.pageX;
        const y = event.pageY - 60; // Above cursor

        this.tooltipElement.style.left = `${x}px`;
        this.tooltipElement.style.top = `${y}px`;
        this.tooltipElement.style.transform = 'translateX(-50%)';
    }

    /**
     * Hide tooltip
     */
    hideTooltip() {
        this.tooltipElement.classList.remove('visible');
    }

    /**
     * Clear all rendered connections
     */
    clearConnections() {
        // Remove all path elements
        const paths = this.svg.querySelectorAll('.connection-path');
        paths.forEach(path => path.remove());

        // Remove all labels
        const labels = this.svg.querySelectorAll('.connection-label');
        labels.forEach(label => label.remove());
    }

    /**
     * Highlight a specific connection
     */
    highlightConnection(fromId, toId) {
        const path = this.svg.querySelector(
            `.connection-path[data-from="${fromId}"][data-to="${toId}"]`
        );

        if (path) {
            path.style.strokeWidth = '4';
            path.style.filter = 'drop-shadow(0 0 6px currentColor)';
        }
    }

    /**
     * Remove highlight from connection
     */
    unhighlightConnection(fromId, toId) {
        const path = this.svg.querySelector(
            `.connection-path[data-from="${fromId}"][data-to="${toId}"]`
        );

        if (path) {
            path.style.strokeWidth = '';
            path.style.filter = '';
        }
    }

    /**
     * Highlight all connections for a specific step
     */
    highlightStepConnections(stepId) {
        const paths = this.svg.querySelectorAll(
            `.connection-path[data-from="${stepId}"], .connection-path[data-to="${stepId}"]`
        );

        paths.forEach(path => {
            path.style.strokeWidth = '4';
            path.style.filter = 'drop-shadow(0 0 6px currentColor)';
        });
    }

    /**
     * Remove highlight from all connections for a step
     */
    unhighlightStepConnections(stepId) {
        const paths = this.svg.querySelectorAll(
            `.connection-path[data-from="${stepId}"], .connection-path[data-to="${stepId}"]`
        );

        paths.forEach(path => {
            path.style.strokeWidth = '';
            path.style.filter = '';
        });
    }

    /**
     * Resize SVG to match container
     */
    resize(width, height) {
        this.svg.setAttribute('width', width);
        this.svg.setAttribute('height', height);
        this.svg.setAttribute('viewBox', `0 0 ${width} ${height}`);
    }

    /**
     * Destroy and cleanup
     */
    destroy() {
        this.clearConnections();

        if (this.tooltipElement && this.tooltipElement.parentNode) {
            this.tooltipElement.parentNode.removeChild(this.tooltipElement);
        }

        const defs = this.svg.querySelector('defs');
        if (defs) {
            defs.remove();
        }
    }
}

// Export for use in other modules
if (typeof module !== 'undefined' && module.exports) {
    module.exports = FlowchartConnector;
}
