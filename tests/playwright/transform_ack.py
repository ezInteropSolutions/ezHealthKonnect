"""
Transform ack-nack-e2e.spec.js:
Convert 6 failing describe blocks from beforeAll/shared page to per-test page fixture.
"""
import re

with open('c:/Projects/ezHealthKonnect/tests/playwright/ack-nack-e2e.spec.js', 'r', encoding='utf-8') as f:
    content = f.read()

# ── Helper: generic replacement with assertion ──────────────────────────────
def replace_once(old, new, label):
    global content
    if old not in content:
        print(f"WARNING: '{label}' pattern NOT FOUND")
        return
    content = content.replace(old, new, 1)
    print(f"OK: replaced '{label}'")

# ── Section 1: ACK Tab Visibility ───────────────────────────────────────────
replace_once(
    "test.describe('ACK Tab Visibility', () => {\n    test.setTimeout(60000);\n\n    let page;\n    test.beforeAll(async ({ browser }) => {\n        test.setTimeout(90000);\n        page = await browser.newPage();\n        await login(page);\n        const iid = await getFirstInterfaceId(page);\n        if (!iid) test.skip();\n        await loadPipelineBuilder(page, iid);\n    });\n\n    test.afterAll(async () => {\n        await page.close();\n    });\n\n    test.afterEach(async () => {\n        await cleanupBuilder(page);\n    });\n\n    // TC-PW-001\n    test('TC-PW-001: ACK tab is hidden for outbound connectors', async () => {",
    "test.describe('ACK Tab Visibility', () => {\n    test.setTimeout(90000);\n\n    async function setupBP(page) {\n        await login(page);\n        const iid = await getFirstInterfaceId(page);\n        if (!iid) return false;\n        await loadPipelineBuilder(page, iid);\n        return true;\n    }\n\n    // TC-PW-001\n    test('TC-PW-001: ACK tab is hidden for outbound connectors', async ({ page }) => {\n        if (!await setupBP(page)) { test.skip(); return; }",
    "ACK Tab Visibility - header"
)

for tc, desc in [
    ('TC-PW-002', "ACK tab hidden when no connector type selected (inbound)"),
    ('TC-PW-003', "ACK tab appears when tcp_mllp_inbound is selected"),
    ('TC-PW-004', "ACK tab disappears when connector type cleared"),
    ('TC-PW-005', "ACK tab hidden for non-MLLP inbound type (file_listener)"),
]:
    replace_once(
        f"    // {tc}\n    test('{tc}: {desc}', async () => {{",
        f"    // {tc}\n    test('{tc}: {desc}', async ({{ page }}) => {{\n        if (!await setupBP(page)) {{ test.skip(); return; }}",
        f"{tc}"
    )

# ── Section 2: Tab Navigation ────────────────────────────────────────────────
old_tab_nav_header = (
    "test.describe('Tab Navigation', () => {\n"
    "    test.setTimeout(60000);\n\n"
    "    let page;\n"
    "    test.beforeAll(async ({ browser }) => {\n"
    "        test.setTimeout(90000);\n"
    "        page = await browser.newPage();\n"
    "        await login(page);\n"
    "        const iid = await getFirstInterfaceId(page);\n"
    "        if (!iid) test.skip();\n"
    "        await loadPipelineBuilder(page, iid);\n"
    "    });\n\n"
    "    test.afterAll(async () => {\n"
    "        await page.close();\n"
    "    });\n\n"
    "    test.beforeEach(async () => {\n"
    "        await page.evaluate(() => {\n"
    "            if (typeof ConnectorConfigBuilder === 'undefined') return;\n"
    "            const existing = document.getElementById('__ack_test_container__');\n"
    "            if (existing) existing.remove();\n"
    "            const container = document.createElement('div');\n"
    "            container.id = '__ack_test_container__';\n"
    "            document.body.appendChild(container);\n"
    "            window.__ackTestBuilder__ = new ConnectorConfigBuilder(container, {}, 'inbound');\n"
    "            window.__ackTestBuilder__.init();\n"
    "            window.__ackTestBuilder__.onConnectorTypeChange('tcp_mllp_inbound');\n"
    "        });\n"
    "    });\n\n"
    "    test.afterEach(async () => {\n"
    "        await cleanupBuilder(page);\n"
    "    });\n\n"
    "    // TC-PW-006\n"
    "    test('TC-PW-006: Clicking Acknowledgment tab shows ACK panel, hides Connection panel', async () => {"
)
new_tab_nav_header = (
    "test.describe('Tab Navigation', () => {\n"
    "    test.setTimeout(90000);\n\n"
    "    async function setupMLLP(page) {\n"
    "        await login(page);\n"
    "        const iid = await getFirstInterfaceId(page);\n"
    "        if (!iid) return false;\n"
    "        await loadPipelineBuilder(page, iid);\n"
    "        const ok = await page.evaluate(() => {\n"
    "            if (typeof ConnectorConfigBuilder === 'undefined') return false;\n"
    "            const existing = document.getElementById('__ack_test_container__');\n"
    "            if (existing) existing.remove();\n"
    "            const container = document.createElement('div');\n"
    "            container.id = '__ack_test_container__';\n"
    "            document.body.appendChild(container);\n"
    "            window.__ackTestBuilder__ = new ConnectorConfigBuilder(container, {}, 'inbound');\n"
    "            window.__ackTestBuilder__.init();\n"
    "            window.__ackTestBuilder__.onConnectorTypeChange('tcp_mllp_inbound');\n"
    "            return true;\n"
    "        });\n"
    "        return ok;\n"
    "    }\n\n"
    "    // TC-PW-006\n"
    "    test('TC-PW-006: Clicking Acknowledgment tab shows ACK panel, hides Connection panel', async ({ page }) => {\n"
    "        if (!await setupMLLP(page)) { test.skip(); return; }"
)
replace_once(old_tab_nav_header, new_tab_nav_header, "Tab Navigation - header")

for tc, desc in [
    ('TC-PW-007', "Clicking Connection tab shows Connection panel, hides ACK panel"),
    ('TC-PW-008', 'Connection tab is active by default (has "active" class)'),
]:
    replace_once(
        f"    // {tc}\n    test('{tc}: {desc}', async () => {{",
        f"    // {tc}\n    test('{tc}: {desc}', async ({{ page }}) => {{\n        if (!await setupMLLP(page)) {{ test.skip(); return; }}",
        f"{tc}"
    )

# ── Section 3: ACK Field Defaults ───────────────────────────────────────────
old_defaults_header = (
    "test.describe('ACK Field Defaults', () => {\n"
    "    test.setTimeout(60000);\n\n"
    "    let page;\n"
    "    test.beforeAll(async ({ browser }) => {\n"
    "        test.setTimeout(90000);\n"
    "        page = await browser.newPage();\n"
    "        await login(page);\n"
    "        const iid = await getFirstInterfaceId(page);\n"
    "        if (!iid) test.skip();\n"
    "        await loadPipelineBuilder(page, iid);\n\n"
    "        // Set up builder once for all default tests\n"
    "        await page.evaluate(() => {\n"
    "            if (typeof ConnectorConfigBuilder === 'undefined') return;\n"
    "            const container = document.createElement('div');\n"
    "            container.id = '__ack_test_container__';\n"
    "            document.body.appendChild(container);\n"
    "            window.__ackTestBuilder__ = new ConnectorConfigBuilder(container, {}, 'inbound');\n"
    "            window.__ackTestBuilder__.init();\n"
    "            window.__ackTestBuilder__.onConnectorTypeChange('tcp_mllp_inbound');\n"
    "        });\n"
    "    });\n\n"
    "    test.afterAll(async () => {\n"
    "        await cleanupBuilder(page);\n"
    "        await page.close();\n"
    "    });\n\n"
    "    // TC-PW-009\n"
    "    test('TC-PW-009: ACK Mode select defaults to \"immediate\"', async () => {"
)
new_defaults_header = (
    "test.describe('ACK Field Defaults', () => {\n"
    "    test.setTimeout(90000);\n\n"
    "    async function setupDefaults(page) {\n"
    "        await login(page);\n"
    "        const iid = await getFirstInterfaceId(page);\n"
    "        if (!iid) return false;\n"
    "        await loadPipelineBuilder(page, iid);\n"
    "        return page.evaluate(() => {\n"
    "            if (typeof ConnectorConfigBuilder === 'undefined') return false;\n"
    "            const existing = document.getElementById('__ack_test_container__');\n"
    "            if (existing) existing.remove();\n"
    "            const container = document.createElement('div');\n"
    "            container.id = '__ack_test_container__';\n"
    "            document.body.appendChild(container);\n"
    "            window.__ackTestBuilder__ = new ConnectorConfigBuilder(container, {}, 'inbound');\n"
    "            window.__ackTestBuilder__.init();\n"
    "            window.__ackTestBuilder__.onConnectorTypeChange('tcp_mllp_inbound');\n"
    "            return true;\n"
    "        });\n"
    "    }\n\n"
    "    // TC-PW-009\n"
    "    test('TC-PW-009: ACK Mode select defaults to \"immediate\"', async ({ page }) => {\n"
    "        if (!await setupDefaults(page)) { test.skip(); return; }"
)
replace_once(old_defaults_header, new_defaults_header, "ACK Field Defaults - header")

for tc, desc in [
    ('TC-PW-010', 'On Error select defaults to "suppress"'),
    ('TC-PW-011', 'Sending Application input is empty, placeholder references ezHealthKonnect'),
    ('TC-PW-012', 'Sending Facility input is empty, placeholder references EHK'),
    ('TC-PW-013', 'Success Text input is empty, placeholder references "received successfully"'),
    ('TC-PW-014', 'Error Text input is empty, placeholder references "processing error"'),
    ('TC-PW-015', 'Script textarea is empty'),
]:
    replace_once(
        f"    // {tc}\n    test('{tc}: {desc}', async () => {{",
        f"    // {tc}\n    test('{tc}: {desc}', async ({{ page }}) => {{\n        if (!await setupDefaults(page)) {{ test.skip(); return; }}",
        f"{tc}"
    )

# ── Section 4: Collapsible Group Behaviour ──────────────────────────────────
old_coll_header = (
    "test.describe('Collapsible Group Behaviour', () => {\n"
    "    test.setTimeout(60000);\n\n"
    "    let page;\n"
    "    test.beforeAll(async ({ browser }) => {\n"
    "        test.setTimeout(90000);\n"
    "        page = await browser.newPage();\n"
    "        await login(page);\n"
    "        const iid = await getFirstInterfaceId(page);\n"
    "        if (!iid) test.skip();\n"
    "        await loadPipelineBuilder(page, iid);\n\n"
    "        await page.evaluate(() => {\n"
    "            if (typeof ConnectorConfigBuilder === 'undefined') return;\n"
    "            const container = document.createElement('div');\n"
    "            container.id = '__ack_test_container__';\n"
    "            document.body.appendChild(container);\n"
    "            window.__ackTestBuilder__ = new ConnectorConfigBuilder(container, {}, 'inbound');\n"
    "            window.__ackTestBuilder__.init();\n"
    "            window.__ackTestBuilder__.onConnectorTypeChange('tcp_mllp_inbound');\n"
    "        });\n"
    "        // Switch to ACK tab so groups are visible\n"
    "        await clickAckTab(page);\n"
    "    });\n\n"
    "    test.afterAll(async () => {\n"
    "        await cleanupBuilder(page);\n"
    "        await page.close();\n"
    "    });\n\n"
    "    function getGroupCollapsedStates(page) {"
)
new_coll_header = (
    "test.describe('Collapsible Group Behaviour', () => {\n"
    "    test.setTimeout(90000);\n\n"
    "    async function setupCollapsible(page) {\n"
    "        await login(page);\n"
    "        const iid = await getFirstInterfaceId(page);\n"
    "        if (!iid) return false;\n"
    "        await loadPipelineBuilder(page, iid);\n"
    "        const ok = await page.evaluate(() => {\n"
    "            if (typeof ConnectorConfigBuilder === 'undefined') return false;\n"
    "            const existing = document.getElementById('__ack_test_container__');\n"
    "            if (existing) existing.remove();\n"
    "            const container = document.createElement('div');\n"
    "            container.id = '__ack_test_container__';\n"
    "            document.body.appendChild(container);\n"
    "            window.__ackTestBuilder__ = new ConnectorConfigBuilder(container, {}, 'inbound');\n"
    "            window.__ackTestBuilder__.init();\n"
    "            window.__ackTestBuilder__.onConnectorTypeChange('tcp_mllp_inbound');\n"
    "            return true;\n"
    "        });\n"
    "        if (!ok) return false;\n"
    "        await clickAckTab(page);\n"
    "        return true;\n"
    "    }\n\n"
    "    function getGroupCollapsedStates(page) {"
)
replace_once(old_coll_header, new_coll_header, "Collapsible - header")

for tc, desc in [
    ('TC-PW-016', 'Basic group is expanded by default (no "collapsed" class)'),
    ('TC-PW-017', 'Sender Identity group is collapsed by default'),
    ('TC-PW-018', 'Message Text group is collapsed by default'),
    ('TC-PW-019', 'Custom Script group is collapsed by default'),
    ('TC-PW-020', 'Clicking a collapsed group header expands it'),
    ('TC-PW-021', 'Clicking an expanded group header collapses it'),
]:
    replace_once(
        f"    // {tc}\n    test('{tc}: {desc}', async () => {{",
        f"    // {tc}\n    test('{tc}: {desc}', async ({{ page }}) => {{\n        if (!await setupCollapsible(page)) {{ test.skip(); return; }}",
        f"{tc}"
    )

# ── Section 5: Config Collection ────────────────────────────────────────────
old_cfg_header = (
    "test.describe('Config Collection via getConfig()', () => {\n"
    "    test.setTimeout(60000);\n\n"
    "    let page;\n"
    "    test.beforeAll(async ({ browser }) => {\n"
    "        test.setTimeout(90000);\n"
    "        page = await browser.newPage();\n"
    "        await login(page);\n"
    "        const iid = await getFirstInterfaceId(page);\n"
    "        if (!iid) test.skip();\n"
    "        await loadPipelineBuilder(page, iid);\n"
    "    });\n\n"
    "    test.afterAll(async () => {\n"
    "        await page.close();\n"
    "    });\n\n"
    "    test.beforeEach(async () => {\n"
    "        // Fresh builder for each test\n"
    "        await page.evaluate(() => {\n"
    "            const existing = document.getElementById('__ack_test_container__');\n"
    "            if (existing) existing.remove();\n"
    "            if (typeof ConnectorConfigBuilder === 'undefined') return;\n"
    "            const container = document.createElement('div');\n"
    "            container.id = '__ack_test_container__';\n"
    "            document.body.appendChild(container);\n"
    "            window.__ackTestBuilder__ = new ConnectorConfigBuilder(container, {}, 'inbound');\n"
    "            window.__ackTestBuilder__.init();\n"
    "            window.__ackTestBuilder__.onConnectorTypeChange('tcp_mllp_inbound');\n"
    "        });\n"
    "    });\n\n"
    "    test.afterEach(async () => {\n"
    "        await cleanupBuilder(page);\n"
    "    });\n\n"
    "    // TC-PW-022\n"
    "    test('TC-PW-022: getConfig() returns config.ack.mode = \"immediate\" by default', async () => {"
)
new_cfg_header = (
    "test.describe('Config Collection via getConfig()', () => {\n"
    "    test.setTimeout(90000);\n\n"
    "    async function setupFreshBuilder(page) {\n"
    "        await login(page);\n"
    "        const iid = await getFirstInterfaceId(page);\n"
    "        if (!iid) return false;\n"
    "        await loadPipelineBuilder(page, iid);\n"
    "        return page.evaluate(() => {\n"
    "            const existing = document.getElementById('__ack_test_container__');\n"
    "            if (existing) existing.remove();\n"
    "            if (typeof ConnectorConfigBuilder === 'undefined') return false;\n"
    "            const container = document.createElement('div');\n"
    "            container.id = '__ack_test_container__';\n"
    "            document.body.appendChild(container);\n"
    "            window.__ackTestBuilder__ = new ConnectorConfigBuilder(container, {}, 'inbound');\n"
    "            window.__ackTestBuilder__.init();\n"
    "            window.__ackTestBuilder__.onConnectorTypeChange('tcp_mllp_inbound');\n"
    "            return true;\n"
    "        });\n"
    "    }\n\n"
    "    // TC-PW-022\n"
    "    test('TC-PW-022: getConfig() returns config.ack.mode = \"immediate\" by default', async ({ page }) => {\n"
    "        if (!await setupFreshBuilder(page)) { test.skip(); return; }"
)
replace_once(old_cfg_header, new_cfg_header, "Config Collection - header")

for tc, desc in [
    ('TC-PW-023', 'getConfig() returns config.ack.mode = "none" when changed'),
    ('TC-PW-024', 'getConfig() returns config.ack.on_error = "nack" when changed'),
    ('TC-PW-025', 'getConfig() captures sending_app (MSH-3) after typing'),
    ('TC-PW-026', 'getConfig() captures sending_facility (MSH-4) after typing'),
    ('TC-PW-027', 'getConfig() captures text_success after typing'),
    ('TC-PW-028', 'getConfig() captures text_error after typing'),
    ('TC-PW-029', 'getConfig() captures custom ACK script after typing'),
]:
    replace_once(
        f"    // {tc}\n    test('{tc}: {desc}', async () => {{",
        f"    // {tc}\n    test('{tc}: {desc}', async ({{ page }}) => {{\n        if (!await setupFreshBuilder(page)) {{ test.skip(); return; }}",
        f"{tc}"
    )

# ── Section 6: Security ──────────────────────────────────────────────────────
old_sec_header = (
    "test.describe('Security \u2014 ACK config field sanitisation', () => {\n"
    "    test.setTimeout(60000);\n\n"
    "    let page;\n"
    "    test.beforeAll(async ({ browser }) => {\n"
    "        test.setTimeout(90000);\n"
    "        page = await browser.newPage();\n"
    "        await login(page);\n"
    "        const iid = await getFirstInterfaceId(page);\n"
    "        if (!iid) test.skip();\n"
    "        await loadPipelineBuilder(page, iid);\n"
    "    });\n\n"
    "    test.afterAll(async () => {\n"
    "        await page.close();\n"
    "    });\n\n"
    "    test.beforeEach(async () => {\n"
    "        await page.evaluate(() => {\n"
    "            const existing = document.getElementById('__ack_test_container__');\n"
    "            if (existing) existing.remove();\n"
    "            if (typeof ConnectorConfigBuilder === 'undefined') return;\n"
    "            const container = document.createElement('div');\n"
    "            container.id = '__ack_test_container__';\n"
    "            document.body.appendChild(container);\n"
    "            window.__ackTestBuilder__ = new ConnectorConfigBuilder(container, {}, 'inbound');\n"
    "            window.__ackTestBuilder__.init();\n"
    "            window.__ackTestBuilder__.onConnectorTypeChange('tcp_mllp_inbound');\n"
    "        });\n"
    "    });\n\n"
    "    test.afterEach(async () => {\n"
    "        await cleanupBuilder(page);\n"
    "    });\n\n"
    "    // TC-PW-039: XSS payload in sending_app does not execute as script\n"
    "    test('TC-PW-039: XSS payload in sending_app field does not execute JS', async () => {"
)
new_sec_header = (
    "test.describe('Security \u2014 ACK config field sanitisation', () => {\n"
    "    test.setTimeout(90000);\n\n"
    "    async function setupSecurity(page) {\n"
    "        await login(page);\n"
    "        const iid = await getFirstInterfaceId(page);\n"
    "        if (!iid) return false;\n"
    "        await loadPipelineBuilder(page, iid);\n"
    "        return page.evaluate(() => {\n"
    "            const existing = document.getElementById('__ack_test_container__');\n"
    "            if (existing) existing.remove();\n"
    "            if (typeof ConnectorConfigBuilder === 'undefined') return false;\n"
    "            const container = document.createElement('div');\n"
    "            container.id = '__ack_test_container__';\n"
    "            document.body.appendChild(container);\n"
    "            window.__ackTestBuilder__ = new ConnectorConfigBuilder(container, {}, 'inbound');\n"
    "            window.__ackTestBuilder__.init();\n"
    "            window.__ackTestBuilder__.onConnectorTypeChange('tcp_mllp_inbound');\n"
    "            return true;\n"
    "        });\n"
    "    }\n\n"
    "    // TC-PW-039: XSS payload in sending_app does not execute as script\n"
    "    test('TC-PW-039: XSS payload in sending_app field does not execute JS', async ({ page }) => {\n"
    "        if (!await setupSecurity(page)) { test.skip(); return; }"
)
replace_once(old_sec_header, new_sec_header, "Security - header")

replace_once(
    "    // TC-PW-040: XSS payload in pre-loaded config is escaped in the rendered HTML\n"
    "    test('TC-PW-040: Pre-loaded XSS config is escaped in rendered HTML (no raw <script> in DOM)', async () => {",
    "    // TC-PW-040: XSS payload in pre-loaded config is escaped in the rendered HTML\n"
    "    test('TC-PW-040: Pre-loaded XSS config is escaped in rendered HTML (no raw <script> in DOM)', async ({ page }) => {\n"
    "        if (!await setupSecurity(page)) { test.skip(); return; }",
    "TC-PW-040"
)

with open('c:/Projects/ezHealthKonnect/tests/playwright/ack-nack-e2e.spec.js', 'w', encoding='utf-8') as f:
    f.write(content)

remaining = content.count("test.beforeAll(async ({ browser })")
print(f"\nRemaining 'beforeAll(browser)' hooks: {remaining}")
print("Done.")
