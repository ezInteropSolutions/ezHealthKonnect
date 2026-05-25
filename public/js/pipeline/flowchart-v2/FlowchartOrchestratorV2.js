/**
 * FlowchartOrchestratorV2 - Main coordinator for V2 flowchart system
 *
 * Responsibilities:
 * - Coordinate between CoordinateSystem, LayoutEngine, and Canvas
 * - Handle user interactions (drag, pan, zoom)
 * - Provide clean interface for PipelineBuilder integration
 * - Match V1 FlowchartRenderer interface for drop-in replacement
 */
class FlowchartOrchestratorV2 {
    constructor(container, pipelineBuilder) {
        this.container = container;
        this.pipelineBuilder = pipelineBuilder;

        // Initialize core components
        this.coords = new FlowchartCoordinateSystem();
        this.layoutEngine = new HorizontalLayoutEngine();
        this.canvas = new FlowchartCanvas(container, this.coords);

        // State
        this.steps = [];
        this.layout = null;

        // Interaction controllers (Phase 4)
        this.isDragging = false;
        this.isPanning = false;
        this.draggedNode = null;
        this.panStart = null;
        this.hasMoved = false; // Track if mouse moved during drag
        this.zoomLocked = true; // Zoom lock state (locked by default)

        // Connect mode for manually creating arrows
        this.connectMode = false;
        this.connectSourceStep = null;

        // Initialize interactions
        this.setupInteractions();
        this.setupZoomLockButton();
        this.setupConnectModeButton();
        this.setupAutoSequenceButton();

        console.log('✅ FlowchartOrchestratorV2 initialized');
    }

    /**
     * Main render method - called by PipelineBuilder
     */
    render(steps) {
        if (!steps || steps.length === 0) {
            console.warn('⚠️ No steps to render in flowchart');
            this.canvas.clear();
            return;
        }

        console.log(`🎨 FlowchartOrchestratorV2 rendering ${steps.length} steps`);

        this.steps = steps;

        // Calculate layout
        this.layout = this.layoutEngine.calculateLayout(steps);

        // Load and apply saved positions (if any)
        this.loadSavedPositions();

        // Load saved connections (replaces auto-generated if user has edited)
        this.loadSavedConnections();

        // Render to canvas
        this.canvas.render(this.layout, steps);

        console.log('✅ Flowchart V2 rendered successfully');
    }

    /**
     * Refresh current view
     */
    refresh() {
        if (this.steps.length > 0) {
            this.render(this.steps);
        }
    }

    /**
     * Get main canvas container (for PipelineBuilder integration)
     */
    getCanvas() {
        return this.canvas.mainContainer;
    }

    /**
     * Setup user interactions
     */
    setupInteractions() {
        // Mouse wheel zoom
        this.container.addEventListener('wheel', (e) => this.handleWheel(e), { passive: false });

        // Pan and drag
        this.container.addEventListener('mousedown', (e) => this.handleMouseDown(e));
        document.addEventListener('mousemove', (e) => this.handleMouseMove(e));
        document.addEventListener('mouseup', (e) => this.handleMouseUp(e));

        // Prevent context menu
        this.container.addEventListener('contextmenu', (e) => e.preventDefault());

        console.log('✅ Interactions setup complete');
    }

    /**
     * Handle mouse wheel for zooming
     */
    handleWheel(e) {
        e.preventDefault();

        // Check if zoom is locked
        if (this.zoomLocked) {
            console.log('🔒 Zoom locked - ignoring wheel event');
            return;
        }

        const rect = this.container.getBoundingClientRect();
        const mouseX = e.clientX - rect.left;
        const mouseY = e.clientY - rect.top;

        // Zoom in/out
        const zoomDelta = e.deltaY > 0 ? 0.9 : 1.1;
        const currentZoom = this.coords.zoom;
        const newZoom = currentZoom * zoomDelta;

        this.coords.setZoom(newZoom, mouseX, mouseY);
        this.canvas.applyTransform();

        console.log(`🔍 Zoom: ${this.coords.zoom.toFixed(2)}x`);
    }

    /**
     * Handle mouse down - start drag or pan
     */
    handleMouseDown(e) {
        // Check if clicking on a step node (includes container steps)
        const clickedNode = e.target.closest('.flowchart-step-node');

        if (clickedNode) {
            // Start node drag
            this.isDragging = true;
            this.draggedNode = clickedNode;
            this.dragStartX = e.clientX;
            this.dragStartY = e.clientY;
            this.ctrlKeyPressed = e.ctrlKey;

            // Get current position
            const stepId = clickedNode.getAttribute('data-step-id');
            const pos = this.layout.positions.get(stepId);
            if (pos) {
                this.dragNodeStartPos = { x: pos.x, y: pos.y };
            }

            clickedNode.style.cursor = 'grabbing';
            clickedNode.style.zIndex = '1000';

            this.hasMoved = false;

            console.log(`🖱️ Started dragging step: ${stepId}, Ctrl: ${this.ctrlKeyPressed}`);
        } else {
            // Start canvas pan
            this.isPanning = true;
            this.panStart = {
                x: e.clientX,
                y: e.clientY,
                panX: this.coords.panX,
                panY: this.coords.panY
            };

            this.container.style.cursor = 'grabbing';

            console.log('🖱️ Started panning canvas');
        }
    }

    /**
     * Handle mouse move - drag node or pan canvas
     */
    handleMouseMove(e) {
        if (this.isDragging && this.draggedNode) {
            // Drag node
            const dx = e.clientX - this.dragStartX;
            const dy = e.clientY - this.dragStartY;

            // Mark as moved if mouse moved more than 5px
            if (Math.abs(dx) > 5 || Math.abs(dy) > 5) {
                this.hasMoved = true;
            }

            // Apply movement in world coordinates
            const worldDx = dx / this.coords.zoom;
            const worldDy = dy / this.coords.zoom;

            const newX = this.dragNodeStartPos.x + worldDx;
            const newY = this.dragNodeStartPos.y + worldDy;

            // Move step node
            this.draggedNode.style.left = `${newX}px`;
            this.draggedNode.style.top = `${newY}px`;

            // Update layout position
            const stepId = this.draggedNode.getAttribute('data-step-id');
            const pos = this.layout.positions.get(stepId);
            if (pos) {
                pos.x = newX;
                pos.y = newY;
            }

            // Redraw connections
            this.canvas.ctx.clearRect(0, 0, this.canvas.canvas.width, this.canvas.canvas.height);
            this.canvas.renderConnections(this.layout.connections, this.layout.positions);

        } else if (this.isPanning && this.panStart) {
            // Pan canvas
            const dx = e.clientX - this.panStart.x;
            const dy = e.clientY - this.panStart.y;

            this.coords.panX = this.panStart.panX + dx;
            this.coords.panY = this.panStart.panY + dy;

            this.canvas.applyTransform();
        }
    }

    /**
     * Handle mouse up - end drag or pan
     */
    handleMouseUp(e) {
        if (this.isDragging && this.draggedNode) {
            // Reset visual feedback
            this.draggedNode.style.cursor = 'pointer';
            this.draggedNode.style.zIndex = '';

            // Get step ID
            const stepId = this.draggedNode.getAttribute('data-step-id');

            // If didn't move, treat as click
            if (!this.hasMoved) {
                const step = this.steps.find(s => s.id === stepId);

                // Don't open properties if we're in the middle of creating a connection
                if (this.canvas.isConnecting) {
                    console.log('🔗 Skipping properties panel - connection in progress');
                }
                // Ctrl+Click always enters/uses connect mode (convenient shortcut)
                else if (this.ctrlKeyPressed && step) {
                    console.log('🔗 Ctrl+Click detected - using connect mode');
                    this.handleConnectModeClick(step);
                }
                // Check if in connect mode (via button)
                else if (this.connectMode && step) {
                    this.handleConnectModeClick(step);
                }
                // Open properties panel
                else if (step && window.propertiesPanel && typeof window.propertiesPanel.showStepProperties === 'function') {
                    console.log('📋 Click detected - opening properties for:', stepId);
                    window.propertiesPanel.showStepProperties(step, false);
                }
            } else {
                console.log(`✅ Finished dragging step: ${stepId}`);
                // Save position
                this.savePositions();
            }

            this.isDragging = false;
            this.draggedNode = null;
            this.dragStartX = null;
            this.dragStartY = null;
            this.dragNodeStartPos = null;
            this.hasMoved = false;
            this.ctrlKeyPressed = false;
        }

        if (this.isPanning) {
            this.container.style.cursor = '';

            console.log('✅ Finished panning canvas');

            this.isPanning = false;
            this.panStart = null;
        }
    }

    /**
     * Setup zoom lock button
     */
    setupZoomLockButton() {
        const zoomLockBtn = document.getElementById('zoomLockBtn');
        if (!zoomLockBtn) return;

        // Set initial visual state (locked by default)
        const icon = zoomLockBtn.querySelector('i');
        if (this.zoomLocked) {
            icon.className = 'fas fa-lock';
            zoomLockBtn.title = 'Unlock Zoom (Ctrl+L)';
            zoomLockBtn.style.color = '#ef4444'; // Red when locked
        }

        zoomLockBtn.addEventListener('click', () => {
            this.toggleZoomLock();
        });

        // Keyboard shortcut: Ctrl+L
        document.addEventListener('keydown', (e) => {
            if (e.ctrlKey && e.key === 'l') {
                e.preventDefault();
                this.toggleZoomLock();
            }
        });

        console.log('✅ Zoom lock button setup complete (locked by default)');
    }

    /**
     * Toggle zoom lock
     */
    toggleZoomLock() {
        this.zoomLocked = !this.zoomLocked;

        const zoomLockBtn = document.getElementById('zoomLockBtn');
        if (zoomLockBtn) {
            const icon = zoomLockBtn.querySelector('i');
            if (this.zoomLocked) {
                icon.className = 'fas fa-lock';
                zoomLockBtn.title = 'Unlock Zoom (Ctrl+L)';
                zoomLockBtn.style.color = '#ef4444'; // Red when locked
                console.log('🔒 Zoom locked');
            } else {
                icon.className = 'fas fa-lock-open';
                zoomLockBtn.title = 'Lock Zoom (Ctrl+L)';
                zoomLockBtn.style.color = '';
                console.log('🔓 Zoom unlocked');
            }
        }
    }

    /**
     * Setup connect mode button
     */
    setupConnectModeButton() {
        const connectModeBtn = document.getElementById('connectModeBtn');
        if (!connectModeBtn) return;

        connectModeBtn.addEventListener('click', () => {
            this.toggleConnectMode();
        });

        console.log('✅ Connect mode button setup complete');
    }

    /**
     * Toggle connect mode
     */
    toggleConnectMode() {
        this.connectMode = !this.connectMode;
        this.connectSourceStep = null;

        const connectModeBtn = document.getElementById('connectModeBtn');
        if (connectModeBtn) {
            if (this.connectMode) {
                connectModeBtn.style.background = '#3b82f6';
                connectModeBtn.style.color = 'white';
                connectModeBtn.title = 'Connect Mode: ON - Click source step, then target step';
                console.log('🔗 Connect mode enabled');
                this.showNotification('Connect mode ON: Click source step, then click target step to create arrow', 'info');
            } else {
                connectModeBtn.style.background = '';
                connectModeBtn.style.color = '';
                connectModeBtn.title = 'Connect Mode: Click step 1, then step 2 to create arrow';
                console.log('🔗 Connect mode disabled');
            }
        }
    }

    /**
     * Handle click in connect mode
     */
    handleConnectModeClick(step) {
        if (!this.connectSourceStep) {
            // First click - select source
            this.connectSourceStep = step;
            console.log('🔗 Source step selected:', step.stepName);
            this.showNotification(`Source: ${step.stepName} - Now Ctrl+Click target step`, 'info');

            // Highlight source node (save original border to restore later)
            const node = this.canvas.getStepNode(step.id);
            if (node) {
                node._origBorder = node.style.border;
                node._origBoxShadow = node.style.boxShadow;
                node.style.border = '3px solid #3b82f6';
                node.style.boxShadow = '0 0 0 4px rgba(59, 130, 246, 0.3)';
            }
        } else {
            // Second click - create connection
            if (this.connectSourceStep.id === step.id) {
                // Clicked same step - cancel
                console.log('🔗 Cancelled - clicked same step');
                this.showNotification('Connection cancelled', 'warning');
            } else {
                console.log(`🔗 Creating connection: ${this.connectSourceStep.stepName} → ${step.stepName}`);

                // Check if connection already exists
                const exists = this.layout.connections.some(c =>
                    c.from === this.connectSourceStep.id && c.to === step.id
                );

                if (exists) {
                    this.showNotification('Connection already exists', 'warning');
                } else {
                    // Add connection to layout
                    if (this.layout && this.layout.connections) {
                        this.layout.connections.push({
                            type: 'custom',
                            from: this.connectSourceStep.id,
                            to: step.id,
                            fromStep: this.connectSourceStep,
                            toStep: step
                        });

                        // Save ALL connections to localStorage (preserves deletions too)
                        this.saveAllConnections();

                        // Re-render connections
                        this.canvas.ctx.clearRect(0, 0, this.canvas.canvas.width, this.canvas.canvas.height);
                        this.canvas.renderConnections(this.layout.connections, this.layout.positions);

                        this.showNotification(`Connected: ${this.connectSourceStep.stepName} → ${step.stepName}`, 'success');
                    }
                }
            }

            // Remove highlight (restore original border)
            const sourceNode = this.canvas.getStepNode(this.connectSourceStep.id);
            if (sourceNode) {
                sourceNode.style.border = sourceNode._origBorder || '2px solid #f8bbd9';
                sourceNode.style.boxShadow = sourceNode._origBoxShadow || '';
            }

            // Reset for next connection
            this.connectSourceStep = null;
        }
    }

    /**
     * Save ALL connections to localStorage (preserves user edits including deletions)
     */
    saveAllConnections() {
        if (!this.layout || !this.layout.connections) return;

        // Save all connections with their types
        const connections = this.layout.connections.map(c => ({
            from: c.from,
            to: c.to,
            type: c.type || 'sequential'
        }));

        const key = this.getConnectionsStorageKey();
        localStorage.setItem(key, JSON.stringify({
            edited: true,  // Flag that user has edited connections
            connections: connections
        }));
        console.log(`💾 Saved ${connections.length} connections to localStorage`);
    }

    /**
     * Load saved connections from pipeline model (database) or localStorage
     * Priority: 1. Pipeline model (database) 2. localStorage (legacy/backup)
     */
    loadSavedConnections() {
        if (!this.layout) return;

        // First try to load from pipeline model (database - source of truth)
        const pipelineConnections = this.pipelineBuilder?.pipeline?.connections;

        if (pipelineConnections && pipelineConnections.length > 0) {
            console.log(`📂 Loading ${pipelineConnections.length} connections from pipeline model (database)`);

            const restoredConnections = [];
            pipelineConnections.forEach(conn => {
                const fromStep = this.steps.find(s => s.id === conn.from);
                const toStep = this.steps.find(s => s.id === conn.to);

                if (fromStep && toStep) {
                    restoredConnections.push({
                        type: conn.type || 'sequential',
                        from: conn.from,
                        to: conn.to,
                        fromStep: fromStep,
                        toStep: toStep
                    });
                }
            });

            if (restoredConnections.length > 0) {
                this.layout.connections = restoredConnections;
                console.log(`✅ Restored ${restoredConnections.length} connections from database`);
                return;
            }
        }

        // Fallback: try localStorage (for backward compatibility)
        const key = this.getConnectionsStorageKey();
        const saved = localStorage.getItem(key);

        if (saved) {
            try {
                const data = JSON.parse(saved);

                if (data.edited && data.connections) {
                    const restoredConnections = [];

                    data.connections.forEach(conn => {
                        const fromStep = this.steps.find(s => s.id === conn.from);
                        const toStep = this.steps.find(s => s.id === conn.to);

                        if (fromStep && toStep) {
                            restoredConnections.push({
                                type: conn.type || 'sequential',
                                from: conn.from,
                                to: conn.to,
                                fromStep: fromStep,
                                toStep: toStep
                            });
                        }
                    });

                    this.layout.connections = restoredConnections;
                    console.log(`📂 Restored ${restoredConnections.length} connections from localStorage (legacy)`);
                }
            } catch (e) {
                console.warn('Failed to load saved connections from localStorage:', e);
            }
        }
    }

    /**
     * Get storage key for connections
     */
    getConnectionsStorageKey() {
        const interfaceId = this.pipelineBuilder?.interfaceId || 'default';
        const messageType = this.pipelineBuilder?.messageType || 'default';
        return `flowchart_v2_connections_${interfaceId}_${messageType}`;
    }

    /**
     * Show in-app notification
     */
    showNotification(message, type = 'info') {
        const notification = document.createElement('div');
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
            max-width: 400px;
        `;
        notification.textContent = message;
        document.body.appendChild(notification);
        setTimeout(() => notification.remove(), 3000);
    }

    /**
     * Reset view to default zoom/pan
     */
    resetView() {
        this.coords.reset();
        this.canvas.applyTransform();
        console.log('🔄 View reset to default');
    }

    /**
     * Fit all steps in viewport
     */
    fitToScreen() {
        if (!this.layout || !this.layout.bounds) {
            return;
        }

        const containerRect = this.container.getBoundingClientRect();
        const bounds = this.layout.bounds;

        // Calculate zoom to fit
        const zoomX = containerRect.width / bounds.width;
        const zoomY = containerRect.height / bounds.height;
        const zoom = Math.min(zoomX, zoomY) * 0.9; // 90% to add padding

        // Calculate pan to center
        const panX = (containerRect.width - bounds.width * zoom) / 2 - bounds.minX * zoom;
        const panY = (containerRect.height - bounds.height * zoom) / 2 - bounds.minY * zoom;

        this.coords.zoom = zoom;
        this.coords.panX = panX;
        this.coords.panY = panY;

        this.canvas.applyTransform();

        console.log(`📐 Fit to screen: zoom=${zoom.toFixed(2)}, pan=(${panX.toFixed(0)}, ${panY.toFixed(0)})`);
    }

    /**
     * Get step node element by ID
     */
    getStepNode(stepId) {
        return this.canvas.getStepNode(stepId);
    }

    /**
     * Highlight a specific step
     */
    highlightStep(stepId) {
        const node = this.getStepNode(stepId);
        if (node) {
            node.style.borderColor = '#1e3a8a';
            node.style.boxShadow = '0 0 0 3px rgba(30, 58, 138, 0.3)';
        }
    }

    /**
     * Clear all highlights
     */
    clearHighlights() {
        const nodes = this.canvas.nodesContainer.querySelectorAll('.flowchart-step-node');
        nodes.forEach(node => {
            node.style.borderColor = '#f8bbd9';
            node.style.boxShadow = '0 2px 4px rgba(0,0,0,0.1)';
        });
    }

    /**
     * Handle window resize
     */
    handleResize() {
        this.canvas.resize();
        this.canvas.applyTransform();
    }

    /**
     * Get localStorage key for saving positions
     */
    getStorageKey() {
        const interfaceId = this.pipelineBuilder?.interfaceId || 'unknown';
        const messageType = this.pipelineBuilder?.messageType || 'unknown';
        return `flowchart_v2_positions_${interfaceId}_${messageType}`;
    }

    /**
     * Save current positions to pipeline model (which saves to database)
     */
    savePositions() {
        if (!this.layout || !this.layout.positions) {
            return;
        }

        // Check if steps are available
        if (!this.steps || this.steps.length === 0) {
            console.log('💾 No steps available for saving positions');
            return;
        }

        // Save positions directly to the step objects
        let savedCount = 0;
        this.layout.positions.forEach((pos, stepId) => {
            // Find the step in the flat steps array
            const step = this.steps.find(s => s.id === stepId);
            if (step) {
                step.position_x = pos.x;
                step.position_y = pos.y;
                savedCount++;
                console.log(`  💾 Saved position for step ${step.stepName || step.step_name}: (${pos.x}, ${pos.y})`);
            }
        });

        console.log(`💾 Saved ${savedCount} step positions to step objects`);

        // Mark pipeline as unsaved so it gets saved to database
        if (window.pipelineBuilder) {
            window.pipelineBuilder.markAsUnsaved();
        }
    }

    /**
     * Load saved positions from pipeline steps (database)
     */
    loadSavedPositions() {
        if (!this.layout || !this.layout.positions) {
            return;
        }

        // Check if steps are available
        if (!this.steps || this.steps.length === 0) {
            console.log('📂 No steps available for loading positions');
            return;
        }

        let appliedCount = 0;
        const occupiedPositions = new Map(); // "x,y" -> stepId

        // First pass: load saved positions and detect overlaps
        const savedPositions = [];
        this.layout.positions.forEach((pos, stepId) => {
            const step = this.steps.find(s => s.id === stepId);
            if (step && step.position_x !== undefined && step.position_y !== undefined && step.position_x !== null && step.position_y !== null) {
                savedPositions.push({ stepId, pos, step, x: step.position_x, y: step.position_y });
            }
        });

        // Apply positions, offsetting duplicates
        for (const entry of savedPositions) {
            const key = `${entry.x},${entry.y}`;
            if (occupiedPositions.has(key)) {
                // Overlap detected - offset this step down by (stepHeight + rowGap)
                const offsetY = entry.y + 120; // 80px stepHeight + 40px rowGap
                entry.pos.x = entry.x;
                entry.pos.y = offsetY;
                occupiedPositions.set(`${entry.x},${offsetY}`, entry.stepId);
                appliedCount++;
                console.log(`  📂 Offset overlapping step ${entry.step.stepName || entry.step.step_name}: (${entry.x}, ${entry.y}) → (${entry.x}, ${offsetY})`);
            } else {
                entry.pos.x = entry.x;
                entry.pos.y = entry.y;
                occupiedPositions.set(key, entry.stepId);
                appliedCount++;
            }
        }

        if (appliedCount > 0) {
            console.log(`📂 Loaded ${appliedCount} saved positions from database`);
        } else {
            console.log('📂 No saved positions found - using auto-layout');
        }
    }

    /**
     * Clear saved positions from localStorage
     */
    clearSavedPositions() {
        const key = this.getStorageKey();
        localStorage.removeItem(key);
        console.log('🗑️ Cleared saved positions from localStorage');
    }

    /**
     * Destroy and cleanup
     */
    destroy() {
        // Remove event listeners
        this.container.removeEventListener('wheel', this.handleWheel);
        this.container.removeEventListener('mousedown', this.handleMouseDown);
        document.removeEventListener('mousemove', this.handleMouseMove);
        document.removeEventListener('mouseup', this.handleMouseUp);

        // Destroy canvas
        this.canvas.destroy();

        console.log('🗑️ FlowchartOrchestratorV2 destroyed');
    }

    /**
     * Get current layout data
     */
    getLayout() {
        return this.layout;
    }

    /**
     * Get coordinate system
     */
    getCoordinateSystem() {
        return this.coords;
    }

    /**
     * Export current state for debugging
     */
    exportState() {
        return {
            steps: this.steps.length,
            layout: this.layout ? {
                positions: this.layout.positions.size,
                connections: this.layout.connections.length,
                swimLanes: Object.keys(this.layout.swimLanes).length,
                bounds: this.layout.bounds
            } : null,
            transform: this.coords.getState(),
            viewportSize: {
                width: this.container.clientWidth,
                height: this.container.clientHeight
            }
        };
    }

    /**
     * Auto-sequence steps based on connections
     *
     * Algorithm:
     * 1. Build dependency graph from connections
     * 2. Find root steps (no incoming connections) → sequence = 10
     * 3. For sequential connections: next step = predecessor sequence + 10
     * 4. For conditional branches (TRUE/FALSE): all targets get SAME sequence (alternatives)
     * 5. For join points: sequence = max(all incoming) + 10
     *
     * Key insight for conditionals:
     * - If-Else/Switch branches are ALTERNATIVES - only ONE executes at runtime
     * - So all branch targets should have the same sequence number
     * - The step AFTER the branches (join point) waits for whichever ran
     *
     * @param {boolean} silent - If true, don't show notifications (used for auto-save)
     */
    autoSequenceSteps(silent = false) {
        if (!this.steps || this.steps.length === 0) {
            if (!silent) this.showNotification('No steps to sequence', 'warning');
            return;
        }

        if (!this.layout || !this.layout.connections || this.layout.connections.length === 0) {
            // No connections yet - just ensure steps have some sequence
            if (!silent) this.showNotification('No connections found - create connections first', 'warning');
            return;
        }

        console.log('🔢 Starting auto-sequencing...');
        console.log(`   Steps: ${this.steps.length}, Connections: ${this.layout.connections.length}`);

        // Build adjacency lists
        const incomingConnections = new Map();  // stepId -> [{from, type}]
        const outgoingConnections = new Map();  // stepId -> [{to, type}]

        // Initialize all steps
        this.steps.forEach(step => {
            incomingConnections.set(step.id, []);
            outgoingConnections.set(step.id, []);
        });

        // Populate from connections
        this.layout.connections.forEach(conn => {
            if (incomingConnections.has(conn.to)) {
                incomingConnections.get(conn.to).push({
                    from: conn.from,
                    type: conn.type || 'sequential'
                });
            }
            if (outgoingConnections.has(conn.from)) {
                outgoingConnections.get(conn.from).push({
                    to: conn.to,
                    type: conn.type || 'sequential'
                });
            }
        });

        // Find root steps (no incoming connections)
        const rootSteps = this.steps.filter(step =>
            incomingConnections.get(step.id).length === 0
        );

        if (rootSteps.length === 0) {
            // If no roots found, pick the first step as root
            rootSteps.push(this.steps[0]);
            console.log('   ⚠️ No root steps found, using first step as root');
        }

        console.log(`   Root steps: ${rootSteps.map(s => s.stepName).join(', ')}`);

        // BFS to assign sequence numbers
        const sequences = new Map();  // stepId -> sequence
        const visited = new Set();
        const queue = [];

        // Start with root steps at sequence 10
        rootSteps.forEach(step => {
            sequences.set(step.id, 10);
            queue.push(step.id);
        });

        while (queue.length > 0) {
            const currentId = queue.shift();

            if (visited.has(currentId)) continue;
            visited.add(currentId);

            const currentSeq = sequences.get(currentId);
            const outgoing = outgoingConnections.get(currentId) || [];

            // Group outgoing connections by type
            const conditionalTargets = [];  // TRUE/FALSE branches
            const sequentialTargets = [];   // Regular sequential

            outgoing.forEach(conn => {
                if (conn.type === 'conditional_true' || conn.type === 'conditional_false') {
                    conditionalTargets.push(conn);
                } else {
                    sequentialTargets.push(conn);
                }
            });

            // Handle conditional branches - ALL get SAME sequence (they're alternatives)
            if (conditionalTargets.length > 0) {
                const branchSeq = currentSeq + 10;
                console.log(`   🔀 Conditional branch from ${currentId} (seq ${currentSeq}) → all targets get seq ${branchSeq}`);

                conditionalTargets.forEach(conn => {
                    // Only update if not already set or if new seq is higher
                    const existingSeq = sequences.get(conn.to);
                    if (existingSeq === undefined || branchSeq > existingSeq) {
                        sequences.set(conn.to, branchSeq);
                    }
                    if (!visited.has(conn.to)) {
                        queue.push(conn.to);
                    }
                });
            }

            // Handle sequential connections
            sequentialTargets.forEach(conn => {
                const targetId = conn.to;
                const targetIncoming = incomingConnections.get(targetId) || [];

                // Calculate sequence based on ALL incoming connections
                let maxIncomingSeq = currentSeq;
                targetIncoming.forEach(inc => {
                    const incSeq = sequences.get(inc.from);
                    if (incSeq !== undefined && incSeq > maxIncomingSeq) {
                        maxIncomingSeq = incSeq;
                    }
                });

                const newSeq = maxIncomingSeq + 10;
                const existingSeq = sequences.get(targetId);

                // For join points (multiple incoming), use max + 10
                if (existingSeq === undefined || newSeq > existingSeq) {
                    sequences.set(targetId, newSeq);
                }

                if (!visited.has(targetId)) {
                    queue.push(targetId);
                }
            });
        }

        // Handle any disconnected steps (give them high sequence)
        let maxSeq = 10;
        sequences.forEach(seq => {
            if (seq > maxSeq) maxSeq = seq;
        });

        this.steps.forEach(step => {
            if (!sequences.has(step.id)) {
                maxSeq += 10;
                sequences.set(step.id, maxSeq);
                console.log(`   ⚠️ Disconnected step ${step.stepName} assigned sequence ${maxSeq}`);
            }
        });

        // Apply sequences to steps
        let changedCount = 0;
        this.steps.forEach(step => {
            const newSeq = sequences.get(step.id);
            if (newSeq !== undefined && step.sequence !== newSeq) {
                console.log(`   📝 ${step.stepName}: ${step.sequence} → ${newSeq}`);
                step.sequence = newSeq;
                changedCount++;
            }
        });

        console.log(`✅ Auto-sequencing complete: ${changedCount} steps updated`);

        // Only mark unsaved and re-render if called manually (not during save)
        if (!silent) {
            // Mark pipeline as unsaved
            if (window.pipelineBuilder) {
                window.pipelineBuilder.markAsUnsaved();
            }

            // Re-render flowchart
            this.render(this.steps);

            this.showNotification(`Auto-sequenced ${changedCount} steps based on connections`, 'success');
        }

        return sequences;
    }

    /**
     * Setup auto-sequence button
     */
    setupAutoSequenceButton() {
        const autoSeqBtn = document.getElementById('autoSequenceBtn');
        if (!autoSeqBtn) return;

        autoSeqBtn.addEventListener('click', () => {
            this.autoSequenceSteps();
        });

        // Keyboard shortcut: Ctrl+Shift+S for auto-sequence
        document.addEventListener('keydown', (e) => {
            if (e.ctrlKey && e.shiftKey && e.key === 'S') {
                e.preventDefault();
                this.autoSequenceSteps();
            }
        });

        console.log('✅ Auto-sequence button setup complete (Ctrl+Shift+S shortcut)');
    }

}

// Export for use in other modules
if (typeof module !== 'undefined' && module.exports) {
    module.exports = FlowchartOrchestratorV2;
}
