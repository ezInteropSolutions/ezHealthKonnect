# Loop Type Providers Architecture

## Overview

The Loop Type Provider system enables format-specific loop configurations in a no-code friendly way.
Instead of asking users to enter technical paths like "IN1" or "OBX", we present human-readable
options like "Each Insurance Record" or "Each Test Result".

## Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                      LoopTypeRegistry                           │
│  - Manages all registered providers                             │
│  - Auto-detects message format                                  │
│  - Returns appropriate provider                                 │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│                    BaseLoopTypeProvider                         │
│  - Abstract base class                                          │
│  - Defines interface for all providers                          │
│  - Common utility methods                                       │
└─────────────────────────────────────────────────────────────────┘
                              │
          ┌───────────────────┼───────────────────┐
          ▼                   ▼                   ▼
┌─────────────────┐ ┌─────────────────┐ ┌─────────────────┐
│ HL7LoopProvider │ │FHIRLoopProvider │ │GenericLoopProv. │
│                 │ │                 │ │                 │
│ - Insurance     │ │ - Bundle Entry  │ │ - Array items   │
│ - Test Results  │ │ - Identifiers   │ │ - Fixed count   │
│ - Diagnoses     │ │ - Names         │ │ - Custom path   │
│ - Contacts      │ │ - Addresses     │ │                 │
└─────────────────┘ └─────────────────┘ └─────────────────┘
```

## Data Structure

Each loop option is defined as:

```javascript
{
    id: 'insurance',              // Unique identifier
    icon: 'fas fa-hospital',      // FontAwesome icon
    label: 'Each Insurance',      // Short label for card
    description: 'Insurance plans and coverage', // Tooltip/description
    technicalPath: 'IN1',         // Actual path used by executor
    category: 'records',          // Grouping category
    availableVariables: [         // Variables available in loop body
        { name: 'item', description: 'Current insurance record' },
        { name: 'position', description: 'Position number (1, 2, 3...)' },
        { name: 'total', description: 'Total number of records' }
    ]
}
```

## Adding a New Format Provider

### Step 1: Create the Provider Class

```javascript
// File: public/js/pipeline/utils/loopProviders/X12LoopProvider.js

class X12LoopProvider extends BaseLoopTypeProvider {
    constructor() {
        super('x12', 'X12 EDI');
    }

    getLoopOptions() {
        return [
            {
                id: 'claim-lines',
                icon: 'fas fa-file-invoice-dollar',
                label: 'Each Claim Line',
                description: 'Individual service lines in the claim',
                technicalPath: 'loop2400',
                category: 'transactions'
            },
            // ... more options
        ];
    }

    detectFormat(message) {
        // Return true if message is X12 format
        return message?.startsWith?.('ISA') || false;
    }
}
```

### Step 2: Register the Provider

```javascript
// In LoopTypeRegistry initialization
LoopTypeRegistry.register(new X12LoopProvider());
```

### Step 3: Provider is Automatically Available

The UI will automatically show X12 options when an X12 message is detected.

## Provider Interface

All providers must implement:

| Method | Description |
|--------|-------------|
| `getLoopOptions()` | Returns array of loop option objects |
| `detectFormat(message)` | Returns true if message matches this format |
| `getFormatId()` | Returns unique format identifier |
| `getFormatName()` | Returns human-readable format name |

## UI Integration

The ForEachLoopBuilder uses the registry to:

1. Detect message format from pipeline context
2. Get appropriate provider
3. Render visual cards for each loop option
4. Handle selection and set technical path automatically

```javascript
// In ForEachLoopBuilder
const provider = LoopTypeRegistry.getProviderForMessage(currentMessage);
const options = provider.getLoopOptions();
this.renderLoopOptionCards(options);
```

## Categories

Options are grouped into categories for better organization:

| Category | Description | Example Options |
|----------|-------------|-----------------|
| `records` | Repeating record groups | Insurance, Diagnoses |
| `results` | Test/observation results | Lab Results, Vitals |
| `contacts` | People/contact info | Next of Kin, Providers |
| `clinical` | Clinical data | Allergies, Medications |
| `utility` | Non-format-specific | Fixed Count, Custom |

## Best Practices

1. **Human-First Labels**: Use plain English, not technical terms
2. **Consistent Icons**: Use FontAwesome icons that visually represent the data
3. **Helpful Descriptions**: Explain what the option does in simple terms
4. **Smart Defaults**: Pre-select common options when possible
5. **Hide Complexity**: Show "Advanced" option only when needed

## File Structure

```
public/js/pipeline/utils/loopProviders/
├── BaseLoopTypeProvider.js    # Abstract base class
├── HL7LoopProvider.js         # HL7 v2.x specific options
├── FHIRLoopProvider.js        # FHIR R4 specific options
├── GenericLoopProvider.js     # Format-agnostic options
└── LoopTypeRegistry.js        # Provider registry and detection
```

## Future Extensions

- **CDA Loop Provider**: For CCD/CDA documents
- **X12 Loop Provider**: For EDI transactions (837, 835, etc.)
- **CSV Loop Provider**: For delimited data files
- **Custom Provider API**: Allow users to define custom loop types
