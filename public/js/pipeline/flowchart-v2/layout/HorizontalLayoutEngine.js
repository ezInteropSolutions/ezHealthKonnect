/**
 * HorizontalLayoutEngine - Horizontal swim lane layout with grid positioning
 *
 * Features:
 * - Left-to-right flow
 * - Swim lanes for Pre/Core/Post layers
 * - Grid-based column allocation (3-4 steps per column)
 * - Fork detection and branching
 */
class HorizontalLayoutEngine {
    constructor(config = {}) {
        // Grid configuration
        this.config = {
            stepWidth: config.stepWidth || 160,
            stepHeight: config.stepHeight || 80,
            columnGap: config.columnGap || 80,        // Horizontal spacing between columns
            rowGap: config.rowGap || 40,              // Vertical spacing between rows
            swimLaneGap: config.swimLaneGap || 60,    // Gap between swim lanes
            swimLaneHeaderHeight: config.swimLaneHeaderHeight || 40,
            maxStepsPerColumn: config.maxStepsPerColumn || 4,
            startX: config.startX || 100,
            startY: config.startY || 100
        };

        // Layout results
        this.positions = new Map();      // stepId -> {x, y, width, height}
        this.connections = [];           // Array of connection objects
        this.swimLanes = {};             // Layer swim lane metadata

        console.log('✅ HorizontalLayoutEngine initialized', this.config);
    }

    /**
     * Main entry point - calculate horizontal layout
     */
    calculateLayout(steps) {
        if (!steps || steps.length === 0) {
            return { positions: new Map(), connections: [], swimLanes: {}, bounds: {} };
        }

        console.log(`📐 Calculating horizontal layout for ${steps.length} steps`);

        // Reset state
        this.positions.clear();
        this.connections = [];
        this.swimLanes = {};

        // Group steps (single flow)
        const stepsByLayer = this.groupStepsByLayer(steps);

        // Calculate swim lane layout
        this.calculateSwimLanes(stepsByLayer);

        // Position steps in single lane
        this.positionStepsInSwimLane('main', stepsByLayer.main);

        // Calculate connections
        this.calculateConnections(steps);

        // Calculate bounding box
        const bounds = this.calculateBounds();

        console.log('✅ Layout calculated:', {
            positions: this.positions.size,
            connections: this.connections.length,
            swimLanes: Object.keys(this.swimLanes).length,
            bounds
        });

        return {
            positions: this.positions,
            connections: this.connections,
            swimLanes: this.swimLanes,
            bounds
        };
    }

    /**
     * Group steps - Single flow (no layer distinction)
     */
    groupStepsByLayer(steps) {
        // Sort by sequence
        const sortedSteps = [...steps].sort((a, b) => a.sequence - b.sequence);

        // Return all steps in a single "main" layer
        const grouped = {
            main: sortedSteps
        };

        console.log('📊 Steps grouped:', {
            total: sortedSteps.length
        });

        return grouped;
    }

    /**
     * Calculate single swim lane (no layer distinction)
     */
    calculateSwimLanes(stepsByLayer) {
        const steps = stepsByLayer.main || [];

        if (steps.length === 0) {
            return;
        }

        // Calculate rows needed (max 4 steps per column)
        const columns = Math.ceil(steps.length / this.config.maxStepsPerColumn);
        const rows = Math.min(steps.length, this.config.maxStepsPerColumn);

        const contentHeight = rows * (this.config.stepHeight + this.config.rowGap);

        // Single main swim lane without header
        this.swimLanes.main = {
            name: 'Pipeline Steps',
            y: this.config.startY,
            headerHeight: 0, // No header
            contentY: this.config.startY,
            contentHeight: contentHeight,
            totalHeight: contentHeight,
            columns: columns,
            rows: rows
        };

        console.log('📐 Single lane layout:', this.swimLanes.main);
    }

    /**
     * Position steps within a swim lane using column grid
     */
    positionStepsInSwimLane(layerKey, steps) {
        if (steps.length === 0) return;

        const swimLane = this.swimLanes[layerKey];
        if (!swimLane) return;

        let currentX = this.config.startX;
        let currentY = swimLane.contentY;
        let stepsInCurrentColumn = 0;
        let currentColumn = 0;

        steps.forEach((step, index) => {
            // Move to next column if current column is full
            if (stepsInCurrentColumn >= this.config.maxStepsPerColumn) {
                currentX += this.config.stepWidth + this.config.columnGap;
                currentY = swimLane.contentY;
                stepsInCurrentColumn = 0;
                currentColumn++;
            }

            // Position step
            this.positions.set(step.id, {
                x: currentX,
                y: currentY,
                width: this.config.stepWidth,
                height: this.config.stepHeight,
                layer: layerKey,
                column: currentColumn,
                row: stepsInCurrentColumn
            });

            // Move to next row in column
            currentY += this.config.stepHeight + this.config.rowGap;
            stepsInCurrentColumn++;
        });
    }

    /**
     * Calculate connections between steps (including If-Then-Else routing)
     */
    calculateConnections(steps) {
        this.connections = [];

        // STEP 0: Create connections from step's own parentConditionalStepId/branchType
        // This is the authoritative source for conditional branch membership
        this.addConnectionsFromBranchProperties(steps);

        // First pass: Identify which steps have conditional routing
        const stepsWithRouting = new Set();

        steps.forEach((step) => {
            // Check if this is an If-Then-Else step with conditional routing
            // Use VisualStep utility for OOP-compliant type detection
            const isLogicStep = VisualStep.isConditionalStep(step);

            if (isLogicStep && step.config && step.config.conditions) {
                const hasRouting = this.hasConditionalRoutingNewFormat(step.config.conditions);
                if (hasRouting) {
                    stepsWithRouting.add(step.id);
                }
            } else if (step.subType === 'control' && step.config && step.config.ifThenElse) {
                const hasConditionalRouting = this.hasConditionalRouting(step.config.ifThenElse);
                if (hasConditionalRouting) {
                    stepsWithRouting.add(step.id);
                }
            }
        });

        // Second pass: Create connections
        steps.forEach((step, index) => {
            // Use VisualStep utility for OOP-compliant type detection
            const isLogicStep = VisualStep.isConditionalStep(step);

            // NEW FORMAT: step.config.conditions[].onTrue/onFalse
            if (isLogicStep && step.config && step.config.conditions) {
                console.log('🔍 [Auto-Connection] Checking If-Then-Else step:', step.stepName);
                console.log('   stepType:', step.stepType);
                console.log('   Config:', step.config);

                const hasRouting = this.hasConditionalRoutingNewFormat(step.config.conditions);

                if (hasRouting) {
                    // Add conditional routing connections (NEW FORMAT)
                    this.addConditionalConnectionsNewFormat(step, step.config.conditions, steps);
                    console.log(`   ✅ Added conditional routing for step ${step.stepName} - skipping sequential connection`);
                    return; // Don't add sequential connection FROM this step
                } else {
                    console.log('   ❌ No routing configured');
                }
            }

            // OLD FORMAT (backward compatibility): step.config.ifThenElse
            else if (step.subType === 'control' && step.config && step.config.ifThenElse) {
                const ifThenElse = step.config.ifThenElse;
                const hasConditionalRouting = this.hasConditionalRouting(ifThenElse);

                if (hasConditionalRouting) {
                    // Add conditional routing connections (OLD FORMAT)
                    this.addConditionalConnections(step, ifThenElse, steps);
                    console.log(`🔀 Added conditional routing (old format) for step ${step.sequence} - skipping sequential connection`);
                    return; // Don't add sequential connection FROM this step
                }
            }

            // Default sequential connection - skip for now, handled by branch-aware logic below
        });

        // PHASE 3: Create sequential connections with branch awareness
        this.addBranchAwareSequentialConnections(steps);

        console.log(`🔗 Created ${this.connections.length} connections`);
    }

    /**
     * Add sequential connections with awareness of branch chains
     * Steps in the same branch connect to each other
     * Steps NOT in a branch connect by sequence number
     */
    addBranchAwareSequentialConnections(steps) {
        // APPROACH 1: Connect steps within the same branch (same parentConditionalStepId + branchType)
        this.addBranchChainConnections(steps);

        // APPROACH 2: Connect steps on same Y row by X position (for non-branch steps)
        this.addRowBasedConnections(steps);
    }

    /**
     * Connect steps that belong to the same conditional branch
     * Steps with same parentConditionalStepId and branchType form a chain
     */
    addBranchChainConnections(steps) {
        // Group steps by their branch (parentConditionalStepId + branchType)
        const branchGroups = new Map();

        steps.forEach(step => {
            if (step.parentConditionalStepId && step.branchType) {
                const branchKey = `${step.parentConditionalStepId}_${step.branchType}`;
                if (!branchGroups.has(branchKey)) {
                    branchGroups.set(branchKey, []);
                }
                branchGroups.get(branchKey).push(step);
            }
        });

        // For each branch, sort by sequence and connect
        branchGroups.forEach((branchSteps, branchKey) => {
            // Sort by sequence number
            branchSteps.sort((a, b) => (a.sequence || 0) - (b.sequence || 0));

            console.log(`🔗 [Branch Chain] ${branchKey}: ${branchSteps.map(s => s.stepName).join(' → ')}`);

            // Create sequential connections within this branch
            for (let i = 0; i < branchSteps.length - 1; i++) {
                const currentStep = branchSteps[i];
                const nextStep = branchSteps[i + 1];

                // Skip if current step has conditional routing
                if (this.stepHasConditionalRouting(currentStep)) {
                    continue;
                }

                // Skip if nextStep already has incoming conditional connection
                const hasIncomingConditional = this.connections.some(conn =>
                    (conn.type === 'conditional_true' || conn.type === 'conditional_false') &&
                    conn.to === nextStep.id
                );

                if (hasIncomingConditional) {
                    continue;
                }

                // Check if connection already exists
                const connectionExists = this.connections.some(conn =>
                    conn.from === currentStep.id && conn.to === nextStep.id
                );

                if (!connectionExists) {
                    console.log(`➡️ Adding branch-chain sequential: ${currentStep.stepName} → ${nextStep.stepName}`);
                    this.connections.push({
                        type: 'sequential',
                        from: currentStep.id,
                        to: nextStep.id,
                        fromStep: currentStep,
                        toStep: nextStep,
                        source: 'branch_chain_sequential'
                    });
                }
            }
        });
    }

    /**
     * Connect steps on the same Y row by X position (for steps not in branches)
     */
    addRowBasedConnections(steps) {
        const Y_TOLERANCE = 50;

        // Group steps by Y band (approximate row)
        const rowGroups = new Map();
        steps.forEach(step => {
            const y = step.position_y || 0;
            const bandKey = Math.round(y / Y_TOLERANCE) * Y_TOLERANCE;

            if (!rowGroups.has(bandKey)) {
                rowGroups.set(bandKey, []);
            }
            rowGroups.get(bandKey).push(step);
        });

        // Sort each row by X position and create sequential connections
        rowGroups.forEach((rowSteps, bandY) => {
            rowSteps.sort((a, b) => (a.position_x || 0) - (b.position_x || 0));

            for (let i = 0; i < rowSteps.length - 1; i++) {
                const currentStep = rowSteps[i];
                const nextStep = rowSteps[i + 1];

                // Skip if current step has conditional routing
                if (this.stepHasConditionalRouting(currentStep)) {
                    continue;
                }

                // Skip if nextStep already has an incoming connection
                const hasIncomingConnection = this.connections.some(conn =>
                    conn.to === nextStep.id
                );

                if (hasIncomingConnection) {
                    continue;
                }

                // Check if connection already exists
                const connectionExists = this.connections.some(conn =>
                    conn.from === currentStep.id && conn.to === nextStep.id
                );

                if (!connectionExists) {
                    console.log(`➡️ Adding row-based sequential: ${currentStep.stepName} → ${nextStep.stepName} (Y band: ${bandY})`);
                    this.connections.push({
                        type: 'sequential',
                        from: currentStep.id,
                        to: nextStep.id,
                        fromStep: currentStep,
                        toStep: nextStep,
                        source: 'row_based_sequential'
                    });
                }
            }
        });
    }

    /**
     * Check if a step has conditional routing configured
     */
    stepHasConditionalRouting(step) {
        // Use VisualStep utility for OOP-compliant type detection
        const isLogicStep = VisualStep.isConditionalStep(step);

        if (isLogicStep && step.config && step.config.conditions) {
            return this.hasConditionalRoutingNewFormat(step.config.conditions);
        }
        return false;
    }

    /**
     * Check if If-Then-Else config has conditional routing
     */
    hasConditionalRouting(ifThenElse) {
        if (!ifThenElse || !ifThenElse.conditions) return false;

        return ifThenElse.conditions.some(condition => {
            // Check if any THEN action routes to a step
            if (condition.thenActions) {
                const hasThenRoute = condition.thenActions.some(action =>
                    action.type === 'route_to_step' && action.stepId
                );
                if (hasThenRoute) return true;
            }

            // Check if any ELSE action routes to a step
            if (condition.elseActions) {
                const hasElseRoute = condition.elseActions.some(action =>
                    action.type === 'route_to_step' && action.stepId
                );
                if (hasElseRoute) return true;
            }

            return false;
        });
    }

    /**
     * Check if NEW FORMAT conditions have routing (step.config.conditions[].onTrue/onFalse)
     */
    hasConditionalRoutingNewFormat(conditions) {
        if (!conditions || !Array.isArray(conditions)) return false;

        return conditions.some(condition => {
            // Support both onTrue/onFalse and ifTrue/ifFalse naming
            const trueAction = condition.onTrue || condition.ifTrue;
            const falseAction = condition.onFalse || condition.ifFalse;

            const hasTrueRoute = trueAction && trueAction.action === 'route_to_step' && trueAction.stepId;
            const hasFalseRoute = falseAction && falseAction.action === 'route_to_step' && falseAction.stepId;
            return hasTrueRoute || hasFalseRoute;
        });
    }

    /**
     * Add conditional routing connections for NEW FORMAT (step.config.conditions[].onTrue/onFalse)
     */
    addConditionalConnectionsNewFormat(step, conditions, allSteps) {
        if (!conditions || !Array.isArray(conditions)) return;

        conditions.forEach((condition, condIndex) => {
            // Support both onTrue/onFalse and ifTrue/ifFalse naming
            const trueAction = condition.onTrue || condition.ifTrue;
            const falseAction = condition.onFalse || condition.ifFalse;

            // Add TRUE route connection (green)
            if (trueAction && trueAction.action === 'route_to_step' && trueAction.stepId) {
                const targetStep = allSteps.find(s => s.id === trueAction.stepId);
                console.log(`   🔗 Creating TRUE connection: ${step.stepName} → ${targetStep?.stepName || trueAction.stepId}`);

                this.connections.push({
                    type: 'conditional_true',
                    from: step.id,
                    to: trueAction.stepId,
                    fromStep: step,
                    toStep: targetStep,
                    conditionName: condition.condition?.field || `Condition ${condIndex + 1}`,
                    label: 'TRUE'
                });
            }

            // Add FALSE route connection (red)
            if (falseAction && falseAction.action === 'route_to_step' && falseAction.stepId) {
                const targetStep = allSteps.find(s => s.id === falseAction.stepId);
                console.log(`   🔗 Creating FALSE connection: ${step.stepName} → ${targetStep?.stepName || falseAction.stepId}`);

                this.connections.push({
                    type: 'conditional_false',
                    from: step.id,
                    to: falseAction.stepId,
                    fromStep: step,
                    toStep: targetStep,
                    conditionName: condition.condition?.field || `Condition ${condIndex + 1}`,
                    label: 'FALSE'
                });
            }
        });
    }

    /**
     * Add conditional routing connections for OLD FORMAT (step.config.ifThenElse)
     */
    addConditionalConnections(step, ifThenElse, allSteps) {
        if (!ifThenElse || !ifThenElse.conditions) return;

        ifThenElse.conditions.forEach((condition, condIndex) => {
            // Add THEN route connections (green)
            if (condition.thenActions) {
                condition.thenActions.forEach(action => {
                    if (action.type === 'route_to_step' && action.stepId) {
                        this.connections.push({
                            type: 'conditional_true',
                            from: step.id,
                            to: action.stepId,
                            fromStep: step,
                            toStep: allSteps.find(s => s.id === action.stepId),
                            conditionName: condition.name || `Condition ${condIndex + 1}`,
                            label: 'TRUE'
                        });
                    }
                });
            }

            // Add ELSE route connections (red)
            if (condition.elseActions) {
                condition.elseActions.forEach(action => {
                    if (action.type === 'route_to_step' && action.stepId) {
                        this.connections.push({
                            type: 'conditional_false',
                            from: step.id,
                            to: action.stepId,
                            fromStep: step,
                            toStep: allSteps.find(s => s.id === action.stepId),
                            conditionName: condition.name || `Condition ${condIndex + 1}`,
                            label: 'FALSE'
                        });
                    }
                });
            }
        });
    }

    /**
     * Add connections from step's own parentConditionalStepId and branchType properties
     * This is the authoritative source for conditional branch membership
     */
    addConnectionsFromBranchProperties(steps) {
        steps.forEach(step => {
            // Check if this step has branch properties set
            if (step.parentConditionalStepId && step.branchType) {
                const parentStep = steps.find(s => s.id === step.parentConditionalStepId);

                if (parentStep) {
                    const connectionType = step.branchType === 'true' ? 'conditional_true' : 'conditional_false';
                    const label = step.branchType === 'true' ? 'TRUE' : 'FALSE';

                    console.log(`🔗 [Branch Property] Creating ${label} connection: ${parentStep.stepName} → ${step.stepName}`);

                    // Check if this connection already exists (avoid duplicates)
                    const exists = this.connections.some(conn =>
                        conn.from === parentStep.id &&
                        conn.to === step.id &&
                        conn.type === connectionType
                    );

                    if (!exists) {
                        this.connections.push({
                            type: connectionType,
                            from: parentStep.id,
                            to: step.id,
                            fromStep: parentStep,
                            toStep: step,
                            label: label,
                            source: 'branch_property' // Mark as coming from step's own property
                        });
                    }
                } else {
                    console.warn(`⚠️ [Branch Property] Parent conditional step ${step.parentConditionalStepId} not found for step ${step.stepName}`);
                }
            }
        });
    }

    /**
     * Calculate overall bounding box
     */
    calculateBounds() {
        if (this.positions.size === 0) {
            return { minX: 0, minY: 0, maxX: 0, maxY: 0, width: 0, height: 0 };
        }

        let minX = Infinity, minY = Infinity, maxX = -Infinity, maxY = -Infinity;

        this.positions.forEach(pos => {
            minX = Math.min(minX, pos.x);
            minY = Math.min(minY, pos.y);
            maxX = Math.max(maxX, pos.x + pos.width);
            maxY = Math.max(maxY, pos.y + pos.height);
        });

        // Add padding
        const padding = 100;
        minX -= padding;
        minY -= padding;
        maxX += padding;
        maxY += padding;

        return {
            minX,
            minY,
            maxX,
            maxY,
            width: maxX - minX,
            height: maxY - minY
        };
    }

    /**
     * Get position for a specific step
     */
    getPosition(stepId) {
        return this.positions.get(stepId);
    }

    /**
     * Get all positions
     */
    getAllPositions() {
        return this.positions;
    }

    /**
     * Get all connections
     */
    getConnections() {
        return this.connections;
    }

    /**
     * Get swim lane metadata
     */
    getSwimLanes() {
        return this.swimLanes;
    }
}

// Export for use in other modules
if (typeof module !== 'undefined' && module.exports) {
    module.exports = HorizontalLayoutEngine;
}
