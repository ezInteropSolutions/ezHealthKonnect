// js/modules/healthcare-rules.js
// Healthcare-specific validation rules for ezHealthKonnect

class HealthcareRules {
    constructor() {
        this.rules = new Map();
        this.initializeRules();
    }

    /**
     * Initialize healthcare-specific validation rules
     */
    initializeRules() {
        // Patient Demographics Rules
        this.addRule('patient_demographics', {
            name: 'Patient Demographics Validation',
            description: 'Validates patient demographic information consistency',
            segments: ['PID', 'PV1'],
            severity: 'warning',
            validator: this.validatePatientDemographics.bind(this)
        });

        // Date/Time Consistency Rules
        this.addRule('datetime_consistency', {
            name: 'Date/Time Consistency',
            description: 'Validates logical date/time relationships',
            segments: ['PID', 'PV1', 'EVN', 'OBR', 'OBX'],
            severity: 'error',
            validator: this.validateDateTimeConsistency.bind(this)
        });

        // Clinical Data Integrity Rules
        this.addRule('clinical_integrity', {
            name: 'Clinical Data Integrity',
            description: 'Validates clinical data relationships and constraints',
            segments: ['OBR', 'OBX', 'DG1', 'AL1'],
            severity: 'warning',
            validator: this.validateClinicalIntegrity.bind(this)
        });

        // Administrative Workflow Rules
        this.addRule('administrative_workflow', {
            name: 'Administrative Workflow',
            description: 'Validates administrative workflow consistency',
            segments: ['MSH', 'EVN', 'PV1'],
            severity: 'warning',
            validator: this.validateAdministrativeWorkflow.bind(this)
        });

        // Insurance and Financial Rules
        this.addRule('insurance_financial', {
            name: 'Insurance and Financial Validation',
            description: 'Validates insurance and financial information',
            segments: ['IN1', 'GT1', 'PV1'],
            severity: 'warning',
            validator: this.validateInsuranceFinancial.bind(this)
        });

        // Medication and Allergy Rules
        this.addRule('medication_allergy', {
            name: 'Medication and Allergy Safety',
            description: 'Validates medication safety and allergy interactions',
            segments: ['AL1', 'OBR', 'OBX'],
            severity: 'error',
            validator: this.validateMedicationAllergy.bind(this)
        });

        // Next of Kin and Emergency Contact Rules
        this.addRule('nok_emergency_contact', {
            name: 'Next of Kin and Emergency Contact',
            description: 'Validates next of kin and emergency contact information',
            segments: ['NK1', 'PID'],
            severity: 'warning',
            validator: this.validateNokEmergencyContact.bind(this)
        });

        // Laboratory Results Rules
        this.addRule('laboratory_results', {
            name: 'Laboratory Results Validation',
            description: 'Validates laboratory results and reference ranges',
            segments: ['OBR', 'OBX'],
            severity: 'warning',
            validator: this.validateLaboratoryResults.bind(this)
        });
    }

    /**
     * Main validation entry point
     */
    validateMessage(parsedMessage, messageType) {
        const violations = [];
        
        for (const [ruleId, rule] of this.rules) {
            try {
                const ruleViolations = rule.validator(parsedMessage, messageType);
                if (ruleViolations && ruleViolations.length > 0) {
                    violations.push(...ruleViolations.map(v => ({
                        ...v,
                        ruleId,
                        ruleName: rule.name,
                        ruleDescription: rule.description,
                        defaultSeverity: rule.severity
                    })));
                }
            } catch (error) {
                violations.push({
                    type: 'RULE_EXECUTION_ERROR',
                    message: `Error executing rule ${rule.name}: ${error.message}`,
                    severity: 'warning',
                    ruleId,
                    ruleName: rule.name
                });
            }
        }
        
        return violations;
    }

    /**
     * Patient Demographics Validation
     */
    validatePatientDemographics(parsedMessage, messageType) {
        const violations = [];
        const pid = parsedMessage.PID;
        
        if (!pid) return violations;

        // Check for required patient identifiers
        if (!pid['3'] || (Array.isArray(pid['3']) && pid['3'].length === 0)) {
            violations.push({
                type: 'MISSING_PATIENT_ID',
                message: 'Patient identifier list (PID.3) is required but missing',
                severity: 'error',
                segments: ['PID'],
                fields: ['PID.3']
            });
        }

        // Check patient name consistency
        if (pid['5']) {
            const patientNames = Array.isArray(pid['5']) ? pid['5'] : [pid['5']];
            const legalNames = patientNames.filter(name => 
                !name.includes('ALIAS') && !name.includes('MAIDEN')
            );
            
            if (legalNames.length === 0) {
                violations.push({
                    type: 'MISSING_LEGAL_NAME',
                    message: 'Patient must have at least one legal name in PID.5',
                    severity: 'error',
                    segments: ['PID'],
                    fields: ['PID.5']
                });
            }
        }

        // Date of birth validation
        if (pid['7']) {
            const dob = this.parseHL7Date(pid['7']);
            const now = new Date();
            
            if (dob > now) {
                violations.push({
                    type: 'FUTURE_DATE_OF_BIRTH',
                    message: 'Patient date of birth cannot be in the future',
                    severity: 'error',
                    segments: ['PID'],
                    fields: ['PID.7'],
                    value: pid['7']
                });
            }
            
            // Check for unrealistic age
            const ageYears = (now - dob) / (365.25 * 24 * 60 * 60 * 1000);
            if (ageYears > 150) {
                violations.push({
                    type: 'UNREALISTIC_AGE',
                    message: `Patient age appears to be ${Math.floor(ageYears)} years, which may be unrealistic`,
                    severity: 'warning',
                    segments: ['PID'],
                    fields: ['PID.7'],
                    value: pid['7']
                });
            }
        }

        // Death indicator and death date consistency
        if (pid['30'] === 'Y' && !pid['29']) {
            violations.push({
                type: 'MISSING_DEATH_DATE',
                message: 'Patient death indicator is Y but death date/time is missing',
                severity: 'warning',
                segments: ['PID'],
                fields: ['PID.29', 'PID.30']
            });
        }
        
        if (pid['29'] && pid['30'] !== 'Y') {
            violations.push({
                type: 'DEATH_DATE_WITHOUT_INDICATOR',
                message: 'Patient death date/time provided but death indicator is not Y',
                severity: 'warning',
                segments: ['PID'],
                fields: ['PID.29', 'PID.30']
            });
        }

        // Gender validation
        if (pid['8'] && !['M', 'F', 'O', 'U'].includes(pid['8'])) {
            violations.push({
                type: 'INVALID_GENDER_CODE',
                message: `Invalid gender code: ${pid['8']}. Valid codes: M, F, O, U`,
                severity: 'error',
                segments: ['PID'],
                fields: ['PID.8'],
                value: pid['8']
            });
        }

        return violations;
    }

    /**
     * Date/Time Consistency Validation
     */
    validateDateTimeConsistency(parsedMessage, messageType) {
        const violations = [];
        const now = new Date();
        
        // Collect all dates from message
        const dates = {
            messageDateTime: this.getFieldValue(parsedMessage, 'MSH.7'),
            eventDateTime: this.getFieldValue(parsedMessage, 'EVN.2'),
            patientDOB: this.getFieldValue(parsedMessage, 'PID.7'),
            admitDateTime: this.getFieldValue(parsedMessage, 'PV1.44'),
            dischargeDateTime: this.getFieldValue(parsedMessage, 'PV1.45'),
            observationDateTime: this.getFieldValue(parsedMessage, 'OBX.14')
        };

        // Message date/time validation
        if (dates.messageDateTime) {
            const msgDate = this.parseHL7Date(dates.messageDateTime);
            if (msgDate > now) {
                violations.push({
                    type: 'FUTURE_MESSAGE_TIME',
                    message: 'Message date/time cannot be in the future',
                    severity: 'warning',
                    segments: ['MSH'],
                    fields: ['MSH.7'],
                    value: dates.messageDateTime
                });
            }
        }

        // Admit/discharge date consistency
        if (dates.admitDateTime && dates.dischargeDateTime) {
            const admitDate = this.parseHL7Date(dates.admitDateTime);
            const dischargeDate = this.parseHL7Date(dates.dischargeDateTime);
            
            if (dischargeDate < admitDate) {
                violations.push({
                    type: 'DISCHARGE_BEFORE_ADMIT',
                    message: 'Discharge date/time cannot be before admit date/time',
                    severity: 'error',
                    segments: ['PV1'],
                    fields: ['PV1.44', 'PV1.45'],
                    values: [dates.admitDateTime, dates.dischargeDateTime]
                });
            }
        }

        // Patient DOB vs other dates
        if (dates.patientDOB) {
            const dobDate = this.parseHL7Date(dates.patientDOB);
            
            if (dates.admitDateTime) {
                const admitDate = this.parseHL7Date(dates.admitDateTime);
                if (admitDate < dobDate) {
                    violations.push({
                        type: 'ADMIT_BEFORE_BIRTH',
                        message: 'Admit date/time cannot be before patient date of birth',
                        severity: 'error',
                        segments: ['PID', 'PV1'],
                        fields: ['PID.7', 'PV1.44'],
                        values: [dates.patientDOB, dates.admitDateTime]
                    });
                }
            }
        }

        // Event time vs message time
        if (dates.eventDateTime && dates.messageDateTime) {
            const eventDate = this.parseHL7Date(dates.eventDateTime);
            const msgDate = this.parseHL7Date(dates.messageDateTime);
            
            // Event time should not be significantly after message time
            const timeDiff = Math.abs(msgDate - eventDate) / (1000 * 60); // minutes
            if (timeDiff > 1440) { // More than 24 hours
                violations.push({
                    type: 'EVENT_MESSAGE_TIME_MISMATCH',
                    message: 'Event time and message time differ by more than 24 hours',
                    severity: 'warning',
                    segments: ['MSH', 'EVN'],
                    fields: ['MSH.7', 'EVN.2'],
                    values: [dates.messageDateTime, dates.eventDateTime]
                });
            }
        }

        return violations;
    }

    /**
     * Clinical Data Integrity Validation
     */
    validateClinicalIntegrity(parsedMessage, messageType) {
        const violations = [];

        // Validate OBX observations
        if (parsedMessage.OBX) {
            const observations = Array.isArray(parsedMessage.OBX) ? parsedMessage.OBX : [parsedMessage.OBX];
            
            observations.forEach((obx, index) => {
                // Check value type consistency
                if (obx['2'] && obx['5']) {
                    const valueType = obx['2'];
                    const value = obx['5'];
                    
                    if (valueType === 'NM' && value && !/^-?\d*\.?\d+$/.test(value)) {
                        violations.push({
                            type: 'OBX_VALUE_TYPE_MISMATCH',
                            message: `OBX value type is NM but value "${value}" is not numeric`,
                            severity: 'error',
                            segments: ['OBX'],
                            fields: [`OBX[${index}].2`, `OBX[${index}].5`],
                            values: [valueType, value]
                        });
                    }
                }

                // Check for required observation status
                if (!obx['11']) {
                    violations.push({
                        type: 'MISSING_OBX_STATUS',
                        message: 'Observation result status (OBX.11) is required',
                        severity: 'error',
                        segments: ['OBX'],
                        fields: [`OBX[${index}].11`]
                    });
                }

                // Validate abnormal flags vs reference range
                if (obx['8'] && obx['7']) {
                    const abnormalFlags = obx['8'];
                    const referenceRange = obx['7'];
                    
                    if ((abnormalFlags.includes('H') || abnormalFlags.includes('L')) && !referenceRange) {
                        violations.push({
                            type: 'ABNORMAL_FLAG_WITHOUT_RANGE',
                            message: 'Abnormal flags present but reference range is missing',
                            severity: 'warning',
                            segments: ['OBX'],
                            fields: [`OBX[${index}].7`, `OBX[${index}].8`],
                            values: [referenceRange, abnormalFlags]
                        });
                    }
                }
            });
        }

        // Validate allergy information
        if (parsedMessage.AL1) {
            const allergies = Array.isArray(parsedMessage.AL1) ? parsedMessage.AL1 : [parsedMessage.AL1];
            
            allergies.forEach((al1, index) => {
                // Check for allergen identification
                if (!al1['3']) {
                    violations.push({
                        type: 'MISSING_ALLERGEN_CODE',
                        message: 'Allergen code/description (AL1.3) is required',
                        severity: 'error',
                        segments: ['AL1'],
                        fields: [`AL1[${index}].3`]
                    });
                }

                // Validate severity consistency
                if (al1['4'] && al1['5']) {
                    const severity = al1['4'];
                    const reaction = al1['5'];
                    
                    // Check for severe reactions with mild severity
                    if (severity === 'MI' && reaction && 
                        (reaction.toLowerCase().includes('anaphyl') || 
                         reaction.toLowerCase().includes('shock'))) {
                        violations.push({
                            type: 'SEVERITY_REACTION_MISMATCH',
                            message: 'Severe reaction reported with mild severity',
                            severity: 'warning',
                            segments: ['AL1'],
                            fields: [`AL1[${index}].4`, `AL1[${index}].5`],
                            values: [severity, reaction]
                        });
                    }
                }
            });
        }

        return violations;
    }

    /**
     * Administrative Workflow Validation
     */
    validateAdministrativeWorkflow(parsedMessage, messageType) {
        const violations = [];
        
        // Message type consistency
        const msgType = this.getFieldValue(parsedMessage, 'MSH.9');
        const eventTypeCode = this.getFieldValue(parsedMessage, 'EVN.1');
        
        if (msgType && eventTypeCode) {
            const expectedEvents = this.getExpectedEventTypes(msgType);
            if (expectedEvents.length > 0 && !expectedEvents.includes(eventTypeCode)) {
                violations.push({
                    type: 'MESSAGE_EVENT_MISMATCH',
                    message: `Event type ${eventTypeCode} not expected for message type ${msgType}`,
                    severity: 'warning',
                    segments: ['MSH', 'EVN'],
                    fields: ['MSH.9', 'EVN.1'],
                    values: [msgType, eventTypeCode]
                });
            }
        }

        // Patient class vs admission type consistency
        const patientClass = this.getFieldValue(parsedMessage, 'PV1.2');
        const admissionType = this.getFieldValue(parsedMessage, 'PV1.4');
        
        if (patientClass === 'O' && admissionType && ['A', 'E', 'L', 'N'].includes(admissionType)) {
            violations.push({
                type: 'OUTPATIENT_ADMISSION_TYPE_MISMATCH',
                message: 'Outpatient class with inpatient admission type',
                severity: 'warning',
                segments: ['PV1'],
                fields: ['PV1.2', 'PV1.4'],
                values: [patientClass, admissionType]
            });
        }

        // Emergency department validation
        if (patientClass === 'E') {
            const assignedLocation = this.getFieldValue(parsedMessage, 'PV1.3');
            if (assignedLocation && !assignedLocation.toLowerCase().includes('ed') && 
                !assignedLocation.toLowerCase().includes('emergency')) {
                violations.push({
                    type: 'EMERGENCY_LOCATION_MISMATCH',
                    message: 'Emergency patient class but location does not appear to be ED',
                    severity: 'warning',
                    segments: ['PV1'],
                    fields: ['PV1.2', 'PV1.3'],
                    values: [patientClass, assignedLocation]
                });
            }
        }

        return violations;
    }

    /**
     * Insurance and Financial Validation
     */
    validateInsuranceFinancial(parsedMessage, messageType) {
        const violations = [];
        
        // Check for guarantor when insurance is present
        if (parsedMessage.IN1 && !parsedMessage.GT1) {
            violations.push({
                type: 'INSURANCE_WITHOUT_GUARANTOR',
                message: 'Insurance information present but guarantor information missing',
                severity: 'warning',
                segments: ['IN1', 'GT1'],
                fields: ['IN1', 'GT1']
            });
        }

        // Validate financial class consistency
        const financialClass = this.getFieldValue(parsedMessage, 'PV1.20');
        if (financialClass && parsedMessage.IN1) {
            const insurance = Array.isArray(parsedMessage.IN1) ? parsedMessage.IN1[0] : parsedMessage.IN1;
            const planId = insurance ? insurance['2'] : null;
            
            if (financialClass.toLowerCase().includes('self') && planId) {
                violations.push({
                    type: 'SELF_PAY_WITH_INSURANCE',
                    message: 'Financial class indicates self-pay but insurance information present',
                    severity: 'warning',
                    segments: ['PV1', 'IN1'],
                    fields: ['PV1.20', 'IN1.2'],
                    values: [financialClass, planId]
                });
            }
        }

        return violations;
    }

    /**
     * Medication and Allergy Safety Validation
     */
    validateMedicationAllergy(parsedMessage, messageType) {
        const violations = [];
        
        if (!parsedMessage.AL1) return violations;

        const allergies = Array.isArray(parsedMessage.AL1) ? parsedMessage.AL1 : [parsedMessage.AL1];
        const drugAllergies = allergies.filter(al1 => al1['2'] === 'DA');
        
        if (drugAllergies.length > 0 && parsedMessage.OBR) {
            const orders = Array.isArray(parsedMessage.OBR) ? parsedMessage.OBR : [parsedMessage.OBR];
            
            orders.forEach((obr, orderIndex) => {
                const orderCode = obr['4'];
                if (orderCode && orderCode.toLowerCase().includes('drug')) {
                    violations.push({
                        type: 'DRUG_ORDER_WITH_DRUG_ALLERGY',
                        message: 'Drug order present for patient with documented drug allergies - review needed',
                        severity: 'warning',
                        segments: ['AL1', 'OBR'],
                        fields: ['AL1.2', `OBR[${orderIndex}].4`],
                        note: 'Clinical review recommended to ensure drug compatibility'
                    });
                }
            });
        }

        return violations;
    }

    /**
     * Next of Kin and Emergency Contact Validation
     */
    validateNokEmergencyContact(parsedMessage, messageType) {
        const violations = [];
        
        if (!parsedMessage.NK1) return violations;

        const nextOfKin = Array.isArray(parsedMessage.NK1) ? parsedMessage.NK1 : [parsedMessage.NK1];
        
        nextOfKin.forEach((nk1, index) => {
            // Check for required information
            if (!nk1['2']) {
                violations.push({
                    type: 'MISSING_NOK_NAME',
                    message: 'Next of kin name (NK1.2) is required',
                    severity: 'warning',
                    segments: ['NK1'],
                    fields: [`NK1[${index}].2`]
                });
            }

            if (!nk1['3']) {
                violations.push({
                    type: 'MISSING_NOK_RELATIONSHIP',
                    message: 'Next of kin relationship (NK1.3) is required',
                    severity: 'warning',
                    segments: ['NK1'],
                    fields: [`NK1[${index}].3`]
                });
            }

            // Check for contact information
            if (!nk1['4'] && !nk1['5']) {
                violations.push({
                    type: 'MISSING_NOK_CONTACT',
                    message: 'Next of kin should have address or phone number',
                    severity: 'warning',
                    segments: ['NK1'],
                    fields: [`NK1[${index}].4`, `NK1[${index}].5`]
                });
            }
        });

        return violations;
    }

    /**
     * Laboratory Results Validation
     */
    validateLaboratoryResults(parsedMessage, messageType) {
        const violations = [];
        
        if (!parsedMessage.OBX) return violations;

        const observations = Array.isArray(parsedMessage.OBX) ? parsedMessage.OBX : [parsedMessage.OBX];
        
        observations.forEach((obx, index) => {
            const observationType = obx['3'];
            const value = obx['5'];
            const units = obx['6'];
            const referenceRange = obx['7'];
            const abnormalFlags = obx['8'];
            
            // Lab results should have units
            if (observationType && observationType.toLowerCase().includes('lab') && value && !units) {
                violations.push({
                    type: 'LAB_RESULT_MISSING_UNITS',
                    message: 'Laboratory result missing units of measure',
                    severity: 'warning',
                    segments: ['OBX'],
                    fields: [`OBX[${index}].6`],
                    context: `Observation: ${observationType}, Value: ${value}`
                });
            }

            // Numeric results should have reference ranges
            if (obx['2'] === 'NM' && value && !referenceRange) {
                violations.push({
                    type: 'NUMERIC_RESULT_MISSING_RANGE',
                    message: 'Numeric result missing reference range',
                    severity: 'warning',
                    segments: ['OBX'],
                    fields: [`OBX[${index}].7`],
                    context: `Observation: ${observationType}, Value: ${value}`
                });
            }

            // Critical values should be flagged
            if (value && this.isCriticalValue(observationType, value) && !abnormalFlags) {
                violations.push({
                    type: 'CRITICAL_VALUE_NOT_FLAGGED',
                    message: 'Potentially critical value not flagged as abnormal',
                    severity: 'error',
                    segments: ['OBX'],
                    fields: [`OBX[${index}].8`],
                    context: `Observation: ${observationType}, Value: ${value}`
                });
            }
        });

        return violations;
    }

    /**
     * Helper Methods
     */
    
    addRule(ruleId, rule) {
        this.rules.set(ruleId, rule);
    }

    removeRule(ruleId) {
        return this.rules.delete(ruleId);
    }

    getRule(ruleId) {
        return this.rules.get(ruleId);
    }

    getAllRules() {
        return Array.from(this.rules.entries()).map(([id, rule]) => ({
            id,
            name: rule.name,
            description: rule.description,
            segments: rule.segments,
            severity: rule.severity
        }));
    }

    parseHL7Date(dateString) {
        if (!dateString) return null;
        
        // HL7 date format: YYYYMMDDHHMMSS[.SSSS][+/-ZZZZ]
        const cleanDate = dateString.replace(/[+\-]\d{4}$/, ''); // Remove timezone
        
        if (cleanDate.length >= 8) {
            const year = parseInt(cleanDate.substring(0, 4));
            const month = parseInt(cleanDate.substring(4, 6)) - 1; // JS months are 0-based
            const day = parseInt(cleanDate.substring(6, 8));
            const hour = cleanDate.length >= 10 ? parseInt(cleanDate.substring(8, 10)) : 0;
            const minute = cleanDate.length >= 12 ? parseInt(cleanDate.substring(10, 12)) : 0;
            const second = cleanDate.length >= 14 ? parseInt(cleanDate.substring(12, 14)) : 0;
            
            return new Date(year, month, day, hour, minute, second);
        }
        
        return null;
    }

    getFieldValue(message, fieldPath) {
        const parts = fieldPath.split('.');
        let current = message;
        
        for (const part of parts) {
            if (current && typeof current === 'object') {
                current = current[part];
            } else {
                return null;
            }
        }
        
        return current;
    }

    getExpectedEventTypes(messageType) {
        const eventMap = {
            'ADT^A01': ['A01'],
            'ADT^A02': ['A02'],
            'ADT^A03': ['A03'],
            'ADT^A04': ['A04'],
            'ADT^A05': ['A05'],
            'ADT^A08': ['A08'],
            'ORM^O01': ['O01'],
            'ORU^R01': ['R01'],
            'MDM^T02': ['T02']
        };
        
        return eventMap[messageType] || [];
    }

    isCriticalValue(observationType, value) {
        if (!observationType || !value) return false;
        
        const numericValue = parseFloat(value);
        if (isNaN(numericValue)) return false;
        
        // Basic critical value thresholds (simplified for demo)
        const criticalRanges = {
            'glucose': { low: 40, high: 400 },
            'potassium': { low: 2.5, high: 6.0 },
            'sodium': { low: 120, high: 160 },
            'hemoglobin': { low: 7.0, high: 20.0 },
            'platelet': { low: 50, high: 1000 },
            'creatinine': { low: 0, high: 5.0 }
        };
        
        for (const [test, range] of Object.entries(criticalRanges)) {
            if (observationType.toLowerCase().includes(test)) {
                return numericValue < range.low || numericValue > range.high;
            }
        }
        
        return false;
    }

    // Enable/disable specific rules
    enableRule(ruleId) {
        const rule = this.rules.get(ruleId);
        if (rule) {
            rule.enabled = true;
        }
    }

    disableRule(ruleId) {
        const rule = this.rules.get(ruleId);
        if (rule) {
            rule.enabled = false;
        }
    }

    // Custom rule configuration
    configureRule(ruleId, config) {
        const rule = this.rules.get(ruleId);
        if (rule) {
            Object.assign(rule, config);
        }
    }
}

// Export for use in other modules
export { HealthcareRules };