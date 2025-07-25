// js/core/wizard-functions.js - Extracted core wizard functions
// CRITICAL: These functions MUST remain global for HTML onclick handlers

/**
 * Core Wizard Functions - Global Scope
 * These functions are called directly from HTML and must remain accessible
 */

// ✅ PRESERVED: Core wizard control functions
function openInterfaceWizard() {
    console.log('🧙‍♂️ Opening Interface Wizard...');
    const wizardOverlay = document.getElementById('wizardModalOverlay');
    if (wizardOverlay) {
        wizardOverlay.classList.add('show');
        console.log('✅ Wizard modal opened successfully');
    } else {
        console.error('❌ Wizard modal overlay not found');
    }
}

function closeWizard() {
    const wizardOverlay = document.getElementById('wizardModalOverlay');
    if (wizardOverlay) {
        wizardOverlay.classList.remove('show');
    }
}

// ✅ PRESERVED: FHIR Mapping Functions (called from HTML onclick)
function toggleResourceCard(button) {
    const card = button.closest('.fhir-resource-card');
    const icon = button.querySelector('.toggle-icon');
    
    card.classList.toggle('collapsed');
    
    if (card.classList.contains('collapsed')) {
        icon.style.transform = 'rotate(0deg)';
    } else {
        icon.style.transform = 'rotate(90deg)';
    }
}

function openMappingDialog(fhirPath) {
    if (window.fhirMapping) {
        window.fhirMapping.openMappingDialog(fhirPath);
    }
}

function closeMappingDialog() {
    if (window.fhirMapping) {
        window.fhirMapping.closeMappingDialog();
    }
}

function saveMapping() {
    if (window.fhirMapping) {
        window.fhirMapping.saveMapping();
    }
}

// ✅ PRESERVED: Configuration Modal Functions (called from HTML onclick)
function closeLoadConfigModal() {
    if (window.fhirMapping) {
        window.fhirMapping.closeLoadConfigModal();
    }
}

function closeSaveConfigModal() {
    if (window.fhirMapping) {
        window.fhirMapping.closeSaveConfigModal();
    }
}

function selectConfiguration(configId) {
    // Clear previous selection
    document.querySelectorAll('.config-item.selected').forEach(el => {
        el.classList.remove('selected');
    });
    
    // Select clicked configuration
    const configItem = document.querySelector(`[data-config-id="${configId}"]`);
    if (configItem) {
        configItem.classList.add('selected');
        const loadBtn = document.getElementById('loadSelectedBtn');
        if (loadBtn) {
            loadBtn.disabled = false;
        }
    }
}

function loadSelectedConfig() {
    if (window.fhirMapping) {
        window.fhirMapping.loadSelectedConfiguration();
    }
}

function saveConfigurationToDb() {
    if (window.fhirMapping) {
        window.fhirMapping.saveConfigurationToDb();
    }
}

// ✅ PRESERVED: Modal Functions (called from HTML onclick)
function closeCreateModal() {
    const modal = document.getElementById('createModal');
    if (modal) {
        modal.style.display = 'none';
    }
}

function closeEditModal() {
    const modal = document.getElementById('editModal');
    if (modal) {
        modal.style.display = 'none';
    }
}

function closeDetailsModal() {
    const modal = document.getElementById('detailsModal');
    if (modal) {
        modal.style.display = 'none';
    }
}

// ✅ PRESERVED: User Management (called from HTML onclick)
function logout() {
    if (confirm('Are you sure you want to logout?')) {
        // Call existing logout functionality
        if (window.location) {
            window.location.href = '/auth/logout';
        }
    }
}

// ✅ PRESERVED: Environment Configuration Helper
function getApiBaseUrl() {
    // Check for environment variables in different formats
    const envVars = [
        'API_BASE_URL',
        'REACT_APP_API_URL', 
        'VUE_APP_API_URL',
        'VITE_API_URL'
    ];
    
    // Try window.ENV first (if you expose .env to frontend)
    if (typeof window !== 'undefined' && window.ENV) {
        for (const varName of envVars) {
            if (window.ENV[varName]) {
                console.log(`🔧 Using API URL from window.ENV.${varName}: ${window.ENV[varName]}`);
                return window.ENV[varName];
            }
        }
    }
    
    // Try process.env (if available in build)
    if (typeof process !== 'undefined' && process.env) {
        for (const varName of envVars) {
            if (process.env[varName]) {
                console.log(`🔧 Using API URL from process.env.${varName}: ${process.env[varName]}`);
                return process.env[varName];
            }
        }
    }
    
    // Dynamic fallback
    const currentHost = window.location.hostname;
    const apiPort = (typeof process !== 'undefined' && process.env.API_PORT) || 
                   (typeof window !== 'undefined' && window.ENV?.API_PORT) || 
                   '8080';
    const apiUrl = `http://${currentHost}:${apiPort}`;
    
    console.log(`🔧 Using dynamic API URL: ${apiUrl}`);
    return apiUrl;
}

// ✅ PRESERVED: FHIRFocusedMapping conditional loading
async function loadHL7Structure(messageType) {
    // Don't load if no message type specified
    if (!messageType || messageType === 'auto-detect') {
        console.log('⏸️ Skipping HL7 structure load - no message type specified');
        return;
    }
    
    try {
        console.log(`🔍 Loading HL7 structure for: ${messageType}`);
        
        // Use correct API URL with environment variables
        const apiBaseUrl = getApiBaseUrl();
        const response = await fetch(`${apiBaseUrl}/api/fhir/transform/structure/hl7/${encodeURIComponent(messageType)}`);
        
        if (!response.ok) {
            throw new Error(`HTTP ${response.status}: ${response.statusText}`);
        }
        
        const structure = await response.json();
        console.log('✅ HL7 structure loaded successfully');
        return structure;
    } catch (error) {
        console.warn(`⚠️ Could not load HL7 structure for ${messageType}:`, error.message);
        // Don't show error to user on page load - this is expected
        return null;
    }
}

async function loadDefaultConfiguration(messageType) {
    // Don't load if no message type specified
    if (!messageType || messageType === 'auto-detect') {
        console.log('⏸️ Skipping default configuration load - no message type specified');
        return;
    }
    
    try {
        console.log(`🔍 Loading default configuration for: ${messageType}`);
        
        // Use correct API URL with environment variables
        const apiBaseUrl = getApiBaseUrl();
        const response = await fetch(`${apiBaseUrl}/api/wizard/mapping/rules/${encodeURIComponent(messageType)}?default=true`);
        
        if (!response.ok) {
            throw new Error(`HTTP ${response.status}: ${response.statusText}`);
        }
        
        const config = await response.json();
        console.log('✅ Default configuration loaded successfully');
        return config;
    } catch (error) {
        console.warn(`⚠️ Could not load default configuration for ${messageType}:`, error.message);
        // Don't show error to user on page load - this is expected
        return null;
    }
}

// ✅ PRESERVED: FHIRFocusedMapping initialization
function FHIRFocusedMapping(messageType = null) {
    console.log('🔧 Initializing FHIR Focused Mapping...');
    
    // Only load configuration if messageType is provided
    if (messageType && messageType !== 'auto-detect') {
        // Initialize the mapping class when needed
        if (!window.fhirMapping) {
            // Lazy load the FHIRFocusedMapping class
            if (typeof FHIRFocusedMappingClass !== 'undefined') {
                window.fhirMapping = new FHIRFocusedMappingClass();
            }
        }
    } else {
        console.log('⏸️ FHIR mapping initialized but not loading configuration until message type is available');
    }
}

// ✅ MAKE FUNCTIONS GLOBALLY ACCESSIBLE
// This ensures all functions remain available for HTML onclick handlers
window.openInterfaceWizard = openInterfaceWizard;
window.closeWizard = closeWizard;
window.toggleResourceCard = toggleResourceCard;
window.openMappingDialog = openMappingDialog;
window.closeMappingDialog = closeMappingDialog;
window.saveMapping = saveMapping;
window.closeLoadConfigModal = closeLoadConfigModal;
window.closeSaveConfigModal = closeSaveConfigModal;
window.selectConfiguration = selectConfiguration;
window.loadSelectedConfig = loadSelectedConfig;
window.saveConfigurationToDb = saveConfigurationToDb;
window.closeCreateModal = closeCreateModal;
window.closeEditModal = closeEditModal;
window.closeDetailsModal = closeDetailsModal;
window.logout = logout;
window.getApiBaseUrl = getApiBaseUrl;
window.loadHL7Structure = loadHL7Structure;
window.loadDefaultConfiguration = loadDefaultConfiguration;
window.FHIRFocusedMapping = FHIRFocusedMapping;

console.log('✅ Core wizard functions loaded and globally accessible');