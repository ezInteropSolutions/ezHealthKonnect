class Step4Mapping {
    constructor(handler) {
        this.handler = handler;
        this.editableMappings = new Map();
        this.currentEditMapping = null;
    }

    openEditModal(mappingData) {
        const modal = document.getElementById('editMappingOverlay');
        if (!modal) return;

        this.currentEditMapping = mappingData;
        this.populateEditModal(mappingData);
        modal.classList.add('show');
    }

    closeEditModal() {
        const modal = document.getElementById('editMappingOverlay');
        if (modal) {
            modal.classList.remove('show');
            this.currentEditMapping = null;
        }
    }

    populateEditModal(mapping) {
        // Update modal fields
        const hl7Path = document.getElementById('currentHL7Path');
        const hl7Value = document.getElementById('currentHL7Value');
        const fhirPath = document.getElementById('currentFHIRPath');
        const fhirValue = document.getElementById('currentFHIRValue');

        if (hl7Path) hl7Path.textContent = mapping.hl7Field || 'N/A';
        if (hl7Value) hl7Value.textContent = `"${mapping.hl7Value || ''}"`;
        if (fhirPath) fhirPath.textContent = mapping.fhirPath || 'N/A';
        if (fhirValue) fhirValue.textContent = `"${mapping.fhirValue || ''}"`;

        // Populate dropdowns
        this.populateHL7Fields();
        this.populateFHIRFields();
    }

    populateHL7Fields() {
        const select = document.getElementById('newHL7Field');
        if (!select) return;

        const parsedData = this.getParsedHL7Data();
        if (!parsedData?.enhancedSegments) return;

        let options = '<option value="">Select HL7 Field...</option>';
        
        Object.entries(parsedData.enhancedSegments).forEach(([segment, data]) => {
            if (data.fields) {
                options += `<optgroup label="${segment} Segment">`;
                Object.entries(data.fields).forEach(([fieldId, field]) => {
                    const path = `${segment}.${fieldId}`;
                    const desc = field.description || field.name || '';
                    options += `<option value="${path}">${path} - ${desc}</option>`;
                });
                options += '</optgroup>';
            }
        });

        select.innerHTML = options;
    }

    populateFHIRFields() {
        const select = document.getElementById('newFHIRField');
        if (!select) return;

        const fhirFields = {
            'Patient': [
                { path: 'Patient.id', desc: 'Patient ID' },
                { path: 'Patient.identifier[0].value', desc: 'MRN' },
                { path: 'Patient.name[0].family', desc: 'Family Name' },
                { path: 'Patient.name[0].given[0]', desc: 'Given Name' },
                { path: 'Patient.gender', desc: 'Gender' },
                { path: 'Patient.birthDate', desc: 'Date of Birth' },
                { path: 'Patient.address[0].line[0]', desc: 'Address Line' },
                { path: 'Patient.address[0].city', desc: 'City' },
                { path: 'Patient.address[0].state', desc: 'State' },
                { path: 'Patient.address[0].postalCode', desc: 'Zip Code' },
                { path: 'Patient.telecom[0].value', desc: 'Phone Number' }
            ],
            'Encounter': [
                { path: 'Encounter.status', desc: 'Status' },
                { path: 'Encounter.class.code', desc: 'Class' },
                { path: 'Encounter.subject.reference', desc: 'Patient Reference' },
                { path: 'Encounter.period.start', desc: 'Start Time' },
                { path: 'Encounter.period.end', desc: 'End Time' }
            ],
            'Observation': [
                { path: 'Observation.status', desc: 'Status' },
                { path: 'Observation.code.coding[0].code', desc: 'Code' },
                { path: 'Observation.code.coding[0].display', desc: 'Display' },
                { path: 'Observation.valueQuantity.value', desc: 'Value' },
                { path: 'Observation.valueQuantity.unit', desc: 'Unit' }
            ]
        };

        let options = '<option value="">Select FHIR Field...</option>';
        
        Object.entries(fhirFields).forEach(([resource, fields]) => {
            options += `<optgroup label="${resource}">`;
            fields.forEach(field => {
                options += `<option value="${field.path}">${field.path} - ${field.desc}</option>`;
            });
            options += '</optgroup>';
        });

        select.innerHTML = options;
    }

    async saveMapping() {
        if (!this.currentEditMapping) return;

        const newHL7Field = document.getElementById('newHL7Field')?.value;
        const newFHIRField = document.getElementById('newFHIRField')?.value;
        const transformType = document.querySelector('input[name="transformType"]:checked')?.value || 'direct';

        if (!newHL7Field || !newFHIRField) {
            this.handler.showNotification('Please select both HL7 and FHIR fields', 'warning');
            return;
        }

        // Save the mapping
        this.editableMappings.set(this.currentEditMapping.fhirPath, {
            originalHL7Field: this.currentEditMapping.hl7Field,
            originalFHIRField: this.currentEditMapping.fhirPath,
            newHL7Field: newHL7Field,
            newFHIRField: newFHIRField,
            transformType: transformType,
            timestamp: new Date().toISOString()
        });

        this.handler.showNotification(`✅ Mapping updated: ${newHL7Field} → ${newFHIRField}`, 'success');
        this.closeEditModal();
        
        // Refresh display
        await this.applyMappingChanges();
    }

    async applyMappingChanges() {
        // In a real implementation, this would re-transform the data
        console.log('🔄 Applying mapping changes:', this.editableMappings);
        
        // Refresh the resource display
        if (this.handler.resources) {
            this.handler.resources.displayResources(this.handler.transformationData);
        }
    }

    getParsedHL7Data() {
        return this.handler.wizard?.parsedHL7Data?.data || null;
    }

    extractDetailedMappings(resource, path = '') {
        const mappings = [];
        
        for (const [key, value] of Object.entries(resource)) {
            if (key === 'resourceType' || key === 'meta') continue;
            
            const currentPath = path ? `${path}.${key}` : key;
            
            if (value && typeof value === 'object' && !Array.isArray(value)) {
                mappings.push(...this.extractDetailedMappings(value, currentPath));
            } else if (Array.isArray(value)) {
                value.forEach((item, index) => {
                    if (typeof item === 'object') {
                        mappings.push(...this.extractDetailedMappings(item, `${currentPath}[${index}]`));
                    } else {
                        mappings.push({
                            fhirPath: `${currentPath}[${index}]`,
                            value: item,
                            hl7Path: this.getHL7PathForFHIR(currentPath)
                        });
                    }
                });
            } else if (value !== null && value !== undefined) {
                mappings.push({
                    fhirPath: currentPath,
                    value: value,
                    hl7Path: this.getHL7PathForFHIR(currentPath)
                });
            }
        }
        
        return mappings;
    }

    getHL7PathForFHIR(fhirPath) {
        // Mapping rules - would be loaded from configuration
        const mappingRules = {
            'id': 'PID.3',
            'identifier[0].value': 'PID.3.1',
            'name[0].family': 'PID.5.1',
            'name[0].given[0]': 'PID.5.2',
            'gender': 'PID.8',
            'birthDate': 'PID.7',
            'address[0].line[0]': 'PID.11.1',
            'address[0].city': 'PID.11.3',
            'address[0].state': 'PID.11.4',
            'address[0].postalCode': 'PID.11.5'
        };
        
        return mappingRules[fhirPath] || null;
    }

    reset() {
        this.editableMappings.clear();
        this.currentEditMapping = null;
    }
}