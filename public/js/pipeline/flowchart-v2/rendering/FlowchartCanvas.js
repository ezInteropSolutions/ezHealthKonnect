/**
 * FlowchartCanvas - Main rendering orchestrator for V2 flowchart
 *
 * Features:
 * - HTML Canvas for connection lines
 * - DOM elements for step nodes (better interaction)
 * - Swim lane backgrounds
 * - Coordinate system integration
 */
class FlowchartCanvas {
    constructor(container, coordinateSystem) {
        this.container = container;
        this.coords = coordinateSystem;

        // Canvas layers
        this.canvas = null;
        this.ctx = null;
        this.nodesContainer = null;
        this.swimLanesContainer = null;

        // State
        this.layout = null;
        this.steps = null;
        this.connectionHitAreas = []; // Store connection hit areas for click detection

        this.init();
        console.log('✅ FlowchartCanvas initialized');
    }

    /**
     * Initialize canvas and DOM structure
     */
    init() {
        // Create main container
        this.mainContainer = document.createElement('div');
        this.mainContainer.className = 'flowchart-v2-canvas';
        this.mainContainer.style.cssText = `
            position: relative;
            width: 100%;
            height: 100%;
            overflow: hidden;
            background: #f8fafc;
        `;

        // Create swim lanes container (background)
        this.swimLanesContainer = document.createElement('div');
        this.swimLanesContainer.className = 'flowchart-swim-lanes';
        this.swimLanesContainer.style.cssText = `
            position: absolute;
            top: 0;
            left: 0;
            pointer-events: none;
            transform-origin: 0 0;
        `;

        // Create canvas for connections
        this.canvas = document.createElement('canvas');
        this.canvas.className = 'flowchart-connections-canvas';
        this.canvas.style.cssText = `
            position: absolute;
            top: 0;
            left: 0;
            pointer-events: auto;
            cursor: pointer;
            z-index: 5;
        `;
        this.ctx = this.canvas.getContext('2d');

        // Add right-click context menu for arrow deletion (keeping for backward compat)
        this.canvas.addEventListener('contextmenu', (e) => this.handleCanvasRightClick(e));

        // Add click detection for delete buttons on arrows
        this.canvas.addEventListener('click', (e) => this.handleCanvasClick(e));

        // Add hover feedback for delete buttons
        this.canvas.addEventListener('mousemove', (e) => this.handleCanvasHover(e));

        // Create nodes container
        this.nodesContainer = document.createElement('div');
        this.nodesContainer.className = 'flowchart-nodes';
        this.nodesContainer.style.cssText = `
            position: absolute;
            top: 0;
            left: 0;
            transform-origin: 0 0;
            z-index: 10;
            pointer-events: none;
        `;

        // Assemble layers (bottom to top)
        this.mainContainer.appendChild(this.swimLanesContainer);
        this.mainContainer.appendChild(this.canvas);
        this.mainContainer.appendChild(this.nodesContainer);

        // Append to parent container
        this.container.appendChild(this.mainContainer);

        // Set initial canvas size
        this.resize();

        // Start animation loop for flowing lines
        this.flowAnimationOffset = 0;
        this.startFlowAnimation();
    }

    /**
     * Start continuous animation for flowing lines
     */
    startFlowAnimation() {
        const animate = () => {
            this.flowAnimationOffset += 0.8;  // Animation speed (increased for better visibility)
            if (this.flowAnimationOffset > 30) {
                this.flowAnimationOffset = 0;  // Reset to loop seamlessly
            }

            // Re-render if we have connections
            if (this.layout && this.layout.connections && this.layout.connections.length > 0) {
                this.ctx.clearRect(0, 0, this.canvas.width, this.canvas.height);
                this.renderConnections(this.layout.connections, this.layout.positions);

                // Redraw hover delete button if needed
                if (this.hoveredHitArea) {
                    this.ctx.save();
                    this.coords.applyToCanvasContext(this.ctx);
                    this.drawHoverDeleteButton(this.hoveredHitArea.midX, this.hoveredHitArea.midY);
                    this.ctx.restore();
                }
            }

            this.animationFrame = requestAnimationFrame(animate);
        };

        animate();
    }

    /**
     * Resize canvas to match container
     */
    resize() {
        const rect = this.container.getBoundingClientRect();
        this.canvas.width = rect.width;
        this.canvas.height = rect.height;
        this.canvas.style.width = `${rect.width}px`;
        this.canvas.style.height = `${rect.height}px`;
    }

    /**
     * Main render method
     */
    render(layout, steps) {
        this.layout = layout;
        this.steps = steps;

        console.log('🎨 FlowchartCanvas rendering...', {
            steps: steps.length,
            positions: layout.positions.size,
            connections: layout.connections.length
        });

        // Clear all
        this.clear();

        // Render swim lanes
        this.renderSwimLanes(layout.swimLanes);

        // Render step nodes
        this.renderStepNodes(layout.positions, steps);

        // Render connections
        this.renderConnections(layout.connections, layout.positions);

        // Apply current zoom/pan
        this.applyTransform();
    }

    /**
     * Clear all rendered content
     */
    clear() {
        // Clear canvas
        this.ctx.clearRect(0, 0, this.canvas.width, this.canvas.height);

        // Clear DOM containers
        this.swimLanesContainer.innerHTML = '';
        this.nodesContainer.innerHTML = '';
    }

    /**
     * Render swim lane backgrounds (disabled - simple horizontal flow)
     */
    renderSwimLanes(swimLanes) {
        // No swim lanes needed - simple horizontal flow
        console.log('📐 Swim lanes skipped (simple flow mode)');
    }

    /**
     * Render step nodes as DOM elements
     */
    renderStepNodes(positions, steps) {
        steps.forEach(step => {
            const pos = positions.get(step.id);
            if (!pos) return;

            const node = this.createStepNode(step, pos);
            this.nodesContainer.appendChild(node);
        });
    }

    /**
     * Create a step node DOM element
     */
    createStepNode(step, pos) {
        const node = document.createElement('div');
        node.className = 'flowchart-step-node';
        node.id = `flowchart-v2-node-${step.id}`;
        node.setAttribute('data-step-id', step.id);
        node.style.cssText = `
            position: absolute;
            left: ${pos.x}px;
            top: ${pos.y}px;
            width: ${pos.width}px;
            height: ${pos.height}px;
            background: white;
            border: 2px solid #f8bbd9;
            border-radius: 8px;
            padding: 8px;
            cursor: pointer;
            transition: all 0.2s;
            display: flex;
            flex-direction: column;
            align-items: center;
            justify-content: center;
            box-shadow: 0 2px 4px rgba(0,0,0,0.1);
            pointer-events: auto;
        `;

        // Sequence badge
        const badge = document.createElement('div');
        badge.className = 'step-sequence-badge';
        badge.style.cssText = `
            position: absolute;
            top: 4px;
            left: 4px;
            background: #2563eb;
            color: white;
            font-size: 10px;
            font-weight: 600;
            padding: 2px 6px;
            border-radius: 4px;
            z-index: 2;
        `;
        badge.textContent = step.sequence;

        // Delete button
        const deleteBtn = document.createElement('button');
        deleteBtn.className = 'step-delete-btn';
        deleteBtn.innerHTML = '<i class="fas fa-trash-alt"></i>';
        deleteBtn.style.cssText = `
            position: absolute;
            top: 4px;
            right: 4px;
            background: #ef4444;
            color: white;
            border: none;
            border-radius: 4px;
            width: 24px;
            height: 24px;
            display: flex;
            align-items: center;
            justify-content: center;
            cursor: pointer;
            font-size: 10px;
            opacity: 0;
            transition: opacity 0.2s;
            z-index: 2;
        `;
        deleteBtn.title = 'Delete step';

        // Prevent delete button from triggering drag/click on parent step node
        deleteBtn.addEventListener('mousedown', (e) => {
            e.stopPropagation();
            e.preventDefault();
        });

        // Delete button click handler
        deleteBtn.addEventListener('click', (e) => {
            e.stopPropagation();
            e.preventDefault();
            this.handleDeleteStep(step);
        });

        // Icon (FontAwesome) - grayscale
        const icon = document.createElement('i');
        icon.className = this.getStepIcon(step);
        icon.style.cssText = `
            font-size: 28px;
            margin-bottom: 6px;
            color: #6b7280;
        `;

        // Name
        const name = document.createElement('div');
        name.style.cssText = `
            font-size: 12px;
            font-weight: 500;
            text-align: center;
            overflow: hidden;
            text-overflow: ellipsis;
            white-space: nowrap;
            max-width: 100%;
        `;
        name.textContent = step.stepName || 'Unnamed';
        name.title = step.stepName;

        // OUTPUT connector port (right side - always visible)
        const outputPort = document.createElement('div');
        outputPort.className = 'step-output-port';
        outputPort.style.cssText = `
            position: absolute;
            right: -8px;
            top: 50%;
            transform: translateY(-50%);
            width: 16px;
            height: 16px;
            background: #3b82f6;
            border: 3px solid white;
            border-radius: 50%;
            cursor: pointer;
            z-index: 10;
            box-shadow: 0 2px 4px rgba(0,0,0,0.2);
        `;
        outputPort.title = 'Output: Click to auto-connect or drag to choose target';
        outputPort.setAttribute('data-port-type', 'output');
        outputPort.setAttribute('data-step-id', step.id);

        // INPUT connector port (left side - always visible)
        const inputPort = document.createElement('div');
        inputPort.className = 'step-input-port';
        inputPort.style.cssText = `
            position: absolute;
            left: -8px;
            top: 50%;
            transform: translateY(-50%);
            width: 16px;
            height: 16px;
            background: #10b981;
            border: 3px solid white;
            border-radius: 50%;
            z-index: 10;
            box-shadow: 0 2px 4px rgba(0,0,0,0.2);
        `;
        inputPort.title = 'Input port';
        inputPort.setAttribute('data-port-type', 'input');
        inputPort.setAttribute('data-step-id', step.id);

        // Output port interaction
        outputPort.addEventListener('mouseenter', () => {
            outputPort.style.transform = 'translateY(-50%) scale(1.3)';
            outputPort.style.background = '#2563eb';

            // Highlight next available step for auto-connection
            const nextStep = this.findNextAvailableStep(step);
            if (nextStep) {
                const nextNode = this.getStepNode(nextStep.id);
                if (nextNode) {
                    nextNode.style.borderColor = '#10b981';
                    nextNode.style.borderWidth = '3px';
                    nextNode.style.boxShadow = '0 0 0 3px rgba(16, 185, 129, 0.3)';
                }
            }
        });

        outputPort.addEventListener('mouseleave', () => {
            outputPort.style.transform = 'translateY(-50%) scale(1)';
            outputPort.style.background = '#3b82f6';

            // Clear highlight
            const nextStep = this.findNextAvailableStep(step);
            if (nextStep) {
                const nextNode = this.getStepNode(nextStep.id);
                if (nextNode) {
                    nextNode.style.borderColor = '#f8bbd9';
                    nextNode.style.borderWidth = '2px';
                    nextNode.style.boxShadow = '0 2px 4px rgba(0,0,0,0.1)';
                }
            }
        });

        // Prevent port clicks from triggering node drag
        outputPort.addEventListener('mousedown', (e) => {
            e.stopPropagation();
            console.log('🔵 OUTPUT PORT clicked - starting connection from:', step.id);
            this.startConnection(step, outputPort, e);
        });

        node.appendChild(badge);
        node.appendChild(deleteBtn);
        node.appendChild(inputPort);
        node.appendChild(outputPort);
        node.appendChild(icon);
        node.appendChild(name);

        // Hover effect - show delete button
        node.addEventListener('mouseenter', () => {
            node.style.borderColor = '#1e3a8a';
            node.style.boxShadow = '0 4px 12px rgba(30, 58, 138, 0.3)';
            node.style.transform = 'translateY(-2px)';
            deleteBtn.style.opacity = '1';
        });

        node.addEventListener('mouseleave', () => {
            node.style.borderColor = '#f8bbd9';
            node.style.boxShadow = '0 2px 4px rgba(0,0,0,0.1)';
            node.style.transform = 'translateY(0)';
            deleteBtn.style.opacity = '0';
        });

        // Click handler is in FlowchartOrchestratorV2 (detects click vs drag)

        return node;
    }

    /**
     * Find the next available step to auto-connect to
     * Logic: Find the nearest unconnected step to the right
     */
    findNextAvailableStep(sourceStep) {
        if (!this.layout || !this.steps) return null;

        const sourcePos = this.layout.positions.get(sourceStep.id);
        if (!sourcePos) return null;

        // Get all steps that are:
        // 1. To the right of source (x > sourceX)
        // 2. Not already connected from source
        // 3. Not the source itself
        const candidates = [];

        for (const step of this.steps) {
            if (step.id === sourceStep.id) continue;

            const stepPos = this.layout.positions.get(step.id);
            if (!stepPos) continue;

            // Check if already connected
            const alreadyConnected = this.layout.connections.some(
                conn => conn.from === sourceStep.id && conn.to === step.id
            );

            if (alreadyConnected) continue;

            // Only consider steps to the right
            if (stepPos.x > sourcePos.x) {
                const distance = Math.sqrt(
                    Math.pow(stepPos.x - sourcePos.x, 2) +
                    Math.pow(stepPos.y - sourcePos.y, 2)
                );

                candidates.push({
                    step: step,
                    distance: distance,
                    xDistance: stepPos.x - sourcePos.x,
                    yDistance: Math.abs(stepPos.y - sourcePos.y)
                });
            }
        }

        if (candidates.length === 0) return null;

        // Sort by horizontal distance first (prefer steps directly to the right),
        // then by vertical distance as tiebreaker
        candidates.sort((a, b) => {
            // Prefer steps on similar horizontal line (within 100px vertically)
            const aSameLine = a.yDistance < 100;
            const bSameLine = b.yDistance < 100;

            if (aSameLine && !bSameLine) return -1;
            if (!aSameLine && bSameLine) return 1;

            // Both on similar line or both not - sort by horizontal distance
            return a.xDistance - b.xDistance;
        });

        return candidates[0].step;
    }

    /**
     * Start creating a connection from a connector handle
     */
    startConnection(sourceStep, connectorHandle, startEvent) {
        console.log('🔗 Starting connection from step:', sourceStep.id);

        // Set flag to prevent click event from opening properties
        this.isConnecting = true;
        this.connectionDragged = false;

        // Store suggested next step for quick-click (if user releases immediately)
        this.suggestedNextStep = this.findNextAvailableStep(sourceStep);
        console.log('📍 Suggested next step:', this.suggestedNextStep?.id || 'none');

        // ALWAYS enable manual drag mode - let user choose target
        console.log('📍 Manual connection mode enabled - drag to target step');

        // Visual feedback - highlight source
        const sourceNode = this.getStepNode(sourceStep.id);
        if (sourceNode) {
            sourceNode.style.borderColor = '#3b82f6';
            sourceNode.style.borderWidth = '3px';
        }

        // Create temporary line to follow cursor
        const tempLine = { active: false };

        const handleMouseMove = (e) => {
            if (!tempLine.active) {
                tempLine.active = true;
                console.log('📍 Starting to draw connection line...');
            }

            // Mark that we've dragged (moved mouse)
            this.connectionDragged = true;

            // Draw temporary connection line
            this.drawTempConnection(sourceStep, e);

            // Highlight potential target ports/nodes
            const targetElement = document.elementFromPoint(e.clientX, e.clientY);
            const targetPort = targetElement?.closest('.step-input-port');
            const targetNode = targetElement?.closest('.flowchart-step-node');

            // Reset all input port highlights
            document.querySelectorAll('.step-input-port').forEach(port => {
                if (port.getAttribute('data-step-id') !== sourceStep.id) {
                    port.style.background = '#10b981';  // Green
                    port.style.transform = 'translateY(-50%) scale(1)';
                }
            });

            // Highlight hovered target
            if (targetPort && targetPort.getAttribute('data-step-id') !== sourceStep.id) {
                const targetId = targetPort.getAttribute('data-step-id');
                console.log('🎯 Hovering over INPUT PORT of step:', targetId);
                targetPort.style.background = '#059669';  // Darker green
                targetPort.style.transform = 'translateY(-50%) scale(1.5)';
            } else if (targetNode && targetNode.getAttribute('data-step-id') !== sourceStep.id) {
                const targetId = targetNode.getAttribute('data-step-id');
                console.log('🎯 Hovering over STEP NODE:', targetId);
                targetNode.style.borderColor = '#10b981';
                targetNode.style.borderWidth = '3px';
            }
        };

        const handleMouseUp = (e) => {
            // Remove listeners
            document.removeEventListener('mousemove', handleMouseMove);
            document.removeEventListener('mouseup', handleMouseUp);

            // Clear connecting flag after a short delay to let click events process
            setTimeout(() => {
                this.isConnecting = false;
                this.connectionDragged = false;
            }, 100);

            // Reset all node and port appearances
            if (sourceNode) {
                sourceNode.style.borderColor = '#f8bbd9';
                sourceNode.style.borderWidth = '2px';
            }

            // Reset all input ports
            document.querySelectorAll('.step-input-port').forEach(port => {
                port.style.background = '#10b981';
                port.style.transform = 'translateY(-50%) scale(1)';
            });

            // Reset all node borders
            document.querySelectorAll('.flowchart-step-node').forEach(node => {
                if (node !== sourceNode) {
                    node.style.borderColor = '#f8bbd9';
                    node.style.borderWidth = '2px';
                }
            });

            // Clear temporary line
            this.clearTempConnection();

            // Check if mouse is over a step node or input port
            const targetElement = document.elementFromPoint(e.clientX, e.clientY);
            console.log('🎯 Mouse released at:', e.clientX, e.clientY);
            console.log('🎯 Element at drop point:', targetElement?.className, targetElement?.tagName);

            // Check for input port first (more specific)
            const targetPort = targetElement?.closest('.step-input-port');
            let targetNode = null;

            console.log('🔍 Target port found:', targetPort ? 'YES' : 'NO');
            console.log('🔍 Searching for target node...');

            if (targetPort) {
                // Dropped on input port - get step from port
                const targetStepId = targetPort.getAttribute('data-step-id');
                const targetStep = this.steps.find(s => s.id === targetStepId);

                console.log('🟢 Dropped on INPUT PORT - Target step ID:', targetStepId);
                console.log('🟢 Target step found in steps array:', targetStep ? 'YES' : 'NO');

                if (targetStep && targetStep.id !== sourceStep.id) {
                    console.log('✅ Connection created to input port:', sourceStep.id, '→', targetStep.id);
                    this.createConnection(sourceStep, targetStep);
                } else {
                    console.log('❌ Cannot connect to self');
                }
            } else {
                // Check for step node
                targetNode = targetElement?.closest('.flowchart-step-node');
                console.log('🔍 Target node found:', targetNode ? 'YES' : 'NO');

                if (targetNode) {
                    const targetStepId = targetNode.getAttribute('data-step-id');
                    const targetStep = this.steps.find(s => s.id === targetStepId);

                    console.log('🟦 Dropped on STEP NODE - Target step ID:', targetStepId);
                    console.log('🟦 Target step found in steps array:', targetStep ? 'YES' : 'NO');

                    if (targetStep && targetStep.id !== sourceStep.id) {
                        console.log('✅ Connection created to step:', sourceStep.id, '→', targetStep.id);
                        this.createConnection(sourceStep, targetStep);
                    } else {
                        console.log('❌ Cannot connect to self');
                    }
                } else {
                    // No target found - use suggested step if available AND user didn't drag
                    if (!this.connectionDragged && this.suggestedNextStep) {
                        console.log('✨ Quick-click: auto-connecting to suggested step:', this.suggestedNextStep.id);
                        this.createConnection(sourceStep, this.suggestedNextStep);
                    } else {
                        console.log('❌ No target step or port found at drop location');
                        console.log('❌ Drop coordinates:', e.clientX, e.clientY);
                        console.log('❌ Element classes:', targetElement?.className);
                        console.log('❌ Connection dragged:', this.connectionDragged);
                    }
                }
            }

            // Clear suggested step
            this.suggestedNextStep = null;
        };

        // Listen for mouse movement and release
        document.addEventListener('mousemove', handleMouseMove);
        document.addEventListener('mouseup', handleMouseUp);
    }

    /**
     * Draw temporary connection line while dragging
     */
    drawTempConnection(sourceStep, mouseEvent) {
        if (!this.layout || !this.layout.positions) return;

        const sourcePos = this.layout.positions.get(sourceStep.id);
        if (!sourcePos) return;

        // Clear and redraw
        this.ctx.clearRect(0, 0, this.canvas.width, this.canvas.height);
        this.renderConnections(this.layout.connections, this.layout.positions);

        // Draw temp line
        this.ctx.save();
        this.coords.applyToCanvasContext(this.ctx);

        const fromX = sourcePos.x + sourcePos.width;
        const fromY = sourcePos.y + sourcePos.height / 2;

        // Convert mouse coordinates to world space
        const rect = this.canvas.getBoundingClientRect();
        const mouseX = mouseEvent.clientX - rect.left;
        const mouseY = mouseEvent.clientY - rect.top;
        const worldCoords = this.coords.screenToWorld(mouseX, mouseY);

        // Draw dashed line
        this.ctx.strokeStyle = '#3b82f6';
        this.ctx.lineWidth = 3;
        this.ctx.setLineDash([8, 4]);
        this.ctx.lineCap = 'round';
        this.ctx.beginPath();
        this.ctx.moveTo(fromX, fromY);
        this.ctx.lineTo(worldCoords.x, worldCoords.y);
        this.ctx.stroke();
        this.ctx.setLineDash([]); // Reset dash

        this.ctx.restore();
    }

    /**
     * Clear temporary connection line
     */
    clearTempConnection() {
        this.ctx.clearRect(0, 0, this.canvas.width, this.canvas.height);
        if (this.layout && this.layout.connections) {
            this.renderConnections(this.layout.connections, this.layout.positions);
        }
    }

    /**
     * Create a new connection between two steps
     */
    createConnection(fromStep, toStep) {
        if (!this.layout || !this.layout.connections) return;

        // Check if connection already exists
        const exists = this.layout.connections.some(conn =>
            conn.from === fromStep.id && conn.to === toStep.id
        );

        if (exists) {
            this.showNotification('Connection already exists!', 'warning');
            return;
        }

        // Add new connection (sequential = blue)
        this.layout.connections.push({
            type: 'sequential',  // Blue connection for regular flow
            from: fromStep.id,
            to: toStep.id,
            fromStep: fromStep,
            toStep: toStep
        });

        // Re-render
        this.ctx.clearRect(0, 0, this.canvas.width, this.canvas.height);
        this.renderConnections(this.layout.connections, this.layout.positions);

        // Save connections to localStorage (via orchestrator)
        if (window.pipelineBuilder?.flowchartRenderer?.saveAllConnections) {
            window.pipelineBuilder.flowchartRenderer.saveAllConnections();
        }

        console.log('✅ Connection added to layout (sequential/blue)');
    }

    /**
     * Get FontAwesome icon class for step type (distinct grayscale icons)
     */
    getStepIcon(step) {
        // DEBUG: Log step details
        console.log('🔍 Getting icon for step:', {
            id: step.id,
            name: step.stepName,
            templateId: step.templateId,
            stepType: step.stepType,
            subType: step.subType
        });

        // Check template ID first (ACTUAL IDs from ToolboxManager)
        const templateIconMap = {
            'validate-fields': 'fas fa-shield-alt',            // Shield for validation
            'enrich-api': 'fas fa-cloud',                      // Cloud for API
            'enrich-database': 'fas fa-server',                // Server for database (different from generic database)
            'enrich-script': 'fas fa-file-code',               // Code file for script
            'hl7-fhir-mapping': 'fas fa-exchange-alt',         // Exchange for transform
            'field-mapping': 'fas fa-sitemap',                 // Sitemap for mapping
            'if-then-else': 'fas fa-code-branch',              // Branch for conditionals
            'code-system-mapping': 'fas fa-table',             // Table for code mapping
            'switch-case': 'fas fa-project-diagram',           // Diagram for switch
            'for-each-loop': 'fas fa-redo',                    // Loop icon
            'validate-fhir': 'fas fa-check-circle',            // Check for FHIR validation
            'deliver-fhir': 'fas fa-paper-plane',              // Send for delivery
            'try-catch': 'fas fa-life-ring',                   // Life ring for error handling
            'retry-logic': 'fas fa-sync-alt',                  // Sync for retry
            'hl7-segment-extractor': 'fas fa-cut',             // Cut for extract
            'fhir-resource-builder': 'fas fa-hammer',          // Hammer for builder
            'remove-duplicates': 'fas fa-compress',            // Compress for dedup
            'data-masking': 'fas fa-eye-slash'                 // Eye slash for masking
        };

        // Try template ID first
        if (step.templateId && templateIconMap[step.templateId]) {
            console.log('✅ Icon matched by templateId:', step.templateId, '→', templateIconMap[step.templateId]);
            return templateIconMap[step.templateId];
        }

        // Check step name patterns for better differentiation
        const stepName = (step.stepName || '').toLowerCase();
        if (stepName.includes('database') || stepName.includes('db')) {
            console.log('✅ Icon matched by name pattern (database)');
            return 'fas fa-server';
        }
        if (stepName.includes('api') || stepName.includes('rest') || stepName.includes('http')) {
            console.log('✅ Icon matched by name pattern (api)');
            return 'fas fa-cloud';
        }
        if (stepName.includes('script') || stepName.includes('javascript') || stepName.includes('code')) {
            console.log('✅ Icon matched by name pattern (script)');
            return 'fas fa-file-code';
        }
        if (stepName.includes('validate') || stepName.includes('validation')) {
            console.log('✅ Icon matched by name pattern (validation)');
            return 'fas fa-shield-alt';
        }
        if (stepName.includes('mapping') || stepName.includes('map')) {
            console.log('✅ Icon matched by name pattern (mapping)');
            return 'fas fa-sitemap';
        }

        // Fallback to stepType (check full type first, then base type)
        const typeIconMap = {
            'connector.inbound': 'fas fa-download',
            'connector.outbound': 'fas fa-upload',
            'field_validation': 'fas fa-shield-alt',
            'fhir_validation': 'fas fa-check-circle',
            'enrichment.api': 'fas fa-cloud',
            'enrichment.database': 'fas fa-server',
            'enrichment.script': 'fas fa-file-code',
            'hl7_fhir_transform': 'fas fa-exchange-alt',
            'field_mapping': 'fas fa-sitemap',
            'if_then_else': 'fas fa-code-branch',
            'switch_case': 'fas fa-project-diagram',
            'control.loop': 'fas fa-redo',
            'control.try_catch': 'fas fa-life-ring',
            'control.retry': 'fas fa-sync-alt',
            'core.validation': 'fas fa-shield-alt',
            'pre.validation': 'fas fa-shield-alt',
            'core.enrichment': 'fas fa-plus-square',
            'core.mapping': 'fas fa-sitemap',
            'core.transformation': 'fas fa-exchange-alt',
            'control': 'fas fa-code-branch',
            validation: 'fas fa-shield-alt',
            enrichment: 'fas fa-plus-square',
            mapping: 'fas fa-sitemap',
            transformation: 'fas fa-exchange-alt',
            script: 'fas fa-file-code',
            database: 'fas fa-server',
            api: 'fas fa-cloud'
        };

        const type = step.stepType || step.subType || 'custom';
        const icon = typeIconMap[type] || 'fas fa-cube';
        console.log('✅ Icon matched by stepType:', type, '→', icon);
        return icon;
    }

    /**
     * Handle step deletion with in-app confirmation dialog
     */
    handleDeleteStep(step) {
        // Create custom confirmation dialog
        const modal = document.createElement('div');
        modal.style.cssText = `
            position: fixed;
            top: 0;
            left: 0;
            right: 0;
            bottom: 0;
            background: rgba(0, 0, 0, 0.5);
            display: flex;
            align-items: center;
            justify-content: center;
            z-index: 10000;
        `;

        const dialog = document.createElement('div');
        dialog.style.cssText = `
            background: white;
            padding: 24px;
            border-radius: 8px;
            box-shadow: 0 8px 16px rgba(0, 0, 0, 0.3);
            max-width: 400px;
            width: 90%;
        `;

        dialog.innerHTML = `
            <h3 style="margin: 0 0 16px 0; color: #1e293b; font-size: 18px; font-weight: 600;">
                <i class="fas fa-trash-alt" style="color: #ef4444; margin-right: 8px;"></i>
                Delete Step
            </h3>
            <p style="margin: 0 0 24px 0; color: #64748b; font-size: 14px;">
                Are you sure you want to delete <strong>"${step.stepName}"</strong>?
                <br><br>
                This action cannot be undone.
            </p>
            <div style="display: flex; gap: 12px; justify-content: flex-end;">
                <button id="cancelDeleteBtn" style="
                    padding: 8px 16px;
                    border: 1px solid #e2e8f0;
                    background: white;
                    color: #64748b;
                    border-radius: 6px;
                    cursor: pointer;
                    font-size: 14px;
                    font-weight: 500;
                ">Cancel</button>
                <button id="confirmDeleteBtn" style="
                    padding: 8px 16px;
                    border: none;
                    background: #ef4444;
                    color: white;
                    border-radius: 6px;
                    cursor: pointer;
                    font-size: 14px;
                    font-weight: 500;
                ">Delete Step</button>
            </div>
        `;

        modal.appendChild(dialog);
        document.body.appendChild(modal);

        // Handle cancel
        const cancelBtn = dialog.querySelector('#cancelDeleteBtn');
        cancelBtn.addEventListener('click', () => {
            document.body.removeChild(modal);
        });

        // Handle confirm
        const confirmBtn = dialog.querySelector('#confirmDeleteBtn');
        confirmBtn.addEventListener('click', () => {
            console.log('🗑️ Deleting step:', step.id);

            // Get PipelineBuilder instance
            if (window.pipelineBuilder && typeof window.pipelineBuilder.deleteStep === 'function') {
                window.pipelineBuilder.deleteStep(step.id);

                // Re-render flowchart after deletion
                if (window.pipelineBuilder.viewMode === 'flowchart' && window.pipelineBuilder.flowchartRenderer) {
                    // Use getAllStepsFlat if getAllSteps doesn't exist (backward compatibility)
                    const getAllStepsMethod = window.pipelineBuilder.getAllSteps || window.pipelineBuilder.getAllStepsFlat;
                    if (getAllStepsMethod) {
                        const allSteps = getAllStepsMethod.call(window.pipelineBuilder);
                        window.pipelineBuilder.flowchartRenderer.render(allSteps);
                    } else {
                        console.error('❌ No method to get all steps');
                    }
                }
            } else {
                console.error('❌ PipelineBuilder.deleteStep not available');
            }

            document.body.removeChild(modal);
        });

        // Close on background click
        modal.addEventListener('click', (e) => {
            if (e.target === modal) {
                document.body.removeChild(modal);
            }
        });

        // Focus confirm button
        confirmBtn.focus();
    }

    /**
     * Render connections on canvas
     */
    renderConnections(connections, positions) {
        this.ctx.save();

        // Apply coordinate transform
        this.coords.applyToCanvasContext(this.ctx);

        // Clear connection hit areas
        this.connectionHitAreas = [];

        connections.forEach(conn => {
            this.renderConnection(conn, positions);
        });

        this.ctx.restore();
    }

    /**
     * Render a single connection
     */
    renderConnection(connection, positions) {
        const fromPos = positions.get(connection.from);
        const toPos = positions.get(connection.to);

        if (!fromPos || !toPos) return;

        // Connection points
        const fromX = fromPos.x + fromPos.width;  // Right edge
        const fromY = fromPos.y + fromPos.height / 2;
        const toX = toPos.x;  // Left edge
        const toY = toPos.y + toPos.height / 2;

        // Choose color based on connection type
        let strokeColor, arrowColor, lineWidth, label;

        if (connection.type === 'conditional_true') {
            // TRUE path - Green
            strokeColor = '#16a34a';
            arrowColor = '#16a34a';
            lineWidth = 4;
            label = connection.label || 'TRUE';
        } else if (connection.type === 'conditional_false') {
            // FALSE path - Red
            strokeColor = '#dc2626';
            arrowColor = '#dc2626';
            lineWidth = 4;
            label = connection.label || 'FALSE';
        } else if (connection.type === 'custom') {
            // Custom user-created connection - Blue (same as sequential)
            strokeColor = '#1e40af';
            arrowColor = '#1e40af';
            lineWidth = 4;
            label = null;
        } else {
            // Sequential - Dark blue
            strokeColor = '#1e40af';
            arrowColor = '#1e40af';
            lineWidth = 4;
            label = null;
        }

        // Draw glowing line with animated flow
        this.drawFlowingLine(fromX, fromY, toX, toY, strokeColor, lineWidth);

        // Draw label for conditional connections
        if (label) {
            this.drawConnectionLabel(fromX, fromY, toX, toY, label, strokeColor);
        }

        // Store hit area for hover/click detection (along entire line)
        this.connectionHitAreas.push({
            connection: connection,
            fromX: fromX,
            fromY: fromY,
            toX: toX,
            toY: toY,
            midX: (fromX + toX) / 2,
            midY: (fromY + toY) / 2,
            threshold: 15,  // Pixels away from line to detect hover
            color: strokeColor
        });
    }

    /**
     * Draw delete button at specific position (shown on hover)
     */
    drawHoverDeleteButton(midX, midY) {
        const radius = 16;

        // Background circle with shadow
        this.ctx.save();
        this.ctx.shadowColor = 'rgba(0, 0, 0, 0.3)';
        this.ctx.shadowBlur = 4;
        this.ctx.shadowOffsetX = 0;
        this.ctx.shadowOffsetY = 2;

        this.ctx.fillStyle = '#ef4444';
        this.ctx.beginPath();
        this.ctx.arc(midX, midY, radius, 0, Math.PI * 2);
        this.ctx.fill();

        this.ctx.shadowColor = 'transparent';

        // White border
        this.ctx.strokeStyle = 'white';
        this.ctx.lineWidth = 2;
        this.ctx.stroke();

        // Draw X icon
        this.ctx.strokeStyle = 'white';
        this.ctx.lineWidth = 3;
        this.ctx.lineCap = 'round';

        const xSize = 8;
        this.ctx.beginPath();
        this.ctx.moveTo(midX - xSize, midY - xSize);
        this.ctx.lineTo(midX + xSize, midY + xSize);
        this.ctx.moveTo(midX + xSize, midY - xSize);
        this.ctx.lineTo(midX - xSize, midY + xSize);
        this.ctx.stroke();

        this.ctx.restore();
    }

    /**
     * Draw flowing animated line with glow effect
     */
    drawFlowingLine(fromX, fromY, toX, toY, color, lineWidth) {
        // Initialize animation offset if not exists
        if (!this.flowAnimationOffset) {
            this.flowAnimationOffset = 0;
        }

        // Draw stronger glow effect (outer layer)
        this.ctx.shadowBlur = 12;
        this.ctx.shadowColor = color;
        this.ctx.strokeStyle = color;
        this.ctx.lineWidth = lineWidth + 4;
        this.ctx.lineCap = 'round';
        this.ctx.globalAlpha = 0.4;

        this.ctx.beginPath();
        this.ctx.moveTo(fromX, fromY);
        this.ctx.lineTo(toX, toY);
        this.ctx.stroke();

        // Draw solid line
        this.ctx.shadowBlur = 0;
        this.ctx.globalAlpha = 1;
        this.ctx.lineWidth = lineWidth;

        this.ctx.beginPath();
        this.ctx.moveTo(fromX, fromY);
        this.ctx.lineTo(toX, toY);
        this.ctx.stroke();

        // Draw animated dashed flow on top (white flowing dots)
        this.ctx.strokeStyle = '#ffffff';
        this.ctx.lineWidth = lineWidth - 1;
        this.ctx.globalAlpha = 0.9;
        this.ctx.lineDashOffset = -this.flowAnimationOffset;
        this.ctx.setLineDash([12, 18]);  // Slightly longer dashes for better visibility

        this.ctx.beginPath();
        this.ctx.moveTo(fromX, fromY);
        this.ctx.lineTo(toX, toY);
        this.ctx.stroke();

        // Reset
        this.ctx.setLineDash([]);
        this.ctx.globalAlpha = 1;
        this.ctx.shadowBlur = 0;

        // No arrow head needed - animated dashes show direction clearly
    }

    /**
     * Draw label on connection line
     */
    drawConnectionLabel(fromX, fromY, toX, toY, label, color) {
        // Calculate midpoint
        const midX = (fromX + toX) / 2;
        const midY = (fromY + toY) / 2;

        // Draw background rectangle
        this.ctx.font = 'bold 11px Arial';
        const metrics = this.ctx.measureText(label);
        const padding = 4;
        const boxWidth = metrics.width + padding * 2;
        const boxHeight = 16;

        this.ctx.fillStyle = 'white';
        this.ctx.fillRect(
            midX - boxWidth / 2,
            midY - boxHeight / 2,
            boxWidth,
            boxHeight
        );

        // Draw border
        this.ctx.strokeStyle = color;
        this.ctx.lineWidth = 1;
        this.ctx.strokeRect(
            midX - boxWidth / 2,
            midY - boxHeight / 2,
            boxWidth,
            boxHeight
        );

        // Draw label text
        this.ctx.fillStyle = color;
        this.ctx.textAlign = 'center';
        this.ctx.textBaseline = 'middle';
        this.ctx.fillText(label, midX, midY);
    }

    /**
     * Apply current zoom/pan transform
     */
    applyTransform() {
        const transform = this.coords.getCSSTransform();
        this.swimLanesContainer.style.transform = transform;
        this.nodesContainer.style.transform = transform;

        // Redraw canvas connections with new transform
        if (this.layout && this.steps) {
            this.ctx.clearRect(0, 0, this.canvas.width, this.canvas.height);
            this.renderConnections(this.layout.connections, this.layout.positions);
        }
    }

    /**
     * Handle canvas hover (show delete button and update cursor)
     */
    handleCanvasHover(e) {
        const rect = this.canvas.getBoundingClientRect();
        const mouseX = e.clientX - rect.left;
        const mouseY = e.clientY - rect.top;

        // Use CoordinateSystem to transform screen to world coordinates
        const worldCoords = this.coords.screenToWorld(mouseX, mouseY);

        // Find if hovering over any connection
        let hoveredConnection = null;
        let hoveredHitArea = null;

        if (this.connectionHitAreas) {
            for (const hitArea of this.connectionHitAreas) {
                const distance = this.distanceToLine(
                    worldCoords.x, worldCoords.y,
                    hitArea.fromX, hitArea.fromY,
                    hitArea.toX, hitArea.toY
                );

                if (distance <= hitArea.threshold) {
                    hoveredConnection = hitArea.connection;
                    hoveredHitArea = hitArea;
                    break;
                }
            }
        }

        // Redraw if hover state changed
        if (this.hoveredConnection !== hoveredConnection) {
            this.hoveredConnection = hoveredConnection;
            this.hoveredHitArea = hoveredHitArea;

            // Re-render to show/hide delete button (null check for layout)
            if (this.layout && this.layout.connections && this.layout.positions) {
                this.ctx.clearRect(0, 0, this.canvas.width, this.canvas.height);
                this.renderConnections(this.layout.connections, this.layout.positions);

                // Draw delete button on hovered connection
                if (hoveredHitArea) {
                    this.ctx.save();
                    this.coords.applyToCanvasContext(this.ctx);
                    this.drawHoverDeleteButton(hoveredHitArea.midX, hoveredHitArea.midY);
                    this.ctx.restore();
                }
            }
        }

        // Update cursor
        this.canvas.style.cursor = hoveredConnection ? 'pointer' : 'default';
        this.canvas.title = hoveredConnection ? 'Click to delete connection' : '';
    }

    /**
     * Handle canvas click (delete hovered connection)
     */
    handleCanvasClick(e) {
        // If clicking on a hovered connection, delete it
        if (this.hoveredConnection) {
            console.log('✅ Deleting hovered connection');
            this.handleDeleteConnection(this.hoveredConnection);

            // Clear hover state
            this.hoveredConnection = null;
            this.hoveredHitArea = null;
            this.canvas.style.cursor = 'default';
            this.canvas.title = '';
        }
    }

    /**
     * Handle canvas right-click (for arrow deletion - kept for backward compatibility)
     */
    handleCanvasRightClick(e) {
        e.preventDefault(); // Prevent default context menu
        console.log('🖱️ Right-click detected on canvas');

        const rect = this.canvas.getBoundingClientRect();
        const clickX = e.clientX - rect.left;
        const clickY = e.clientY - rect.top;

        // Use CoordinateSystem to transform screen to world coordinates
        const worldCoords = this.coords.screenToWorld(clickX, clickY);

        console.log('🎯 Right-click coordinates:', {
            screenX: clickX,
            screenY: clickY,
            worldX: worldCoords.x,
            worldY: worldCoords.y,
            zoom: this.coords.zoom,
            panX: this.coords.panX,
            panY: this.coords.panY,
            hitAreasCount: this.connectionHitAreas.length
        });

        // Check if click is near any connection line
        for (const hitArea of this.connectionHitAreas) {
            const distance = this.distanceToLine(
                worldCoords.x, worldCoords.y,
                hitArea.fromX, hitArea.fromY,
                hitArea.toX, hitArea.toY
            );

            console.log('📏 Distance to connection:', {
                from: hitArea.connection.from,
                to: hitArea.connection.to,
                distance,
                threshold: hitArea.threshold
            });

            if (distance <= hitArea.threshold) {
                // Right-clicked on arrow
                console.log('✅ Arrow hit detected - deleting');
                this.handleDeleteConnection(hitArea.connection);
                return;
            }
        }

        console.log('❌ No arrow hit - click was not near any connection');
    }

    /**
     * Calculate distance from point to line segment
     */
    distanceToLine(px, py, x1, y1, x2, y2) {
        const dx = x2 - x1;
        const dy = y2 - y1;
        const lengthSquared = dx * dx + dy * dy;

        if (lengthSquared === 0) {
            // Line segment is a point
            return Math.sqrt((px - x1) * (px - x1) + (py - y1) * (py - y1));
        }

        // Calculate projection of point onto line
        let t = ((px - x1) * dx + (py - y1) * dy) / lengthSquared;
        t = Math.max(0, Math.min(1, t)); // Clamp to line segment

        // Find closest point on line segment
        const closestX = x1 + t * dx;
        const closestY = y1 + t * dy;

        // Return distance to closest point
        return Math.sqrt((px - closestX) * (px - closestX) + (py - closestY) * (py - closestY));
    }

    /**
     * Handle connection deletion
     */
    async handleDeleteConnection(connection) {
        const confirmed = await this.showConfirmDialog(
            `Delete connection from "${connection.fromStep?.stepName}" to "${connection.toStep?.stepName}"?`,
            {
                title: 'Delete Connection',
                confirmText: 'Delete',
                cancelText: 'Cancel',
                type: 'danger'
            }
        );

        if (confirmed) {
            console.log('🗑️ Deleting connection:', connection);

            // Remove connection from layout
            if (this.layout && this.layout.connections) {
                const index = this.layout.connections.indexOf(connection);
                if (index > -1) {
                    this.layout.connections.splice(index, 1);
                }

                // Re-render (only if layout exists)
                this.ctx.clearRect(0, 0, this.canvas.width, this.canvas.height);
                this.renderConnections(this.layout.connections, this.layout.positions);

                // Save connections to localStorage (via orchestrator)
                if (window.pipelineBuilder?.flowchartRenderer?.saveAllConnections) {
                    window.pipelineBuilder.flowchartRenderer.saveAllConnections();
                }
            }

            // Notify user
            this.showNotification('Connection deleted', 'success');
        }
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
        `;
        notification.textContent = message;
        document.body.appendChild(notification);
        setTimeout(() => notification.remove(), 3000);
    }

    /**
     * Show in-app confirm dialog
     */
    showConfirmDialog(message, options = {}) {
        return new Promise((resolve) => {
            const {
                title = 'Confirm',
                confirmText = 'Yes',
                cancelText = 'Cancel',
                type = 'warning'
            } = options;

            const colors = {
                warning: { primary: '#f59e0b', bg: '#fef3c7' },
                danger: { primary: '#ef4444', bg: '#fee2e2' }
            };
            const color = colors[type] || colors.warning;

            const overlay = document.createElement('div');
            overlay.style.cssText = `
                position: fixed; top: 0; left: 0; right: 0; bottom: 0;
                background: rgba(0, 0, 0, 0.5);
                display: flex; align-items: center; justify-content: center;
                z-index: 100000;
            `;

            overlay.innerHTML = `
                <div style="background: white; border-radius: 12px; max-width: 400px; width: 90%; overflow: hidden; box-shadow: 0 20px 25px -5px rgba(0, 0, 0, 0.1);">
                    <div style="padding: 16px 20px; background: ${color.bg}; display: flex; align-items: center; gap: 10px;">
                        <span style="font-size: 20px;">${type === 'danger' ? '⚠️' : '❓'}</span>
                        <span style="font-weight: 600; color: #1e293b;">${title}</span>
                    </div>
                    <div style="padding: 20px; color: #475569; font-size: 14px;">${message}</div>
                    <div style="padding: 12px 20px; background: #f8fafc; display: flex; justify-content: flex-end; gap: 10px;">
                        <button class="cancel-btn" style="padding: 8px 16px; border: 1px solid #e2e8f0; background: white; color: #64748b; border-radius: 6px; cursor: pointer;">${cancelText}</button>
                        <button class="confirm-btn" style="padding: 8px 16px; border: none; background: ${color.primary}; color: white; border-radius: 6px; cursor: pointer;">${confirmText}</button>
                    </div>
                </div>
            `;

            document.body.appendChild(overlay);

            const cleanup = (result) => { overlay.remove(); resolve(result); };

            overlay.querySelector('.confirm-btn').onclick = () => cleanup(true);
            overlay.querySelector('.cancel-btn').onclick = () => cleanup(false);
            overlay.onclick = (e) => { if (e.target === overlay) cleanup(false); };
        });
    }

    /**
     * Get DOM node for a step
     */
    getStepNode(stepId) {
        return document.getElementById(`flowchart-v2-node-${stepId}`);
    }

    /**
     * Destroy and cleanup
     */
    destroy() {
        if (this.mainContainer && this.mainContainer.parentNode) {
            this.mainContainer.parentNode.removeChild(this.mainContainer);
        }
    }
}

// Export for use in other modules
if (typeof module !== 'undefined' && module.exports) {
    module.exports = FlowchartCanvas;
}
