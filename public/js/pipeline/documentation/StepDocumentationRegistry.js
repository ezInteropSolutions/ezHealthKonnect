/**
 * StepDocumentationRegistry — lookup for per-step-type "Documentation" tab content.
 *
 * Same shape as StepBuilderRegistry (public/js/pipeline/components/StepBuilderRegistry.js):
 * each category file under public/js/pipeline/documentation/ self-registers its own
 * step types at the bottom of the file. PropertiesPanel.js's getStepDocumentation()
 * looks entries up here instead of holding one giant hardcoded object itself.
 */
class StepDocumentationRegistry {
    static _registry = {};
    static _aliases = {};

    static register(stepType, docsObject) {
        this._registry[stepType] = docsObject;
    }

    static registerAlias(aliasType, targetType) {
        this._aliases[aliasType] = targetType;
    }

    static get(stepType) {
        return this._registry[stepType] || this._registry[this._aliases[stepType]];
    }
}
