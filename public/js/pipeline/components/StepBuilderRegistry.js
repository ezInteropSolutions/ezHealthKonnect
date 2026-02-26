/**
 * StepBuilderRegistry — singleton registry that maps step types to builder classes.
 *
 * Usage:
 *   StepBuilderRegistry.register('file_parser', FileParserBuilder);
 *   const builder = StepBuilderRegistry.create('file_parser', panelInstance);
 *   if (builder) { html = builder.render(step); ... }
 *
 * Builder contract:
 *   render(step)          → string  HTML to insert into the properties panel
 *   collectConfig(step)   → void    reads DOM, writes values into step.config
 *   destroy()             → void    tears down AC refs / event listeners
 */
class StepBuilderRegistry {
    static _registry = {};

    static register(stepType, BuilderClass) {
        this._registry[stepType] = BuilderClass;
    }

    /**
     * Create a fresh builder instance for the given step type.
     * @param {string} stepType
     * @param {object} panel - PropertiesPanel instance (passed to builder constructor)
     * @returns {object|null} builder instance, or null if no builder is registered
     */
    static create(stepType, panel) {
        const Cls = this._registry[stepType];
        return Cls ? new Cls(panel) : null;
    }

    static has(stepType) {
        return !!this._registry[stepType];
    }
}
