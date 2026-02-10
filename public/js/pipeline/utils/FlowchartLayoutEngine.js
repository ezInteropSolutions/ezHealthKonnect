/**
 * FlowchartLayoutEngine - Intelligent Auto-Layout for Pipeline Steps
 *
 * Features:
 * - Auto-detects conditional forks from if-then-else routing
 * - Positions branches horizontally with smart spacing
 * - Detects merge points where paths rejoin
 * - Handles multi-level nesting
 * - Optimizes for minimal canvas space
 * - Single unified canvas (no layer grouping)
 */
class FlowchartLayoutEngine {
    constructor(config = {}) {
        // Layout configuration
        this.config = {
            stepSpacing: config.stepSpacing || 120,           // Vertical spacing between steps
            branchSpacing: config.branchSpacing || 180,       // Horizontal spacing for fork branches
            startX: config.startX || 400,                     // Center X position
            startY: config.startY || 80,                      // Top margin
            stepWidth: config.stepWidth || 140,               // Step node width
            stepHeight: config.stepHeight || 90,              // Step node height
            // Container system config
            containerPaddingX: config.containerPaddingX || 30,
            containerPaddingY: config.containerPaddingY || 20,
            containerHeaderHeight: config.containerHeaderHeight || 40,
            containerZoneGap: config.containerZoneGap || 15,
            containerMinWidth: config.containerMinWidth || 220,
            containerMinHeight: config.containerMinHeight || 160,
        };

        this.positions = new Map();
        this.connections = [];
        this.visited = new Set();
        this.containers = new Map(); // containerStepId -> { step, children: { zone: [steps] } }
        this.childStepIds = new Set(); // IDs of steps that are inside containers
    }

    /**
     * Main entry point - calculate positions for all steps
     */
    calculateLayout(steps) {
        if (!steps || steps.length === 0) {
            return { positions: new Map(), connections: [], containers: new Map() };
        }

        // Reset state
        this.positions.clear();
        this.connections = [];
        this.visited.clear();
        this.containers.clear();
        this.childStepIds.clear();

        // Identify container relationships before layout
        this.identifyContainers(steps);

        // Filter out child steps from main sequence (they'll be positioned inside containers)
        const topLevelSteps = steps.filter(s => !this.childStepIds.has(s.id));

        // Sort all steps by sequence (single unified list)
        const sortedSteps = [...topLevelSteps].sort((a, b) => (a.sequence || 0) - (b.sequence || 0));

        // Calculate layout for top-level steps (containers are positioned here with their children inside)
        const totalHeight = this.calculateSequenceLayout(
            sortedSteps,
            this.config.startY,
            this.config.startX
        );

        return {
            positions: this.positions,
            connections: this.connections,
            containers: this.containers,
            totalHeight: totalHeight + this.config.startY
        };
    }

    /**
     * Identify container steps and their children.
     * Builds this.containers map and this.childStepIds set.
     * Uses parentStepId/containerZone properties, falling back to config arrays.
     */
    identifyContainers(steps) {
        const stepMap = new Map(steps.map(s => [s.id, s]));

        // First pass: detect containers and collect children from config arrays
        for (const step of steps) {
            if (!VisualStep.isContainerStep(step)) continue;

            const zones = VisualStep.getContainerZones(step);
            const children = {};
            zones.forEach(z => { children[z] = []; });

            const config = step.config || {};

            if (VisualStep.isTryCatchStep(step)) {
                // Try-Catch: 3 config arrays
                ['try', 'catch', 'finally'].forEach(zone => {
                    const key = zone + 'Steps';
                    const ids = config[key] || [];
                    ids.forEach(id => {
                        const child = stepMap.get(id);
                        if (child) {
                            children[zone].push(child);
                            this.childStepIds.add(id);
                        }
                    });
                });
            } else if (VisualStep.isLoopStep(step)) {
                // Loop: childStepIds array
                const ids = config.childStepIds || [];
                ids.forEach(id => {
                    const child = stepMap.get(id);
                    if (child) {
                        children.body.push(child);
                        this.childStepIds.add(id);
                    }
                });
            } else if (VisualStep.isRetryStep(step)) {
                // Retry: childSteps array
                const ids = config.childSteps || [];
                ids.forEach(id => {
                    const child = stepMap.get(id);
                    if (child) {
                        children.body.push(child);
                        this.childStepIds.add(id);
                    }
                });
            }

            this.containers.set(step.id, { step, children });
        }

        // Second pass: also check parentStepId property (from DB) as override
        for (const step of steps) {
            if (!step.parentStepId || !step.containerZone) continue;
            const containerInfo = this.containers.get(step.parentStepId);
            if (!containerInfo) continue;

            const zone = step.containerZone;
            if (containerInfo.children[zone]) {
                // Avoid duplicates
                if (!containerInfo.children[zone].find(s => s.id === step.id)) {
                    containerInfo.children[zone].push(step);
                    this.childStepIds.add(step.id);
                }
            }
        }

        console.log(`📦 [Layout] Identified ${this.containers.size} containers, ${this.childStepIds.size} child steps`);
    }

    /**
     * Calculate layout for steps in sequence order
     * NO-CODE FRIENDLY: Auto-connects steps by sequence order
     */
    calculateSequenceLayout(steps, startY, centerX) {
        let currentY = startY;
        let maxY = startY;
        let i = 0;

        // Group steps by sequence number for handling duplicates
        const stepsBySequence = this.groupStepsBySequence(steps);
        const sequences = Array.from(stepsBySequence.keys()).sort((a, b) => a - b);

        console.log(`📊 [Layout] Layer has ${steps.length} steps in ${sequences.length} sequence groups:`, sequences);

        let prevSequenceSteps = [];

        for (const sequence of sequences) {
            const currentSteps = stepsBySequence.get(sequence);

            // Position all steps at this sequence level
            const numSteps = currentSteps.length;
            const totalWidth = (numSteps - 1) * this.config.branchSpacing;
            let startX = centerX - totalWidth / 2;

            currentSteps.forEach((step, idx) => {
                if (this.visited.has(step.id)) return;

                const stepX = numSteps > 1 ? startX + idx * this.config.branchSpacing : centerX;

                // Check if this is a container step (Loop, Try-Catch, Retry)
                const containerInfo = this.containers.get(step.id);
                if (containerInfo) {
                    const containerLayout = this.layoutContainer(step, containerInfo, currentY, stepX);
                    maxY = Math.max(maxY, containerLayout.maxY);
                    // Adjust currentY for next step after the container
                    currentY = containerLayout.maxY;
                }
                // Check if this is a fork point (If-Then-Else with routing)
                else if (this.detectFork(step, steps).isFork) {
                    const forkInfo = this.detectFork(step, steps);
                    const forkLayout = this.layoutFork(
                        step,
                        forkInfo,
                        steps,
                        currentY,
                        stepX
                    );

                    maxY = Math.max(maxY, forkLayout.maxY);
                } else {
                    // Position regular step
                    this.positions.set(step.id, {
                        x: stepX,
                        y: currentY,
                        width: this.config.stepWidth,
                        height: this.config.stepHeight
                    });

                    this.visited.add(step.id);
                }

                // Create connections FROM previous sequence steps TO this step
                // This creates the automatic sequential flow for no-code experience
                if (prevSequenceSteps.length > 0 && !this.visited.has(step.id + '_connected_from')) {
                    prevSequenceSteps.forEach(prevStep => {
                        // Check if prev step already has explicit routing (fork)
                        if (!this.isForkStep(prevStep)) {
                            console.log(`🔗 [Auto-Connect] ${prevStep.stepName} (seq ${prevStep.sequence}) → ${step.stepName} (seq ${step.sequence})`);
                            this.connections.push({
                                type: 'sequential',
                                from: prevStep.id,
                                to: step.id
                            });
                        }
                    });
                    // Mark as connected to avoid duplicate connections
                    this.visited.add(step.id + '_connected_from');
                }
            });

            prevSequenceSteps = currentSteps;
            currentY += this.config.stepSpacing;
            maxY = Math.max(maxY, currentY);
        }

        return maxY - startY;
    }

    /**
     * Group steps by their sequence number
     * Used for handling multiple steps at the same sequence level
     */
    groupStepsBySequence(steps) {
        const grouped = new Map();

        steps.forEach(step => {
            const seq = step.sequence || 0;
            if (!grouped.has(seq)) {
                grouped.set(seq, []);
            }
            grouped.get(seq).push(step);
        });

        return grouped;
    }

    /**
     * Detect if a step is a fork point (if-then-else with routing)
     */
    detectFork(step, allSteps) {
        if (step.stepType !== 'control' || step.subType !== 'conditional') {
            return { isFork: false };
        }

        const config = step.config;
        if (!config || !config.conditions || config.conditions.length === 0) {
            return { isFork: false };
        }

        const condition = config.conditions[0];
        const trueAction = condition.ifTrue;
        const falseAction = condition.ifFalse;

        console.log('🔍 [Auto-Connection] Checking if-then-else step:', step.stepName);
        console.log('   TRUE action:', trueAction);
        console.log('   FALSE action:', falseAction);

        // Check if either branch routes to a different step
        const hasRouting =
            (trueAction && trueAction.action === 'route_to_step' && trueAction.stepId) ||
            (falseAction && falseAction.action === 'route_to_step' && falseAction.stepId);

        if (!hasRouting) {
            console.log('   ❌ No routing configured - no auto-connections will be created');
            return { isFork: false };
        }

        // Find target steps
        const trueBranch = this.findBranch(trueAction, allSteps);
        const falseBranch = this.findBranch(falseAction, allSteps);

        console.log('   ✅ Routing detected!');
        console.log('      TRUE branch → Step ID:', trueAction?.stepId, '(found:', trueBranch.steps.length > 0, ')');
        console.log('      FALSE branch → Step ID:', falseAction?.stepId, '(found:', falseBranch.steps.length > 0, ')');

        return {
            isFork: true,
            condition: config.conditions[0],
            trueBranch,
            falseBranch
        };
    }

    /**
     * Find the steps in a branch
     */
    findBranch(action, allSteps) {
        if (!action || action.action !== 'route_to_step' || !action.stepId) {
            return { steps: [], targetStepId: null };
        }

        const targetStep = allSteps.find(s => s.id === action.stepId);
        if (!targetStep) {
            return { steps: [], targetStepId: action.stepId };
        }

        return {
            steps: [targetStep],
            targetStepId: targetStep.id
        };
    }

    /**
     * Layout a fork (if-then-else with branches)
     */
    layoutFork(forkStep, forkInfo, allSteps, startY, centerX) {
        // Position the fork step itself
        this.positions.set(forkStep.id, {
            x: centerX,
            y: startY,
            width: this.config.stepWidth,
            height: this.config.stepHeight
        });
        this.visited.add(forkStep.id);

        const forkBottomY = startY + this.config.stepHeight + 40; // Extra space for fork connector
        let branchMaxY = forkBottomY;

        // Position TRUE branch (left)
        if (forkInfo.trueBranch.steps.length > 0) {
            const trueX = centerX - this.config.branchSpacing;
            let trueY = forkBottomY;

            forkInfo.trueBranch.steps.forEach(step => {
                this.positions.set(step.id, {
                    x: trueX,
                    y: trueY,
                    width: this.config.stepWidth,
                    height: this.config.stepHeight
                });
                this.visited.add(step.id);

                console.log(`   🔗 Creating TRUE branch connection: ${forkStep.stepName} → ${step.stepName}`);
                this.connections.push({
                    type: 'true-branch',
                    from: forkStep.id,
                    to: step.id,
                    label: 'TRUE'
                });

                trueY += this.config.stepSpacing;
            });

            branchMaxY = Math.max(branchMaxY, trueY);
        }

        // Position FALSE branch (right)
        if (forkInfo.falseBranch.steps.length > 0) {
            const falseX = centerX + this.config.branchSpacing;
            let falseY = forkBottomY;

            forkInfo.falseBranch.steps.forEach(step => {
                this.positions.set(step.id, {
                    x: falseX,
                    y: falseY,
                    width: this.config.stepWidth,
                    height: this.config.stepHeight
                });
                this.visited.add(step.id);

                console.log(`   🔗 Creating FALSE branch connection: ${forkStep.stepName} → ${step.stepName}`);
                this.connections.push({
                    type: 'false-branch',
                    from: forkStep.id,
                    to: step.id,
                    label: 'FALSE'
                });

                falseY += this.config.stepSpacing;
            });

            branchMaxY = Math.max(branchMaxY, falseY);
        }

        // Find merge point (next sequential step after both branches)
        const mergeStep = this.findMergePoint(forkInfo, allSteps, forkStep.sequence);

        if (mergeStep) {
            const mergeY = branchMaxY + this.config.stepSpacing / 2;

            this.positions.set(mergeStep.id, {
                x: centerX,
                y: mergeY,
                width: this.config.stepWidth,
                height: this.config.stepHeight
            });
            this.visited.add(mergeStep.id);

            // Add merge connections from both branches
            if (forkInfo.trueBranch.steps.length > 0) {
                const lastTrueStep = forkInfo.trueBranch.steps[forkInfo.trueBranch.steps.length - 1];
                this.connections.push({
                    type: 'merge',
                    from: lastTrueStep.id,
                    to: mergeStep.id
                });
            }

            if (forkInfo.falseBranch.steps.length > 0) {
                const lastFalseStep = forkInfo.falseBranch.steps[forkInfo.falseBranch.steps.length - 1];
                this.connections.push({
                    type: 'merge',
                    from: lastFalseStep.id,
                    to: mergeStep.id
                });
            }

            branchMaxY = mergeY + this.config.stepHeight;
        }

        return {
            maxY: branchMaxY,
            nextY: branchMaxY + this.config.stepSpacing,
            lastStepId: mergeStep ? mergeStep.id : forkStep.id
        };
    }

    /**
     * Find the merge point where branches rejoin
     */
    findMergePoint(forkInfo, allSteps, forkSequence) {
        // Get the last steps of each branch
        const trueLastStep = forkInfo.trueBranch.steps[forkInfo.trueBranch.steps.length - 1];
        const falseLastStep = forkInfo.falseBranch.steps[forkInfo.falseBranch.steps.length - 1];

        if (!trueLastStep && !falseLastStep) return null;

        // Find the maximum sequence from both branches
        const maxBranchSeq = Math.max(
            trueLastStep?.sequence || 0,
            falseLastStep?.sequence || 0,
            forkSequence
        );

        // Find next sequential step that hasn't been visited
        return allSteps.find(s =>
            s.sequence > maxBranchSeq &&
            !this.visited.has(s.id) &&
            !this.isForkStep(s)
        );
    }

    /**
     * Layout a container step (Loop, Try-Catch, Retry) with its children inside.
     * Returns { maxY } so main layout continues below.
     */
    layoutContainer(containerStep, containerInfo, startY, centerX) {
        const cfg = this.config;
        const zones = VisualStep.getContainerZones(containerStep);
        const isTryCatch = zones.length > 1;

        // Calculate child positions within each zone
        const zoneLayouts = [];
        let totalChildrenWidth = 0;
        let maxZoneHeight = 0;

        for (const zone of zones) {
            const children = containerInfo.children[zone] || [];
            // Sort children by sequence
            children.sort((a, b) => (a.sequence || 0) - (b.sequence || 0));

            // Each zone lays out children vertically
            const zoneChildPositions = [];
            let childY = 0;
            let zoneWidth = cfg.containerMinWidth;

            children.forEach((child, idx) => {
                const pos = {
                    step: child,
                    relX: 0,  // Relative to zone center
                    relY: childY,
                    width: cfg.stepWidth,
                    height: cfg.stepHeight
                };
                zoneChildPositions.push(pos);
                zoneWidth = Math.max(zoneWidth, cfg.stepWidth + cfg.containerPaddingX * 2);
                childY += cfg.stepSpacing;
            });

            const zoneContentHeight = children.length > 0
                ? (children.length - 1) * cfg.stepSpacing + cfg.stepHeight
                : 40; // Empty zone min height

            const zoneHeight = Math.max(cfg.containerMinHeight - cfg.containerHeaderHeight, zoneContentHeight + cfg.containerPaddingY * 2);

            zoneLayouts.push({
                zone,
                children: zoneChildPositions,
                width: zoneWidth,
                height: zoneHeight,
                contentHeight: zoneContentHeight
            });

            maxZoneHeight = Math.max(maxZoneHeight, zoneHeight);
        }

        // For Try-Catch: zones arranged side by side
        // For Loop/Retry: single zone
        let containerWidth, containerHeight;
        const zoneLabelHeight = 24;

        if (isTryCatch) {
            // Side-by-side zones with gaps
            containerWidth = zoneLayouts.reduce((sum, z) => sum + z.width, 0)
                + (zoneLayouts.length - 1) * cfg.containerZoneGap
                + cfg.containerPaddingX * 2;
            containerHeight = cfg.containerHeaderHeight + maxZoneHeight + zoneLabelHeight + cfg.containerPaddingY * 2;
        } else {
            const singleZone = zoneLayouts[0];
            containerWidth = Math.max(cfg.containerMinWidth, singleZone.width + cfg.containerPaddingX * 2);
            containerHeight = cfg.containerHeaderHeight + singleZone.height + cfg.containerPaddingY * 2;
        }

        // Ensure minimum dimensions
        containerWidth = Math.max(containerWidth, cfg.containerMinWidth);
        containerHeight = Math.max(containerHeight, cfg.containerMinHeight);

        // Store container position (center-based like regular steps)
        const containerPos = {
            x: centerX,
            y: startY,
            width: containerWidth,
            height: containerHeight,
            isContainer: true,
            containerType: containerStep.stepType,
            zones: []
        };

        // Now position children in absolute coordinates and build zone metadata
        const containerLeft = centerX - containerWidth / 2;
        const childrenStartY = startY + cfg.containerHeaderHeight + cfg.containerPaddingY;

        if (isTryCatch) {
            // Position zones side by side
            let zoneX = containerLeft + cfg.containerPaddingX;

            zoneLayouts.forEach(zl => {
                const zoneCenterX = zoneX + zl.width / 2;
                const zoneTop = childrenStartY + zoneLabelHeight;

                // Zone metadata for rendering
                containerPos.zones.push({
                    zone: zl.zone,
                    x: zoneX,
                    y: childrenStartY,
                    width: zl.width,
                    height: maxZoneHeight + zoneLabelHeight,
                    centerX: zoneCenterX
                });

                // Position children within zone
                zl.children.forEach(cp => {
                    const absX = zoneCenterX;
                    const absY = zoneTop + cfg.containerPaddingY + cp.relY;

                    this.positions.set(cp.step.id, {
                        x: absX,
                        y: absY,
                        width: cp.width,
                        height: cp.height
                    });
                    this.visited.add(cp.step.id);
                });

                // Internal connections between children in same zone
                for (let i = 0; i < zl.children.length - 1; i++) {
                    this.connections.push({
                        type: 'container-internal',
                        from: zl.children[i].step.id,
                        to: zl.children[i + 1].step.id
                    });
                }

                zoneX += zl.width + cfg.containerZoneGap;
            });
        } else {
            // Single zone (Loop/Retry) - children centered vertically
            const singleZone = zoneLayouts[0];
            const zoneCenterX = centerX;
            const zoneTop = childrenStartY;

            containerPos.zones.push({
                zone: singleZone.zone,
                x: containerLeft + cfg.containerPaddingX,
                y: zoneTop,
                width: containerWidth - cfg.containerPaddingX * 2,
                height: singleZone.height,
                centerX: zoneCenterX
            });

            singleZone.children.forEach(cp => {
                const absX = zoneCenterX;
                const absY = zoneTop + cfg.containerPaddingY + cp.relY;

                this.positions.set(cp.step.id, {
                    x: absX,
                    y: absY,
                    width: cp.width,
                    height: cp.height
                });
                this.visited.add(cp.step.id);
            });

            // Internal connections
            for (let i = 0; i < singleZone.children.length - 1; i++) {
                this.connections.push({
                    type: 'container-internal',
                    from: singleZone.children[i].step.id,
                    to: singleZone.children[i + 1].step.id
                });
            }
        }

        // Store container position
        this.positions.set(containerStep.id, containerPos);
        this.visited.add(containerStep.id);

        const maxY = startY + containerHeight;

        console.log(`📦 [Layout] Container "${containerStep.stepName}" (${containerStep.stepType}): ${containerWidth}x${containerHeight} at (${centerX}, ${startY}), ${zones.length} zones, maxY=${maxY}`);

        return { maxY };
    }

    /**
     * Check if a step is a fork step
     */
    isForkStep(step) {
        if (step.stepType !== 'control' || step.subType !== 'conditional') {
            return false;
        }

        const config = step.config;
        if (!config || !config.conditions) return false;

        const condition = config.conditions[0];
        if (!condition) return false;

        return (
            (condition.ifTrue && condition.ifTrue.action === 'route_to_step') ||
            (condition.ifFalse && condition.ifFalse.action === 'route_to_step')
        );
    }

    /**
     * Find step index by ID
     */
    findStepIndex(steps, stepId) {
        return steps.findIndex(s => s.id === stepId);
    }

    /**
     * Get node center coordinates
     */
    getNodeCenter(stepId) {
        const pos = this.positions.get(stepId);
        if (!pos) return { x: 0, y: 0 };

        return {
            x: pos.x,
            y: pos.y + pos.height / 2
        };
    }

    /**
     * Get connection points for a step (top and bottom)
     */
    getConnectionPoints(stepId) {
        const pos = this.positions.get(stepId);
        if (!pos) return null;

        return {
            top: { x: pos.x, y: pos.y },
            bottom: { x: pos.x, y: pos.y + pos.height },
            left: { x: pos.x - pos.width / 2, y: pos.y + pos.height / 2 },
            right: { x: pos.x + pos.width / 2, y: pos.y + pos.height / 2 },
            center: { x: pos.x, y: pos.y + pos.height / 2 }
        };
    }

    /**
     * Calculate bounding box for all positioned steps
     */
    getBoundingBox() {
        if (this.positions.size === 0) {
            return { minX: 0, minY: 0, maxX: 0, maxY: 0, width: 0, height: 0 };
        }

        let minX = Infinity, minY = Infinity, maxX = -Infinity, maxY = -Infinity;

        this.positions.forEach(pos => {
            const left = pos.x - pos.width / 2;
            const right = pos.x + pos.width / 2;
            const top = pos.y;
            const bottom = pos.y + pos.height;

            minX = Math.min(minX, left);
            minY = Math.min(minY, top);
            maxX = Math.max(maxX, right);
            maxY = Math.max(maxY, bottom);
        });

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
     * Adjust layout to fit within canvas bounds
     */
    fitToCanvas(canvasWidth, canvasHeight, padding = 40) {
        const bbox = this.getBoundingBox();

        if (bbox.width === 0 || bbox.height === 0) return;

        // Calculate scale to fit
        const scaleX = (canvasWidth - padding * 2) / bbox.width;
        const scaleY = (canvasHeight - padding * 2) / bbox.height;
        const scale = Math.min(scaleX, scaleY, 1); // Don't scale up, only down

        // Calculate offset to center
        const offsetX = padding - bbox.minX * scale + (canvasWidth - bbox.width * scale) / 2;
        const offsetY = padding - bbox.minY * scale;

        // Apply transformation to all positions
        const transformedPositions = new Map();
        this.positions.forEach((pos, id) => {
            transformedPositions.set(id, {
                x: pos.x * scale + offsetX,
                y: pos.y * scale + offsetY,
                width: pos.width * scale,
                height: pos.height * scale
            });
        });

        this.positions = transformedPositions;
    }
}

// Export for use in other modules
if (typeof module !== 'undefined' && module.exports) {
    module.exports = FlowchartLayoutEngine;
}
