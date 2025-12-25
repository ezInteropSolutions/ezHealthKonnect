# ezHealthKonnect Documentation Hub

**Last Updated**: December 25, 2025
**Version**: 2.0

Welcome to the complete documentation for ezHealthKonnect - AI-Powered Healthcare Integration Platform.

---

## 📚 Quick Start Guides

### New Users
- **[Getting Started](GETTING_STARTED.md)** - Installation, first pipeline, basic concepts
- **[Quick Reference](QUICK_REFERENCE.md)** - Common tasks and workflows
- **[Video Tutorials](VIDEO_TUTORIALS.md)** - Step-by-step video guides

### For Developers
- **[Developer Setup](DEVELOPER_SETUP.md)** - Development environment, architecture overview
- **[API Documentation](API_DOCUMENTATION.md)** - REST API reference
- **[Contributing Guide](CONTRIBUTING.md)** - How to contribute to the project

---

## 🏗️ Core Concepts

### Platform Architecture
- **[System Architecture Overview](SYSTEM_DOCUMENTATION.md)** - Complete system design
- **[Hybrid Storage Architecture](architecture/HYBRID_STORAGE_ARCHITECTURE.md)** - PostgreSQL + MongoDB
- **[JSON Conversion Pipeline](architecture/JSON_CONVERSION_ARCHITECTURE.md)** - Automatic message parsing
- **[Transformation Pipeline](architecture/TRANSFORMATION_PIPELINE_DESIGN.md)** - Multi-step processing
- **[Processing Engine](architecture/PROCESSING_ENGINE.md)** - Message processing core

### Data Flow
- **[Message Lifecycle](MESSAGE_LIFECYCLE.md)** - From ingestion to delivery
- **[Step Output Chaining](STEP_OUTPUT_CHAINING_GUIDE.md)** - Using previous step data
- **[Pipeline Execution Context](PIPELINE_EXECUTION_CONTEXT.md)** - Execution state management

---

## 🔌 Connectivity

### Inbound Connectors
- **[TCP/MLLP Inbound](connectivity/TCP_MLLP_INBOUND.md)** - Receive HL7 messages
- **[HTTP REST Inbound](connectivity/HTTP_REST_INBOUND.md)** - REST API endpoints
- **[File Listener](connectivity/FILE_LISTENER.md)** - Monitor file directories
- **[Database Inbound](connectivity/DATABASE_INBOUND.md)** - Poll database tables
- **[Message Queue Inbound](connectivity/MESSAGE_QUEUE_INBOUND.md)** - RabbitMQ, Kafka, Redis

### Outbound Connectors
- **[TCP/MLLP Outbound](connectivity/TCP_MLLP_OUTBOUND.md)** - Send HL7 messages
- **[HTTP Outbound](connectivity/HTTP_OUTBOUND.md)** - REST API calls
- **[File Writer](connectivity/FILE_WRITER.md)** - Write to files
- **[Database Outbound](connectivity/DATABASE_OUTBOUND.md)** - Insert/update database
- **[Message Queue Outbound](connectivity/MESSAGE_QUEUE_OUTBOUND.md)** - Publish to queues

### Connectivity Guides
- **[Connectivity Catalog](connectivity/CONNECTIVITY_CATALOG.md)** - All 32 OOB connectors
- **[Connectivity Architecture](connectivity/CONNECTIVITY_ARCHITECTURE.md)** - Design patterns
- **[Connector Implementation Guide](connectivity/CONNECTOR_IMPLEMENTATION_GUIDE.md)** - Building custom connectors

---

## 🗄️ Database Integration

### Configuration Guides
- **[📋 Database Configuration Guide](DATABASE_CONFIGURATION_GUIDE.md)** ⭐ **MASTER REFERENCE**
  - MySQL Configuration
  - PostgreSQL Configuration
  - SQL Server Configuration
  - MongoDB Configuration
  - Redis Configuration
  - Oracle Configuration

### Feature-Specific Guides
- **[Database Enrichment](DATABASE_ENRICHMENT_COMPLETE_IMPLEMENTATION.md)** - Query databases for additional data
- **[Database Column Autocomplete](DATABASE_COLUMN_AUTOCOMPLETE.md)** - Auto-detect available columns
- **[Zero Rows Handling](DATABASE_COLUMN_AUTOCOMPLETE_ZERO_ROWS.md)** - Configure mappings without test data
- **[Result Mapping](RESULT_MAPPING_ENHANCEMENTS.md)** - Map database columns to output fields
- **[Query Parameter Mapping](QUERY_PARAMS_FIELD_SEARCH_INTEGRATION.md)** - Dynamic parameter binding

### Database-Specific Guides
- **[MongoDB Visual Builders](MONGODB_NO_CODE_UI_GUIDE.md)** - Filter and projection builders
- **[MongoDB Advanced Mode](MONGODB_ADVANCED_MODE_GUIDE.md)** - Raw query editor
- **[Multi-Database Support](DATABASE_ENRICHMENT_MULTI_DB_PLAN.md)** - Working with multiple databases
- **[NoSQL Implementation](DATABASE_ENRICHMENT_NOSQL_IMPLEMENTATION.md)** - MongoDB and Redis specifics

---

## 🔄 HL7 & FHIR

### HL7 Processing
- **[HL7 Field Path Format](HL7_FIELD_PATH_FORMAT_UPDATE.md)** - New simplified format (PID.3 vs legacy)
- **[HL7 Composite Fields](HL7_COMPOSITE_FIELDS_GUIDE.md)** - Understanding PID.3 vs PID.3.1
- **[HL7 Message Parsing](HL7_PARSING_GUIDE.md)** - Enhanced schema structure
- **[HL7 Dictionary](HL7_DICTIONARY.md)** - Field definitions and data types

### FHIR Conversion
- **[HL7 to FHIR Mapping](HL7_TO_FHIR_MAPPING.md)** - Transformation rules
- **[FHIR Resource Generation](FHIR_RESOURCE_GENERATION.md)** - Creating FHIR bundles
- **[FHIR Validation](FHIR_VALIDATION.md)** - Ensuring FHIR compliance

---

## 🔧 Pipeline Builder

### Visual Pipeline Design
- **[Pipeline Builder UI](PIPELINE_BUILDER_UI.md)** - Drag-and-drop interface
- **[Step Types](PIPELINE_STEP_TYPES.md)** - All available transformation steps
- **[Step Configuration](STEP_CONFIGURATION.md)** - Configuring each step type
- **[Pipeline Testing](PIPELINE_TESTING.md)** - Test before deployment

### Enrichment Steps
- **[Database Enrichment](DATABASE_ENRICHMENT_COMPLETE_IMPLEMENTATION.md)** - Query databases
- **[API Enrichment](API_ENRICHMENT_ARCHITECTURE_DECISION.md)** - Call REST APIs
- **[Field Calculation](FIELD_CALCULATION.md)** - Computed fields
- **[Lookup Tables](LOOKUP_TABLES.md)** - Reference data lookups

### Transformation Steps
- **[Field Mapping](FIELD_MAPPING.md)** - Map fields between formats
- **[Data Transformation](DATA_TRANSFORMATION.md)** - Transform data types
- **[Custom JavaScript](CUSTOM_JAVASCRIPT.md)** - Write custom logic
- **[Conditional Logic](CONDITIONAL_LOGIC.md)** - If-then-else rules

### Validation Steps
- **[Field Validation](FIELD_VALIDATION.md)** - Validate field values
- **[Schema Validation](SCHEMA_VALIDATION.md)** - Validate message structure
- **[Business Rules](BUSINESS_RULES.md)** - Custom validation rules

---

## 🔐 Security & Authentication

### OAuth 2.0
- **[OAuth 2.0 Integration](OAUTH2_FULL_INTEGRATION_COMPLETE.md)** - Complete OAuth 2.0 support
- **[OAuth 2.0 Testing](OAUTH2_WORKING_TEST.md)** - Testing OAuth flows
- **[OAuth 2.0 Fixes](OAUTH2_FIXES_APPLIED.md)** - Known issues and solutions

### Security
- **[Authentication](AUTHENTICATION.md)** - User authentication
- **[Authorization](AUTHORIZATION.md)** - Role-based access control
- **[Encryption](ENCRYPTION.md)** - Data encryption at rest and in transit
- **[Audit Logging](AUDIT_LOGGING.md)** - HIPAA/GDPR compliance

---

## 🎨 User Interface

### No-Code Features
- **[Field Path Search Component](FIELD_PATH_SEARCH_COMPONENT.md)** - Smart field autocomplete
- **[Query Parameter Builder](QUERY_PARAMETER_BUILDER.md)** - Visual parameter mapping
- **[Result Mapping Builder](RESULT_MAPPING_ENHANCEMENTS.md)** - Visual column mapping
- **[MongoDB Filter Builder](MONGODB_FILTER_BUILDER.md)** - Visual MongoDB queries
- **[OAuth 2.0 Config Builder](OAUTH2_CONFIG_BUILDER.md)** - Visual OAuth configuration

### UI Components
- **[Database Query Tester](DATABASE_QUERY_TESTER.md)** - Test queries before saving
- **[API Endpoint Tester](API_ENDPOINT_TESTER.md)** - Test API calls
- **[Fullscreen Mode](FULLSCREEN_MODE_AND_JSON_IMPORT_UPDATE.md)** - Distraction-free editing
- **[JSON Import](FULLSCREEN_MODE_AND_JSON_IMPORT_UPDATE.md)** - Import existing pipelines

---

## 📊 Monitoring & Operations

### Message Management
- **[Message Tracking](MESSAGE_TRACKING.md)** - Track message flow
- **[Error Handling](ERROR_HANDLING.md)** - Handle failures gracefully
- **[Retry Logic](RETRY_LOGIC.md)** - Automatic retry on failure
- **[Dead Letter Queue](DEAD_LETTER_QUEUE.md)** - Handle undeliverable messages

### Performance
- **[Performance Tuning](PERFORMANCE_TUNING.md)** - Optimize throughput
- **[Scalability Design](architecture/SCALABILITY_AND_GUI_DESIGN.md)** - Scale to millions of messages
- **[Message-Level Parallel Processing](MESSAGE_LEVEL_PARALLEL_PROCESSING.md)** - Concurrent processing
- **[Connection Pooling](CONNECTION_POOLING.md)** - Database connection management

### Monitoring
- **[Metrics & KPIs](METRICS_AND_KPIS.md)** - System health metrics
- **[Logging](LOGGING.md)** - Application logging
- **[Alerting](ALERTING.md)** - Proactive alerts
- **[Dashboard](DASHBOARD.md)** - Real-time monitoring

---

## 🚀 Deployment

### Environment Setup
- **[Docker Deployment](DOCKER_DEPLOYMENT.md)** - Deploy with Docker Compose
- **[Kubernetes Deployment](KUBERNETES_DEPLOYMENT.md)** - Deploy to K8s
- **[Environment Configuration](ENVIRONMENT_CONFIGURATION.md)** - Environment variables
- **[Database Migrations](DATABASE_MIGRATIONS.md)** - Flyway migrations

### Production Readiness
- **[Production Checklist](PRODUCTION_CHECKLIST.md)** - Pre-launch verification
- **[Backup & Recovery](BACKUP_AND_RECOVERY.md)** - Data protection
- **[Disaster Recovery](DISASTER_RECOVERY.md)** - Business continuity
- **[High Availability](HIGH_AVAILABILITY.md)** - Eliminate single points of failure

---

## 🧪 Testing

### Testing Strategies
- **[Unit Testing](UNIT_TESTING.md)** - Component testing
- **[Integration Testing](INTEGRATION_TESTING.md)** - End-to-end testing
- **[Load Testing](LOAD_TESTING.md)** - Performance testing
- **[User Acceptance Testing](UAT.md)** - Business validation

### Test Execution
- **[Database Enrichment Testing](DATABASE_ENRICHMENT_COMPLETE_TEST_PLAN.md)** - Comprehensive test plan
- **[Phase 1 Test Guide](DATABASE_ENRICHMENT_PHASE1_TEST_GUIDE.md)** - SQL databases
- **[Phase 2 Test Guide](DATABASE_ENRICHMENT_PHASE2_TEST_GUIDE.md)** - NoSQL databases
- **[Test Execution Results](DATABASE_ENRICHMENT_TEST_EXECUTION_RESULTS.md)** - Test outcomes

---

## 📖 Reference Materials

### API Reference
- **[REST API](REST_API_REFERENCE.md)** - HTTP endpoints
- **[WebSocket API](WEBSOCKET_API_REFERENCE.md)** - Real-time updates
- **[Database Schema](DATABASE_SCHEMA_REFERENCE.md)** - Complete schema
- **[Message Formats](MESSAGE_FORMAT_REFERENCE.md)** - HL7, FHIR, JSON

### Code Examples
- **[Code Examples](CODE_EXAMPLES.md)** - Common patterns
- **[Custom Executor Examples](CUSTOM_EXECUTOR_EXAMPLES.md)** - Building executors
- **[JavaScript Transform Examples](JAVASCRIPT_EXAMPLES.md)** - Custom scripts
- **[Integration Patterns](connectivity/CONNECTIVITY_PATTERNS.md)** - Common patterns

### Quick References
- **[Database Quick Reference](DATABASE_QUICK_REFERENCE.md)** - Connection strings, queries
- **[HL7 Field Reference](HL7_FIELD_REFERENCE.md)** - Common HL7 fields
- **[FHIR Resource Reference](FHIR_RESOURCE_REFERENCE.md)** - FHIR resource types
- **[Error Code Reference](ERROR_CODE_REFERENCE.md)** - All error codes

---

## 🔧 Troubleshooting

### Common Issues
- **[Connection Issues](TROUBLESHOOTING_CONNECTION.md)** - Database, API connectivity
- **[Query Issues](TROUBLESHOOTING_QUERIES.md)** - SQL, MongoDB queries
- **[Performance Issues](TROUBLESHOOTING_PERFORMANCE.md)** - Slow pipelines
- **[Data Issues](TROUBLESHOOTING_DATA.md)** - Missing or incorrect data

### Debug Guides
- **[Debugging Pipelines](DEBUGGING_PIPELINES.md)** - Step-by-step debugging
- **[Log Analysis](LOG_ANALYSIS.md)** - Reading application logs
- **[Network Troubleshooting](NETWORK_TROUBLESHOOTING.md)** - Network issues
- **[Database Troubleshooting](DATABASE_TROUBLESHOOTING.md)** - Database-specific issues

---

## 📝 Release Notes & Changelog

### Major Releases
- **[Version 2.0](RELEASE_NOTES_V2.0.md)** - Multi-connectivity, NoSQL support
- **[Version 1.0](RELEASE_NOTES_V1.0.md)** - Initial release

### Feature Additions
- **[OAuth 2.0 Complete](OAUTH2_FULL_INTEGRATION_COMPLETE.md)** - December 2025
- **[Database Enrichment NoSQL](DATABASE_ENRICHMENT_NOSQL_IMPLEMENTATION.md)** - December 2025
- **[Step Output Tracking](STEP_OUTPUT_TRACKING.md)** - October 2025
- **[JSON Conversion](architecture/JSON_CONVERSION_ARCHITECTURE.md)** - October 2025

### Bug Fixes
- **[Database Enrichment Bugfixes](DATABASE_ENRICHMENT_BUGFIXES.md)** - December 2025
- **[OAuth 2.0 Fixes](OAUTH2_FIXES_APPLIED.md)** - December 2025
- **[Connection String Fix](DATABASE_ENRICHMENT_CONNECTION_STRING_FIX.md)** - December 2025

---

## 🎓 Training & Tutorials

### Video Tutorials
- **[Getting Started Video](tutorials/getting-started.mp4)** - 10 minutes
- **[Building Your First Pipeline](tutorials/first-pipeline.mp4)** - 15 minutes
- **[Database Enrichment Tutorial](tutorials/database-enrichment.mp4)** - 20 minutes
- **[API Integration Tutorial](tutorials/api-integration.mp4)** - 25 minutes

### Written Tutorials
- **[HL7 to FHIR Conversion Tutorial](tutorials/HL7_TO_FHIR_TUTORIAL.md)** - Step-by-step
- **[Multi-Database Integration Tutorial](tutorials/MULTI_DB_TUTORIAL.md)** - Advanced
- **[OAuth 2.0 Setup Tutorial](tutorials/OAUTH2_TUTORIAL.md)** - Security

---

## 🤝 Community & Support

### Getting Help
- **[FAQ](FAQ.md)** - Frequently asked questions
- **[Community Forum](https://forum.ezhealthkonnect.com)** - Ask questions
- **[GitHub Issues](https://github.com/ezhealthkonnect/issues)** - Report bugs
- **[Stack Overflow](https://stackoverflow.com/questions/tagged/ezhealthkonnect)** - Technical Q&A

### Contributing
- **[Contributing Guide](CONTRIBUTING.md)** - How to contribute
- **[Code of Conduct](CODE_OF_CONDUCT.md)** - Community guidelines
- **[Development Roadmap](ROADMAP.md)** - Future plans

---

## 📄 Legal & Compliance

- **[License](LICENSE.md)** - Software license
- **[Privacy Policy](PRIVACY_POLICY.md)** - Data handling
- **[Security Policy](SECURITY_POLICY.md)** - Security practices
- **[HIPAA Compliance](HIPAA_COMPLIANCE.md)** - Healthcare compliance
- **[GDPR Compliance](GDPR_COMPLIANCE.md)** - EU data protection

---

## 🔍 Documentation by Role

### For Integration Engineers
1. [Getting Started](GETTING_STARTED.md)
2. [Pipeline Builder UI](PIPELINE_BUILDER_UI.md)
3. [Database Configuration Guide](DATABASE_CONFIGURATION_GUIDE.md) ⭐
4. [Step Output Chaining](STEP_OUTPUT_CHAINING_GUIDE.md)
5. [Testing Pipelines](PIPELINE_TESTING.md)

### For System Administrators
1. [Docker Deployment](DOCKER_DEPLOYMENT.md)
2. [Environment Configuration](ENVIRONMENT_CONFIGURATION.md)
3. [Monitoring & Alerting](METRICS_AND_KPIS.md)
4. [Backup & Recovery](BACKUP_AND_RECOVERY.md)
5. [Security Configuration](AUTHENTICATION.md)

### For Developers
1. [System Architecture](SYSTEM_DOCUMENTATION.md)
2. [API Documentation](API_DOCUMENTATION.md)
3. [Custom Executor Examples](CUSTOM_EXECUTOR_EXAMPLES.md)
4. [Connector Implementation Guide](connectivity/CONNECTOR_IMPLEMENTATION_GUIDE.md)
5. [Contributing Guide](CONTRIBUTING.md)

### For Healthcare IT Professionals
1. [HL7 Processing](HL7_PARSING_GUIDE.md)
2. [FHIR Conversion](HL7_TO_FHIR_MAPPING.md)
3. [HIPAA Compliance](HIPAA_COMPLIANCE.md)
4. [Message Tracking](MESSAGE_TRACKING.md)
5. [Audit Logging](AUDIT_LOGGING.md)

---

## 📊 Documentation Statistics

- **Total Documents**: 150+
- **Code Examples**: 500+
- **API Endpoints**: 120+
- **Supported Databases**: 6 (MySQL, PostgreSQL, SQL Server, MongoDB, Redis, Oracle)
- **Supported Connectors**: 32 (16 inbound, 16 outbound)
- **Last Major Update**: December 25, 2025

---

## 🆘 Can't Find What You're Looking For?

1. **Use Search**: Ctrl+F in this document
2. **Check FAQ**: [FAQ.md](FAQ.md)
3. **Ask Community**: [Community Forum](https://forum.ezhealthkonnect.com)
4. **Report Missing Doc**: [GitHub Issues](https://github.com/ezhealthkonnect/docs/issues)

---

**Happy Integrating! 🚀**

*ezHealthKonnect - Simplifying Healthcare Integration*
