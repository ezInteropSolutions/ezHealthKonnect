/**
 * CoordinateSystem - Unified coordinate transformation system
 *
 * Handles conversion between:
 * - World coordinates (logical positions of nodes)
 * - Screen coordinates (actual pixel positions on canvas)
 * - Supports zoom and pan transformations
 */
class FlowchartCoordinateSystem {
    constructor() {
        // Transform state
        this.zoom = 1.0;           // Scale factor (0.1 to 3.0)
        this.panX = 0;             // X offset in screen pixels
        this.panY = 0;             // Y offset in screen pixels

        // Limits
        this.minZoom = 0.1;
        this.maxZoom = 3.0;

        console.log('✅ CoordinateSystem initialized');
    }

    /**
     * Convert world coordinates to screen coordinates
     */
    worldToScreen(worldX, worldY) {
        return {
            x: worldX * this.zoom + this.panX,
            y: worldY * this.zoom + this.panY
        };
    }

    /**
     * Convert screen coordinates to world coordinates
     */
    screenToWorld(screenX, screenY) {
        return {
            x: (screenX - this.panX) / this.zoom,
            y: (screenY - this.panY) / this.zoom
        };
    }

    /**
     * Apply zoom transformation
     */
    setZoom(newZoom, centerX = null, centerY = null) {
        // Clamp zoom
        newZoom = Math.max(this.minZoom, Math.min(this.maxZoom, newZoom));

        // Zoom toward center point if provided
        if (centerX !== null && centerY !== null) {
            // Convert center to world coords with old zoom
            const worldCenter = this.screenToWorld(centerX, centerY);

            // Update zoom
            const oldZoom = this.zoom;
            this.zoom = newZoom;

            // Adjust pan to keep world center at same screen position
            const newScreenCenter = this.worldToScreen(worldCenter.x, worldCenter.y);
            this.panX += (centerX - newScreenCenter.x);
            this.panY += (centerY - newScreenCenter.y);
        } else {
            this.zoom = newZoom;
        }

        return this.zoom;
    }

    /**
     * Apply pan transformation
     */
    setPan(deltaX, deltaY) {
        this.panX += deltaX;
        this.panY += deltaY;
    }

    /**
     * Reset to default view
     */
    reset() {
        this.zoom = 1.0;
        this.panX = 0;
        this.panY = 0;
    }

    /**
     * Get current transform as CSS matrix
     */
    getCSSTransform() {
        return `translate(${this.panX}px, ${this.panY}px) scale(${this.zoom})`;
    }

    /**
     * Get current transform for Canvas context
     */
    applyToCanvasContext(ctx) {
        ctx.setTransform(this.zoom, 0, 0, this.zoom, this.panX, this.panY);
    }

    /**
     * Calculate bounding box in screen coordinates
     */
    worldBoundsToScreen(worldBounds) {
        const topLeft = this.worldToScreen(worldBounds.minX, worldBounds.minY);
        const bottomRight = this.worldToScreen(worldBounds.maxX, worldBounds.maxY);

        return {
            left: topLeft.x,
            top: topLeft.y,
            right: bottomRight.x,
            bottom: bottomRight.y,
            width: bottomRight.x - topLeft.x,
            height: bottomRight.y - topLeft.y
        };
    }

    /**
     * Get transform state for serialization
     */
    getState() {
        return {
            zoom: this.zoom,
            panX: this.panX,
            panY: this.panY
        };
    }

    /**
     * Restore transform state
     */
    setState(state) {
        this.zoom = state.zoom || 1.0;
        this.panX = state.panX || 0;
        this.panY = state.panY || 0;
    }
}

// Export for use in other modules
if (typeof module !== 'undefined' && module.exports) {
    module.exports = FlowchartCoordinateSystem;
}
