class Step4Resources {
    constructor(handler) {
        this.handler = handler;
        this.expandedResources = new Set();
    }

    setupEventListeners() {
        document.addEventListener('click', (e) => {
            if (e.target.closest('.resource-toggle')) {
                const card = e.target.closest('.fhir-resource-card');
                const index = card?.dataset.index;
                if (index) this.toggleResource(index);
            }
        });
    }

    displayResources(transformationData) {
        const container = document.getElementById('resourcesSection');
        if (!container || !transformationData?.fhirResources) return;

        container.innerHTML = '<h3>📦 FHIR Resources</h3>';
        
        transformationData.fhirResources.forEach((resource, index) => {
            container.appendChild(this.createResourceCard(resource, index));
        });
    }

    createResourceCard(resource, index) {
        const card = document.createElement('div');
        card.className = 'fhir-resource-card';
        card.dataset.index = index;
        
        card.innerHTML = `
            <div class="resource-header">
                <div class="resource-title">
                    <div class="resource-icon">${this.getResourceIcon(resource.resourceType)}</div>
                    <div>
                        <h4>${resource.resourceType}</h4>
                        <p>${this.getResourceDescription(resource)}</p>
                    </div>
                </div>
                <button class="resource-toggle">🔽</button>
            </div>
            <div class="resource-content">
                ${this.renderResourceMappings(resource)}
            </div>
        `;
        
        return card;
    }

    getResourceIcon(type) {
        const icons = {
            'Patient': '👤',
            'Encounter': '🏥',
            'Observation': '🔬',
            'Bundle': '📦'
        };
        return icons[type] || '📄';
    }

    getResourceDescription(resource) {
        if (resource.resourceType === 'Patient' && resource.name?.[0]) {
            const name = resource.name[0];
            return `${name.given?.join(' ') || ''} ${name.family || ''}`.trim();
        }
        return resource.id || 'Generated Resource';
    }

    renderResourceMappings(resource) {
        // Simplified mapping display
        return '<p>Mapping details...</p>';
    }

    toggleResource(index) {
        const card = document.querySelector(`[data-index="${index}"]`);
        if (card) {
            card.classList.toggle('collapsed');
            this.expandedResources.has(index) 
                ? this.expandedResources.delete(index)
                : this.expandedResources.add(index);
        }
    }

    reset() {
        this.expandedResources.clear();
    }
}