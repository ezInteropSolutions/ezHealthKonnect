/**
 * ControlFlowStepsDocs — Documentation for control.* steps
 *
 * control.loop.
 *
 * Self-registers into StepDocumentationRegistry at load time — this file must be
 * loaded (via <script>) after StepDocumentationRegistry.js and before any step's
 * Documentation tab is opened. Mirrors the StepBuilderRegistry.register() pattern
 * already used by every step's Configuration-tab builder
 * (public/js/pipeline/components/StepBuilderRegistry.js).
 */

(function () {
    const docs = {
            'control.loop': {
                description: 'Container step that executes nested steps in a loop. Supports three loop types: For Each (iterate over collections like OBX segments), For (repeat N times), and While (condition-based). Child steps are visually nested inside the loop container and execute on each iteration with access to loop variables.',
                useCases: [
                    'Process all OBX segments in a lab result message (For Each over enhancedSegments.OBX)',
                    'Transform multiple diagnosis codes (For Each over DG1 segments)',
                    'Retry failed operations up to N times (For loop with retry logic)',
                    'Poll external API until data is ready (While loop with condition check)',
                    'Process patient encounters in batch (For Each over encounter list)',
                    'Apply transformations to all observations (For Each with nested transform steps)',
                    'Generate multiple FHIR resources from repeating HL7 segments'
                ],
                example: {
                    description: 'Process all OBX segments and transform to FHIR Observations',
                    loopType: 'foreach',
                    collection: 'enhancedSegments.OBX',
                    itemVariable: 'observation',
                    indexVariable: 'index',
                    childStepIds: ['validate-obx', 'transform-to-fhir', 'enrich-observation'],
                    maxIterations: 1000,
                    breakOnError: false,
                    continueOnEmpty: true
                },
                parameters: [
                    { name: 'loopType', type: 'enum', required: true, description: 'Type of loop: "foreach" (iterate collection), "for" (repeat N times), "while" (condition-based)' },
                    { name: 'collection', type: 'string', required: false, description: 'For "foreach": Field path to the array/collection to iterate over. Example: "enhancedSegments.OBX" for all OBX segments.' },
                    { name: 'itemVariable', type: 'string', required: false, description: 'Variable name for current item. Access as loop.{name} in child steps. Default: "item"' },
                    { name: 'indexVariable', type: 'string', required: false, description: 'Variable name for current index. Access as loop.{name} in child steps. Default: "index"' },
                    { name: 'iterations', type: 'number', required: false, description: 'For "for" loops: Number of times to execute the loop body.' },
                    { name: 'condition', type: 'object', required: false, description: 'For "while" loops: Condition object with field, operator, and value. Loop continues while condition is true.' },
                    { name: 'childStepIds', type: 'Array<string>', required: true, description: 'IDs of steps to execute in the loop body. Steps execute in order for each iteration.' },
                    { name: 'maxIterations', type: 'number', required: false, description: 'Safety limit to prevent infinite loops. Default: 1000' },
                    { name: 'breakOnError', type: 'boolean', required: false, description: 'Stop loop execution if a child step fails. Default: false (continue to next iteration)' },
                    { name: 'continueOnEmpty', type: 'boolean', required: false, description: 'Continue pipeline if collection is empty. Default: true' }
                ],
                loopVariables: {
                    title: 'Available Loop Variables',
                    description: 'Variables available inside child steps during loop execution:',
                    variables: [
                        { name: 'loop.{itemVariable}', description: 'Current item being processed (For Each only)', example: 'loop.observation' },
                        { name: 'loop.{indexVariable}', description: 'Current iteration index (0-based)', example: 'loop.index' },
                        { name: 'loop.iteration', description: 'Current iteration number (1-based)', example: 'loop.iteration' },
                        { name: 'loop.isFirst', description: 'True if this is the first iteration', example: 'loop.isFirst' },
                        { name: 'loop.isLast', description: 'True if this is the last iteration', example: 'loop.isLast' },
                        { name: 'loop.length', description: 'Total number of items (For Each only)', example: 'loop.length' },
                        { name: 'loop.total', description: 'Total number of iterations (For loop only)', example: 'loop.total' }
                    ]
                },
                loopTypes: {
                    title: 'Loop Type Comparison',
                    types: [
                        {
                            type: 'foreach',
                            name: 'For Each',
                            description: 'Iterates over each item in a collection',
                            useCase: 'Process all OBX segments, transform multiple diagnosis codes',
                            config: 'Requires: collection path'
                        },
                        {
                            type: 'for',
                            name: 'For (N times)',
                            description: 'Repeats execution a fixed number of times',
                            useCase: 'Retry operations, batch processing with fixed count',
                            config: 'Requires: iterations count'
                        },
                        {
                            type: 'while',
                            name: 'While',
                            description: 'Continues while a condition is true',
                            useCase: 'Poll until ready, process until quota reached',
                            config: 'Requires: condition (field, operator, value)'
                        }
                    ]
                },
                bestPractices: [
                    {
                        practice: 'Always set maxIterations',
                        reason: 'Prevents infinite loops that could hang the pipeline',
                        example: 'Set maxIterations to a reasonable value like 100 or 1000 based on expected data size'
                    },
                    {
                        practice: 'Use meaningful variable names',
                        reason: 'Makes child step configuration more readable',
                        example: 'Use "observation" instead of "item" when iterating OBX segments'
                    },
                    {
                        practice: 'Consider breakOnError setting',
                        reason: 'Decide if one failed iteration should stop all processing',
                        example: 'Set breakOnError=true for critical data, false for best-effort processing'
                    },
                    {
                        practice: 'Handle empty collections',
                        reason: 'Messages may not always have the expected segments',
                        example: 'Set continueOnEmpty=true to gracefully handle messages without OBX segments'
                    }
                ],
                troubleshooting: [
                    {
                        issue: 'Loop not executing any iterations',
                        cause: 'Collection path is incorrect or collection is empty',
                        fix: 'Verify collection path using the field selector. Check if continueOnEmpty is set correctly.'
                    },
                    {
                        issue: 'Loop stops after max iterations',
                        cause: 'maxIterations limit reached',
                        fix: 'Increase maxIterations if you expect more items, or check for infinite loop conditions in While loops'
                    },
                    {
                        issue: 'Child steps not receiving loop variables',
                        cause: 'Variable names not matching between loop config and child step config',
                        fix: 'Ensure itemVariable and indexVariable names match what child steps expect'
                    },
                    {
                        issue: 'While loop runs forever',
                        cause: 'Condition never becomes false',
                        fix: 'Verify condition field is being updated by child steps. Always have maxIterations as safety limit.'
                    }
                ]
            },
    };
    Object.keys(docs).forEach((stepType) => StepDocumentationRegistry.register(stepType, docs[stepType]));
})();
