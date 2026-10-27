# Building HTML safely in the browser UI

Every page in `public/` builds markup through **`HtmlSafe`** (`public/js/core/html-safe.js`), so data is escaped unless code explicitly says it is markup. Interface names, step names, message content, partner documents, AI answers and server errors are all data. The CSP still allows inline handlers (`'unsafe-inline'`, required by about 900 existing `onclick` attributes), so escaping is the defense against stored and reflected XSS.

CI enforces this: `npm run lint:xss` (ESLint with `eslint-plugin-no-unsanitized`, configured in `eslint.config.js`) fails the build when any `innerHTML`, `outerHTML`, `insertAdjacentHTML` or `document.write` receives anything other than HtmlSafe markup.

## The API

| Call | Use it for |
|---|---|
| ``HtmlSafe.html`<li>${name}</li>` `` | All markup. Every `${}` value is escaped, including backticks (as `&#96;`). Values that are already `SafeHtml` pass through unchanged. Arrays are rendered item by item with no separator. |
| `HtmlSafe.raw('<span class="badge">OOB</span>')` | A **constant** markup string. Never pass data to it. |
| `HtmlSafe.escapeHtml(s)` | A plain escaped string, only for code that cannot use the tag. |
| `HtmlSafe.sanitize(html)` | Third-party rich text (FHIR narratives). Keeps formatting and removes everything executable. |

Set text with `textContent` wherever no markup is needed. `DOMUtils.createElement(tag, attrs, content)` inserts a plain-string `content` as text; pass `HtmlSafe.html` for markup.

## Rules (each one corresponds to a real bug found during the conversion)

1. **Never put data inside an inline handler.** `onclick="open('${id}')"` breaks out with a single quote even after HTML escaping. Use `data-*` attributes and read `this.dataset.id`, or pass a numeric index and look the record up.
2. **Don't concatenate SafeHtml values.** `a + b` and `html += x` turn markup into a plain string, which is then escaped and shows as visible tags. Push to an array and render it with ``HtmlSafe.html`${parts}` ``.
3. **Arrays are always truthy.** `rows || '<p>None</p>'` never shows the empty state. Write `rows.length ? rows : HtmlSafe.raw('<p>None</p>')`.
4. **Escape once.** Don't call `escapeHtml()` on a value that goes into `HtmlSafe.html`, and don't return escaped strings from helpers whose output is interpolated (search highlighters, for example). Return `HtmlSafe.html` from the helper instead.
5. **Join text arrays yourself.** `.join('')` is only unnecessary for markup arrays inside `HtmlSafe.html`. For text (ids, camelCase names, labels) keep the join.
6. **SafeHtml is not a string.** `.trim()` and `.substring()` don't exist on it; convert with `String(x)` first if you really need text.
7. **Element markup:** return `HtmlSafe.raw(el.outerHTML)` when a helper builds DOM and returns its markup.
8. **URLs:** use `encodeURIComponent` for path and query parts. HTML escaping is not URL encoding.
9. **Restoring a button's content** after a "Saving…" state: save and restore `childNodes`, never `innerHTML` strings.
10. **Markdown renderers** (ezCompanion, docs): escape first, add fixed tags, allow only `http(s)`/`mailto` links, return `HtmlSafe.raw`. Code that runs regexes over escaped text must account for `&#96;`.

## Pipeline step builders

`render(step)` must return HtmlSafe markup. `PropertiesPanel._builderMarkup` shows a plain-string result as text and logs an error, and `tests/playwright/toolbox-step-coverage.spec.js` fails if any registered builder returns a string or double-escapes.

## Tests

`tests/playwright/ui-xss-regression.spec.js` plants hostile values (interface and step names, templates, alert rules, user names, code templates, Mirth files, reports, FHIR resources, clinical documents, message data, DLQ rows, ezCompanion answers, Test Pipeline results) and checks that nothing runs and that each value appears as text, escaped exactly once.
