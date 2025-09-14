# Comprehensive Project Analysis
**Generated:** 2025-09-08T13:39:50.009815
**Project Root:** C:\Projects\ezHealthKonnect
**Total Files Analyzed:** 3,524 (11,476,685 total lines)

## File Statistics by Category

| Category | Files | Lines | Avg Lines/File | Critical Files |
|----------|-------|-------|----------------|----------------|
| Javascript | 67 | 31,665 | 472 | 61 |
| Python | 4 | 1,193 | 298 | 0 |
| Go | 30 | 17,448 | 581 | 25 |
| Html | 5 | 1,825 | 365 | 5 |
| Css | 11 | 7,807 | 709 | 11 |
| Sql | 13 | 3,228 | 248 | 13 |
| Config | 22 | 14,978 | 680 | 0 |
| Markdown | 4 | 4,136 | 1034 | 0 |
| Shell | 6 | 1,505 | 250 | 0 |
| Docker | 1 | 25 | 25 | 0 |
| Other | 6 | 498 | 83 | 0 |
| Uncategorized | 3355 | 11,392,377 | 3395 | 3351 |

---

## Directory-Specific Analysis

### 📂 config/ Directory
**Files:** 4 | **Total Lines:** 997 | **Types:** go, javascript
- `config\database.js` (590 lines)
- `config\config.go` (379 lines)
- `config\session.js` (15 lines)
- `config\environment.js` (13 lines)

### 📂 controllers/ Directory
**Files:** 12 | **Total Lines:** 7,331 | **Types:** go, javascript
- `controllers\fhir_transform_controller.go` (1431 lines)
- `controllers\wizardController.js` (1046 lines)
- `controllers\hl7_fhir_transformation_controller.go` (830 lines)
- `controllers\schema_fhir_transform_controller.go` (808 lines)
- `controllers\interfacesController.js` (796 lines)
- `controllers\WizardMappingController.js` (632 lines)
- `controllers\wizard_api_controller.go` (624 lines)
- `controllers\hl7_controller.go` (353 lines)
- ... and 4 more files

### 📂 database/ Directory
**Files:** 11 | **Total Lines:** 2,981 | **Types:** sql
- `database\init\hl7_to_fhir_mapping.sql` (743 lines)
- `database\backups\backup_20250826_002704.sql` (698 lines)
- `database\backups\backup_20250826_001828.sql` (676 lines)
- `database\migrations\V5__Add_Missing_FHIR_Tables.sql` (263 lines)
- `database\migrations\V1__schema_only.sql` (178 lines)
- `database\backups\backup_20250826_001652.sql` (146 lines)
- `database\init\01-initalize database.sql` (130 lines)
- `database\migrations\V6__Correct_Field_Mapping_Format.sql` (66 lines)
- ... and 3 more files

### 📂 fhir/ Directory
**Files:** 4 | **Total Lines:** 2,138 | **Types:** go
- `fhir\value_transformers.go` (614 lines)
- `fhir\validation_bundle.go` (547 lines)
- `fhir\transformation_engine.go` (504 lines)
- `fhir\schema_loader.go` (473 lines)

### 📂 hl7/ Directory
**Files:** 4 | **Total Lines:** 2,750 | **Types:** go
- `hl7\real_schema_parser.go` (1160 lines)
- `hl7\parser.go` (713 lines)
- `hl7\unified_parser.go` (519 lines)
- `hl7\types.go` (358 lines)

### 📂 middleware/ Directory
**Files:** 1 | **Total Lines:** 41 | **Types:** javascript
- `middleware\auth.js` (41 lines)

### 📂 migrations/ Directory
**Files:** 3 | **Total Lines:** 565 | **Types:** javascript, sql
- `migrations\mongodb\init_collections.js` (318 lines)
- `migrations\postgres\001_create_fhir_tables.sql` (163 lines)
- `migrations\postgres\002_create_indexes.sql` (84 lines)

### 📂 public/ Directory
**Files:** 60 | **Total Lines:** 34,300 | **Types:** css, html, javascript, uncategorized
- `public\js\wizard\step4-integration.js` (3101 lines)
- `public\js\interfaces.js` (1543 lines)
- `public\css\segment-viewer.css` (1291 lines)
- `public\js\wizard\segment-viewer.js` (1188 lines)
- `public\js\wizard\step-handlers.js` (1165 lines)
- `public\js\modules\hl7-schemas.js` (1163 lines)
- `public\css\interfaces.css` (1147 lines)
- `public\js\modules\validation-ui.js` (1082 lines)
- ... and 52 more files

### 📂 routes/ Directory
**Files:** 5 | **Total Lines:** 581 | **Types:** javascript
- `routes\users.js` (261 lines)
- `routes\auth.js` (150 lines)
- `routes\wizardRoutes.js` (122 lines)
- `routes\interfacesRoutes.js` (32 lines)
- `routes\index.js` (16 lines)

### 📂 schemas/ Directory
**Files:** 3349 | **Total Lines:** 11,391,675 | **Types:** uncategorized
- `schemas\hl7\v2.8\CCR_I16.gz` (40121 lines)
- `schemas\hl7\v2.8\CCR_I18.gz` (40103 lines)
- `schemas\hl7\v2.8\CCR_I17.gz` (40085 lines)
- `schemas\hl7\v2.8\CQU_I19.gz` (36386 lines)
- `schemas\hl7\v2.8\CCU_I20.gz` (35563 lines)
- `schemas\hl7\v2.8\CCM_I21.gz` (35000 lines)
- `schemas\hl7\v2.7.1\CQU_I19.gz` (31345 lines)
- `schemas\hl7\v2.8\CCI_I22.gz` (30090 lines)
- ... and 3341 more files

### 📂 services/ Directory
**Files:** 13 | **Total Lines:** 7,054 | **Types:** go, javascript
- `services\hl7_fhir_transform_service_v3.go` (1616 lines)
- `services\hl7_fhir_transform_service.go` (1332 lines)
- `services\WizardMappingService.js` (656 lines)
- `services\wizardConfigService.js` (585 lines)
- `services\transformers\generic_resource_transformer.go` (521 lines)
- `services\datatypes\fhir_datatype_utils.go` (520 lines)
- `services\message_resource_identifier.go` (375 lines)
- `services\hl7_fhir_transform_service_v2.go` (349 lines)
- ... and 5 more files

**Summary:** 92 functions, 49 classes, 0 routes across 67 files

## JavaScript/TypeScript Files (67 files)

### 🔥 public\js\wizard\step4-integration.js
**Type:** JavaScript | **Lines:** 3101 | **Directory:** public\js\wizard
**Classes:** `FHIRMappingStepHandler, globally, is, BaseStepHandler`
**Functions:** `FHIRMappingStepHandler`

### 🔥 public\js\interfaces.js
**Type:** JavaScript | **Lines:** 1543 | **Directory:** public\js
**Functions:** `setupTooltips, calculatePagination, setupSidebarTooltips, performAutoRefresh, loadUserInfo, loadInterfaces, closeCreateModal, initializeInterfacesPage, moveTooltip, injectTooltipStyles, setupAutoRefresh, initializeSidebarTooltips`

### 🔥 public\js\wizard\segment-viewer.js
**Type:** JavaScript | **Lines:** 1188 | **Directory:** public\js\wizard
**Classes:** `SegmentViewer, is`

### 🔥 public\js\wizard\step-handlers.js
**Type:** JavaScript | **Lines:** 1165 | **Directory:** public\js\wizard
**Classes:** `BaseStepHandler, ReviewStepHandler, ConfigurationStepHandler, MappingStepHandler, UploadStepHandler, SummaryStepHandler`
**Functions:** `onload, onerror, onchange, onclick, validateStep1Fallback`

### 🔥 public\js\modules\hl7-schemas.js
**Type:** JavaScript | **Lines:** 1163 | **Directory:** public\js\modules
**Classes:** `HL7Schemas`

### 🔥 public\js\modules\validation-ui.js
**Type:** JavaScript | **Lines:** 1082 | **Directory:** public\js\modules
**Classes:** `ValidationUI`

### 🔥 public\js\step4\step4-handler.js
**Type:** JavaScript | **Lines:** 1075 | **Directory:** public\js\step4
**Classes:** `FHIRMappingStepHandler, globally, is`
**Functions:** `onclick, debugEnhancedMapping`

### 🔥 controllers\wizardController.js
**Type:** JavaScript | **Lines:** 1046 | **Directory:** controllers
**Classes:** `WizardController`
**Key Imports:** `uuid`

### 🔥 public\js\core\wizard-navigation.js
**Type:** JavaScript | **Lines:** 884 | **Directory:** public\js\core
**Classes:** `WizardNavigation`
**Functions:** `setupModalButtons, initWizardNavigation, setupListeners`

### 🔥 public\js\wizard\wizard-main.js
**Type:** JavaScript | **Lines:** 874 | **Directory:** public\js\wizard
**Classes:** `InterfaceWizardModal, to, from, FallbackStep4Handler, available, redeclaration`
**Functions:** `closeWizardModal, setupButtons, onload, openInterfaceWizard, onerror, show, initializeWizard`

### 🔥 public\js\wizard\wizard-config-integration.js
**Type:** JavaScript | **Lines:** 845 | **Directory:** public\js\wizard
**Classes:** `InterfaceConfigManager, EnhancedStep4Handler, EnhancedWizardMain`
**Functions:** `finishWizard, initializeWizardEnhancement, debugWizardClose, onStepComplete`

### 🔥 controllers\interfacesController.js
**Type:** JavaScript | **Lines:** 796 | **Directory:** controllers
**Classes:** `InterfacesController`

### 🔥 public\js\modules\healthcare-rules.js
**Type:** JavaScript | **Lines:** 771 | **Directory:** public\js\modules
**Classes:** `HealthcareRules, with, vs, consistency, but, indicates`

### 🔥 public\js\modules\validation-integration.js
**Type:** JavaScript | **Lines:** 739 | **Directory:** public\js\modules
**Classes:** `ValidationIntegration`
**Functions:** `addItems, action`

### 🔥 public\js\step4\step4-styles.js
**Type:** JavaScript | **Lines:** 737 | **Directory:** public\js\step4
**Classes:** `Step4Styles`

### 🔥 public\js\modules\step4-wizard-handler.js
**Type:** JavaScript | **Lines:** 719 | **Directory:** public\js\modules
**Classes:** `FHIRMappingStepHandler`
**Functions:** `showResourceDetails, updateStep4Interface`

### 🔥 public\js\step4\enhanced-mapping-interface.js
**Type:** JavaScript | **Lines:** 701 | **Directory:** public\js\step4
**Classes:** `if, EnhancedMappingInterface`
**Functions:** `testEnhancedMapping`

### 🔥 services\WizardMappingService.js
**Type:** JavaScript | **Lines:** 656 | **Directory:** services
**Classes:** `WizardMappingService`
**Key Imports:** `pg`

### 🔥 controllers\WizardMappingController.js
**Type:** JavaScript | **Lines:** 632 | **Directory:** controllers
**Classes:** `WizardMappingController`
**Key Imports:** `joi`

### 🔥 public\js\components\wizard-component.js
**Type:** JavaScript | **Lines:** 622 | **Directory:** public\js\components
**Functions:** `loadWizardComponent, setupWizardModalEvents, setupConnectivityHandlers`

### 🔥 config\database.js
**Type:** JavaScript | **Lines:** 590 | **Directory:** config
**Classes:** `DatabaseManager`
**Functions:** `findByEmail, logAction, getFullName`
**Key Imports:** `bcryptjs, dotenv, sequelize`

### 🔥 services\wizardConfigService.js
**Type:** JavaScript | **Lines:** 585 | **Directory:** services
**Classes:** `WizardConfigService`

### 🔥 public\js\wizard-config-integration.js
**Type:** JavaScript | **Lines:** 576 | **Directory:** public\js
**Classes:** `InterfaceConfigManager, EnhancedStep4Handler, EnhancedWizardMain`
**Functions:** `finishWizard, initializeWizardEnhancement, onStepComplete`

### 🔥 public\js\modules\hl7-validator.js
**Type:** JavaScript | **Lines:** 556 | **Directory:** public\js\modules
**Classes:** `HL7Validator`
**Functions:** `indexField`

### 🔥 public\js\wizard-connectivity-enhancements.js
**Type:** JavaScript | **Lines:** 467 | **Directory:** public\js
**Classes:** `WizardConnectivityManager`
**Functions:** `resetWizard, initializeConnectivityManager`

**Summary:** 31 functions, 0 classes across 4 files

## Python Files (4 files)

### context_manager.py
**Lines:** 806 | **Directory:** .
**Functions:** `schema_summary`(pg_conn_str), `generate_comprehensive_directory_tree`(root_path), `should_skip_dir`(d), `discover_all_files`(root), `analyze_javascript_typescript_files`(js_files, ts_files), `analyze_html_template_files`(files), `analyze_python_files`(files), `analyze_go_files`(files), `analyze_config_sql_files`(config_files, sql_files), `analyze_directory_specific_patterns`(discovered_files)
**Key Imports:** `psycopg2, typing, pathlib, ast`

### tests\test_hl7_fhir.py
**Lines:** 197 | **Directory:** tests
**Functions:** `load_json_file`(file_path), `check_service_status`(base_url), `test_transformation`(base_url, parsed_hl7_data), `display_results`(result), `save_results`(result, filename), `main`()
**Key Imports:** `requests`

### mcp_generator.py
**Lines:** 166 | **Directory:** .
**Functions:** `is_valid_file`(file_path), `is_excluded`(path), `hash_file`(filepath), `extract_todos`(filepath), `summarize_file`(filepath), `archive_existing_files`(), `find_latest_archive`(), `load_json`(path), `get_changed_files`(current, previous), `extract_summaries`(index, changed_files)
**Key Imports:** `shutil, pathlib, hashlib`

**Summary:** 598 functions/methods, 60 types across 30 files

## Go Files (30 files)

### 🔥 services\hl7_fhir_transform_service_v3.go
**Package:** services | **Lines:** 1616 | **Directory:** services
**Structs:** `HL7FHIRTransformServiceV3`
**Functions:** `NewHL7FHIRTransformServiceV3, Transform, createResourceFromAtomicMappings, extractHL7ValueAtomic, extractComponentFromHL7Field, manualParseComponentValue, transformValueAtomic, setAtomicFieldInResource`
**Methods:** `Transform, createResourceFromAtomicMappings, extractHL7ValueAtomic, extractComponentFromHL7Field, manualParseComponentValue, transformValueAtomic, setAtomicFieldInResource, findChoiceTypeElement`

### 🔥 controllers\fhir_transform_controller.go
**Package:** controllers | **Lines:** 1431 | **Directory:** controllers
**Structs:** `FHIRTransformController, TransformRequest, TransformResponse, TransformationRule, RuleManagementRequest`
**Functions:** `NewFHIRTransformController, RegisterRoutes, GetStatus, Transform, GetRules, CreateRule, UpdateRule, DeleteRule`
**Methods:** `RegisterRoutes, GetStatus, Transform, GetRules, CreateRule, UpdateRule, DeleteRule, GetSchemas`

### 🔥 services\hl7_fhir_transform_service.go
**Package:** services | **Lines:** 1332 | **Directory:** services
**Structs:** `TransformRequest, TransformResponse, MappingStatistics, PerformanceMetrics, ValidationIssue`
**Functions:** `NewHL7FHIRTransformService, loadFHIRSchemasFromGZ, loadEssentialSchemasFromLoader, createResourceFromSchema, initializeRequiredField, validateFieldAgainstSchema, Transform, initializeResponse`
**Methods:** `loadFHIRSchemasFromGZ, loadEssentialSchemasFromLoader, createResourceFromSchema, initializeRequiredField, validateFieldAgainstSchema, Transform, initializeResponse, populateTransformResponse`

### 🔥 hl7\real_schema_parser.go
**Package:** hl7 | **Lines:** 1160 | **Directory:** hl7
**Structs:** `RealHL7Schema, RealSegmentDef, RealFieldDef, RealComponentDef, RealSchemaLoader`
**Functions:** `InitRealSchemaLoader, GetRealSchemaLoader, scanForSchemaFiles, createSampleSchema, LoadRealSchema, normalizeVersionForSchema, loadAndParseSchemaFile, convertOrderedRawSchema`
**Methods:** `LoadRealSchema, loadAndParseSchemaFile, convertOrderedRawSchema, convertOrderedSegment, convertOrderedField, convertComponent, GetStats`

### 🔥 controllers\hl7_fhir_transformation_controller.go
**Package:** controllers | **Lines:** 830 | **Directory:** controllers
**Structs:** `HL7FHIRTransformationController`
**Functions:** `NewHL7FHIRTransformationController, RegisterRoutes, TransformHL7ToFHIR, GetTransformationStatus, TestTransformation, ValidateTransformation, GetMessageTemplates, GetFieldMappings`
**Methods:** `RegisterRoutes, TransformHL7ToFHIR, GetTransformationStatus, TestTransformation, ValidateTransformation, GetMessageTemplates, GetFieldMappings, GetValueSetMappings`

### 🔥 controllers\schema_fhir_transform_controller.go
**Package:** controllers | **Lines:** 808 | **Directory:** controllers
**Structs:** `SchemaFHIRTransformController, MappingRule, HL7FieldStructure, HL7FieldDefinition, HL7ComponentDefinition`
**Functions:** `NewSchemaFHIRTransformController, RegisterRoutes, GetStatus, ListSchemas, Transform, ValidateOnly, GetRules, CreateRule`
**Methods:** `RegisterRoutes, GetStatus, ListSchemas, Transform, ValidateOnly, GetRules, CreateRule, UpdateRule`

### 🔥 hl7\parser.go
**Package:** hl7 | **Lines:** 713 | **Directory:** hl7
**Structs:** `EnhancedFieldData, FieldRepetition, FieldComponent`
**Functions:** `ParseHL7MessageEnhanced, ParseHL7Field, ConvertBasicToEnhancedWithDelimiters, convertEnhancedFieldToSubfields, DemonstrateDelimiterParsing, extractFieldPosition, TestSpecificExample, extractMessageType`

### 🔥 controllers\wizard_api_controller.go
**Package:** controllers | **Lines:** 624 | **Directory:** controllers
**Structs:** `WizardController, WizardParseRequest, WizardParseResponse, WizardParsedData, WizardMetadata`
**Functions:** `NewWizardController, toWizardView, fromWizardView, getHL7FieldName, RegisterRoutes, ParseHL7, GetMappingRules, SaveMappingRules`
**Methods:** `toWizardView, fromWizardView, getHL7FieldName, RegisterRoutes, ParseHL7, GetMappingRules, SaveMappingRules, TestTransformation`

### 🔥 fhir\value_transformers.go
**Package:** fhir | **Lines:** 614 | **Directory:** fhir
**Functions:** `extractHL7Value, transformValue, transformToFHIRIdentifier, transformToFHIRHumanName, transformToFHIRAddress, transformToFHIRContactPoint, transformToFHIRDate, transformToFHIRDateTime`
**Methods:** `extractHL7Value, transformValue, transformToFHIRIdentifier, transformToFHIRHumanName, transformToFHIRAddress, transformToFHIRContactPoint, transformToFHIRDate, transformToFHIRDateTime`

### 🔥 fhir\validation_bundle.go
**Package:** fhir | **Lines:** 547 | **Directory:** fhir
**Functions:** `validateResourcesAgainstSchemas, validateResourceAgainstSchema, validateRequiredElement, validateMustSupportElement, validateFieldAgainstElement, validateCardinality, validateDataType, validateConstraints`
**Methods:** `validateResourcesAgainstSchemas, validateResourceAgainstSchema, validateRequiredElement, validateMustSupportElement, validateFieldAgainstElement, validateCardinality, validateDataType, validateConstraints`

### 🔥 services\transformers\generic_resource_transformer.go
**Package:** transformers | **Lines:** 521 | **Directory:** services\transformers
**Structs:** `FieldMapping, GenericResourceTransformer`
**Functions:** `NewGenericResourceTransformer, CreateResource, createBaseResource, applyFieldMappings, applyFieldMapping, transformValue, transformMSH9ToCoding, addGenericNarrativeText`
**Methods:** `CreateResource, createBaseResource, applyFieldMappings, applyFieldMapping, transformValue, transformMSH9ToCoding, addGenericNarrativeText, generateMessageHeaderNarrative`

### 🔥 services\datatypes\fhir_datatype_utils.go
**Package:** datatypes | **Lines:** 520 | **Directory:** services\datatypes
**Structs:** `FHIRDataTypeUtils`
**Functions:** `NewFHIRDataTypeUtils, TransformCXToIdentifier, TransformSSNToIdentifier, TransformXPNToHumanName, TransformXTNToContactPoint, processComplexXTNSingle, processComplexXTN, createContactPointFromComponent`
**Methods:** `TransformCXToIdentifier, TransformSSNToIdentifier, TransformXPNToHumanName, TransformXTNToContactPoint, processComplexXTNSingle, processComplexXTN, createContactPointFromComponent, createSimpleContactPoint`

### 🔥 hl7\unified_parser.go
**Package:** hl7 | **Lines:** 519 | **Directory:** hl7
**Structs:** `SchemaLoader`
**Functions:** `InitSchemaLoader, GetSchemaLoader, SetMaxCacheSize, ClearCache, GetCacheStats, ListAvailableSchemas, ParseHL7Enhanced, convertBasicToMapFormatFixed`
**Methods:** `SetMaxCacheSize, ClearCache, GetCacheStats, ListAvailableSchemas`

### 🔥 fhir\transformation_engine.go
**Package:** fhir | **Lines:** 504 | **Directory:** fhir
**Structs:** `TransformationEngine, TransformationConfig, TransformationRequest, TransformationResponse, ValidationError`
**Functions:** `NewTransformationEngine, Transform, loadRequiredSchemas, applyTransformationRules, initializeResourceFromSchema, getDefaultValueForDataType, extractMessageType, loadMappingRules`
**Methods:** `Transform, loadRequiredSchemas, applyTransformationRules, initializeResourceFromSchema, getDefaultValueForDataType, extractMessageType, loadMappingRules, errorResponse`

### 🔥 fhir\schema_loader.go
**Package:** fhir | **Lines:** 473 | **Directory:** fhir
**Structs:** `FHIRSchema, FHIRElement, FHIRSchemaStats, FHIRSchemaLoader`
**Functions:** `InitFHIRSchemaLoader, scanVersionAwareFHIRSchemas, GetFHIRSchemaLoader, LoadFHIRSchema, resolveSchemaPath, tryAlternativePaths, loadAndParseFHIRSchema, validateFHIRSchema`
**Methods:** `LoadFHIRSchema, resolveSchemaPath, tryAlternativePaths, loadAndParseFHIRSchema, validateFHIRSchema, GetStats, ClearCache, ListAvailableSchemas`

## HTML/Template Files (5 files)

### 🔥 public\interface-wizard.html
**Lines:** 960 | **Directory:** public
**Input Types:** `text`
**Template Variables:** `i, stepTitles[currentStep - 1], step, currentStep, totalSteps`

### 🔥 public\dashboard.html
**Lines:** 263 | **Directory:** public
**Scripts:** `dashboard.js`
**Stylesheets:** `dashboard.css`

### 🔥 public\user-management.html
**Lines:** 260 | **Directory:** public
**Forms:** 2
**Input Types:** `text, password, email, hidden`
**Scripts:** `user-management.js`
**Stylesheets:** `dashboard.css, user-management.css`

### 🔥 public\interfaces.html
**Lines:** 203 | **Directory:** public
**Scripts:** `modal-components.js, step5-summary-fix.js, wizard-functions.js, wizard-navigation.js, wizard-modal-close-fix.js`
**Stylesheets:** `interfaces.css, interface-wizard.css, dashboard.css, interface-cards.css, wizard-modal.css`

### 🔥 public\login.html
**Lines:** 139 | **Directory:** public
**Forms:** 1
**Input Types:** `checkbox, password, email`
**Scripts:** `login.js`
**Stylesheets:** `style.css`

## Configuration & SQL Files (35 files)

### package-lock.json
**Lines:** 4386 | **Size:** 162465 bytes | **Directory:** .
**JSON Keys:** `name, version, lockfileVersion, requires, packages`

### parsedhl7.json
**Lines:** 2603 | **Size:** 146926 bytes | **Directory:** .
_JSON parsing failed_

### tests\parsedhl7.json
**Lines:** 2599 | **Size:** 146844 bytes | **Directory:** tests
_JSON parsing failed_

### mcp_output\archives\code_index_2025-07-25_17-14-40.json
**Lines:** 981 | **Size:** 46340 bytes | **Directory:** mcp_output\archives

### mcp_output\code_index.json
**Lines:** 968 | **Size:** 45932 bytes | **Directory:** mcp_output

### mcp_output\archives\code_index_2025-07-23_23-48-43.json
**Lines:** 875 | **Size:** 42388 bytes | **Directory:** mcp_output\archives

### node-api\package-lock.json
**Lines:** 868 | **Size:** 31661 bytes | **Directory:** node-api
**JSON Keys:** `name, version, lockfileVersion, requires, packages`

### 🔥 database\init\hl7_to_fhir_mapping.sql
**Lines:** 743 | **Size:** 50724 bytes | **Directory:** database\init
### 🔥 database\backups\backup_20250826_002704.sql
**Lines:** 698 | **Size:** 20397 bytes | **Directory:** database\backups
### 🔥 database\backups\backup_20250826_001828.sql
**Lines:** 676 | **Size:** 18515 bytes | **Directory:** database\backups
### 🔥 database\migrations\V5__Add_Missing_FHIR_Tables.sql
**Lines:** 263 | **Size:** 11403 bytes | **Directory:** database\migrations
### mcp_output\archives\changed_files_2025-07-25_17-14-40.json
**Lines:** 258 | **Size:** 8792 bytes | **Directory:** mcp_output\archives

### mcp_output\changed_files.json
**Lines:** 254 | **Size:** 8653 bytes | **Directory:** mcp_output

### mcp_output\archives\changed_files_2025-07-23_23-48-43.json
**Lines:** 226 | **Size:** 7613 bytes | **Directory:** mcp_output\archives

### 🔥 database\migrations\V1__schema_only.sql
**Lines:** 178 | **Size:** 6369 bytes | **Directory:** database\migrations
## Uncategorized Files (3355 files)

- `ezHealthKonnect structure.docx` (120 lines)
- `go.mod` (45 lines)
- `go.sum` (105 lines)
- `rm` (0 lines)
- `public\assets\logos\ezHealthKonnect.jpeg` (321 lines)
- `public\assets\logos\ezInteropSolutions.jpeg` (111 lines)
- `schemas\fhir\R4\profiles\us-core\AllergyIntolerance.gz` (29 lines)
- `schemas\fhir\R4\profiles\us-core\CarePlan.gz` (58 lines)
- `schemas\fhir\R4\profiles\us-core\CareTeam.gz` (33 lines)
- `schemas\fhir\R4\profiles\us-core\Condition.gz` (33 lines)
- ... and 3345 more files
