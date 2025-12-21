# Documentation and Test File Cleanup Plan

## Current State
- **127 markdown files** in root directory
- **19 test files** and scripts in root directory
- Most are outdated implementation logs, debug guides, and interim status reports

## Cleanup Strategy

### KEEP (Essential Documentation - 12 files)

#### Primary References
1. **CLAUDE.md** - AI assistant project guide (KEEP)
2. **SYSTEM_DOCUMENTATION.md** - Complete consolidated reference (KEEP)
3. **STEP_OUTPUT_REFERENCE_GUIDE.md** - Step output usage guide (KEEP - just created)

#### Architecture Documents
4. **ARCHITECTURE_REFERENCE.md** - Design patterns and principles (KEEP)
5. **INTERFACE_CONFIGURATION_ENGINE.md** - Configuration engine design (KEEP)
6. **TRANSFORMATION_PIPELINE_DESIGN.md** - Pipeline architecture (KEEP)
7. **HYBRID_STORAGE_ARCHITECTURE.md** - PostgreSQL + MongoDB design (KEEP)
8. **SCALABILITY_AND_GUI_DESIGN.md** - Scale + UI architecture (KEEP)
9. **JSON_CONVERSION_ARCHITECTURE.md** - JSON conversion pipeline (KEEP)

#### Connectivity Documentation
10. **CONNECTIVITY_CATALOG.md** - 32 connector catalog (KEEP)
11. **CONNECTOR_IMPLEMENTATION_GUIDE.md** - Connector development guide (KEEP)
12. **CONNECTIVITY_ARCHITECTURE.md** - Connectivity system architecture (KEEP)
13. **CONNECTIVITY_CLOUD_AND_SECURITY.md** - Cloud integration and security (KEEP)
14. **CONNECTIVITY_PATTERNS.md** - Integration pattern explanations (KEEP)

#### Integration Guides
15. **INTEGRATION_GUIDE.md** - User integration guide (KEEP)

**Total to KEEP: 15 files**

### ARCHIVE (Historical/Debug - Move to docs/archive/)

#### Implementation Logs (completed work)
- STEP_OUTPUT_IMPLEMENTATION_SUMMARY.md (move to docs/archive/)
- All files with "_COMPLETE", "_FIX", "_IMPLEMENTATION", "_LOG", "_STATUS" suffixes
- Debug guides (DEBUG_*, XPATH_DEBUG_GUIDE.md, etc.)
- Historical analysis docs (GAP_ANALYSIS, ALIGNMENT_SUMMARY, etc.)

#### Interim Status Reports
- PHASE_*_PROGRESS.md files
- *_STATUS.md files
- VERIFICATION_CHECKLIST.md
- DEPLOYMENT_COMPLETE.md

#### Specific Feature Logs
- All FHIR_RECEIVER_* files (feature complete, docs in SYSTEM_DOCUMENTATION.md)
- All PIPELINE_BUILDER_* files (feature complete)
- All VALIDATION_* files (feature complete)
- All XPATH_* files (feature complete)
- All FIELD_PATH_* files (feature complete)

**Total to ARCHIVE: ~100 files**

### DELETE (Redundant/Obsolete)

#### Duplicate/Redundant
- MICROSERVICE_ARCHITECTURE.md (covered in ARCHITECTURE_REFERENCE.md)
- UNIVERSAL_PIPELINE_ARCHITECTURE.md (covered in TRANSFORMATION_PIPELINE_DESIGN.md)
- PIPELINE_BUILDER_QUICKSTART.md (user guide moved to SYSTEM_DOCUMENTATION.md)
- HOW_TO_CONFIGURE_VALIDATION_MODE_UI.md (UI docs in SYSTEM_DOCUMENTATION.md)

#### Obsolete
- NEXT_STEPS_MONGODB_INTEGRATION.md (MongoDB integrated)
- UNIFIED_INTEGRATION_PLAN.md (completed, status in SYSTEM_DOCUMENTATION.md)
- QUICK_WINS_* (temporary analysis docs)
- CACHE_REFRESH_GUIDE.md (covered in system docs)

**Total to DELETE: ~12 files**

### Test Files - KEEP vs CLEAN

#### KEEP (Active Tests)
- test-pipeline-api.sh (active test script)
- test_hl7_send.js (active integration test)
- test_tcp_message.js (active TCP test)
- setup.sh (environment setup)
- start.sh (service startup)

#### MOVE to tests/ directory
- test_*.js files (17 files) → move to tests/integration/
- debug.sh → move to tests/scripts/

#### KEEP in root
- TestPipelineoutput.json (recent test output - can be gitignored)

## Proposed Structure After Cleanup

```
ezHealthKonnect/
├── CLAUDE.md                                    (AI guide)
├── SYSTEM_DOCUMENTATION.md                      (Complete reference)
├── STEP_OUTPUT_REFERENCE_GUIDE.md               (Step output usage)
│
├── architecture/
│   ├── ARCHITECTURE_REFERENCE.md                (Design patterns)
│   ├── INTERFACE_CONFIGURATION_ENGINE.md        (Config engine)
│   ├── TRANSFORMATION_PIPELINE_DESIGN.md        (Pipeline architecture)
│   ├── HYBRID_STORAGE_ARCHITECTURE.md           (Storage design)
│   ├── SCALABILITY_AND_GUI_DESIGN.md            (Scale + UI)
│   └── JSON_CONVERSION_ARCHITECTURE.md          (JSON conversion)
│
├── connectivity/
│   ├── CONNECTIVITY_CATALOG.md                  (32 connectors)
│   ├── CONNECTOR_IMPLEMENTATION_GUIDE.md        (Dev guide)
│   ├── CONNECTIVITY_ARCHITECTURE.md             (System architecture)
│   ├── CONNECTIVITY_CLOUD_AND_SECURITY.md       (Cloud + security)
│   └── CONNECTIVITY_PATTERNS.md                 (Integration patterns)
│
├── INTEGRATION_GUIDE.md                         (User guide)
│
├── tests/
│   ├── integration/
│   │   ├── test_*.js (17 files)
│   │   └── test-pipeline-api.sh
│   └── scripts/
│       ├── debug.sh
│       ├── setup.sh
│       └── start.sh
│
└── docs/
    └── archive/
        └── [~100 historical/debug/implementation log files]
```

## Execution Plan

### Phase 1: Create Directory Structure
```bash
mkdir -p architecture
mkdir -p connectivity
mkdir -p tests/integration
mkdir -p tests/scripts
mkdir -p docs/archive
```

### Phase 2: Move Architecture Docs
```bash
mv ARCHITECTURE_REFERENCE.md architecture/
mv INTERFACE_CONFIGURATION_ENGINE.md architecture/
mv TRANSFORMATION_PIPELINE_DESIGN.md architecture/
mv HYBRID_STORAGE_ARCHITECTURE.md architecture/
mv SCALABILITY_AND_GUI_DESIGN.md architecture/
mv JSON_CONVERSION_ARCHITECTURE.md architecture/
```

### Phase 3: Move Connectivity Docs
```bash
mv CONNECTIVITY_CATALOG.md connectivity/
mv CONNECTOR_IMPLEMENTATION_GUIDE.md connectivity/
mv CONNECTIVITY_ARCHITECTURE.md connectivity/
mv CONNECTIVITY_CLOUD_AND_SECURITY.md connectivity/
mv CONNECTIVITY_PATTERNS.md connectivity/
```

### Phase 4: Move Test Files
```bash
mv test_*.js tests/integration/
mv test-pipeline-api.sh tests/integration/
mv debug.sh tests/scripts/
mv setup.sh tests/scripts/
mv start.sh tests/scripts/
```

### Phase 5: Archive Historical Docs
```bash
# Archive all implementation logs, debug guides, status reports
mv *_COMPLETE.md docs/archive/
mv *_FIX.md docs/archive/
mv *_IMPLEMENTATION.md docs/archive/
mv *_LOG.md docs/archive/
mv *_STATUS.md docs/archive/
mv DEBUG_*.md docs/archive/
mv PHASE_*.md docs/archive/
# ... (full list in execution script)
```

### Phase 6: Delete Redundant Files
```bash
rm MICROSERVICE_ARCHITECTURE.md
rm UNIVERSAL_PIPELINE_ARCHITECTURE.md
rm PIPELINE_BUILDER_QUICKSTART.md
rm HOW_TO_CONFIGURE_VALIDATION_MODE_UI.md
# ... (full list in execution script)
```

### Phase 7: Update CLAUDE.md References
Update all file path references in CLAUDE.md to point to new locations.

## Benefits

1. **Clarity**: 15 essential docs in organized structure vs 127 files in root
2. **Discoverability**: Grouped by topic (architecture, connectivity, tests)
3. **Maintainability**: Clear separation of active vs archived docs
4. **History Preserved**: Nothing deleted permanently, just archived
5. **Git Friendliness**: Cleaner root directory, easier to navigate

## Risk Mitigation

- All moves are reversible (Git history preserved)
- Archive directory preserves all historical docs
- No hard deletions of important information
- Can restore any file if needed

## Ready to Execute?

Would you like me to:
1. Execute the full cleanup automatically
2. Execute phase-by-phase for review
3. Modify the plan first
