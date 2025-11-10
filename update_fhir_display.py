#!/usr/bin/env python3
"""
Update FHIR display to show only deliveryPayload with copy button
and add transformation steps visualization
"""
import sys
sys.stdout.reconfigure(encoding='utf-8')

with open('public/js/messages.js', 'r', encoding='utf-8') as f:
    content = f.read()

# Find the line where we format the FHIR output
old_format = '''                                    this.formatMessageContent(JSON.stringify(output.transformedMessage))'''

# Replace with deliveryPayload extraction and copy button
new_format = '''                                    this.renderFHIRBundle(output.transformedMessage)'''

content = content.replace(old_format, new_format)

# Now add the renderFHIRBundle helper method after formatMessageContent
insert_point = content.find('    formatMessageContent(')
if insert_point > 0:
    # Find the end of formatMessageContent method
    brace_count = 0
    started = False
    end_point = insert_point
    for i in range(insert_point, len(content)):
        if content[i] == '{':
            brace_count += 1
            started = True
        elif content[i] == '}':
            brace_count -= 1
            if started and brace_count == 0:
                end_point = i + 1
                break

    # Add the new method after formatMessageContent
    new_method = '''

    renderFHIRBundle(fhirData) {
        if (!fhirData) return '<div style="padding: 2rem; text-align: center; color: #cbd5e1;">No FHIR data</div>';

        // Extract deliveryPayload if it exists, otherwise use the whole object
        const bundleToShow = fhirData.deliveryPayload || fhirData.fhirBundle || fhirData;
        const bundleStr = JSON.stringify(bundleToShow, null, 2);
        const bundleId = 'fhir-bundle-' + Date.now();

        return `
            <div style="position: relative;">
                <div style="position: absolute; top: 0.5rem; right: 0.5rem; z-index: 10;">
                    <button onclick="messageManager.copyToClipboard('${bundleId}')"
                            style="background: #ec4899; color: white; border: none; padding: 0.5rem 1rem; border-radius: 6px; cursor: pointer; font-size: 0.85rem; display: flex; align-items: center; gap: 0.5rem; box-shadow: 0 2px 4px rgba(0,0,0,0.1);"
                            onmouseover="this.style.background='#db2777'"
                            onmouseout="this.style.background='#ec4899'">
                        <svg style="width: 16px; height: 16px;" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8 16H6a2 2 0 01-2-2V6a2 2 0 012-2h8a2 2 0 012 2v2m-6 12h8a2 2 0 002-2v-8a2 2 0 00-2-2h-8a2 2 0 00-2 2v8a2 2 0 002 2z"/>
                        </svg>
                        Copy FHIR Bundle
                    </button>
                </div>
                <pre id="${bundleId}" style="background: #f8fafc; padding: 1rem; padding-top: 3rem; border-radius: 6px; border: 1px solid #e2e8f0; max-height: 500px; overflow: auto; margin: 0;"><code style="color: #1e293b; font-size: 0.85rem;">${this.escapeHtml(bundleStr)}</code></pre>
            </div>
        `;
    }

    copyToClipboard(elementId) {
        const element = document.getElementById(elementId);
        if (!element) return;

        const text = element.textContent;
        navigator.clipboard.writeText(text).then(() => {
            // Show success message
            const btn = event.target.closest('button');
            const originalHTML = btn.innerHTML;
            btn.innerHTML = `
                <svg style="width: 16px; height: 16px;" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7"/>
                </svg>
                Copied!
            `;
            setTimeout(() => {
                btn.innerHTML = originalHTML;
            }, 2000);
        }).catch(err => {
            console.error('Failed to copy:', err);
        });
    }
'''

    content = content[:end_point] + new_method + content[end_point:]
    print("Added renderFHIRBundle and copyToClipboard methods")

with open('public/js/messages.js', 'w', encoding='utf-8') as f:
    f.write(content)

print("Done!")
