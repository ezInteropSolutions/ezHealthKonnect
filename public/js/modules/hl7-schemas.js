// js/modules/hl7-schemas.js
// HL7 field definitions and validation rules for ezHealthKonnect

class HL7Schemas {
    constructor() {
        this.schemas = new Map();
        this.valueSets = new Map();
        this.initializeSchemas();
        this.initializeValueSets();
    }

    /**
     * Initialize HL7 message schemas
     */
    initializeSchemas() {
        // ADT^A01 - Admit/Visit Notification
        this.schemas.set('ADT_A01', {
            name: 'ADT^A01 - Admit/Visit Notification',
            description: 'Patient admission or visit notification',
            segments: {
                MSH: {
                    name: 'Message Header',
                    required: true,
                    fields: this.getMSHFields()
                },
                EVN: {
                    name: 'Event Type',
                    required: false,
                    fields: this.getEVNFields()
                },
                PID: {
                    name: 'Patient Identification',
                    required: true,
                    fields: this.getPIDFields()
                },
                PD1: {
                    name: 'Additional Demographics',
                    required: false,
                    fields: this.getPD1Fields()
                },
                ROL: {
                    name: 'Role',
                    required: false,
                    repeating: true,
                    fields: this.getROLFields()
                },
                NK1: {
                    name: 'Next of Kin',
                    required: false,
                    repeating: true,
                    fields: this.getNK1Fields()
                },
                PV1: {
                    name: 'Patient Visit',
                    required: true,
                    fields: this.getPV1Fields()
                },
                PV2: {
                    name: 'Patient Visit - Additional Info',
                    required: false,
                    fields: this.getPV2Fields()
                },
                AL1: {
                    name: 'Allergy Information',
                    required: false,
                    repeating: true,
                    fields: this.getAL1Fields()
                },
                DG1: {
                    name: 'Diagnosis',
                    required: false,
                    repeating: true,
                    fields: this.getDG1Fields()
                },
                GT1: {
                    name: 'Guarantor',
                    required: false,
                    repeating: true,
                    fields: this.getGT1Fields()
                },
                IN1: {
                    name: 'Insurance',
                    required: false,
                    repeating: true,
                    fields: this.getIN1Fields()
                }
            }
        });

        // ORM^O01 - Order Message
        this.schemas.set('ORM_O01', {
            name: 'ORM^O01 - General Order Message',
            description: 'Order message for various types of orders',
            segments: {
                MSH: {
                    name: 'Message Header',
                    required: true,
                    fields: this.getMSHFields()
                },
                PID: {
                    name: 'Patient Identification',
                    required: true,
                    fields: this.getPIDFields()
                },
                PV1: {
                    name: 'Patient Visit',
                    required: false,
                    fields: this.getPV1Fields()
                },
                ORC: {
                    name: 'Common Order',
                    required: true,
                    fields: this.getORCFields()
                },
                OBR: {
                    name: 'Observation Request',
                    required: true,
                    fields: this.getOBRFields()
                },
                OBX: {
                    name: 'Observation/Result',
                    required: false,
                    repeating: true,
                    fields: this.getOBXFields()
                }
            }
        });

        // ORU^R01 - Results Message
        this.schemas.set('ORU_R01', {
            name: 'ORU^R01 - Observation Result',
            description: 'Unsolicited observation results',
            segments: {
                MSH: {
                    name: 'Message Header',
                    required: true,
                    fields: this.getMSHFields()
                },
                PID: {
                    name: 'Patient Identification',
                    required: true,
                    fields: this.getPIDFields()
                },
                PV1: {
                    name: 'Patient Visit',
                    required: false,
                    fields: this.getPV1Fields()
                },
                OBR: {
                    name: 'Observation Request',
                    required: true,
                    fields: this.getOBRFields()
                },
                OBX: {
                    name: 'Observation/Result',
                    required: true,
                    repeating: true,
                    fields: this.getOBXFields()
                }
            }
        });

        // MDM^T02 - Document Management
        this.schemas.set('MDM_T02', {
            name: 'MDM^T02 - Document Management',
            description: 'Medical document management message',
            segments: {
                MSH: {
                    name: 'Message Header',
                    required: true,
                    fields: this.getMSHFields()
                },
                EVN: {
                    name: 'Event Type',
                    required: true,
                    fields: this.getEVNFields()
                },
                PID: {
                    name: 'Patient Identification',
                    required: true,
                    fields: this.getPIDFields()
                },
                PV1: {
                    name: 'Patient Visit',
                    required: true,
                    fields: this.getPV1Fields()
                },
                TXA: {
                    name: 'Transcription Document Header',
                    required: true,
                    fields: this.getTXAFields()
                }
            }
        });
    }

    /**
     * MSH - Message Header Fields
     */
    getMSHFields() {
        return {
            '1': {
                name: 'Field Separator',
                dataType: 'ST',
                required: true,
                maxLength: 1,
                description: 'Field separator character'
            },
            '2': {
                name: 'Encoding Characters',
                dataType: 'ST',
                required: true,
                maxLength: 4,
                description: 'Encoding characters',
                pattern: {
                    regex: '^\\^~\\\\&$',
                    description: 'Must be ^~\\&'
                }
            },
            '3': {
                name: 'Sending Application',
                dataType: 'HD',
                required: true,
                maxLength: 227,
                description: 'Sending application'
            },
            '4': {
                name: 'Sending Facility',
                dataType: 'HD',
                required: true,
                maxLength: 227,
                description: 'Sending facility'
            },
            '5': {
                name: 'Receiving Application',
                dataType: 'HD',
                required: true,
                maxLength: 227,
                description: 'Receiving application'
            },
            '6': {
                name: 'Receiving Facility',
                dataType: 'HD',
                required: true,
                maxLength: 227,
                description: 'Receiving facility'
            },
            '7': {
                name: 'Date/Time of Message',
                dataType: 'TS',
                required: true,
                description: 'Date and time message was created'
            },
            '8': {
                name: 'Security',
                dataType: 'ST',
                required: false,
                maxLength: 40,
                description: 'Security information'
            },
            '9': {
                name: 'Message Type',
                dataType: 'MSG',
                required: true,
                description: 'Message type',
                binding: {
                    valueSet: 'messageTypes',
                    strength: 'required'
                }
            },
            '10': {
                name: 'Message Control ID',
                dataType: 'ST',
                required: true,
                maxLength: 20,
                description: 'Unique message identifier'
            },
            '11': {
                name: 'Processing ID',
                dataType: 'PT',
                required: true,
                description: 'Processing ID',
                binding: {
                    valueSet: 'processingIds',
                    strength: 'required'
                }
            },
            '12': {
                name: 'Version ID',
                dataType: 'VID',
                required: true,
                description: 'HL7 version',
                binding: {
                    valueSet: 'hl7Versions',
                    strength: 'required'
                }
            }
        };
    }

    /**
     * PID - Patient Identification Fields
     */
    getPIDFields() {
        return {
            '1': {
                name: 'Set ID',
                dataType: 'SI',
                required: false,
                description: 'Sequence number for patient ID segment'
            },
            '2': {
                name: 'Patient ID (External)',
                dataType: 'CX',
                required: false,
                description: 'External patient identifier'
            },
            '3': {
                name: 'Patient Identifier List',
                dataType: 'CX',
                required: true,
                repeating: true,
                description: 'Internal patient identifiers'
            },
            '4': {
                name: 'Alternate Patient ID',
                dataType: 'CX',
                required: false,
                repeating: true,
                description: 'Alternative patient identifiers'
            },
            '5': {
                name: 'Patient Name',
                dataType: 'XPN',
                required: true,
                repeating: true,
                description: 'Patient legal name'
            },
            '6': {
                name: 'Mother\'s Maiden Name',
                dataType: 'XPN',
                required: false,
                repeating: true,
                description: 'Mother\'s maiden name'
            },
            '7': {
                name: 'Date/Time of Birth',
                dataType: 'TS',
                required: false,
                description: 'Patient date of birth'
            },
            '8': {
                name: 'Administrative Sex',
                dataType: 'IS',
                required: false,
                description: 'Patient gender',
                binding: {
                    valueSet: 'administrativeSex',
                    strength: 'required'
                }
            },
            '9': {
                name: 'Patient Alias',
                dataType: 'XPN',
                required: false,
                repeating: true,
                description: 'Patient aliases'
            },
            '10': {
                name: 'Race',
                dataType: 'CE',
                required: false,
                repeating: true,
                description: 'Patient race',
                binding: {
                    valueSet: 'raceCategories',
                    strength: 'preferred'
                }
            },
            '11': {
                name: 'Patient Address',
                dataType: 'XAD',
                required: false,
                repeating: true,
                description: 'Patient address'
            },
            '12': {
                name: 'County Code',
                dataType: 'IS',
                required: false,
                description: 'County code'
            },
            '13': {
                name: 'Phone Number - Home',
                dataType: 'XTN',
                required: false,
                repeating: true,
                description: 'Home phone numbers'
            },
            '14': {
                name: 'Phone Number - Business',
                dataType: 'XTN',
                required: false,
                repeating: true,
                description: 'Business phone numbers'
            },
            '15': {
                name: 'Primary Language',
                dataType: 'CE',
                required: false,
                description: 'Primary language',
                binding: {
                    valueSet: 'languages',
                    strength: 'preferred'
                }
            },
            '16': {
                name: 'Marital Status',
                dataType: 'CE',
                required: false,
                description: 'Marital status',
                binding: {
                    valueSet: 'maritalStatus',
                    strength: 'required'
                }
            },
            '17': {
                name: 'Religion',
                dataType: 'CE',
                required: false,
                description: 'Religious affiliation',
                binding: {
                    valueSet: 'religions',
                    strength: 'preferred'
                }
            },
            '18': {
                name: 'Patient Account Number',
                dataType: 'CX',
                required: false,
                description: 'Patient account number'
            },
            '19': {
                name: 'SSN Number',
                dataType: 'ST',
                required: false,
                maxLength: 16,
                description: 'Social Security Number',
                pattern: {
                    regex: '^\\d{3}-?\\d{2}-?\\d{4}$',
                    description: 'Valid SSN format (XXX-XX-XXXX)'
                }
            },
            '20': {
                name: 'Driver\'s License Number',
                dataType: 'DLN',
                required: false,
                description: 'Driver\'s license number'
            },
            '21': {
                name: 'Mother\'s Identifier',
                dataType: 'CX',
                required: false,
                repeating: true,
                description: 'Mother\'s patient identifiers'
            },
            '22': {
                name: 'Ethnic Group',
                dataType: 'CE',
                required: false,
                repeating: true,
                description: 'Ethnic group',
                binding: {
                    valueSet: 'ethnicGroups',
                    strength: 'preferred'
                }
            },
            '23': {
                name: 'Birth Place',
                dataType: 'ST',
                required: false,
                maxLength: 250,
                description: 'Birth place'
            },
            '24': {
                name: 'Multiple Birth Indicator',
                dataType: 'ID',
                required: false,
                description: 'Multiple birth indicator',
                binding: {
                    valueSet: 'yesNoIndicator',
                    strength: 'required'
                }
            },
            '25': {
                name: 'Birth Order',
                dataType: 'NM',
                required: false,
                description: 'Birth order'
            },
            '26': {
                name: 'Citizenship',
                dataType: 'CE',
                required: false,
                repeating: true,
                description: 'Citizenship',
                binding: {
                    valueSet: 'countries',
                    strength: 'preferred'
                }
            },
            '27': {
                name: 'Veterans Military Status',
                dataType: 'CE',
                required: false,
                description: 'Veterans military status'
            },
            '28': {
                name: 'Nationality',
                dataType: 'CE',
                required: false,
                description: 'Nationality',
                binding: {
                    valueSet: 'countries',
                    strength: 'preferred'
                }
            },
            '29': {
                name: 'Patient Death Date and Time',
                dataType: 'TS',
                required: false,
                description: 'Patient death date and time'
            },
            '30': {
                name: 'Patient Death Indicator',
                dataType: 'ID',
                required: false,
                description: 'Patient death indicator',
                binding: {
                    valueSet: 'yesNoIndicator',
                    strength: 'required'
                }
            }
        };
    }

    /**
     * PV1 - Patient Visit Fields
     */
    getPV1Fields() {
        return {
            '1': {
                name: 'Set ID',
                dataType: 'SI',
                required: false,
                description: 'Sequence number for visit segment'
            },
            '2': {
                name: 'Patient Class',
                dataType: 'IS',
                required: true,
                description: 'Patient class (I=Inpatient, O=Outpatient, E=Emergency, etc.)',
                binding: {
                    valueSet: 'patientClass',
                    strength: 'required'
                }
            },
            '3': {
                name: 'Assigned Patient Location',
                dataType: 'PL',
                required: false,
                description: 'Patient location (room, bed, etc.)'
            },
            '4': {
                name: 'Admission Type',
                dataType: 'IS',
                required: false,
                description: 'Type of admission',
                binding: {
                    valueSet: 'admissionTypes',
                    strength: 'required'
                }
            },
            '5': {
                name: 'Preadmit Number',
                dataType: 'CX',
                required: false,
                description: 'Preadmission identifier'
            },
            '6': {
                name: 'Prior Patient Location',
                dataType: 'PL',
                required: false,
                description: 'Previous patient location'
            },
            '7': {
                name: 'Attending Doctor',
                dataType: 'XCN',
                required: false,
                repeating: true,
                description: 'Attending physician'
            },
            '8': {
                name: 'Referring Doctor',
                dataType: 'XCN',
                required: false,
                repeating: true,
                description: 'Referring physician'
            },
            '9': {
                name: 'Consulting Doctor',
                dataType: 'XCN',
                required: false,
                repeating: true,
                description: 'Consulting physician'
            },
            '10': {
                name: 'Hospital Service',
                dataType: 'IS',
                required: false,
                description: 'Hospital service',
                binding: {
                    valueSet: 'hospitalServices',
                    strength: 'preferred'
                }
            },
            '11': {
                name: 'Temporary Location',
                dataType: 'PL',
                required: false,
                description: 'Temporary location'
            },
            '12': {
                name: 'Preadmit Test Indicator',
                dataType: 'IS',
                required: false,
                description: 'Preadmit test indicator'
            },
            '13': {
                name: 'Re-admission Indicator',
                dataType: 'IS',
                required: false,
                description: 'Re-admission indicator'
            },
            '14': {
                name: 'Admit Source',
                dataType: 'IS',
                required: false,
                description: 'Admission source',
                binding: {
                    valueSet: 'admitSources',
                    strength: 'preferred'
                }
            },
            '15': {
                name: 'Ambulatory Status',
                dataType: 'IS',
                required: false,
                repeating: true,
                description: 'Ambulatory status'
            },
            '16': {
                name: 'VIP Indicator',
                dataType: 'IS',
                required: false,
                description: 'VIP indicator'
            },
            '17': {
                name: 'Admitting Doctor',
                dataType: 'XCN',
                required: false,
                repeating: true,
                description: 'Admitting physician'
            },
            '18': {
                name: 'Patient Type',
                dataType: 'IS',
                required: false,
                description: 'Patient type'
            },
            '19': {
                name: 'Visit Number',
                dataType: 'CX',
                required: false,
                description: 'Visit number'
            },
            '20': {
                name: 'Financial Class',
                dataType: 'FC',
                required: false,
                repeating: true,
                description: 'Financial class'
            },
            '36': {
                name: 'Discharge Disposition',
                dataType: 'IS',
                required: false,
                description: 'Discharge disposition',
                binding: {
                    valueSet: 'dischargeDispositions',
                    strength: 'preferred'
                }
            },
            '44': {
                name: 'Admit Date/Time',
                dataType: 'TS',
                required: false,
                description: 'Admission date and time'
            },
            '45': {
                name: 'Discharge Date/Time',
                dataType: 'TS',
                required: false,
                description: 'Discharge date and time'
            }
        };
    }

    /**
     * Additional segment field definitions
     */
    getEVNFields() {
        return {
            '1': {
                name: 'Event Type Code',
                dataType: 'ID',
                required: false,
                description: 'Event type code'
            },
            '2': {
                name: 'Recorded Date/Time',
                dataType: 'TS',
                required: true,
                description: 'When event was recorded'
            },
            '3': {
                name: 'Date/Time Planned Event',
                dataType: 'TS',
                required: false,
                description: 'When event was planned'
            },
            '4': {
                name: 'Event Reason Code',
                dataType: 'IS',
                required: false,
                description: 'Reason for event'
            },
            '5': {
                name: 'Operator ID',
                dataType: 'XCN',
                required: false,
                repeating: true,
                description: 'Who performed the event'
            },
            '6': {
                name: 'Event Occurred',
                dataType: 'TS',
                required: false,
                description: 'When event actually occurred'
            }
        };
    }

    getAL1Fields() {
        return {
            '1': {
                name: 'Set ID',
                dataType: 'SI',
                required: true,
                description: 'Sequence number'
            },
            '2': {
                name: 'Allergen Type Code',
                dataType: 'IS',
                required: false,
                description: 'Type of allergen',
                binding: {
                    valueSet: 'allergenTypes',
                    strength: 'required'
                }
            },
            '3': {
                name: 'Allergen Code/Mnemonic/Description',
                dataType: 'CE',
                required: true,
                description: 'Allergen identification'
            },
            '4': {
                name: 'Allergy Severity Code',
                dataType: 'IS',
                required: false,
                description: 'Severity of allergic reaction',
                binding: {
                    valueSet: 'allergySeverity',
                    strength: 'required'
                }
            },
            '5': {
                name: 'Allergy Reaction Code',
                dataType: 'ST',
                required: false,
                repeating: true,
                description: 'Reaction to allergen'
            },
            '6': {
                name: 'Identification Date',
                dataType: 'DT',
                required: false,
                description: 'When allergy was identified'
            }
        };
    }

    // Add more segment field definitions as needed...
    getOBXFields() {
        return {
            '1': {
                name: 'Set ID',
                dataType: 'SI',
                required: false,
                description: 'Sequence number'
            },
            '2': {
                name: 'Value Type',
                dataType: 'ID',
                required: false,
                description: 'Data type of observation value',
                binding: {
                    valueSet: 'observationValueTypes',
                    strength: 'required'
                }
            },
            '3': {
                name: 'Observation Identifier',
                dataType: 'CE',
                required: true,
                description: 'Observation/test identifier'
            },
            '4': {
                name: 'Observation Sub-ID',
                dataType: 'ST',
                required: false,
                description: 'Observation sub-identifier'
            },
            '5': {
                name: 'Observation Value',
                dataType: 'Varies',
                required: false,
                repeating: true,
                description: 'Observation value/result'
            },
            '6': {
                name: 'Units',
                dataType: 'CE',
                required: false,
                description: 'Units of measure'
            },
            '7': {
                name: 'References Range',
                dataType: 'ST',
                required: false,
                description: 'Normal reference range'
            },
            '8': {
                name: 'Abnormal Flags',
                dataType: 'IS',
                required: false,
                repeating: true,
                description: 'Abnormal flags',
                binding: {
                    valueSet: 'abnormalFlags',
                    strength: 'required'
                }
            },
            '11': {
                name: 'Observation Result Status',
                dataType: 'ID',
                required: true,
                description: 'Result status',
                binding: {
                    valueSet: 'observationResultStatus',
                    strength: 'required'
                }
            },
            '14': {
                name: 'Date/Time of Observation',
                dataType: 'TS',
                required: false,
                description: 'When observation was made'
            }
        };
    }

    // Placeholder methods for other segments
    getPD1Fields() { return {}; }
    getROLFields() { return {}; }
    getNK1Fields() { return {}; }
    getPV2Fields() { return {}; }
    getDG1Fields() { return {}; }
    getGT1Fields() { return {}; }
    getIN1Fields() { return {}; }
    getORCFields() { return {}; }
    getOBRFields() { return {}; }
    getTXAFields() { return {}; }

    /**
     * Initialize value sets/code tables
     */
    initializeValueSets() {
        this.valueSets.set('administrativeSex', {
            name: 'Administrative Sex',
            values: [
                { code: 'F', displayName: 'Female' },
                { code: 'M', displayName: 'Male' },
                { code: 'O', displayName: 'Other' },
                { code: 'U', displayName: 'Unknown' }
            ]
        });

        this.valueSets.set('patientClass', {
            name: 'Patient Class',
            values: [
                { code: 'E', displayName: 'Emergency' },
                { code: 'I', displayName: 'Inpatient' },
                { code: 'O', displayName: 'Outpatient' },
                { code: 'P', displayName: 'Preadmit' },
                { code: 'R', displayName: 'Recurring patient' },
                { code: 'B', displayName: 'Obstetrics' },
                { code: 'C', displayName: 'Commercial Account' },
                { code: 'N', displayName: 'Not Applicable' }
            ]
        });

        this.valueSets.set('yesNoIndicator', {
            name: 'Yes/No Indicator',
            values: [
                { code: 'Y', displayName: 'Yes' },
                { code: 'N', displayName: 'No' }
            ]
        });

        this.valueSets.set('maritalStatus', {
            name: 'Marital Status',
            values: [
                { code: 'A', displayName: 'Separated' },
                { code: 'D', displayName: 'Divorced' },
                { code: 'M', displayName: 'Married' },
                { code: 'S', displayName: 'Single' },
                { code: 'W', displayName: 'Widowed' },
                { code: 'C', displayName: 'Common law' },
                { code: 'G', displayName: 'Living together' },
                { code: 'P', displayName: 'Domestic partner' },
                { code: 'R', displayName: 'Registered domestic partner' },
                { code: 'E', displayName: 'Legally Separated' },
                { code: 'N', displayName: 'Annulled' },
                { code: 'I', displayName: 'Interlocutory' },
                { code: 'B', displayName: 'Unmarried' },
                { code: 'U', displayName: 'Unknown' },
                { code: 'O', displayName: 'Other' },
                { code: 'T', displayName: 'Unreported' }
            ]
        });

        this.valueSets.set('admissionTypes', {
            name: 'Admission Types',
            values: [
                { code: 'A', displayName: 'Accident' },
                { code: 'E', displayName: 'Emergency' },
                { code: 'L', displayName: 'Labor and Delivery' },
                { code: 'R', displayName: 'Routine' },
                { code: 'N', displayName: 'Newborn' },
                { code: 'U', displayName: 'Urgent' },
                { code: 'C', displayName: 'Elective' }
            ]
        });

        this.valueSets.set('allergenTypes', {
            name: 'Allergen Types',
            values: [
                { code: 'DA', displayName: 'Drug allergy' },
                { code: 'FA', displayName: 'Food allergy' },
                { code: 'MA', displayName: 'Miscellaneous allergy' },
                { code: 'MC', displayName: 'Miscellaneous contraindication' },
                { code: 'EA', displayName: 'Environmental Allergy' },
                { code: 'AA', displayName: 'Animal Allergy' }
            ]
        });

        this.valueSets.set('allergySeverity', {
            name: 'Allergy Severity',
            values: [
                { code: 'SV', displayName: 'Severe' },
                { code: 'MO', displayName: 'Moderate' },
                { code: 'MI', displayName: 'Mild' },
                { code: 'U', displayName: 'Unknown' }
            ]
        });

        this.valueSets.set('observationResultStatus', {
            name: 'Observation Result Status',
            values: [
                { code: 'C', displayName: 'Corrected' },
                { code: 'D', displayName: 'Deleted' },
                { code: 'F', displayName: 'Final' },
                { code: 'I', displayName: 'Specimen in lab; results pending' },
                { code: 'P', displayName: 'Preliminary' },
                { code: 'R', displayName: 'Results entered -- not verified' },
                { code: 'S', displayName: 'Partial results' },
                { code: 'X', displayName: 'Results cannot be obtained for this observation' },
                { code: 'U', displayName: 'Results status change to final without retransmitting results already sent as preliminary' },
                { code: 'W', displayName: 'Post original as wrong, e.g., transmitted for wrong patient' }
            ]
        });

        this.valueSets.set('abnormalFlags', {
            name: 'Abnormal Flags',
            values: [
                { code: 'L', displayName: 'Below low normal' },
                { code: 'H', displayName: 'Above high normal' },
                { code: 'LL', displayName: 'Below lower panic limits' },
                { code: 'HH', displayName: 'Above upper panic limits' },
                { code: '<', displayName: 'Below absolute low-off instrument scale' },
                { code: '>', displayName: 'Above absolute high-off instrument scale' },
                { code: 'N', displayName: 'Normal' },
                { code: 'A', displayName: 'Abnormal' },
                { code: 'AA', displayName: 'Very abnormal' },
                { code: 'null', displayName: 'No range defined, or normal ranges don\'t apply' },
                { code: 'U', displayName: 'Significant change up' },
                { code: 'D', displayName: 'Significant change down' },
                { code: 'B', displayName: 'Better' },
                { code: 'W', displayName: 'Worse' }
            ]
        });

        // Add more value sets as needed
        this.valueSets.set('messageTypes', {
            name: 'Message Types',
            values: [
                { code: 'ADT^A01', displayName: 'Admit/Visit Notification' },
                { code: 'ADT^A02', displayName: 'Transfer a Patient' },
                { code: 'ADT^A03', displayName: 'Discharge/End Visit' },
                { code: 'ADT^A04', displayName: 'Register a Patient' },
                { code: 'ADT^A05', displayName: 'Pre-admit a Patient' },
                { code: 'ADT^A08', displayName: 'Update Patient Information' },
                { code: 'ORM^O01', displayName: 'Order Message' },
                { code: 'ORU^R01', displayName: 'Observation Result' },
                { code: 'MDM^T02', displayName: 'Document Status Change' }
            ]
        });

        this.valueSets.set('processingIds', {
            name: 'Processing IDs',
            values: [
                { code: 'D', displayName: 'Debugging' },
                { code: 'P', displayName: 'Production' },
                { code: 'T', displayName: 'Training' }
            ]
        });

        this.valueSets.set('hl7Versions', {
            name: 'HL7 Versions',
            values: [
                { code: '2.1', displayName: 'HL7 Version 2.1' },
                { code: '2.2', displayName: 'HL7 Version 2.2' },
                { code: '2.3', displayName: 'HL7 Version 2.3' },
                { code: '2.3.1', displayName: 'HL7 Version 2.3.1' },
                { code: '2.4', displayName: 'HL7 Version 2.4' },
                { code: '2.5', displayName: 'HL7 Version 2.5' },
                { code: '2.5.1', displayName: 'HL7 Version 2.5.1' },
                { code: '2.6', displayName: 'HL7 Version 2.6' },
                { code: '2.7', displayName: 'HL7 Version 2.7' },
                { code: '2.8', displayName: 'HL7 Version 2.8' }
            ]
        });
    }

    /**
     * Get schema by message type
     */
    getSchema(messageType) {
        return this.schemas.get(messageType);
    }

    /**
     * Get value set by name
     */
    getValueSet(valueSetName) {
        return this.valueSets.get(valueSetName);
    }

    /**
     * Get all available schemas
     */
    getAllSchemas() {
        return Array.from(this.schemas.keys()).map(key => ({
            key,
            name: this.schemas.get(key).name,
            description: this.schemas.get(key).description
        }));
    }

    /**
     * Get all available value sets
     */
    getAllValueSets() {
        return Array.from(this.valueSets.keys()).map(key => ({
            key,
            name: this.valueSets.get(key).name,
            valueCount: this.valueSets.get(key).values.length
        }));
    }

    /**
     * Add custom schema
     */
    addSchema(messageType, schema) {
        this.schemas.set(messageType, schema);
    }

    /**
     * Add custom value set
     */
    addValueSet(name, valueSet) {
        this.valueSets.set(name, valueSet);
    }

    /**
     * Validate schema structure
     */
    validateSchema(schema) {
        const errors = [];
        
        if (!schema.name) {
            errors.push('Schema must have a name');
        }
        
        if (!schema.segments || typeof schema.segments !== 'object') {
            errors.push('Schema must have segments object');
        }
        
        // Validate each segment
        for (const [segmentName, segment] of Object.entries(schema.segments || {})) {
            if (!segment.fields || typeof segment.fields !== 'object') {
                errors.push(`Segment ${segmentName} must have fields object`);
            }
            
            // Validate each field
            for (const [fieldPos, field] of Object.entries(segment.fields || {})) {
                if (!field.dataType) {
                    errors.push(`Field ${segmentName}.${fieldPos} must have dataType`);
                }
            }
        }
        
        return {
            isValid: errors.length === 0,
            errors
        };
    }
}

// Export for use in other modules
export { HL7Schemas };