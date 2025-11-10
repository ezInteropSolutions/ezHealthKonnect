import re

# Read the file
with open('public/js/messages.js', 'r', encoding='utf-8') as f:
    content = f.read()

# 1. Fix the copy button in renderFHIRBundle - make it smaller with standard copy icon
old_copy_button = r'''<button onclick="messageManager\.copyToClipboard\('([^']+)'\)"
                            style="background: #ec4899; color: white; border: none; padding: 0\.5rem 1rem; border-radius: 6px; cursor: pointer;">
                        Copy FHIR Bundle
                    </button>'''

new_copy_button = r'''<button onclick="messageManager.copyToClipboard('\1')"
                            style="background: #ec4899; color: white; border: none; padding: 0.4rem 0.75rem; border-radius: 4px; cursor: pointer; font-size: 0.85rem; display: flex; align-items: center; gap: 0.4rem;"
                            title="Copy to clipboard">
                        <svg style="width: 14px; height: 14px;" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8 16H6a2 2 0 01-2-2V6a2 2 0 012-2h8a2 2 0 012 2v2m-6 12h8a2 2 0 002-2v-8a2 2 0 00-2-2h-8a2 2 0 00-2 2v8a2 2 0 002 2z"/>
                        </svg>
                        Copy
                    </button>'''

content = re.sub(old_copy_button, new_copy_button, content, flags=re.MULTILINE | re.DOTALL)

# 2. Update copyToClipboard to handle button text change
old_clipboard = r"btn\.innerHTML = 'Copied!';"
new_clipboard_copied = '''btn.innerHTML = `<svg style="width: 14px; height: 14px;" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7"/>
                        </svg> Copied!`;'''

content = content.replace(old_clipboard, new_clipboard_copied)

old_clipboard_restore = r"btn\.innerHTML = 'Copy FHIR Bundle';"
new_clipboard_restore = '''btn.innerHTML = `<svg style="width: 14px; height: 14px;" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8 16H6a2 2 0 01-2-2V6a2 2 0 012-2h8a2 2 0 012 2v2m-6 12h8a2 2 0 002-2v-8a2 2 0 00-2-2h-8a2 2 0 00-2 2v8a2 2 0 002 2z"/>
                        </svg> Copy`;'''

content = content.replace(old_clipboard_restore, new_clipboard_restore)

# 3. Add copy button to raw message display
# Find the raw message pre tag and add copy button above it
raw_message_pattern = r"(<div class=\"lineage-step-content\">[\s\S]*?Step 1: Message Reception[\s\S]*?<pre style=\"[^\"]*\">)"

def add_raw_copy_button(match):
    pre_tag = match.group(1)
    # Insert copy button container before the pre tag
    button_html = '''<div style="display: flex; justify-content: flex-end; margin-bottom: 0.5rem;">
                        <button onclick="messageManager.copyRawMessage()"
                                style="background: #ec4899; color: white; border: none; padding: 0.4rem 0.75rem; border-radius: 4px; cursor: pointer; font-size: 0.85rem; display: flex; align-items: center; gap: 0.4rem;"
                                title="Copy raw message">
                            <svg style="width: 14px; height: 14px;" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8 16H6a2 2 0 01-2-2V6a2 2 0 012-2h8a2 2 0 012 2v2m-6 12h8a2 2 0 002-2v-8a2 2 0 00-2-2h-8a2 2 0 00-2 2v8a2 2 0 002 2z"/>
                            </svg>
                            Copy
                        </button>
                    </div>
                    ''' + pre_tag
    return button_html

content = re.sub(raw_message_pattern, add_raw_copy_button, content, count=1)

# 4. Add copyRawMessage method after copyToClipboard
copy_raw_method = '''

    copyRawMessage() {
        // Find the raw message content in Step 1
        const rawMessagePre = document.querySelector('.lineage-step-content pre');
        if (!rawMessagePre) return;

        const text = rawMessagePre.textContent;
        navigator.clipboard.writeText(text).then(() => {
            // Find the copy button and update it
            const btn = event.target.closest('button');
            const originalHTML = btn.innerHTML;
            btn.innerHTML = `<svg style="width: 14px; height: 14px;" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7"/>
                        </svg> Copied!`;
            setTimeout(() => {
                btn.innerHTML = originalHTML;
            }, 2000);
        }).catch(err => {
            console.error('Failed to copy:', err);
        });
    }'''

# Find the copyToClipboard method and add copyRawMessage after it
clipboard_method_end = content.find('    copyToClipboard(elementId) {')
if clipboard_method_end != -1:
    # Find the end of copyToClipboard method
    method_end = content.find('    }', clipboard_method_end + 100)
    if method_end != -1:
        content = content[:method_end + 5] + copy_raw_method + content[method_end + 5:]

# Write back
with open('public/js/messages.js', 'w', encoding='utf-8') as f:
    f.write(content)

print("Updated copy icons:")
print("  - Made FHIR copy button smaller with standard icon")
print("  - Added copy button for raw message")
print("  - Updated clipboard feedback with checkmark icon")
