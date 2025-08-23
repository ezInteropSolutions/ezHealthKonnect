# Code Summary (Generated 2025-08-19T16:06:34.068342)

## How to use this document as a context manager
- Purpose: Quickly understand what each file does and where to look for specific behaviors.
- For any file listed below, check the added bullets (Purpose, Technical, Logical role, Business value) to orient yourself fast.
- Use this as a map: locate the file heading, skim its bullets, then jump into the source for details.

### Schema quick reference (HL7 and FHIR)
- HL7 schemas (filesystem layout):
  - `schemas/hl7/v2.1` … `schemas/hl7/v2.8` contain versioned message definitions (`*.gz`).
  - Primary loaders/parsers:
    - `hl7/unified_parser.go` (env-aware facade; uses `EZHEALTHKONNECT_SCHEMA_DIR` if set)
    - `hl7/real_schema_parser.go` (scans/parses real HL7 schema files)
- FHIR schemas (filesystem layout):
  - `schemas/fhir/R4/resources/*.gz`, `schemas/fhir/R4/profiles/us-core/*.gz`, plus datatypes and valuesets.
  - Primary loader:
    - `fhir/schema_loader.go` (version-aware scanning, parsing, caching)
- Helpful endpoints (for quick inspection from the API):
  - HL7: `controllers/hl7_contoller.go` → list schemas, loader status
  - FHIR: `controllers/fhir_transform_controller.go` → list schemas, get schema details

## Python
_Scanned 4 files → 26 functions, 0 classes._

### C:\Projects\ezHealthKonnect\context_manager.py
- **Purpose**: Generates repo-wide code and schema summaries for context-aware tooling.
- **Technical**: Walks directories, filters files, extracts symbols per language, writes Markdown to `context/`.
- **Logical role**: Central context builder consumed by dev utilities and AI assistants.
- **Business value**: Speeds onboarding and change impact analysis by making architecture and APIs discoverable.
- Function `schema_summary`(args: ['pg_conn_str'])
- Function `should_skip_dir`(args: ['d'])
- Function `iter_files`(args: ['root', 'exts'])
- Function `summarize_python`(args: ['root'])
- Function `summarize_node`(args: ['root'])
- Function `summarize_go`(args: ['root'])
- Function `save_text`(args: ['filename', 'text'])

### C:\Projects\ezHealthKonnect\mcp_generator.py
- **Purpose**: Produces change intelligence (diff summaries, checkpoints, code index).
- **Technical**: Hashes files, extracts TODOs, computes deltas vs checkpoints, emits artifacts under `mcp_output/`.
- **Logical role**: Enables incremental documentation and traceability across iterations.
- **Business value**: Improves auditability and reduces regression risk with lightweight change reports.
- Function `is_valid_file`(args: ['file_path'])
- Function `is_excluded`(args: ['path'])
- Function `hash_file`(args: ['filepath'])
- Function `extract_todos`(args: ['filepath'])
- Function `summarize_file`(args: ['filepath'])
- Function `archive_existing_files`(args: [])
- Function `find_latest_archive`(args: [])
- Function `load_json`(args: ['path'])
- Function `get_changed_files`(args: ['current', 'previous'])
- Function `extract_summaries`(args: ['index', 'changed_files'])
- Function `write_diff_summary_md`(args: ['summaries'])
- Function `write_checkpoints`(args: ['summaries'])
- Function `main`(args: [])

### C:\Projects\ezHealthKonnect\tests\test_hl7_fhir.py
- Function `load_json_file`(args: ['file_path'])
- Function `check_service_status`(args: ['base_url'])
- Function `test_transformation`(args: ['base_url', 'parsed_hl7_data'])
- Function `display_results`(args: ['result'])
- Function `save_results`(args: ['result', 'filename'])
- Function `main`(args: [])


## Node.js (JS/TS)
_Scanned 55 files → 138 functions, 64 classes._

### C:\Projects\ezHealthKonnect\app.js
- **Purpose**: Node/Express application entrypoint and lightweight proxy to Go backend.
- **Technical**: Defines auth/status routes, dashboard rendering, and test proxy endpoints for connectivity.
- **Logical role**: Web gateway for UI pages and API session handling.
- **Business value**: Centralizes access control and enables hybrid Node+Go deployment.
- Function `testGoBackendConnectivity`
_Routes:_
- Route GET /api/proxy/test
- Route GET /api/proxy/test-direct-backend
- Route GET /dashboard
- Route GET /api/user-info
- Route GET /api/status
- Route POST /api/login
- Route POST /api/logout

### C:\Projects\ezHealthKonnect\reset-password.js
- **Purpose**: Password reset workflow handler.
- **Technical**: Validates input, updates credentials, and communicates result to the client.
- **Logical role**: Supports account recovery across environments.
- **Business value**: Reduces support burden and improves user retention/security.
- Function `resetPassword`

### C:\Projects\ezHealthKonnect\server.js
- **Purpose**: Boots the Node server and binds the Express app.
- **Technical**: Configures server lifecycle, env loading, and listens on configured port.
- **Logical role**: Operational entrypoint used by process managers/containers.
- **Business value**: Provides predictable startup for deploy and local dev.
- Function `startServer`

### C:\Projects\ezHealthKonnect\config\database.js
- **Purpose**: Database connection manager and helper utilities for Node services.
- **Technical**: Encapsulates connection pooling, configuration, and basic query helpers via `DatabaseManager`.
- **Logical role**: Single responsibility layer for DB access in Node modules.
- **Business value**: Improves reliability and reduces duplicate database code.
- Class `DatabaseManager`

### C:\Projects\ezHealthKonnect\controllers\interfacesController.js
- **Purpose**: REST controller for interface lifecycle management (create, start, pause, delete).
- **Technical**: Implements CRUD and action endpoints, validates input, and returns paginated data.
- **Logical role**: Orchestrates interface records used by transformation flows.
- **Business value**: Enables operations teams to manage message flows without code changes.
- Class `InterfacesController`

### C:\Projects\ezHealthKonnect\controllers\userController.js
- **Purpose**: REST controller for user management.
- **Technical**: Handles user CRUD, role enforcement, and status updates.
- **Logical role**: Central place for user lifecycle and permissions.
- **Business value**: Supports secure, role-based access to functionality.
- Class `UserController`

### C:\Projects\ezHealthKonnect\middleware\auth.js
- **Purpose**: Express middleware for authentication and authorization.
- **Technical**: Verifies tokens/sessions and enforces admin-only routes.
- **Logical role**: Cross-cutting concern for API protection.
- **Business value**: Protects PHI-related endpoints and admin tools.
- Function `requireAuth`
- Function `requireAdmin`
- Function `verifyToken`

### C:\Projects\ezHealthKonnect\node-api\hl7-dictionary-service.js
- **Purpose**: Microservice for HL7 dictionary lookups and enhancement utilities.
- **Technical**: Exposes health and enhancement routes; computes coverage and smart field info.
- **Logical role**: Supplies UI/tools with schema metadata and examples.
- **Business value**: Accelerates mapping by surfacing field semantics.
- Function `getSmartFieldInfo`
_Routes:_
- Route GET /health
- Route GET /api/v1/hl7/test-enhanced
- Route GET /api/v1/hl7/coverage
- Route POST /api/v1/hl7/enhance

### C:\Projects\ezHealthKonnect\public\js\dashboard.js
- **Purpose**: Frontend dashboard orchestration and navigation.
- **Technical**: Renders sections, updates time/user info, and handles route-like UI.
- **Logical role**: Landing UX for operational monitoring.
- **Business value**: Quick visibility into system status and management areas.
- Function `updateTime`
- Function `loadUserInfo`
- Function `logout`
- Function `handleNavigation`
- Function `showDashboardContent`
- Function `showInterfacesContent`
- Function `showMessagesContent`
- Function `showTemplatesContent`
- Function `showMonitoringContent`
- Function `showReportsContent`
- Function `showUserManagementContent`
- Function `showSettingsContent`
- Function `showAccessDenied`
- Function `showNotImplemented`
- Function `updatePageTitle`
- Function `showPlaceholderContent`
- Function `getIconForSection`
- Function `showDashboard`
- Class `from`
- Class `to`

### C:\Projects\ezHealthKonnect\public\js\hl7Service.js
- **Purpose**: Frontend service wrapper for HL7-related API calls.
- **Technical**: Provides methods for fetching schemas, validation, and enhancements.
- **Logical role**: Decouples UI from transport concerns.
- **Business value**: Simplifies UI development and improves maintainability.
- Class `HL7Service`
- Class `async`

### C:\Projects\ezHealthKonnect\public\js\interfaces.js
- **Purpose**: UI for managing integration interfaces.
- **Technical**: Fetches data, renders tables/cards, handles pagination and actions.
- **Logical role**: Operator console for interface lifecycle.
- **Business value**: Reduces operational toil with clear controls.
- Function `initializeInterfacesPage`
- Function `setupTooltips`
- Function `showTooltip`
- Function `hideTooltip`
- Function `moveTooltip`
- Function `loadUserInfo`
- Function `updateTime`
- Function `logout`
- Function `setupEventListeners`
- Function `setupPaginationListener`
- Function `newHandler`
- Function `handlePageSizeChange`
- Function `setupSidebarToggle`
- Function `setupAutoRefresh`
- Function `addAutoRefreshIndicator`
- Function `updateRefreshRate`
- Function `startAutoRefresh`
- Function `stopAutoRefresh`
- Function `handleUserInteraction`
- Function `handleVisibilityChange`
- Function `performAutoRefresh`
- Function `loadInterfaces`
- Function `refreshInterfaces`
- Function `updateSummaryCards`
- Function `calculatePagination`
- Function `renderInterfacesTable`
- Function `createCompactTableRow`
- Function `getMiniActionButtons`
- Function `formatCompactTime`
- Function `formatCompactNumber`
- Function `applyFilters`
- Function `goToPage`
- Function `goToPreviousPage`
- Function `goToNextPage`
- Function `updatePaginationInfo`
- Function `updatePaginationControls`
- Function `startInterface`
- Function `stopInterface`
- Function `pauseInterface`
- Function `deleteInterface`
- Function `showCreateModal`
- Function `closeCreateModal`
- Function `showEditModal`
- Function `closeEditModal`
- Function `showInterfaceDetails`
- Function `closeDetailsModal`
- Function `createDetailsContent`
- Function `handleCreateInterface`
- Function `showSuccess`
- Function `showError`
- Function `setupSidebarTooltips`
- Function `injectTooltipStyles`
- Function `initializeSidebarTooltips`

### C:\Projects\ezHealthKonnect\public\js\login.js
- **Purpose**: Authentication UI flows.
- **Technical**: Manages form state and shows contextual messages.
- **Logical role**: Entry point for secured features.
- **Business value**: Secure access and better UX for users.
- Function `showMessage`
- Function `showRegister`
- Function `showForgotPassword`

### C:\Projects\ezHealthKonnect\public\js\user-management.js
- **Purpose**: Frontend user administration panel.
- **Technical**: Lists users, toggles status, shows stats, and supports CRUD modals.
- **Logical role**: Admin control surface for accounts.
- **Business value**: Enforces governance and least-privilege operations.
- Function `updateTime`
- Function `loadCurrentUser`
- Function `loadUsers`
- Function `getMockUsers`
- Function `updateUserStats`
- Function `renderUsersTable`
- Function `formatDate`
- Function `showCreateUserModal`
- Function `closeCreateUserModal`
- Function `showEditUserModal`
- Function `closeEditUserModal`
- Function `toggleUserStatus`
- Function `refreshUsers`
- Function `showAlert`
- Function `showLoading`
- Function `logout`

### C:\Projects\ezHealthKonnect\public\js\components\header-component.js
- **Purpose**: Loads shared header/navigation component.
- **Technical**: Fetches and injects reusable HTML into pages.
- **Logical role**: Ensures consistent layout across views.
- **Business value**: Improves usability and branding consistency.
- Function `loadHeaderComponent`

### C:\Projects\ezHealthKonnect\public\js\components\modal-components.js
- **Purpose**: Reusable modal dialogs for create/edit/details.
- **Technical**: Dynamically loads and binds modal content/events.
- **Logical role**: Standardizes UI interactions.
- **Business value**: Faster feature delivery with shared components.
- Function `loadModalComponents`
- Function `loadCreateModal`
- Function `loadEditModal`
- Function `loadDetailsModal`

### C:\Projects\ezHealthKonnect\public\js\components\table-component.js
- **Purpose**: Reusable table component for paginated data.
- **Technical**: Handles rendering and basic interactions for tabular content.
- **Logical role**: Reduces duplication in list views.
- **Business value**: Consistent UX across data-heavy screens.
- Function `loadTableComponent`

### C:\Projects\ezHealthKonnect\public\js\components\wizard-component.js
- **Purpose**: Modal wizard scaffolding used by the mapping interface.
- **Technical**: Initializes wizard DOM and binds step events.
- **Logical role**: Container for multi-step workflows.
- **Business value**: Guides users through complex configuration steps.
- Function `loadWizardComponent`
- Function `setupWizardModalEvents`

### C:\Projects\ezHealthKonnect\public\js\core\env-config.js
- **Purpose**: Centralized environment configuration for the frontend.
- **Technical**: Provides environment-aware base URLs and flags via `EnvironmentConfig`.
- **Logical role**: Avoids magic constants across modules.
- **Business value**: Safer deploys with environment-specific tweaks.
- Class `EnvironmentConfig`

### C:\Projects\ezHealthKonnect\public\js\core\wizard-functions.js
- **Purpose**: Core helpers for the interface wizard and mapping dialogs.
- **Technical**: Encapsulates modal orchestration, API calls, and HL7 structure loading.
- **Logical role**: Shared logic used by multiple wizard steps/components.
- **Business value**: Speeds feature development in the wizard UI.
- Function `openInterfaceWizard`
- Function `closeWizard`
- Function `toggleResourceCard`
- Function `openMappingDialog`
- Function `closeMappingDialog`
- Function `saveMapping`
- Function `closeLoadConfigModal`
- Function `closeSaveConfigModal`
- Function `selectConfiguration`
- Function `loadSelectedConfig`
- Function `saveConfigurationToDb`
- Function `closeCreateModal`
- Function `closeEditModal`
- Function `closeDetailsModal`
- Function `logout`
- Function `getApiBaseUrl`
- Function `loadHL7Structure`
- Function `loadDefaultConfiguration`
- Function `FHIRFocusedMapping`
- Class `when`
- Class `if`

### C:\Projects\ezHealthKonnect\public\js\core\wizard-navigation.js
- **Purpose**: Controls navigation within the multi-step wizard.
- **Technical**: Sets up listeners, handles maximize/flow state, and manages progress.
- **Logical role**: UX layer for step progression.
- **Business value**: Reduces user error and abandonment by guiding flows.
- Function `setupListeners`
- Function `setupMaximizeButton`
- Function `initWizardNavigation`
- Class `WizardNavigation`

### C:\Projects\ezHealthKonnect\public\js\modules\healthcare-rules.js
- **Purpose**: Encodes human-readable healthcare rules used for guidance/validation.
- **Technical**: Provides declarative constructs consumed by validation/UI.
- **Logical role**: Domain rule library.
- **Business value**: Captures clinical logic to improve mapping quality.
- Class `HealthcareRules`
- Class `vs`
- Class `with`
- Class `but`
- Class `consistency`
- Class `indicates`

### C:\Projects\ezHealthKonnect\public\js\modules\hl7-schemas.js
- **Purpose**: Client-side access to HL7 schema metadata for the UI.
- **Technical**: `HL7Schemas` fetches and caches schema details for rendering/validation.
- **Logical role**: Bridges backend schema services to the wizard.
- **Business value**: Speeds mapping by surfacing segment/field structure.
- Class `HL7Schemas`

### C:\Projects\ezHealthKonnect\public\js\modules\hl7-validator.js
- **Purpose**: Lightweight HL7 validation utilities in the browser.
- **Technical**: Indexes fields, validates presence/format, and emits issues.
- **Logical role**: Immediate feedback during mapping.
- **Business value**: Reduces downstream errors early.
- Function `indexField`
- Class `HL7Validator`

### C:\Projects\ezHealthKonnect\public\js\modules\step4-wizard-handler.js
- **Purpose**: Specialized handler for Step 4 (FHIR mapping) in the wizard.
- **Technical**: Manages state, events, and API calls related to mapping resources.
- **Logical role**: Coordinates UI and backend during mapping.
- **Business value**: Streamlines creation of accurate mappings.
- Class `FHIRMappingStepHandler`

### C:\Projects\ezHealthKonnect\public\js\modules\validation-integration.js
- **Purpose**: Integrates validation results into the UI.
- **Technical**: Adds issues to views and binds user interactions.
- **Logical role**: Presenting validation feedback.
- **Business value**: Improves data quality and compliance.
- Function `addItems`
- Class `ValidationIntegration`

### C:\Projects\ezHealthKonnect\public\js\modules\validation-ui.js
- **Purpose**: UI components and flows for validation messages.
- **Technical**: Renders warnings/errors and supports filtering.
- **Logical role**: Visibility into mapping and data issues.
- **Business value**: Faster troubleshooting and remediation.
- Class `ValidationUI`

### C:\Projects\ezHealthKonnect\public\js\step4\enhanced-mapping-interface.js
- **Purpose**: Rich mapping interface for configuring FHIR field mappings.
- **Technical**: Advanced DOM logic, interactions, and data binding for mapping.
- **Logical role**: Primary user workspace for transformation setup.
- **Business value**: Shortens time-to-integration for new feeds.
- Class `EnhancedMappingInterface`

### C:\Projects\ezHealthKonnect\public\js\step4\field-mapping-validator.js
- **Purpose**: Client-side validation of field mapping rules.
- **Technical**: Validates rule completeness and consistency.
- **Logical role**: Guardrail before persisting mappings.
- **Business value**: Prevents invalid rules from entering production.
- Class `FieldMappingValidator`

### C:\Projects\ezHealthKonnect\public\js\step4\step4-config-manager.js
- **Purpose**: Manages persistence/loading of Step 4 configuration presets.
- **Technical**: Saves/loads JSON configs and merges with current state.
- **Logical role**: Configuration lifecycle for mapping sessions.
- **Business value**: Reusability and repeatability of mappings.
- Class `Step4ConfigManager`

### C:\Projects\ezHealthKonnect\public\js\step4\step4-handler.js
- **Purpose**: Controller for Step 4 operations and interactions.
- **Technical**: Orchestrates resources, templates, and validation during mapping.
- **Logical role**: Glue between services and UI components in Step 4.
- **Business value**: Ensures efficient, guided mapping flows.
- Class `FHIRMappingStepHandler`
- Class `globally`
- Class `is`

### C:\Projects\ezHealthKonnect\public\js\step4\step4-json-viewer.js
- **Purpose**: Presents configuration and mapping output JSON in a readable way.
- **Technical**: Pretty-prints, collapses/expands nodes, and supports highlighting.
- **Logical role**: Aids understanding and debugging of mapping state.
- **Business value**: Reduces configuration mistakes.
- Class `Step4JsonViewer`

### C:\Projects\ezHealthKonnect\public\js\step4\step4-mapping.js
- **Purpose**: Core data model and utilities for mapping definitions.
- **Technical**: Structures mapping rules and computes derived views.
- **Logical role**: Canonical representation of a mapping session.
- **Business value**: Enables export/import and collaboration.
- Class `Step4Mapping`

### C:\Projects\ezHealthKonnect\public\js\step4\step4-modals.js
- **Purpose**: Modal interactions specific to Step 4 configuration and mapping.
- **Technical**: Opens/closes modals, binds actions, and validates inputs.
- **Logical role**: User interaction layer for mapping operations.
- **Business value**: Streamlines user tasks.
- Class `Step4Modals`

### C:\Projects\ezHealthKonnect\public\js\step4\step4-resources.js
- **Purpose**: Resource utilities for Step 4 (FHIR resources, paths, fields).
- **Technical**: Provides lookups and helpers for schema-driven fields.
- **Logical role**: Abstraction over schema details for the UI.
- **Business value**: Faster configuration with fewer errors.
- Class `Step4Resources`

### C:\Projects\ezHealthKonnect\public\js\step4\step4-styles.js
- **Purpose**: Styling helpers and constants for Step 4 UI components.
- **Technical**: Manages CSS class names and computed styles.
- **Logical role**: Keeps UI cohesive and themeable.
- **Business value**: Professional, consistent look-and-feel.
- Class `Step4Styles`

### C:\Projects\ezHealthKonnect\public\js\step4\step4-templates.js
- **Purpose**: Templating helpers for rendering mapping UIs and summaries.
- **Technical**: Computes field counts and builds HTML snippets.
- **Logical role**: View layer utilities for Step 4.
- **Business value**: Speeds UI development and readability.
- Function `countFields`
- Class `Step4Templates`

### C:\Projects\ezHealthKonnect\public\js\step4\step4-utils.js
- **Purpose**: Utility functions used across Step 4 modules.
- **Technical**: Common operations like deep access, cloning, and path formatting.
- **Logical role**: Shared foundation to reduce duplication.
- **Business value**: Lowers maintenance costs.
- Class `Step4Utils`

### C:\Projects\ezHealthKonnect\public\js\step4\step4-validation.js
- **Purpose**: Validates mappings against simple constraints client-side.
- **Technical**: Runs checks on required/duplicate/format aspects.
- **Logical role**: Early validation before server-side checks.
- **Business value**: Reduces round-trips and speeds iteration.
- Class `Step4Validation`

### C:\Projects\ezHealthKonnect\public\js\wizard\module-loader.js
- **Purpose**: Lazy loader for wizard modules/assets.
- **Technical**: Checks prerequisites and initializes modules on demand.
- **Logical role**: Performance and reliability for wizard boot.
- **Business value**: Faster, smoother UX in large pages.
- Function `checkAndLoad`
- Function `initializeWizardLoader`
- Class `WizardModuleLoader`
- Class `not`
- Class `WizardAutoLoader`
- Class `from`

### C:\Projects\ezHealthKonnect\public\js\wizard\segment-viewer.js
- **Purpose**: Visualizes HL7 segments and fields in the wizard.
- **Technical**: Renders segment structure and supports highlighting/selection.
- **Logical role**: Aids understanding of source data.
- **Business value**: Reduces mapping errors by improving visibility.
- Class `SegmentViewer`
- Class `is`

### C:\Projects\ezHealthKonnect\public\js\wizard\step-handlers.js
- **Purpose**: Base and concrete handlers for each wizard step.
- **Technical**: Defines lifecycle hooks and shared utilities.
- **Logical role**: Framework for multi-step flows.
- **Business value**: Consistency and extensibility across steps.
- Class `for`
- Class `BaseStepHandler`
- Class `ConfigurationStepHandler`
- Class `UploadStepHandler`
- Class `ReviewStepHandler`
- Class `MappingStepHandler`
- Class `SummaryStepHandler`

### C:\Projects\ezHealthKonnect\public\js\wizard\step4-integration.js
- **Purpose**: Bridges Step 4 UI to backend services.
- **Technical**: Calls APIs, synchronizes state, and handles async flows.
- **Logical role**: Integration layer for mapping features.
- **Business value**: Reliable execution of complex mapping operations.
- Class `FHIRMappingStepHandler`
- Class `BaseStepHandler`
- Class `globally`
- Class `is`

### C:\Projects\ezHealthKonnect\public\js\wizard\wizard-main.js
- **Purpose**: Wizard bootstrap and overall UI wiring.
- **Technical**: Initializes buttons, modal, and fallback handlers.
- **Logical role**: Entry for wizard experience.
- **Business value**: Smooth setup of mapping sessions.
- Function `setupButtons`
- Function `initializeWizard`
- Class `redeclaration`
- Class `InterfaceWizardModal`
- Class `FallbackStep4Handler`
- Class `available`

### C:\Projects\ezHealthKonnect\public\js\wizard\wizard-services.js
- **Purpose**: Shared client-side services (notifications, validation, loading, HL7 API).
- **Technical**: Provides service objects for UI composition and background tasks.
- **Logical role**: Infrastructure for wizard and admin pages.
- **Business value**: Faster dev velocity and consistent UX feedback.
- Class `NotificationService`
- Class `ValidationService`
- Class `in`
- Class `HL7Service`
- Class `LoadingService`

### C:\Projects\ezHealthKonnect\routes\interfacesRoutes.js
- **Purpose**: Express router for interface operations.
- **Technical**: Defines RESTful endpoints with session guard via `sessionAuth`.
- **Logical role**: Binds controller to HTTP.
- **Business value**: Clear, maintainable API surface for operations.
- Function `sessionAuth`
_Routes:_
- Route GET /
- Route GET /:interfaceId
- Route POST /
- Route POST /:interfaceId/start
- Route POST /:interfaceId/stop
- Route POST /:interfaceId/pause
- Route DELETE /:interfaceId

### C:\Projects\ezHealthKonnect\scripts\migrate-interfaces.js
- **Purpose**: One-off/ops script to migrate interface data/state.
- **Technical**: Executes migration steps with logs and basic safety checks.
- **Logical role**: Supports evolution of data models.
- **Business value**: Low-risk upgrades without downtime.
- Function `runMigration`

### C:\Projects\ezHealthKonnect\services\auditService.js
- **Purpose**: Centralized audit logging service.
- **Technical**: Writes structured audit records for actions and results.
- **Logical role**: Compliance and forensics data pipeline.
- **Business value**: Meets regulatory requirements and supports incident response.
- Class `AuditService`

### C:\Projects\ezHealthKonnect\services\userService.js
- **Purpose**: User data access/service layer for Node.
- **Technical**: Encapsulates queries and transformations for user entities.
- **Logical role**: Single source of truth for user operations.
- **Business value**: Reduces duplication and enforces consistency.
- Class `UserService`

### C:\Projects\ezHealthKonnect\tests\debug-connectivity.js
- **Purpose**: Quick connectivity test to backend services.
- **Technical**: Simple HTTP calls with logging of responses.
- **Logical role**: Sanity check during local/dev setups.
- **Business value**: Speeds troubleshooting and CI smoke checks.
- Function `testEndpoint`
- Function `runTests`


## Go
_Scanned 30 files → 453 functions/methods, 126 types._

### C:\Projects\ezHealthKonnect\fhir_converter.go
- **Purpose**: Offline converter from FHIR StructureDefinitions to optimized schema JSON used by the app.
- **Technical**: Processes base and US Core profiles, flattens elements, and writes compact schemas.
- **Logical role**: Prepares schema artifacts for fast runtime validation/transformation.
- **Business value**: Improves performance and reliability of FHIR operations.
- Function/Method `NewFHIRConverter`
- Function/Method `ConvertAll`
- Function/Method `createOutputStructure`
- Function/Method `processBaseResources`
- Function/Method `processUSCoreProfiles`
- Function/Method `convertStructureDefinition`
- Function/Method `convertElement`
- Function/Method `saveSchema`
- Function/Method `printStats`
- Function/Method `main`
- Struct `FHIRBundle`
- Struct `FHIREntry`
- Struct `FHIRStructureDefinition`
- Struct `FHIRSnapshot`
- Struct `FHIRDifferential`
- Struct `FHIRElementDefinition`
- Struct `FHIRElementType`
- Struct `FHIRBinding`
- Struct `FHIRConstraint`
- Struct `OptimizedFHIRSchema`
- Struct `OptimizedElement`
- Struct `FHIRConverter`
- Struct `ConversionStats`

### C:\Projects\ezHealthKonnect\main.go
- **Purpose**: Go backend entrypoint initializing config, schema loaders, and HTTP routes.
- **Technical**: Loads env, initializes HL7 and FHIR schema loaders, wires controllers and CORS.
- **Logical role**: Composition root of the Go services.
- **Business value**: Stable server foundation for transformation APIs.
- Function/Method `main`
- Function/Method `min`
- Function/Method `setupWizardAPI`
- Function/Method `corsHandler`
- Function/Method `handleMappingRules`

### C:\Projects\ezHealthKonnect\config\config.go
 - **Purpose**: Central configuration loader and validator for the Go backend.
 - **Technical**: Reads env, validates HL7/FHIR schema directories, and exposes typed config.
 - **Logical role**: Prevents misconfiguration at startup.
 - **Business value**: Reduces runtime incidents due to bad config.
 - Function/Method `Load`
- Function/Method `getEnv`
- Function/Method `getBoolEnv`
- Function/Method `getIntEnv`
- Function/Method `getStringSlice`
- Function/Method `IsDevelopment`
- Function/Method `IsProduction`
- Function/Method `UseFilesystemSchema`
- Function/Method `UseLegacyDictionary`
- Function/Method `GetSchemaDirectory`
- Function/Method `GetFHIRSchemaDirectory`
- Function/Method `GetDictionaryURL`
- Function/Method `GetSchemaConfig`
- Function/Method `GetFHIRConfig`
- Function/Method `ValidateSchemaConfig`
- Function/Method `validateHL7Schemas`
- Function/Method `validateFHIRSchemas`
- Function/Method `LogConfiguration`
- Function/Method `GetDatabaseURL`
- Function/Method `normalizeSSLMode`
- Function/Method `HasDatabaseConfig`
- Struct `Config`

### C:\Projects\ezHealthKonnect\controllers\fhir_resource_controller.go
- **Purpose**: Determines FHIR resources required for each HL7 message type and exposes discovery APIs.
- **Technical**: Computes required/conditional resources, supports interface overrides, and produces statistics.
- **Logical role**: Resource planning layer before transformation.
- **Business value**: Predictable outputs and configurable resource sets tailored to workflows.
- Function/Method `NewFHIRResourceController`
- Function/Method `RegisterRoutes`
- Function/Method `GetResourcesForMessageType`
- Function/Method `GetResourcesForMessageTypeWithData`
- Function/Method `SaveInterfaceOverride`
- Function/Method `TestResourceIdentification`
- Function/Method `countRequiredResources`
- Function/Method `countConditionalResources`
- Function/Method `getTotalPossibleResources`
- Function/Method `createSampleHL7Data`
- Struct `FHIRResourceController`
- Struct `GetResourcesForMessageTypeRequest`
- Struct `GetResourcesForMessageTypeResponse`
- Struct `ResourceIdentificationStatistics`
- Struct `SaveInterfaceOverrideRequest`

### C:\Projects\ezHealthKonnect\controllers\fhir_transform_controller.go
- **Purpose**: Core REST API for transforming HL7 input into FHIR resources/bundles.
- **Technical**: Loads schemas, applies mapping rules, validates results, collects analytics, and logs.
- **Logical role**: Orchestrator for the FHIR transformation pipeline.
- **Business value**: Delivers standardized FHIR content for downstream systems.
- Function/Method `NewFHIRTransformController`
- Function/Method `RegisterRoutes`
- Function/Method `GetStatus`
- Function/Method `Transform`
- Function/Method `GetRules`
- Function/Method `CreateRule`
- Function/Method `UpdateRule`
- Function/Method `DeleteRule`
- Function/Method `GetSchemas`
- Function/Method `GetSchemaDetails`
- Function/Method `ValidateResources`
- Function/Method `TestValidation`
- Function/Method `GetAnalytics`
- Function/Method `GetTransformationLogs`
- Function/Method `generateRequestID`
- Function/Method `getStatCount`
- Function/Method `processHL7Input`
- Function/Method `loadTransformationRules`
- Function/Method `loadRequiredSchemas`
- Function/Method `applyTransformationRules`
- Function/Method `initializeFHIRResource`
- Function/Method `applyRule`
- Function/Method `extractHL7Value`
- Function/Method `transformValue`
- Function/Method `setFHIRPath`
- Function/Method `hasContent`
- Function/Method `lookupCodeMapping`
- Function/Method `validateResources`
- Function/Method `validateRuleAgainstSchema`
- Function/Method `loadSchemasForResources`
- Function/Method `createBundle`
- Function/Method `logTransformation`
- Struct `FHIRTransformController`
- Struct `TransformRequest`
- Struct `TransformResponse`
- Struct `TransformationRule`
- Struct `RuleManagementRequest`

### C:\Projects\ezHealthKonnect\controllers\hl7_contoller.go
- **Purpose**: HL7 parsing/validation endpoints using real schemas.
- **Technical**: Enhances messages with schema metadata, validates structure, and reports stats.
- **Logical role**: Source data quality gate prior to mapping.
- **Business value**: Reduces failures downstream by catching HL7 issues early.
- Function/Method `NewHL7Controller`
- Function/Method `ParseMessage`
- Function/Method `ValidateMessage`
- Function/Method `GetStats`
- Function/Method `convertBasicToEnhanced`
- Function/Method `getSchemaLoaderStats`
- Function/Method `ListSchemas`
- Function/Method `GetSchemaStatus`
- Function/Method `extractMessageTypeCode`
- Function/Method `extractMessageTypeEvent`
- Function/Method `getMessageDescription`
- Struct `HL7Controller`

### C:\Projects\ezHealthKonnect\controllers\hl7_fhir_transformation_controller.go
- **Purpose**: End-to-end HL7→FHIR orchestration for advanced workflows.
- **Technical**: Exposes templates, mappings, analytics, preview/testing, and validation utilities.
- **Logical role**: Composite controller beyond raw transform for UI/ops needs.
- **Business value**: Enables rapid iteration and visibility for integration teams.
- Function/Method `NewHL7FHIRTransformationController`
- Function/Method `RegisterRoutes`
- Function/Method `TransformHL7ToFHIR`
- Function/Method `GetTransformationStatus`
- Function/Method `TestTransformation`
- Function/Method `ValidateTransformation`
- Function/Method `GetMessageTemplates`
- Function/Method `GetFieldMappings`
- Function/Method `GetValueSetMappings`
- Function/Method `GetTransformationAnalytics`
- Function/Method `GetTransformationLogs`
- Function/Method `hasErrorSeverity`
- Function/Method `generateExampleParsedHL7`
- Struct `HL7FHIRTransformationController`

### C:\Projects\ezHealthKonnect\controllers\interface_controller.go
- **Purpose**: CRUD and lifecycle management for integration interfaces.
- **Technical**: Handles state transitions (start/stop/pause), ID generation, and persistence.
- **Logical role**: Manages units of configuration for message pipelines.
- **Business value**: Operational control and governance over integrations.
- Function/Method `NewInterfaceController`
- Function/Method `CreateInterface`
- Function/Method `GetInterfaces`
- Function/Method `GetInterface`
- Function/Method `UpdateInterface`
- Function/Method `DeleteInterface`
- Function/Method `StartInterface`
- Function/Method `StopInterface`
- Function/Method `PauseInterface`
- Function/Method `generateInterfaceID`
- Struct `InterfaceController`
- Struct `CreateInterfaceRequest`
- Struct `InterfaceResponse`

### C:\Projects\ezHealthKonnect\controllers\schema_fhir_transform_controller.go
- **Purpose**: Schema-aware transformation and validation APIs used by the wizard.
- **Technical**: Lists schemas, previews transformations, validates mappings, and manages rules at scale.
- **Logical role**: Backend for interactive mapping/validation UX.
- **Business value**: Shortens mapping cycles and improves correctness.
- Function/Method `NewSchemaFHIRTransformController`
- Function/Method `RegisterRoutes`
- Function/Method `GetStatus`
- Function/Method `ListSchemas`
- Function/Method `Transform`
- Function/Method `ValidateOnly`
- Function/Method `GetRules`
- Function/Method `CreateRule`
- Function/Method `UpdateRule`
- Function/Method `DeleteRule`
- Function/Method `BatchUpdateRules`
- Function/Method `GetUIConfiguration`
- Function/Method `GetMappingSuggestions`
- Function/Method `PreviewTransformation`
- Function/Method `ValidateMapping`
- Function/Method `GetHL7Structure`
- Function/Method `GetFHIRStructure`
- Function/Method `GetAnalytics`
- Function/Method `GetTransformationLogs`
- Function/Method `UpdateConfiguration`
- Function/Method `loadMappingRulesFromDB`
- Function/Method `saveRulesToDB`
- Function/Method `getHL7StructureForMessageType`
- Function/Method `getFHIRStructureForProfile`
- Function/Method `getMappingStatistics`
- Function/Method `validateMappingCompleteness`
- Function/Method `generateFieldMappingSuggestions`
- Function/Method `validateRulesAgainstSchemas`
- Function/Method `autoFixMappingIssues`
- Function/Method `getDefaultMappingsForMessageType`
- Struct `SchemaFHIRTransformController`
- Struct `MappingRule`
- Struct `HL7FieldStructure`
- Struct `HL7FieldDefinition`
- Struct `HL7ComponentDefinition`
- Struct `FHIRResourceStructure`
- Struct `FHIRFieldDefinition`

### C:\Projects\ezHealthKonnect\controllers\system_controller.go
- **Purpose**: Health, metrics, and system info endpoints.
- **Technical**: Reports runtime status, build info, and environment details.
- **Logical role**: Operational observability.
- **Business value**: Facilitates monitoring and support.
- Function/Method `NewSystemController`
- Function/Method `HealthCheck`
- Function/Method `GetInfo`
- Function/Method `GetMetrics`
- Struct `SystemController`

### C:\Projects\ezHealthKonnect\controllers\wizard_api_controller.go
- **Purpose**: Backend for wizard-focused operations (parsing, rules, validation, AI suggestions).
- **Technical**: Adapts between UI view models and backend models; exposes parse/validate endpoints.
- **Logical role**: API surface for interactive mapping wizard.
- **Business value**: Makes complex mapping tasks approachable and efficient.
- Function/Method `NewWizardController`
- Function/Method `toWizardView`
- Function/Method `fromWizardView`
- Function/Method `getHL7FieldName`
- Function/Method `RegisterRoutes`
- Function/Method `ParseHL7`
- Function/Method `GetMappingRules`
- Function/Method `SaveMappingRules`
- Function/Method `TestTransformation`
- Function/Method `CreateInterface`
- Function/Method `GenerateAISuggestions`
- Function/Method `ValidateFHIRResource`
- Function/Method `countTotalFields`
- Function/Method `getDefaultMappingRules`
- Struct `WizardController`
- Struct `WizardParseRequest`
- Struct `WizardParseResponse`
- Struct `WizardParsedData`
- Struct `WizardMetadata`
- Struct `WizardMappingRuleView`
- Struct `MappingRulesResponse`
- Struct `SaveRulesRequest`
- Struct `WizardInterfaceRequest`

### C:\Projects\ezHealthKonnect\fhir\schema_loader.go
- **Purpose**: Runtime FHIR schema loader and cache.
- **Technical**: Scans versioned directories for `.gz` StructureDefinitions, parses, validates, and caches.
- **Logical role**: Provides schemas to validators and transformers.
- **Business value**: Ensures standards compliance and performance at runtime.
- Function/Method `InitFHIRSchemaLoader`
- Function/Method `scanVersionAwareFHIRSchemas`
- Function/Method `GetFHIRSchemaLoader`
- Function/Method `LoadFHIRSchema`
- Function/Method `resolveSchemaPath`
- Function/Method `tryAlternativePaths`
- Function/Method `loadAndParseFHIRSchema`
- Function/Method `validateFHIRSchema`
- Function/Method `GetStats`
- Function/Method `ClearCache`
- Function/Method `ListAvailableSchemas`
- Function/Method `scanForFHIRSchemas`
- Struct `FHIRSchema`
- Struct `FHIRElement`
- Struct `FHIRSchemaStats`
- Struct `FHIRSchemaLoader`

### C:\Projects\ezHealthKonnect\fhir\transformation_engine.go
- **Purpose**: Applies mapping rules to create FHIR resources from HL7 content.
- **Technical**: Loads required schemas, initializes resources, and sets values with type-aware logic.
- **Logical role**: Core transformation logic engine used by controllers/services.
- **Business value**: Accurate, repeatable FHIR generation.
- Function/Method `NewTransformationEngine`
- Function/Method `Transform`
- Function/Method `loadRequiredSchemas`
- Function/Method `applyTransformationRules`
- Function/Method `initializeResourceFromSchema`
- Function/Method `getDefaultValueForDataType`
- Function/Method `extractMessageType`
- Function/Method `loadMappingRules`
- Function/Method `errorResponse`
- Function/Method `getVersionOrDefault`
- Function/Method `getProfileOrDefault`
- Function/Method `getKeys`
- Struct `TransformationEngine`
- Struct `TransformationConfig`
- Struct `TransformationRequest`
- Struct `TransformationResponse`
- Struct `ValidationError`
- Struct `TransformWarning`
- Struct `TransformationMetadata`
- Struct `PerformanceMetrics`
- Struct `MappingRule`

### C:\Projects\ezHealthKonnect\fhir\validation_bundle.go
- **Purpose**: Validates FHIR resources/bundles against loaded schemas and bindings.
- **Technical**: Checks required/mustSupport, cardinality, data types, constraints, and value set bindings.
- **Logical role**: Quality gate before emitting FHIR.
- **Business value**: Reduces integration defects and compliance issues.
- Function/Method `validateResourcesAgainstSchemas`
- Function/Method `validateResourceAgainstSchema`
- Function/Method `validateRequiredElement`
- Function/Method `validateMustSupportElement`
- Function/Method `validateFieldAgainstElement`
- Function/Method `validateCardinality`
- Function/Method `validateDataType`
- Function/Method `validateConstraints`
- Function/Method `validateValueSetBinding`
- Function/Method `isEmpty`
- Function/Method `createFHIRBundle`
- Function/Method `logTransformation`
- Function/Method `GetStatus`
- Function/Method `UpdateConfiguration`

### C:\Projects\ezHealthKonnect\fhir\value_transformers.go
- **Purpose**: HL7→FHIR datatype/value conversions.
- **Technical**: Transforms identifiers, names, addresses, dates, codes, and choice types.
- **Logical role**: Reusable conversion utilities.
- **Business value**: Consistent data representation across resources.
- Function/Method `extractHL7Value`
- Function/Method `transformValue`
- Function/Method `transformToFHIRIdentifier`
- Function/Method `transformToFHIRHumanName`
- Function/Method `transformToFHIRAddress`
- Function/Method `transformToFHIRContactPoint`
- Function/Method `transformToFHIRDate`
- Function/Method `transformToFHIRDateTime`
- Function/Method `transformToFHIRCode`
- Function/Method `transformToFHIRCoding`
- Function/Method `transformToFHIRCodeableConcept`
- Function/Method `transformToBoolean`
- Function/Method `transformToInteger`
- Function/Method `transformToDecimal`
- Function/Method `mapAssigningAuthorityToSystem`
- Function/Method `mapCodingSystemToFHIR`
- Function/Method `mapValueSetCode`
- Function/Method `evaluateCondition`
- Function/Method `setFHIRValue`
- Function/Method `isResourcePopulated`
- Function/Method `ensureRequiredFields`

### C:\Projects\ezHealthKonnect\hl7\parser.go
- **Purpose**: HL7 message parsing and basic enhancement utilities.
- **Technical**: Parses segments/fields/components with delimiter handling and metadata lookup.
- **Logical role**: Converts raw HL7 into structured form for mapping.
- **Business value**: Foundational step enabling reliable transformation.
- Function/Method `ParseHL7MessageEnhanced`
- Function/Method `ParseHL7Field`
- Function/Method `ConvertBasicToEnhancedWithDelimiters`
- Function/Method `convertEnhancedFieldToSubfields`
- Function/Method `DemonstrateDelimiterParsing`
- Function/Method `extractFieldPosition`
- Function/Method `TestSpecificExample`
- Function/Method `extractMessageType`
- Function/Method `getBasicHL7SegmentOrder`
- Function/Method `getFieldName`
- Function/Method `getFieldDescription`
- Function/Method `getFieldDataType`
- Function/Method `getFieldOptionality`
- Function/Method `getComponentName`
- Function/Method `getComponentDataType`
- Function/Method `getComponentDescription`
- Struct `EnhancedFieldData`
- Struct `FieldRepetition`
- Struct `FieldComponent`

### C:\Projects\ezHealthKonnect\hl7\real_schema_parser.go
- **Purpose**: Real HL7 schema loader/parser for versioned `.gz` message definitions.
- **Technical**: Scans `schemas/hl7/v2.x`, parses message/segment/field structures, normalizes versions, caches stats.
- **Logical role**: Source of truth for HL7 structure used by parsing/validation.
- **Business value**: Accurate handling of vendor/version variations.
- Function/Method `InitRealSchemaLoader`
- Function/Method `GetRealSchemaLoader`
- Function/Method `scanForSchemaFiles`
- Function/Method `createSampleSchema`
- Function/Method `LoadRealSchema`
- Function/Method `normalizeVersionForSchema`
- Function/Method `loadAndParseSchemaFile`
- Function/Method `convertOrderedRawSchema`
- Function/Method `convertOrderedSegment`
- Function/Method `min`
- Function/Method `convertOrderedField`
- Function/Method `convertComponent`
- Function/Method `extractSequenceFromRaw`
- Function/Method `extractComponentPosition`
- Function/Method `extractComponentValue`
- Function/Method `GetStats`
- Function/Method `ParseWithRealSchema`
- Function/Method `enhanceWithRealSchema`
- Function/Method `createEnhancedSegmentFromBasic`
- Function/Method `createEnhancedSegmentWithSchema`
- Function/Method `getBasicHL7Order`
- Function/Method `createMessageTypeInfo`
- Function/Method `extractMessageInfoSimple`
- Function/Method `validateRequiredFields`
- Struct `RealHL7Schema`
- Struct `RealSegmentDef`
- Struct `RealFieldDef`
- Struct `RealComponentDef`
- Struct `RealSchemaLoader`
- Struct `RealSchemaStats`
- Struct `segmentForSorting`

### C:\Projects\ezHealthKonnect\hl7\types.go
- **Purpose**: Strongly-typed models for HL7 parsing, validation, and statistics.
- **Technical**: Request/response DTOs, intermediate models, and validator interfaces.
- **Logical role**: Contracts between controllers/services and parsers.
- **Business value**: Safer refactors and clearer APIs.
- Struct `ParseRequest`
- Struct `ParseResponse`
- Struct `ParseMeta`
- Struct `CacheStats`
- Struct `BasicParsedMessage`
- Struct `BasicSegment`
- Struct `EnhancedParsedMessage`
- Struct `MessageTypeInfo`
- Struct `EnhancedSegment`
- Struct `FieldInfo`
- Struct `SubfieldInfo`
- Struct `ValidationError`
- Struct `PositionValidationResult`
- Struct `FieldPosition`
- Struct `DuplicatePosition`
- Struct `ParsingOptions`
- Struct `SchemaValidationResult`
- Struct `DictionaryResponse`
- Struct `MessageStructure`
- Struct `ValidationRule`
- Struct `FieldPositionHelper`
- Struct `ComponentPositionHelper`
- Struct `MessageStatistics`
- Struct `SegmentStats`
- Struct `ValidationSummary`
- Interface `PositionValidator`

### C:\Projects\ezHealthKonnect\hl7\unified_parser.go
- **Purpose**: Environment-aware facade for HL7 schema loading and parsing.
- **Technical**: Resolves schema directory (env/container/filesystem), delegates to real loader, exposes cache/status.
- **Logical role**: Stable API for consuming HL7 schemas across environments.
- **Business value**: Portability and predictable behavior in dev/CI/prod.
- Function/Method `InitSchemaLoader`
- Function/Method `GetSchemaLoader`
- Function/Method `SetMaxCacheSize`
- Function/Method `ClearCache`
- Function/Method `GetCacheStats`
- Function/Method `ListAvailableSchemas`
- Function/Method `ParseHL7Enhanced`
- Function/Method `convertBasicToMapFormatFixed`
- Function/Method `sortFieldsByPosition`
- Function/Method `validateAndFixFieldPositioning`
- Function/Method `isValidFieldKey`
- Function/Method `countErrorsBySeverity`
- Function/Method `extractMessageInfoForParsing`
- Function/Method `createMessageTypeInfoForParsing`
- Function/Method `ConvertBasicToEnhancedMap`
- Function/Method `GetSchemaLoaderStatus`
- Struct `SchemaLoader`

### C:\Projects\ezHealthKonnect\services\hl7_fhir_transform_service.go
- **Purpose**: Schema-driven HL7→FHIR transformation service (monolithic version).
- **Technical**: Loads schemas, applies mappings, validates fields, and returns bundle plus metrics.
- **Logical role**: Core reusable service behind controllers.
- **Business value**: Scalable transformation core for multiple endpoints.
- Function/Method `NewHL7FHIRTransformService`
- Function/Method `loadFHIRSchemasFromGZ`
- Function/Method `loadEssentialSchemasFromLoader`
- Function/Method `createResourceFromSchema`
- Function/Method `initializeRequiredField`
- Function/Method `validateFieldAgainstSchema`
- Function/Method `Transform`
- Function/Method `initializeResponse`
- Function/Method `populateTransformResponse`
- Function/Method `applySchemaBasedTransformations`
- Function/Method `processSchemaBasedMapping`
- Function/Method `setFHIRValueFromSchema`
- Function/Method `hasMeaningfulContent`
- Function/Method `isValueMeaningful`
- Function/Method `getFieldMappings`
- Function/Method `loadFieldMappingsFromDB`
- Function/Method `scanFieldMapping`
- Function/Method `inferTransformFromFHIRPath`
- Function/Method `extractHL7Value`
- Function/Method `buildFieldKey`
- Function/Method `findTargetField`
- Function/Method `extractValueFromField`
- Function/Method `extractComponentFromSubfields`
- Function/Method `manualParseFieldValue`
- Function/Method `transformValue`
- Function/Method `transformMSH9ToCoding`
- Function/Method `transformGender`
- Function/Method `transformPhoneToContactPointEnhanced`
- Function/Method `processXTNField`
- Function/Method `createContactPointFromComponent`
- Function/Method `createSimpleContactPoint`
- Function/Method `transformCXToIdentifier`
- Function/Method `transformXPNToHumanName`
- Function/Method `transformXADToAddress`
- Function/Method `transformEmailToContactPoint`
- Function/Method `transformTSToDate`
- Function/Method `transformCEToCodeableConcept`
- Function/Method `transformSSNToIdentifier`
- Function/Method `transformAccountToIdentifier`
- Function/Method `extractMessageType`
- Function/Method `extractEnhancedSegments`
- Function/Method `createBundle`
- Struct `TransformRequest`
- Struct `TransformResponse`
- Struct `MappingStatistics`
- Struct `PerformanceMetrics`
- Struct `ValidationIssue`
- Struct `FHIRResourceSchema`
- Struct `FHIRFieldDef`
- Struct `FieldMapping`
- Struct `ValueSetMapping`
- Struct `HL7FHIRTransformService`

### C:\Projects\ezHealthKonnect\services\hl7_fhir_transform_service_v2.go
- **Purpose**: Modular evolution of the core transform service.
- **Technical**: Groups mappings by resource and streamlines response building/metrics.
- **Logical role**: Alternative pipeline emphasizing modularity.
- **Business value**: Easier maintenance and targeted optimizations.
- Function/Method `NewHL7FHIRTransformServiceV2`
- Function/Method `Transform`
- Function/Method `performModularTransformation`
- Function/Method `groupMappingsByResourceType`
- Function/Method `getFieldMappings`
- Function/Method `initializeResponse`
- Function/Method `extractMessageType`
- Function/Method `populateTransformResponse`
- Function/Method `createBundle`
- Struct `HL7FHIRTransformServiceV2`

### C:\Projects\ezHealthKonnect\services\hl7_fhir_transform_service_v3.go
- **Purpose**: Atomic-mapping based transformer for complex nested/choice elements.
- **Technical**: Handles choice types, nested arrays, and required-field satisfaction with schema introspection.
- **Logical role**: High-fidelity transformer for demanding resources.
- **Business value**: Better data quality and compliance.
- Function/Method `NewHL7FHIRTransformServiceV3`
- Function/Method `Transform`
- Function/Method `createResourceFromAtomicMappings`
- Function/Method `extractHL7ValueAtomic`
- Function/Method `extractComponentFromHL7Field`
- Function/Method `manualParseComponentValue`
- Function/Method `transformValueAtomic`
- Function/Method `setAtomicFieldInResource`
- Function/Method `findChoiceTypeElement`
- Function/Method `extractChoiceBaseName`
- Function/Method `setNestedFieldFromSchema`
- Function/Method `setNestedArrayField`
- Function/Method `setArrayField`
- Function/Method `normalizeFieldPath`
- Function/Method `getResourceFieldName`
- Function/Method `validateResourceAgainstSchema`
- Function/Method `ensureRequiredFieldsFromSchema`
- Function/Method `isRequiredFieldSatisfied`
- Function/Method `handleRequiredParentField`
- Function/Method `handleRequiredChoiceField`
- Function/Method `handleRequiredChildField`
- Function/Method `nestedFieldExists`
- Function/Method `loadFHIRSchema`
- Function/Method `extractResourceTypes`
- Function/Method `filterMappingsForResource`
- Function/Method `getFieldMappings`
- Function/Method `initializeResponse`
- Function/Method `extractMessageType`
- Function/Method `extractEnhancedSegments`
- Function/Method `populateTransformResponse`
- Function/Method `createBundle`
- Function/Method `transformGender`
- Function/Method `transformPhoneToContactPointEnhanced`
- Function/Method `processXTNField`
- Function/Method `createContactPointFromComponent`
- Function/Method `lookupPhoneUseFromDatabase`
- Function/Method `createSimpleContactPoint`
- Function/Method `transformCXToIdentifier`
- Function/Method `transformXPNToHumanName`
- Function/Method `transformXADToAddress`
- Function/Method `transformEmailToContactPoint`
- Function/Method `transformTSToDate`
- Function/Method `transformCEToCodeableConcept`
- Function/Method `transformSSNToIdentifier`
- Function/Method `transformAccountToIdentifier`
- Function/Method `transformMSH9ToCoding`
- Function/Method `transformTriggerEventToCoding`
- Function/Method `lookupDisplayFromDatabase`
- Function/Method `transformControlIdToReference`
- Function/Method `min`
- Struct `HL7FHIRTransformServiceV3`

### C:\Projects\ezHealthKonnect\services\message_resource_identifier.go
- **Purpose**: Identifies which FHIR resources are needed per HL7 message content.
- **Technical**: Combines templates, overrides, and content checks to produce resource sets.
- **Logical role**: Resource planning before transformation.
- **Business value**: Efficient processing and predictable outputs.
- Function/Method `NewMessageResourceIdentifierService`
- Function/Method `GetResourcesForMessage`
- Function/Method `getOfficialTemplate`
- Function/Method `getInterfaceOverride`
- Function/Method `getBuiltInTemplate`
- Function/Method `filterResourcesByContent`
- Function/Method `segmentsExist`
- Function/Method `evaluateCondition`
- Function/Method `checkField`
- Function/Method `checkSpecimenInOBR`
- Function/Method `checkTaskNeeded`
- Function/Method `checkPractitionerInPRT`
- Function/Method `SaveInterfaceOverride`
- Struct `MessageResourceIdentifierService`
- Struct `FHIRResourceTemplate`
- Struct `ResourceConfig`

### C:\Projects\ezHealthKonnect\services\datatypes\fhir_datatype_utils.go
- **Purpose**: Utilities for transforming HL7 datatypes into FHIR datatypes.
- **Technical**: Normalizes phones/emails, parses components, and builds FHIR datatypes.
- **Logical role**: Shared helpers across transformers.
- **Business value**: Consistent, standards-aligned data.
- Function/Method `NewFHIRDataTypeUtils`
- Function/Method `TransformCXToIdentifier`
- Function/Method `TransformSSNToIdentifier`
- Function/Method `TransformXPNToHumanName`
- Function/Method `TransformXTNToContactPoint`
- Function/Method `processComplexXTNSingle`
- Function/Method `processComplexXTN`
- Function/Method `createContactPointFromComponent`
- Function/Method `createSimpleContactPoint`
- Function/Method `determineContactPointType`
- Function/Method `TransformXADToAddress`
- Function/Method `TransformTSToDate`
- Function/Method `TransformTSToDateTime`
- Function/Method `TransformGender`
- Function/Method `TransformCEToCodeableConcept`
- Function/Method `ParseHL7Component`
- Function/Method `IsValidEmail`
- Function/Method `NormalizePhoneNumber`
- Struct `FHIRDataTypeUtils`

### C:\Projects\ezHealthKonnect\services\mappers\value_mapper.go
- **Purpose**: Value and code mapping helpers (e.g., gender, patient class).
- **Technical**: Lookup and construction of `Coding`/`CodeableConcept` from mapping tables.
- **Logical role**: Centralized mapping logic.
- **Business value**: Simplifies customer-specific adaptations.
- Function/Method `NewValueMapper`
- Function/Method `MapValue`
- Function/Method `MapGender`
- Function/Method `MapPatientClass`
- Function/Method `MapEncounterStatus`
- Function/Method `MapTelecomUse`
- Function/Method `MapAddressUse`
- Function/Method `MapNameUse`
- Function/Method `HasMapping`
- Function/Method `GetAllMappingsForTable`
- Function/Method `CreateCoding`
- Function/Method `CreateCodeableConcept`
- Struct `ValueMapper`

### C:\Projects\ezHealthKonnect\services\schema\fhir_schema_adapter.go
- **Purpose**: Thin adapter around the FHIR schema loader for basic validation.
- **Technical**: Checks required presence (resourceType, narrative) and returns issues.
- **Logical role**: Lightweight guard when full validation is unnecessary.
- **Business value**: Faster feedback loops during development.
- Function/Method `NewFHIRSchemaAdapter`
- Function/Method `ValidateResource`
- Function/Method `IsSchemaAvailable`
- Function/Method `GetSchemaDirectory`
- Struct `FHIRSchemaAdapter`
- Struct `ValidationIssue`

### C:\Projects\ezHealthKonnect\services\transformers\generic_resource_transformer.go
- **Purpose**: Generic transformer applying field mappings to create FHIR resources.
- **Technical**: Sets values, handles arrays, generates narrative, and common transforms.
- **Logical role**: Building block reused by controllers/services.
- **Business value**: Reduces implementation time across use cases.
- Function/Method `NewGenericResourceTransformer`
- Function/Method `CreateResource`
- Function/Method `createBaseResource`
- Function/Method `applyFieldMappings`
- Function/Method `applyFieldMapping`
- Function/Method `transformValue`
- Function/Method `transformMSH9ToCoding`
- Function/Method `addGenericNarrativeText`
- Function/Method `generateMessageHeaderNarrative`
- Function/Method `generatePatientNarrative`
- Function/Method `generateEncounterNarrative`
- Function/Method `extractHL7Value`
- Function/Method `extractComponentValue`
- Function/Method `setFHIRValue`
- Function/Method `addToArrayFieldSmart`
- Function/Method `extractMSH9Component`
- Struct `FieldMapping`
- Struct `GenericResourceTransformer`

### C:\Projects\ezHealthKonnect\tests\debug_current_transformation.go
- Function/Method `main`
- Function/Method `debugDataStructure`
- Function/Method `debugMessageTypeExtraction`
- Function/Method `debugFieldExtraction`
- Function/Method `extractHL7ValueDebug`
- Function/Method `debugTransformations`
- Function/Method `debugDatabaseMappings`
- Function/Method `showExpectedOutput`
- Function/Method `showActionItems`

### C:\Projects\ezHealthKonnect\tests\test_enhanced_service.go
- Function/Method `main`
- Function/Method `testTransformations`
- Function/Method `simulateTransformation`
- Struct `TransformRequest`
- Struct `TransformResponse`
- Struct `MappingStatistics`
- Struct `PerformanceMetrics`

### C:\Projects\ezHealthKonnect\tests\test_hl7_fhir_transformation.go
- Function/Method `main`
- Function/Method `printExpectedOutput`
- Struct `TransformRequest`
- Struct `TransformResponse`
- Struct `MappingStatistics`
- Struct `PerformanceMetrics`
- Struct `ValidationIssue`
