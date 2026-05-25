/**
 * FlowchartRenderer - Main Orchestrator for Flowchart Mode
 *
 * Features:
 * - Manages flowchart mode rendering
 * - Coordinates layout engine and connector
 * - Handles step node creation in compact mode
 * - Manages interactions (click, drag, hover)
 * - Provides zoom and pan controls
 */
class FlowchartRenderer {
    constructor(containerElement, pipelineBuilder) {
        this.container = containerElement;
        this.pipelineBuilder = pipelineBuilder;

        this.flowchartCanvas = null;
        this.svgLayer = null;
        this.layoutEngine = null;
        this.connector = null;

        this.zoomLevel = 1;
        this.panOffset = { x: 0, y: 0 };
        this.selectedStepId = null;

        // Manual connection creation state
        this.connectionDragState = {
            active: false,
            fromStep: null,
            tempLine: null
        };

        // Custom manual connections (stored separately from auto-connections)
        this.manualConnections = [];

        this.init();
    }

    /**
     * Initialize flowchart mode
     */
    init() {
        console.log('🚀 [FlowchartRenderer v1.3] Initializing flowchart mode');

        this.createFlowchartCanvas();
        this.createSVGLayer();

        this.layoutEngine = new FlowchartLayoutEngine({
            stepSpacing: 120,
            branchSpacing: 180,
            startX: 400,
            startY: 80,
            stepWidth: 140,
            stepHeight: 90
        });

        this.connector = new FlowchartConnector(this.svgLayer, this.layoutEngine);

        this.setupEventListeners();

        console.log('✅ [FlowchartRenderer v1.3] Initialization complete');
    }

    /**
     * Create flowchart canvas container
     */
    createFlowchartCanvas() {
        this.flowchartCanvas = document.createElement('div');
        this.flowchartCanvas.className = 'flowchart-canvas';
        this.flowchartCanvas.id = 'flowchartCanvas';
    }

    /**
     * Create SVG layer for connections
     */
    createSVGLayer() {
        this.svgLayer = document.createElementNS('http://www.w3.org/2000/svg', 'svg');
        this.svgLayer.classList.add('flowchart-connections');
        this.svgLayer.setAttribute('width', '100%');
        this.svgLayer.setAttribute('height', '100%');
    }

    /**
     * Render pipeline in flowchart mode
     */
    render(steps) {
        console.log('🎨 [FlowchartRenderer v1.2] Render called with', steps?.length || 0, 'steps');

        if (!steps || steps.length === 0) {
            console.warn('⚠️ No steps to render - showing empty state');
            this.renderEmptyState();
            return;
        }

        // Store steps for redrawing connections
        this.currentSteps = steps;

        // Calculate layout
        console.log('📐 Calculating layout...');
        const layout = this.layoutEngine.calculateLayout(steps);
        console.log('📊 Layout calculated:', {
            positionsCount: layout.positions.size,
            connectionsCount: layout.connections.length,
            totalHeight: layout.totalHeight
        });

        // Load saved positions
        const savedPositions = this.loadSavedPositions();

        // Apply saved positions if they exist
        if (Object.keys(savedPositions).length > 0) {
            steps.forEach(step => {
                const savedPos = savedPositions[step.id];
                if (savedPos) {
                    const position = layout.positions.get(step.id);
                    if (position) {
                        position.x = savedPos.x + position.width / 2;
                        position.y = savedPos.y;
                    }
                }
            });
        }

        // Load and add manual connections
        const savedManualConnections = this.loadManualConnections();
        savedManualConnections.forEach(conn => {
            // Check if this connection already exists
            const exists = layout.connections.some(c =>
                c.from === conn.from && c.to === conn.to
            );
            if (!exists) {
                layout.connections.push({
                    type: 'manual',
                    from: conn.from,
                    to: conn.to
                });
            }
        });

        // Clear existing content
        this.flowchartCanvas.innerHTML = '';

        // Add SVG layer first (connections go behind nodes)
        this.flowchartCanvas.appendChild(this.svgLayer);

        // Render container boxes FIRST (z-index 2, behind step nodes)
        if (layout.containers && layout.containers.size > 0) {
            layout.containers.forEach((containerInfo, containerId) => {
                const position = layout.positions.get(containerId);
                if (position && position.isContainer) {
                    this.renderContainerBox(containerInfo.step, position, containerInfo);
                }
            });
        }

        // Render all step nodes (z-index 3, on top of containers)
        steps.forEach(step => {
            const position = layout.positions.get(step.id);
            if (position && !position.isContainer) {
                this.renderStepNode(step, position);
            }
        });

        // Render connections
        console.log(`🔗 Rendering ${layout.connections.length} connections`);
        this.connector.renderConnections(layout.connections, steps);

        // Update SVG dimensions
        const bbox = this.layoutEngine.getBoundingBox();
        this.connector.resize(bbox.maxX + 100, bbox.maxY + 100);
    }

    /**
     * Render a single step node in compact mode
     */
    renderStepNode(step, position) {
        const node = document.createElement('div');
        node.className = 'step-node-compact';
        node.id = `step-node-${step.id}`;
        node.setAttribute('data-step-id', step.id);
        node.setAttribute('data-step-type', step.stepType);

        // Position the node
        node.style.left = `${position.x - position.width / 2}px`;
        node.style.top = `${position.y}px`;

        // Check if step has routing
        if (this.hasConditionalRouting(step)) {
            node.classList.add('has-routing');
        }

        // Add connector-specific styling
        if (step.stepType === 'connector.inbound') {
            node.classList.add('connector-node', 'connector-inbound');
        } else if (step.stepType === 'connector.outbound') {
            node.classList.add('connector-node', 'connector-outbound');
        }

        // Make node draggable
        this.makeNodeDraggable(node, step);

        // Sequence badge
        const sequenceBadge = document.createElement('div');
        sequenceBadge.className = 'step-sequence-badge';
        sequenceBadge.textContent = step.sequence;

        // Icon
        const icon = document.createElement('div');
        icon.className = 'step-icon-large';
        icon.innerHTML = this.getStepIcon(step);

        // Name
        const name = document.createElement('div');
        name.className = 'step-name-compact';
        name.textContent = step.stepName || 'Unnamed Step';
        name.title = step.stepName; // Tooltip for full name

        // Actions
        const actions = document.createElement('div');
        actions.className = 'step-actions-compact';

        const configBtn = document.createElement('button');
        configBtn.className = 'step-action-btn-compact config';
        configBtn.innerHTML = '<i class="fas fa-cog"></i>';
        configBtn.title = 'Configure';
        configBtn.onclick = (e) => {
            e.stopPropagation();
            this.openStepConfig(step);
        };

        // Connect button - starts connection drag mode
        const connectBtn = document.createElement('button');
        connectBtn.className = 'step-action-btn-compact connect';
        connectBtn.innerHTML = '<i class="fas fa-arrow-right"></i>';
        connectBtn.title = 'Connect to another step';
        connectBtn.onclick = (e) => {
            e.stopPropagation();
            e.preventDefault();
            this.startConnectionMode(step, node);
        };

        const deleteBtn = document.createElement('button');
        deleteBtn.className = 'step-action-btn-compact delete';
        deleteBtn.innerHTML = '<i class="fas fa-trash"></i>';
        deleteBtn.title = 'Delete';
        deleteBtn.onclick = (e) => {
            e.stopPropagation();
            this.deleteStep(step);
        };

        actions.appendChild(configBtn);
        actions.appendChild(connectBtn);
        actions.appendChild(deleteBtn);

        // Drag handle for connection creation (right side of node)
        const dragHandle = document.createElement('div');
        dragHandle.className = 'connection-drag-handle';
        dragHandle.innerHTML = '⟩';
        dragHandle.title = 'Drag to connect to another step';
        dragHandle.setAttribute('data-step-id', step.id);

        // Assemble node
        node.appendChild(sequenceBadge);
        node.appendChild(icon);
        node.appendChild(name);
        node.appendChild(actions);
        node.appendChild(dragHandle);

        // Drag handle - mousedown starts drag connection
        dragHandle.addEventListener('mousedown', (e) => {
            e.stopPropagation();
            e.preventDefault();
            this.startDragConnection(step, node, e);
        });

        // Click handler
        node.onclick = () => this.selectStep(step.id);

        // Hover handlers for connection highlighting
        node.onmouseenter = () => this.connector.highlightStepConnections(step.id);
        node.onmouseleave = () => this.connector.unhighlightStepConnections(step.id);

        this.flowchartCanvas.appendChild(node);
    }

    /**
     * Render a visual container box (Loop)
     */
    renderContainerBox(containerStep, position, containerInfo) {
        const box = document.createElement('div');
        box.className = 'container-box';
        box.id = `container-${containerStep.id}`;
        box.setAttribute('data-container-id', containerStep.id);
        box.setAttribute('data-container-type', containerStep.stepType);

        // Add type-specific class
        if (VisualStep.isLoopStep(containerStep)) {
            box.classList.add('container-loop');
        }

        // Position the container
        box.style.left = `${position.x - position.width / 2}px`;
        box.style.top = `${position.y}px`;
        box.style.width = `${position.width}px`;
        box.style.height = `${position.height}px`;

        // Header
        const header = document.createElement('div');
        header.className = 'container-header';

        const icon = document.createElement('span');
        icon.className = 'container-icon';
        icon.textContent = this.getContainerIcon(containerStep);

        const title = document.createElement('span');
        title.className = 'container-title';
        title.textContent = containerStep.stepName || this.getContainerLabel(containerStep);

        const totalChildren = Object.values(containerInfo.children)
            .reduce((sum, arr) => sum + arr.length, 0);
        const badge = document.createElement('span');
        badge.className = 'container-badge';
        badge.textContent = `${totalChildren} step${totalChildren !== 1 ? 's' : ''}`;

        // Config button in header
        const configBtn = document.createElement('button');
        configBtn.className = 'container-config-btn';
        configBtn.innerHTML = '<i class="fas fa-cog"></i>';
        configBtn.title = 'Configure';
        configBtn.onclick = (e) => {
            e.stopPropagation();
            this.openStepConfig(containerStep);
        };

        header.appendChild(icon);
        header.appendChild(title);
        header.appendChild(badge);
        header.appendChild(configBtn);

        box.appendChild(header);

        // Body with zones
        const body = document.createElement('div');
        body.className = 'container-body';

        if (position.zones) {
            position.zones.forEach(zoneMeta => {
                const zoneDiv = document.createElement('div');
                zoneDiv.className = `container-zone zone-${zoneMeta.zone}`;
                zoneDiv.setAttribute('data-zone', zoneMeta.zone);
                zoneDiv.setAttribute('data-container-id', containerStep.id);

                // Position zone within container
                const containerLeft = position.x - position.width / 2;
                zoneDiv.style.left = `${zoneMeta.x - containerLeft}px`;
                zoneDiv.style.top = `${zoneMeta.y - position.y}px`;
                zoneDiv.style.width = `${zoneMeta.width}px`;
                zoneDiv.style.height = `${zoneMeta.height}px`;

                // Zone label
                const label = document.createElement('div');
                label.className = 'zone-label';
                label.textContent = zoneMeta.zone.toUpperCase();
                zoneDiv.appendChild(label);

                // Empty zone hint
                const children = containerInfo.children[zoneMeta.zone] || [];
                if (children.length === 0) {
                    const hint = document.createElement('div');
                    hint.className = 'zone-empty-hint';
                    hint.textContent = 'Drag steps here';
                    zoneDiv.appendChild(hint);
                }

                body.appendChild(zoneDiv);
            });
        }

        box.appendChild(body);

        // Make header draggable (moves entire container + children)
        this.makeContainerDraggable(box, containerStep, header);

        // Click header to select container
        header.addEventListener('click', (e) => {
            e.stopPropagation();
            this.selectStep(containerStep.id);
        });

        this.flowchartCanvas.appendChild(box);
    }

    /**
     * Get container icon
     */
    getContainerIcon(step) {
        if (VisualStep.isLoopStep(step)) return '\uD83D\uDD01';      // 🔁
        return '\uD83D\uDCE6';                                         // 📦
    }

    /**
     * Get container label
     */
    getContainerLabel(step) {
        if (VisualStep.isLoopStep(step)) return 'For-Each Loop';
        return 'Container';
    }

    /**
     * Make container draggable by its header.
     * Dragging moves the container AND all child step nodes together.
     */
    makeContainerDraggable(containerBox, containerStep, headerElement) {
        let isDragging = false;
        let startX = 0;
        let startY = 0;
        let initialLeft = 0;
        let initialTop = 0;

        headerElement.addEventListener('mousedown', (e) => {
            if (e.target.closest('.container-config-btn')) return;

            isDragging = true;
            startX = e.clientX;
            startY = e.clientY;

            const rect = containerBox.getBoundingClientRect();
            const canvasRect = this.flowchartCanvas.getBoundingClientRect();
            initialLeft = rect.left - canvasRect.left;
            initialTop = rect.top - canvasRect.top;

            containerBox.classList.add('dragging');
            e.stopPropagation();
            e.preventDefault();
        });

        const throttledRedraw = this.throttle(() => {
            this.redrawConnections();
        }, 16);

        document.addEventListener('mousemove', (e) => {
            if (!isDragging) return;

            const deltaX = e.clientX - startX;
            const deltaY = e.clientY - startY;

            const newLeft = initialLeft + deltaX;
            const newTop = initialTop + deltaY;

            // Move container box
            containerBox.style.left = `${newLeft}px`;
            containerBox.style.top = `${newTop}px`;

            // Update container position in layout engine
            const containerPos = this.layoutEngine.positions.get(containerStep.id);
            if (containerPos) {
                const oldCenterX = containerPos.x;
                const oldY = containerPos.y;
                containerPos.x = newLeft + containerPos.width / 2;
                containerPos.y = newTop;

                // Move child step nodes by the same delta
                const moveDeltaX = containerPos.x - oldCenterX;
                const moveDeltaY = containerPos.y - oldY;

                // Update zone positions
                if (containerPos.zones) {
                    containerPos.zones.forEach(z => {
                        z.x += moveDeltaX;
                        z.y += moveDeltaY;
                        z.centerX += moveDeltaX;
                    });
                }

                // Move child steps
                const containerInfo = this.layoutEngine.containers.get(containerStep.id);
                if (containerInfo) {
                    Object.values(containerInfo.children).forEach(childList => {
                        childList.forEach(child => {
                            const childPos = this.layoutEngine.positions.get(child.id);
                            if (childPos) {
                                childPos.x += moveDeltaX;
                                childPos.y += moveDeltaY;

                                // Move the DOM node too
                                const childNode = document.getElementById(`step-node-${child.id}`);
                                if (childNode) {
                                    childNode.style.left = `${childPos.x - childPos.width / 2}px`;
                                    childNode.style.top = `${childPos.y}px`;
                                }
                            }
                        });
                    });
                }
            }

            throttledRedraw();
        });

        document.addEventListener('mouseup', () => {
            if (isDragging) {
                isDragging = false;
                containerBox.classList.remove('dragging');

                // Save container position
                this.saveNodePosition(containerStep.id, {
                    x: parseInt(containerBox.style.left),
                    y: parseInt(containerBox.style.top)
                });
            }
        });
    }

    /**
     * Get icon for step type
     */
    getStepIcon(step) {
        const iconMap = {
            'field_validation': '✅',
            'fhir_validation': '🛡️',
            'enrichment.api': '🌐',
            'enrichment.database': '💾',
            'enrichment.script': '💻',
            'hl7_fhir_transform': '🔄',
            'field_mapping': '🔄',
            'if_then_else': '🔀',
            'switch_case': '🔀',
            'connector.inbound': '📥',
            'connector.outbound': '📤',
            'control.loop': '🔁',
            // Generic fallbacks
            validation: '✅',
            enrichment: '🔍',
            mapping: '🔄',
            transformation: '⚙️',
            control: '🔀',
            custom: '💻',
            database: '💾',
            api: '🌐'
        };

        // Check full stepType first (e.g., 'connector.inbound'), then subType, then base type
        const stepType = step.stepType || '';
        return iconMap[stepType] || iconMap[step.subType] || iconMap[stepType.split('.')[0]] || '⚙️';
    }

    /**
     * Check if step has conditional routing
     */
    hasConditionalRouting(step) {
        if (step.stepType !== 'control' || step.subType !== 'conditional') {
            return false;
        }

        const config = step.config;
        if (!config || !config.conditions) return false;

        return config.conditions.some(cond =>
            (cond.ifTrue && cond.ifTrue.action === 'route_to_step') ||
            (cond.ifFalse && cond.ifFalse.action === 'route_to_step')
        );
    }

    /**
     * Select a step
     */
    selectStep(stepId) {
        // Deselect previous
        if (this.selectedStepId) {
            const prevNode = document.getElementById(`step-node-${this.selectedStepId}`);
            if (prevNode) prevNode.classList.remove('selected');
        }

        // Select new
        this.selectedStepId = stepId;
        const node = document.getElementById(`step-node-${stepId}`);
        if (node) {
            node.classList.add('selected');
        }

        // Notify pipeline builder
        if (this.pipelineBuilder && this.pipelineBuilder.onStepSelected) {
            this.pipelineBuilder.onStepSelected(stepId);
        }
    }

    /**
     * Open step configuration
     */
    openStepConfig(step) {
        if (this.pipelineBuilder && this.pipelineBuilder.openStepProperties) {
            this.pipelineBuilder.openStepProperties(step);
        }
    }

    /**
     * Delete step
     */
    deleteStep(step) {
        if (this.pipelineBuilder && this.pipelineBuilder.deleteStep) {
            this.pipelineBuilder.deleteStep(step.id);
        }
    }

    /**
     * Render empty state
     */
    renderEmptyState() {
        this.flowchartCanvas.innerHTML = `
            <div class="flowchart-empty-state">
                <i class="fas fa-project-diagram"></i>
                <h3>No Steps in Pipeline</h3>
                <p>Drag steps from the toolbox to get started.<br>
                   Your pipeline flow will appear here.</p>
            </div>
        `;
    }

    /**
     * Make a node draggable.
     *
     * Uses a 5-px drag threshold so that simple clicks reliably fire the onclick
     * handler (which opens the properties panel) while intentional drags still
     * reposition the node on the canvas.  The old implementation set isDragging=true
     * on mousedown immediately, so even microscopic hand-tremor movement would move
     * the node AND suppress the click event, preventing properties from opening.
     */
    makeNodeDraggable(node, step) {
        const DRAG_THRESHOLD = 5; // pixels before drag is considered intentional

        let mouseIsDown = false;  // true from mousedown until mouseup
        let dragStarted = false;  // true once we have crossed the DRAG_THRESHOLD
        let startX = 0;
        let startY = 0;
        let initialLeft = 0;
        let initialTop = 0;

        node.addEventListener('mousedown', (e) => {
            // Don't drag if clicking action buttons or drag handle
            if (e.target.closest('.step-action-btn-compact') || e.target.closest('.connection-drag-handle')) {
                return;
            }

            mouseIsDown = true;
            dragStarted = false;
            startX = e.clientX;
            startY = e.clientY;

            // Capture initial position
            const rect = node.getBoundingClientRect();
            const canvasRect = this.flowchartCanvas.getBoundingClientRect();
            initialLeft = rect.left - canvasRect.left;
            initialTop = rect.top - canvasRect.top;

            e.stopPropagation(); // Prevent canvas pan
            // NOTE: do NOT call e.preventDefault() here — it suppresses the
            // subsequent click event in some browsers, blocking properties from opening.
        });

        const throttledRedraw = this.throttle(() => {
            this.redrawConnections();
        }, 16); // 60fps max

        document.addEventListener('mousemove', (e) => {
            if (!mouseIsDown) return;

            const deltaX = e.clientX - startX;
            const deltaY = e.clientY - startY;
            const distance = Math.sqrt(deltaX * deltaX + deltaY * deltaY);

            // Cross the threshold → begin a real drag
            if (!dragStarted) {
                if (distance < DRAG_THRESHOLD) return; // still looks like a click
                dragStarted = true;
                node.classList.add('dragging');

                // Suppress the click event that fires after mouseup so that
                // releasing the mouse after a drag does not also open properties.
                node.addEventListener('click', (ev) => {
                    ev.stopImmediatePropagation();
                    ev.preventDefault();
                }, { once: true, capture: true });
            }

            const newLeft = initialLeft + deltaX;
            const newTop = initialTop + deltaY;

            node.style.left = `${newLeft}px`;
            node.style.top = `${newTop}px`;

            // Update position in layout engine
            const position = this.layoutEngine.positions.get(step.id);
            if (position) {
                position.x = newLeft + position.width / 2;
                position.y = newTop;
            }

            // Highlight container zone if dragging over one
            // Temporarily hide the node so elementFromPoint can see what's behind it
            node.style.pointerEvents = 'none';
            document.querySelectorAll('.container-zone.drag-hover').forEach(z => z.classList.remove('drag-hover'));
            const elementBelow = document.elementFromPoint(e.clientX, e.clientY);
            const zoneBelow = elementBelow?.closest('.container-zone');
            if (zoneBelow) {
                // Don't allow dropping a container into itself
                const targetContainerId = zoneBelow.dataset.containerId;
                if (targetContainerId !== step.id) {
                    zoneBelow.classList.add('drag-hover');
                }
            }
            node.style.pointerEvents = '';

            // Throttled redraw for performance
            throttledRedraw();
        });

        document.addEventListener('mouseup', (e) => {
            if (!mouseIsDown) return;
            mouseIsDown = false;

            if (!dragStarted) return; // Was a plain click — let onclick handle it

            node.classList.remove('dragging');
            dragStarted = false;

            // Check if dropped onto a container zone
            node.style.pointerEvents = 'none';
            const elementBelow = document.elementFromPoint(e.clientX, e.clientY);
            node.style.pointerEvents = '';
            const droppedZone = elementBelow?.closest('.container-zone');

            // Clear all zone highlights
            document.querySelectorAll('.container-zone.drag-hover').forEach(z => z.classList.remove('drag-hover'));

            if (droppedZone) {
                const containerId = droppedZone.dataset.containerId;
                const zone = droppedZone.dataset.zone;

                // Don't allow container to contain itself
                if (containerId && zone && containerId !== step.id) {
                    this.assignStepToContainer(step, containerId, zone);
                    return; // Skip normal save - re-render will reposition
                }
            }

            // Normal drop (not into container) - save position
            this.saveNodePosition(step.id, {
                x: parseInt(node.style.left),
                y: parseInt(node.style.top)
            });
        });
    }

    /**
     * Assign a step to a container zone (from canvas drag-drop).
     * Updates the step's parentStepId/containerZone AND the container's config arrays.
     * Then re-renders to reflow the layout.
     */
    assignStepToContainer(step, containerId, zone) {
        // Find container step
        const allSteps = this.currentSteps || [];
        const containerStep = allSteps.find(s => s.id === containerId);
        if (!containerStep) return;

        // Remove from previous container if any
        if (step.parentStepId && step.parentStepId !== containerId) {
            this.removeStepFromContainer(step);
        }

        // Set parent relationship on the step
        step.parentStepId = containerId;
        step.containerZone = zone;

        // Update container config arrays
        if (!containerStep.config) containerStep.config = {};

        if (VisualStep.isLoopStep(containerStep)) {
            if (!containerStep.config.childStepIds) containerStep.config.childStepIds = [];
            if (!containerStep.config.childStepIds.includes(step.id)) {
                containerStep.config.childStepIds.push(step.id);
            }
        }

        console.log(`📦 Assigned "${step.stepName}" to container "${containerStep.stepName}" zone=${zone}`);

        // Mark pipeline as unsaved and re-render
        this.pipelineBuilder.markAsUnsaved();
        this.refresh();
    }

    /**
     * Remove a step from its current container.
     * Clears parentStepId/containerZone and removes from config arrays.
     */
    removeStepFromContainer(step) {
        if (!step.parentStepId) return;

        const allSteps = this.currentSteps || [];
        const containerStep = allSteps.find(s => s.id === step.parentStepId);

        if (containerStep && containerStep.config) {
            // Remove from loop array
            if (Array.isArray(containerStep.config.childStepIds)) {
                containerStep.config.childStepIds = containerStep.config.childStepIds.filter(id => id !== step.id);
            }
        }

        step.parentStepId = null;
        step.containerZone = null;

        console.log(`📤 Removed "${step.stepName}" from container`);
    }

    /**
     * Redraw all connections
     */
    redrawConnections() {
        if (!this.currentSteps) return;

        // Get current connections from layout
        const connections = this.layoutEngine.connections;

        // Re-render all connections with updated positions
        this.connector.clearConnections();
        this.connector.renderConnections(connections, this.currentSteps);
    }

    /**
     * Save node position to localStorage
     */
    saveNodePosition(stepId, position) {
        const key = `flowchart_positions_${this.pipelineBuilder.interfaceId}_${this.pipelineBuilder.messageType}`;
        let positions = {};

        try {
            const saved = localStorage.getItem(key);
            if (saved) {
                positions = JSON.parse(saved);
            }
        } catch (e) {
            console.warn('Failed to load saved positions:', e);
        }

        positions[stepId] = position;

        try {
            localStorage.setItem(key, JSON.stringify(positions));
        } catch (e) {
            console.warn('Failed to save position:', e);
        }
    }

    /**
     * Load saved node positions
     */
    loadSavedPositions() {
        const key = `flowchart_positions_${this.pipelineBuilder.interfaceId}_${this.pipelineBuilder.messageType}`;

        try {
            const saved = localStorage.getItem(key);
            if (saved) {
                return JSON.parse(saved);
            }
        } catch (e) {
            console.warn('Failed to load saved positions:', e);
        }

        return {};
    }

    // ========================================
    // MANUAL CONNECTION CREATION (NO-CODE)
    // Two methods:
    // 1. Click-based: Click connect button, then click target step
    // 2. Drag-based: Drag from handle to target step
    // ========================================

    /**
     * Start drag connection - mousedown on drag handle
     */
    startDragConnection(fromStep, fromNode, e) {
        console.log('🔗 [Drag Connection] ===== DRAG START =====');
        console.log('🔗 [Drag Connection] From step:', fromStep.stepName, '(seq:', fromStep.sequence, ', id:', fromStep.id, ')');
        console.log('🔗 [Drag Connection] Mouse position:', e.clientX, e.clientY);

        // Store drag state
        this.dragConnection = {
            active: true,
            fromStep: fromStep,
            fromNode: fromNode,
            tempLine: null
        };

        // Create visual line
        const rect = fromNode.getBoundingClientRect();
        const canvasRect = this.flowchartCanvas.getBoundingClientRect();
        const startX = (rect.right - canvasRect.left) / this.zoomLevel;
        const startY = (rect.top + rect.height / 2 - canvasRect.top) / this.zoomLevel;

        const line = document.createElementNS('http://www.w3.org/2000/svg', 'line');
        line.setAttribute('x1', startX);
        line.setAttribute('y1', startY);
        line.setAttribute('x2', startX);
        line.setAttribute('y2', startY);
        line.setAttribute('stroke', '#3b82f6');
        line.setAttribute('stroke-width', '3');
        line.setAttribute('stroke-dasharray', '8,4');
        line.setAttribute('stroke-linecap', 'round');
        line.classList.add('temp-connection-line');
        line.style.pointerEvents = 'none';

        this.svgLayer.appendChild(line);
        this.dragConnection.tempLine = line;
        this.dragConnection.startX = startX;
        this.dragConnection.startY = startY;

        // Highlight source
        fromNode.classList.add('connection-source');

        // Mark all other steps as potential targets
        document.querySelectorAll('.step-node-compact').forEach(node => {
            if (node.id !== `step-node-${fromStep.id}`) {
                node.classList.add('connection-target-available');
            }
        });

        // Add global mouse handlers
        this._dragMoveHandler = (e) => this.handleDragConnectionMove(e);
        this._dragUpHandler = (e) => this.handleDragConnectionEnd(e);
        document.addEventListener('mousemove', this._dragMoveHandler);
        document.addEventListener('mouseup', this._dragUpHandler);

        // Change cursor
        document.body.style.cursor = 'crosshair';
    }

    /**
     * Handle mouse move during drag connection
     */
    handleDragConnectionMove(e) {
        if (!this.dragConnection || !this.dragConnection.active) return;

        const canvasRect = this.flowchartCanvas.getBoundingClientRect();
        const endX = (e.clientX - canvasRect.left) / this.zoomLevel;
        const endY = (e.clientY - canvasRect.top) / this.zoomLevel;

        this.dragConnection.tempLine.setAttribute('x2', endX);
        this.dragConnection.tempLine.setAttribute('y2', endY);

        // Check if over a valid target
        const targetNode = document.elementFromPoint(e.clientX, e.clientY)?.closest('.step-node-compact');

        // Remove previous hover
        document.querySelectorAll('.step-node-compact.connection-target-hover').forEach(n => {
            n.classList.remove('connection-target-hover');
        });

        if (targetNode && targetNode.classList.contains('connection-target-available')) {
            targetNode.classList.add('connection-target-hover');

            // Snap line to target center
            const targetRect = targetNode.getBoundingClientRect();
            const snapX = (targetRect.left + targetRect.width / 2 - canvasRect.left) / this.zoomLevel;
            const snapY = (targetRect.top + targetRect.height / 2 - canvasRect.top) / this.zoomLevel;
            this.dragConnection.tempLine.setAttribute('x2', snapX);
            this.dragConnection.tempLine.setAttribute('y2', snapY);
            this.dragConnection.tempLine.setAttribute('stroke', '#22c55e');
            this.dragConnection.tempLine.setAttribute('stroke-width', '4');

            this.dragConnection.targetNode = targetNode;
        } else {
            this.dragConnection.tempLine.setAttribute('stroke', '#3b82f6');
            this.dragConnection.tempLine.setAttribute('stroke-width', '3');
            this.dragConnection.targetNode = null;
        }
    }

    /**
     * Handle mouse up - complete or cancel drag connection
     */
    handleDragConnectionEnd(e) {
        if (!this.dragConnection || !this.dragConnection.active) return;

        console.log('🔗 [Drag Connection] ===== DRAG END =====');
        console.log('🔗 [Drag Connection] Mouse position:', e.clientX, e.clientY);

        // Remove global handlers
        document.removeEventListener('mousemove', this._dragMoveHandler);
        document.removeEventListener('mouseup', this._dragUpHandler);
        document.body.style.cursor = '';

        // Check if we're over a valid target
        const elementAtPoint = document.elementFromPoint(e.clientX, e.clientY);
        console.log('🔗 [Drag Connection] Element at point:', elementAtPoint?.tagName, elementAtPoint?.className);

        const targetNode = elementAtPoint?.closest('.step-node-compact');
        console.log('🔗 [Drag Connection] Target node:', targetNode?.id);

        if (targetNode && targetNode.classList.contains('connection-target-available')) {
            const targetStepId = targetNode.getAttribute('data-step-id');
            const targetStep = this.currentSteps.find(s => s.id === targetStepId);

            console.log('🔗 [Drag Connection] Target step:', targetStep?.stepName, '(seq:', targetStep?.sequence, ')');

            if (targetStep) {
                // Create the connection
                this.addManualConnection(this.dragConnection.fromStep, targetStep);
                this.showConnectionModeNotification(
                    `✅ Connected: ${this.dragConnection.fromStep.stepName} → ${targetStep.stepName}`,
                    'success'
                );
            }
        } else {
            console.log('🔗 [Drag Connection] No valid target - connection cancelled');
        }

        // Cleanup
        if (this.dragConnection.tempLine) {
            this.dragConnection.tempLine.remove();
        }

        if (this.dragConnection.fromNode) {
            this.dragConnection.fromNode.classList.remove('connection-source');
        }

        document.querySelectorAll('.step-node-compact').forEach(node => {
            node.classList.remove('connection-target-available');
            node.classList.remove('connection-target-hover');
        });

        this.dragConnection = {
            active: false,
            fromStep: null,
            fromNode: null,
            tempLine: null,
            targetNode: null
        };
    }

    /**
     * Start connection mode - user clicked the connect button on a step
     * Now they need to click on the target step
     */
    startConnectionMode(fromStep, fromNode) {
        console.log('🔗 [Connection Mode] Started from:', fromStep.stepName);

        // If already in connection mode, cancel it
        if (this.connectionMode && this.connectionMode.active) {
            this.cancelConnectionMode();
        }

        this.connectionMode = {
            active: true,
            fromStep: fromStep,
            tempLine: null
        };

        // Highlight source step
        fromNode.classList.add('connection-source');

        // Create visual connection line that follows mouse
        this.createConnectionLine(fromNode);

        // Show notification
        this.showConnectionModeNotification(`Click on a step to connect FROM "${fromStep.stepName}"`);

        // Make all other steps clickable as targets
        document.querySelectorAll('.step-node-compact').forEach(node => {
            if (node.id !== `step-node-${fromStep.id}`) {
                node.classList.add('connection-target-available');

                // Add click handler for this connection session
                const handler = (e) => {
                    e.stopPropagation();
                    e.preventDefault();
                    const targetStepId = node.getAttribute('data-step-id');
                    const targetStep = this.currentSteps.find(s => s.id === targetStepId);
                    if (targetStep) {
                        this.completeConnection(targetStep);
                    }
                };

                node._connectionHandler = handler;
                node.addEventListener('click', handler);
            }
        });

        // Add mouse move listener for visual line
        this._mouseMoveHandler = (e) => this.updateConnectionLine(e);
        document.addEventListener('mousemove', this._mouseMoveHandler);

        // Add escape key listener to cancel
        this._escapeHandler = (e) => {
            if (e.key === 'Escape') {
                this.cancelConnectionMode();
            }
        };
        document.addEventListener('keydown', this._escapeHandler);

        // Add click outside listener to cancel
        this._outsideClickHandler = (e) => {
            if (!e.target.closest('.step-node-compact') && !e.target.closest('#connection-mode-notification')) {
                this.cancelConnectionMode();
            }
        };
        setTimeout(() => {
            document.addEventListener('click', this._outsideClickHandler);
        }, 100);
    }

    /**
     * Create visual connection line from source step
     */
    createConnectionLine(fromNode) {
        const rect = fromNode.getBoundingClientRect();
        const canvasRect = this.flowchartCanvas.getBoundingClientRect();

        // Start from the right side of the source node
        const startX = (rect.right - canvasRect.left) / this.zoomLevel;
        const startY = (rect.top + rect.height / 2 - canvasRect.top) / this.zoomLevel;

        const line = document.createElementNS('http://www.w3.org/2000/svg', 'line');
        line.setAttribute('x1', startX);
        line.setAttribute('y1', startY);
        line.setAttribute('x2', startX);
        line.setAttribute('y2', startY);
        line.setAttribute('stroke', '#3b82f6');
        line.setAttribute('stroke-width', '3');
        line.setAttribute('stroke-dasharray', '8,4');
        line.setAttribute('stroke-linecap', 'round');
        line.classList.add('temp-connection-line');
        line.style.pointerEvents = 'none';

        this.svgLayer.appendChild(line);
        this.connectionMode.tempLine = line;
        this.connectionMode.startX = startX;
        this.connectionMode.startY = startY;
    }

    /**
     * Update visual connection line to follow mouse
     */
    updateConnectionLine(e) {
        if (!this.connectionMode || !this.connectionMode.tempLine) return;

        const canvasRect = this.flowchartCanvas.getBoundingClientRect();
        const endX = (e.clientX - canvasRect.left) / this.zoomLevel;
        const endY = (e.clientY - canvasRect.top) / this.zoomLevel;

        this.connectionMode.tempLine.setAttribute('x2', endX);
        this.connectionMode.tempLine.setAttribute('y2', endY);

        // Check if hovering over a valid target
        const target = e.target.closest('.step-node-compact');
        if (target && target.classList.contains('connection-target-available')) {
            // Snap to target center and turn green
            const targetRect = target.getBoundingClientRect();
            const snapX = (targetRect.left + targetRect.width / 2 - canvasRect.left) / this.zoomLevel;
            const snapY = (targetRect.top + targetRect.height / 2 - canvasRect.top) / this.zoomLevel;
            this.connectionMode.tempLine.setAttribute('x2', snapX);
            this.connectionMode.tempLine.setAttribute('y2', snapY);
            this.connectionMode.tempLine.setAttribute('stroke', '#22c55e');
            this.connectionMode.tempLine.setAttribute('stroke-width', '4');
        } else {
            this.connectionMode.tempLine.setAttribute('stroke', '#3b82f6');
            this.connectionMode.tempLine.setAttribute('stroke-width', '3');
        }
    }

    /**
     * Complete the connection when user clicks on target step
     */
    completeConnection(toStep) {
        if (!this.connectionMode || !this.connectionMode.active) return;

        const fromStep = this.connectionMode.fromStep;
        console.log(`🔗 [Connection Mode] Completing: ${fromStep.stepName} → ${toStep.stepName}`);

        // Add the connection
        this.addManualConnection(fromStep, toStep);

        // Show success notification
        this.showConnectionModeNotification(`✅ Connected: ${fromStep.stepName} → ${toStep.stepName}`, 'success');

        // Clean up connection mode
        this.cancelConnectionMode();
    }

    /**
     * Cancel connection mode
     */
    cancelConnectionMode() {
        if (!this.connectionMode) return;

        console.log('🔗 [Connection Mode] Cancelled');

        // Remove visual connection line
        if (this.connectionMode.tempLine) {
            this.connectionMode.tempLine.remove();
        }

        // Remove source highlight
        if (this.connectionMode.fromStep) {
            const sourceNode = document.getElementById(`step-node-${this.connectionMode.fromStep.id}`);
            if (sourceNode) {
                sourceNode.classList.remove('connection-source');
            }
        }

        // Remove target highlights and handlers
        document.querySelectorAll('.step-node-compact').forEach(node => {
            node.classList.remove('connection-target-available');
            node.classList.remove('connection-target-hover');
            if (node._connectionHandler) {
                node.removeEventListener('click', node._connectionHandler);
                delete node._connectionHandler;
            }
        });

        // Remove mouse move listener
        if (this._mouseMoveHandler) {
            document.removeEventListener('mousemove', this._mouseMoveHandler);
            delete this._mouseMoveHandler;
        }

        // Remove escape listener
        if (this._escapeHandler) {
            document.removeEventListener('keydown', this._escapeHandler);
            delete this._escapeHandler;
        }

        // Remove outside click listener
        if (this._outsideClickHandler) {
            document.removeEventListener('click', this._outsideClickHandler);
            delete this._outsideClickHandler;
        }

        // Hide notification
        this.hideConnectionModeNotification();

        this.connectionMode = {
            active: false,
            fromStep: null,
            tempLine: null
        };
    }

    /**
     * Show connection mode notification
     */
    showConnectionModeNotification(message, type = 'info') {
        // Remove existing notification
        this.hideConnectionModeNotification();

        const bgColors = {
            success: '#22c55e',
            warning: '#f59e0b',
            error: '#ef4444',
            info: '#3b82f6'
        };

        const notification = document.createElement('div');
        notification.id = 'connection-mode-notification';
        notification.style.cssText = `
            position: fixed;
            top: 80px;
            left: 50%;
            transform: translateX(-50%);
            background: ${bgColors[type] || bgColors.info};
            color: white;
            padding: 12px 24px;
            border-radius: 8px;
            box-shadow: 0 4px 12px rgba(0, 0, 0, 0.15);
            z-index: 10001;
            font-weight: 500;
            font-size: 14px;
            display: flex;
            align-items: center;
            gap: 8px;
        `;

        const showCancelBtn = type === 'info';
        notification.innerHTML = `
            <span>${message}</span>
            ${showCancelBtn ? '<button style="background: rgba(255,255,255,0.2); border: none; color: white; padding: 4px 8px; border-radius: 4px; cursor: pointer; margin-left: 8px;" onclick="window.pipelineBuilder?.flowchartRenderer?.cancelConnectionMode()">Cancel (Esc)</button>' : ''}
        `;

        document.body.appendChild(notification);

        // Auto-hide success and warning notifications
        if (type === 'success' || type === 'warning') {
            setTimeout(() => this.hideConnectionModeNotification(), 2000);
        }
    }

    /**
     * Hide connection mode notification
     */
    hideConnectionModeNotification() {
        const existing = document.getElementById('connection-mode-notification');
        if (existing) {
            existing.remove();
        }
    }

    /**
     * Add a manual connection between two steps
     * Respects user's intended direction (from → to)
     */
    addManualConnection(fromStep, toStep) {
        console.log(`🔗 [Manual Connection] Creating: ${fromStep.stepName} (seq ${fromStep.sequence}) → ${toStep.stepName} (seq ${toStep.sequence})`);

        // Check if this exact connection already exists
        const exists = this.layoutEngine.connections.some(c =>
            c.from === fromStep.id && c.to === toStep.id
        );

        if (exists) {
            console.log('   ⚠️ Connection already exists');
            this.showConnectionModeNotification('⚠️ Connection already exists', 'warning');
            return;
        }

        // Add to layout engine connections - respect user's direction
        this.layoutEngine.connections.push({
            type: 'manual',
            from: fromStep.id,
            to: toStep.id
        });

        // Save to localStorage
        this.saveManualConnections();

        // Redraw connections
        this.redrawConnections();

        // Notify pipeline builder
        this.pipelineBuilder.markAsUnsaved();

        console.log(`   ✅ Connection created successfully`);
    }

    /**
     * Delete a connection (called from context menu)
     */
    deleteConnection(fromId, toId) {
        console.log(`🗑️ [Manual Connection] Deleting: ${fromId} → ${toId}`);

        // Remove from layout engine
        const index = this.layoutEngine.connections.findIndex(c =>
            c.from === fromId && c.to === toId
        );

        if (index !== -1) {
            this.layoutEngine.connections.splice(index, 1);
            this.saveManualConnections();
            this.redrawConnections();
            this.pipelineBuilder.markAsUnsaved();
        }
    }

    /**
     * Save manual connections to localStorage
     */
    saveManualConnections() {
        const key = `flowchart_connections_${this.pipelineBuilder.interfaceId}_${this.pipelineBuilder.messageType}`;

        const manualConnections = this.layoutEngine.connections
            .filter(c => c.type === 'manual')
            .map(c => ({ from: c.from, to: c.to }));

        try {
            localStorage.setItem(key, JSON.stringify(manualConnections));
        } catch (e) {
            console.warn('Failed to save manual connections:', e);
        }
    }

    /**
     * Load manual connections from localStorage
     */
    loadManualConnections() {
        const key = `flowchart_connections_${this.pipelineBuilder.interfaceId}_${this.pipelineBuilder.messageType}`;

        try {
            const saved = localStorage.getItem(key);
            if (saved) {
                return JSON.parse(saved);
            }
        } catch (e) {
            console.warn('Failed to load manual connections:', e);
        }

        return [];
    }

    /**
     * Setup event listeners
     */
    setupEventListeners() {
        // Canvas pan (drag background to move) - Only if not in flowchart mode
        let isPanning = false;
        let startPan = { x: 0, y: 0 };

        this.container.addEventListener('mousedown', (e) => {
            // Only pan if clicking on empty canvas (not on nodes)
            if (e.target === this.flowchartCanvas || e.target === this.svgLayer) {
                isPanning = true;
                startPan = { x: e.clientX - this.panOffset.x, y: e.clientY - this.panOffset.y };
                this.flowchartCanvas.style.cursor = 'grabbing';
            }
        });

        document.addEventListener('mousemove', (e) => {
            if (isPanning) {
                this.panOffset.x = e.clientX - startPan.x;
                this.panOffset.y = e.clientY - startPan.y;
                this.updateTransform();
            }
        });

        document.addEventListener('mouseup', () => {
            if (isPanning) {
                isPanning = false;
                this.flowchartCanvas.style.cursor = '';
            }
        });

        // Zoom with mouse wheel
        this.container.addEventListener('wheel', (e) => {
            e.preventDefault();

            const delta = e.deltaY > 0 ? -0.1 : 0.1;
            this.zoom(delta);
        }, { passive: false });
    }

    /**
     * Zoom in/out
     */
    zoom(delta) {
        const newZoom = Math.max(0.25, Math.min(2, this.zoomLevel + delta));

        if (newZoom !== this.zoomLevel) {
            this.zoomLevel = newZoom;
            this.updateTransform();

            // Update zoom level display if it exists
            const zoomDisplay = document.getElementById('flowchartZoomLevel');
            if (zoomDisplay) {
                zoomDisplay.textContent = `${Math.round(this.zoomLevel * 100)}%`;
            }
        }
    }

    /**
     * Reset zoom and pan
     */
    resetView() {
        this.zoomLevel = 1;
        this.panOffset = { x: 0, y: 0 };
        this.updateTransform();
    }

    /**
     * Update CSS transform for zoom and pan
     */
    updateTransform() {
        this.flowchartCanvas.style.transform =
            `translate(${this.panOffset.x}px, ${this.panOffset.y}px) scale(${this.zoomLevel})`;
        this.flowchartCanvas.style.transformOrigin = '0 0';
    }

    /**
     * Get flowchart canvas element
     */
    getCanvas() {
        return this.flowchartCanvas;
    }

    /**
     * Refresh flowchart (re-render with current pipeline)
     */
    refresh() {
        if (this.pipelineBuilder && this.pipelineBuilder.getPipeline) {
            const pipeline = this.pipelineBuilder.getPipeline();
            this.render(pipeline.steps || []);
        }
    }

    /**
     * Cleanup and destroy
     */
    /**
     * Throttle function for performance
     */
    throttle(func, delay) {
        let lastCall = 0;
        let timeoutId = null;

        return function(...args) {
            const now = Date.now();
            const timeSinceLastCall = now - lastCall;

            if (timeSinceLastCall >= delay) {
                lastCall = now;
                func.apply(this, args);
            } else {
                clearTimeout(timeoutId);
                timeoutId = setTimeout(() => {
                    lastCall = Date.now();
                    func.apply(this, args);
                }, delay - timeSinceLastCall);
            }
        };
    }

    destroy() {
        if (this.connector) {
            this.connector.destroy();
        }

        if (this.flowchartCanvas && this.flowchartCanvas.parentNode) {
            this.flowchartCanvas.parentNode.removeChild(this.flowchartCanvas);
        }

        this.container = null;
        this.pipelineBuilder = null;
        this.layoutEngine = null;
        this.connector = null;
    }
}

// Export for use in other modules
if (typeof module !== 'undefined' && module.exports) {
    module.exports = FlowchartRenderer;
}
