class Step4Modals {
    constructor(handler) {
        this.handler = handler;
    }

    setupEventListeners() {
        document.addEventListener('click', (e) => {
            if (e.target.classList.contains('json-tab')) {
                this.handler.jsonViewer.switchTab(e.target.dataset.tab);
            }
        });
    }

    openValidationSidebar() {
        const sidebar = document.getElementById('validationSidebar');
        if (sidebar) {
            sidebar.classList.add('show');
            this.loadValidationResults();
        }
    }

    closeValidationSidebar() {
        const sidebar = document.getElementById('validationSidebar');
        if (sidebar) sidebar.classList.remove('show');
    }

    loadValidationResults() {
        const content = document.getElementById('validationContent');
        if (!content) return;

        const validation = this.handler.validation.getValidationResults();
        content.innerHTML = validation.html;
    }
}
