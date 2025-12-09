# XPath Autocomplete Debugging Guide

## Issue
The XPath autocomplete component is not rendering visible input fields in the Field Validation step configuration modal.

## Recent Changes (Debug Version 1.2)

### Added Console Logging

**ValidationRuleBuilder.js** (`?v=1.2`):
- Logs number of `.xpath-autocomplete-container` elements found
- Logs each autocomplete initialization with rule data and options
- Logs total autocompletes successfully initialized

**XPathAutocomplete.js** (`?v=1.2`):
- Logs container element when `render()` is called
- Logs whether input, dropdown, and dropdownList elements were found after rendering

### Fixed Syntax Error
- Fixed escaped `!` character at line 423 (`\!==` → `!==`)

## How to Debug

### Step 1: Hard Refresh Browser
**IMPORTANT**: Clear browser cache to load new versions
- Windows: `Ctrl + Shift + R` or `Ctrl + F5`
- Mac: `Cmd + Shift + R`
- Or: Open DevTools → Network tab → Check "Disable cache"

### Step 2: Open Browser Console
Open Developer Tools (F12) and check Console tab

### Step 3: Test the Flow
1. Go to Pipeline Builder page
2. Drag "Field Validation" step onto canvas
3. Double-click the step card to open configuration modal
4. Watch console for these messages:

```
[ValidationRuleBuilder] Found XPath containers: 1
[ValidationRuleBuilder] Initializing autocomplete for container 0 {...}
[XPathAutocomplete] Rendering into container: <div class="xpath-autocomplete-container">
[XPathAutocomplete] Elements found: {input: true, dropdown: true, dropdownList: true}
[ValidationRuleBuilder] Autocomplete initialized successfully for index 0
[ValidationRuleBuilder] Total autocompletes initialized: 1
```

### Step 4: Check Console Output

#### ✅ Expected Output (Success)
```
[ValidationRuleBuilder] Found XPath containers: 1
[ValidationRuleBuilder] Initializing autocomplete for container 0
[XPathAutocomplete] Rendering into container: <div>
[XPathAutocomplete] Elements found: {input: true, dropdown: true, dropdownList: true}
[ValidationRuleBuilder] Autocomplete initialized successfully
[ValidationRuleBuilder] Total autocompletes initialized: 1
```

#### ❌ Problem Scenarios

**Scenario A: No containers found**
```
[ValidationRuleBuilder] Found XPath containers: 0
[ValidationRuleBuilder] Total autocompletes initialized: 0
```
→ **Issue**: HTML template not rendering `.xpath-autocomplete-container` divs
→ **Check**: `ValidationRuleBuilder.renderFieldPath()` method

**Scenario B: XPathAutocomplete not loaded**
```
[ValidationRuleBuilder] Found XPath containers: 1
[ValidationRuleBuilder] XPathAutocomplete component not loaded!
```
→ **Issue**: Script not loading or `window.XPathAutocomplete` not exported
→ **Check**: Script order in `pipeline-builder.html`
→ **Check**: Export statement in `XPathAutocomplete.js` line 418

**Scenario C: Elements not found after render**
```
[XPathAutocomplete] Rendering into container: <div>
[XPathAutocomplete] Elements found: {input: false, dropdown: false, dropdownList: false}
```
→ **Issue**: `innerHTML` being overwritten or querySelector failing
→ **Check**: CSS selectors in `render()` method
→ **Check**: If container is being cleared after render

**Scenario D: Container is null/undefined**
```
[XPathAutocomplete] Rendering into container: null
```
→ **Issue**: ValidationRuleBuilder passing wrong container reference
→ **Check**: `querySelectorAll('.xpath-autocomplete-container')` results

### Step 5: Inspect DOM
In Browser DevTools:
1. Inspect the modal where "Field Path" should appear
2. Look for these elements:
   ```html
   <div class="xpath-autocomplete-container">
     <div class="xpath-autocomplete">
       <div class="xpath-input-wrapper">
         <input class="xpath-input form-control" type="text">
       </div>
     </div>
   </div>
   ```
3. If `.xpath-autocomplete-container` exists but is empty → render failed
4. If `.xpath-autocomplete` exists but input is hidden → CSS issue
5. If nothing exists → initialization never ran

### Step 6: Check Network Tab
Verify scripts loading with new versions:
- `XPathAutocomplete.js?v=1.2` (200 OK)
- `ValidationRuleBuilder.js?v=1.2` (200 OK)

If you see `?v=1.1` or no version parameter → browser still using cached version

## Common Issues & Solutions

### Issue: "Form-control not found" or blank space
**Cause**: Bootstrap's `form-control` class might be missing styles
**Solution**: Check if Bootstrap CSS is loaded, or add explicit width style

### Issue: Input renders but immediately disappears
**Cause**: Another script or event handler clearing the modal content
**Solution**: Check if PropertiesPanel is re-rendering after initialization

### Issue: Modal opens but Field Path section completely missing
**Cause**: `renderFieldPath()` not being called or conditional rendering hiding it
**Solution**: Check step type configuration and field visibility logic

## Next Steps After Debugging

Once you have console output, you'll know exactly where it's failing:
1. **No containers** → Fix HTML template rendering
2. **No elements found** → Fix render() method or CSS
3. **Initialization fails** → Fix component loading/exports
4. **Everything logs success but still invisible** → CSS display/visibility issue

## File Locations
- Component: `public/js/pipeline/components/XPathAutocomplete.js`
- Builder: `public/js/pipeline/components/ValidationRuleBuilder.js`
- HTML: `public/pipeline-builder.html`
- CSS: `public/css/components/xpath-autocomplete.css`
