# If-Then-Else Step - Testing Guide

**Version:** 1.0
**Date:** December 28, 2025

---

## Quick Start Testing

### Step 1: Access the Pipeline Builder

1. Navigate to: `http://localhost:3000/pipeline-builder.html`
2. Click **"New Pipeline"** or open existing pipeline
3. Select message type: **ADT^A01**

### Step 2: Add If-Then-Else Step

1. Look in the left toolbox under **"Conditional Logic Steps"**
2. Find **"If-Then-Else"** step
3. **Drag** the step to any layer (pre, core, or post)
4. **Double-click** the step to open properties

### Step 3: Configure Your First Condition

**Example: Age-Based Routing**

1. **Condition Name:** `Check Age for Geriatric Care`

2. **IF Section:**
   - Field: `patient.age`
   - Operator: `Greater Than (>)`
   - Value: `65`

3. **THEN Section (if true):**
   - Action: `Set Metadata`
   - Metadata:
     ```json
     {
       "priority": "high",
       "routing": "geriatrics"
     }
     ```

4. **ELSE Section (if false):**
   - Action: `Continue`

5. Click **"Save"**

### Step 4: Test the Configuration

1. Click **"Test Step"** button
2. Provide test input:
   ```json
   {
     "patient": {
       "age": 70
     }
   }
   ```
3. Click **"Run Test"**
4. Verify output has metadata:
   ```json
   {
     "patient": {
       "age": 70
     },
     "_metadata": {
       "priority": "high",
       "routing": "geriatrics"
     }
   }
   ```

---

## Complete Test Scenarios

### Test 1: Simple Value Comparison

**Goal:** Test basic equals operator

**Configuration:**
- Field: `PID.8`
- Operator: `equals`
- Value: `M`
- THEN: Set Field `patient.gender` = `male`
- ELSE: Set Field `patient.gender` = `female`

**Test Cases:**

```json
// Test Case 1: Male
{
  "input": {
    "enhancedSegments": {
      "PID": {
        "fields": [
          { "key": "PID.8", "value": "M" }
        ]
      }
    }
  },
  "expected": {
    "patient": {
      "gender": "male"
    }
  }
}

// Test Case 2: Female
{
  "input": {
    "enhancedSegments": {
      "PID": {
        "fields": [
          { "key": "PID.8", "value": "F" }
        ]
      }
    }
  },
  "expected": {
    "patient": {
      "gender": "female"
    }
  }
}

// Test Case 3: Unknown
{
  "input": {
    "enhancedSegments": {
      "PID": {
        "fields": [
          { "key": "PID.8", "value": "U" }
        ]
      }
    }
  },
  "expected": {
    "patient": {
      "gender": "female"  // Falls to ELSE
    }
  }
}
```

---

### Test 2: Cross-Field Comparison

**Goal:** Validate discharge date > admit date

**Configuration:**
- Field: `PV1.45` (discharge)
- Operator: `less_than_or_equal`
- Compare To Field: `PV1.44` (admit)
- ☑️ Compare to another field
- THEN: Reject with error
- ELSE: Continue

**Test Cases:**

```json
// Test Case 1: Valid dates (discharge > admit)
{
  "input": {
    "enhancedSegments": {
      "PV1": {
        "fields": [
          { "key": "PV1.44", "value": "20231201" },
          { "key": "PV1.45", "value": "20231205" }
        ]
      }
    }
  },
  "expected": {
    "status": "success",
    "continue": true
  }
}

// Test Case 2: Invalid dates (discharge <= admit)
{
  "input": {
    "enhancedSegments": {
      "PV1": {
        "fields": [
          { "key": "PV1.44", "value": "20231205" },
          { "key": "PV1.45", "value": "20231201" }
        ]
      }
    }
  },
  "expected": {
    "status": "error",
    "error": "REJECT: Discharge date must be after admit date"
  }
}

// Test Case 3: Same date
{
  "input": {
    "enhancedSegments": {
      "PV1": {
        "fields": [
          { "key": "PV1.44", "value": "20231201" },
          { "key": "PV1.45", "value": "20231201" }
        ]
      }
    }
  },
  "expected": {
    "status": "error",
    "error": "REJECT: Discharge date must be after admit date"
  }
}
```

---

### Test 3: Numeric Comparison

**Goal:** Route patients based on age

**Configuration:**
- Field: `patient.age`
- Operator: `greater_than`
- Value: `65`
- THEN: Route to geriatrics
- ELSE: Continue

**Test Cases:**

```json
// Test Case 1: Age = 70 (> 65)
{
  "input": {
    "patient": { "age": 70 }
  },
  "expected": {
    "_routing": {
      "destination": "geriatrics"
    }
  }
}

// Test Case 2: Age = 65 (not > 65)
{
  "input": {
    "patient": { "age": 65 }
  },
  "expected": {
    "_routing": {}  // No routing
  }
}

// Test Case 3: Age = 45
{
  "input": {
    "patient": { "age": 45 }
  },
  "expected": {
    "_routing": {}  // No routing
  }
}

// Test Case 4: Age missing
{
  "input": {
    "patient": {}
  },
  "expected": {
    "_routing": {}  // No routing, graceful handling
  }
}
```

---

### Test 4: String Contains

**Goal:** Flag messages containing specific keywords

**Configuration:**
- Field: `notes`
- Operator: `contains`
- Value: `VIP`
- THEN: Set Metadata priority = "vip"
- ELSE: Continue

**Test Cases:**

```json
// Test Case 1: Contains VIP
{
  "input": {
    "notes": "This is a VIP patient"
  },
  "expected": {
    "_metadata": {
      "priority": "vip"
    }
  }
}

// Test Case 2: Case sensitive (does not contain)
{
  "input": {
    "notes": "This is a vip patient"
  },
  "expected": {
    "_metadata": {}  // No metadata
  }
}

// Test Case 3: Empty notes
{
  "input": {
    "notes": ""
  },
  "expected": {
    "_metadata": {}
  }
}
```

---

### Test 5: Empty Field Check

**Goal:** Set default value when field is missing

**Configuration:**
- Field: `PID.13`
- Operator: `is_empty`
- THEN: Set Field `patient.phone` = `000-000-0000`
- ELSE: Copy Field PID.13 → patient.phone

**Test Cases:**

```json
// Test Case 1: Field is empty
{
  "input": {
    "enhancedSegments": {
      "PID": {
        "fields": [
          { "key": "PID.13", "value": "" }
        ]
      }
    }
  },
  "expected": {
    "patient": {
      "phone": "000-000-0000"
    }
  }
}

// Test Case 2: Field has value
{
  "input": {
    "enhancedSegments": {
      "PID": {
        "fields": [
          { "key": "PID.13", "value": "555-1234" }
        ]
      }
    }
  },
  "expected": {
    "patient": {
      "phone": "555-1234"
    }
  }
}

// Test Case 3: Field missing entirely
{
  "input": {
    "enhancedSegments": {
      "PID": {
        "fields": []
      }
    }
  },
  "expected": {
    "patient": {
      "phone": "000-000-0000"
    }
  }
}
```

---

### Test 6: Multiple Conditions

**Goal:** Test sequential execution of multiple conditions

**Configuration:**

**Condition 1:**
- IF: patient.vipFlag equals true
- THEN: Set Metadata priority = "vip"

**Condition 2:**
- IF: patient.age > 65
- THEN: Set Metadata ageGroup = "senior"

**Test Cases:**

```json
// Test Case 1: VIP and Senior
{
  "input": {
    "patient": {
      "vipFlag": true,
      "age": 70
    }
  },
  "expected": {
    "_metadata": {
      "priority": "vip",
      "ageGroup": "senior"
    }
  }
}

// Test Case 2: VIP only
{
  "input": {
    "patient": {
      "vipFlag": true,
      "age": 45
    }
  },
  "expected": {
    "_metadata": {
      "priority": "vip"
    }
  }
}

// Test Case 3: Senior only
{
  "input": {
    "patient": {
      "vipFlag": false,
      "age": 70
    }
  },
  "expected": {
    "_metadata": {
      "ageGroup": "senior"
    }
  }
}

// Test Case 4: Neither
{
  "input": {
    "patient": {
      "vipFlag": false,
      "age": 45
    }
  },
  "expected": {
    "_metadata": {}
  }
}
```

---

## Manual Testing Workflow

### Using Pipeline Test Interface

1. **Open Pipeline Builder**
   - URL: `http://localhost:3000/pipeline-builder.html`

2. **Create Test Pipeline**
   - Add If-Then-Else step
   - Configure condition
   - Save pipeline

3. **Test with Sample Data**
   - Click "Test Pipeline" button
   - Upload sample HL7 message:
     ```
     MSH|^~\&|SENDING_APP|SENDING_FACILITY|RECEIVING_APP|RECEIVING_FACILITY|20231201120000||ADT^A01|MSG00001|P|2.5
     PID|||12345||DOE^JOHN||19581201|M|||123 MAIN ST^^CITY^STATE^12345||555-1234
     PV1||I|ICU^101^01||||123456^SMITH^JOHN|||||||||||VIP
     ```
   - Click "Execute"
   - Review execution log

4. **Verify Output**
   - Check step output in execution log
   - Verify metadata was set
   - Verify routing information
   - Check for errors/warnings

### Using Browser DevTools

1. **Open Console** (F12)

2. **Monitor Execution:**
   ```javascript
   // Watch for conditional logic execution
   console.log('Condition evaluated:', result);
   ```

3. **Check Step Output:**
   ```javascript
   // Inspect output data
   console.log('Step output:', stepOutput);
   console.log('Metadata:', stepOutput._metadata);
   console.log('Routing:', stepOutput._routing);
   ```

---

## Integration Testing

### Test with Real HL7 Messages

1. **Setup Test Interface**
   - Create interface with If-Then-Else in pipeline
   - Configure TCP/MLLP inbound connector
   - Connect to test sender

2. **Send Test Messages**
   ```bash
   # Send via TCP
   echo -e "\x0BMSH|^~\\&|SEND|FAC|REC|FAC|20231201120000||ADT^A01|1|P|2.5\rPID|||12345||DOE^JOHN||19581201|M\rPV1||I|ICU\x1C\x0D" | nc localhost 2575
   ```

3. **Verify Processing**
   - Check message status in interface
   - Review processing logs
   - Verify FHIR output includes metadata
   - Check routing occurred correctly

### Test with Pipeline Execution

1. **Create Complete Pipeline:**
   ```
   Pre-Processing:
   1. Field Validation
   2. If-Then-Else (age check)

   Core Processing:
   3. HL7→FHIR Transform

   Post-Processing:
   4. FHIR Validation
   5. If-Then-Else (quality check)
   ```

2. **Execute End-to-End:**
   - Send message through pipeline
   - Verify each step executes
   - Check If-Then-Else conditions evaluated
   - Verify actions executed correctly

---

## Debugging Tips

### Condition Not Triggering

**Debug Steps:**
1. Add console.log to inspect field value:
   ```javascript
   console.log('Field value:', getNestedValue(data, 'patient.age'));
   ```

2. Check data structure:
   - Is field path correct?
   - Does field exist?
   - Is value the expected type?

3. Verify operator:
   - Numeric vs string comparison
   - Case sensitivity for strings
   - Null/undefined handling

### Action Not Executing

**Debug Steps:**
1. Verify condition evaluates to true/false correctly
2. Check action configuration is complete
3. Review execution logs for errors
4. Test action independently

### Cross-Field Comparison Issues

**Debug Steps:**
1. Verify both fields exist
2. Check both field paths are correct
3. Ensure "Compare to another field" checkbox is checked
4. Log both field values:
   ```javascript
   console.log('Field 1:', field1Value);
   console.log('Field 2:', field2Value);
   ```

---

## Performance Testing

### Load Test Configuration

**Test:** 1000 messages with If-Then-Else step

**Expected Performance:**
- < 1ms per condition evaluation
- < 2ms per action execution
- No memory leaks after 10,000 messages

**Monitor:**
- CPU usage should remain < 50%
- Memory should stabilize after initial load
- No errors in logs

### Optimization Tips

1. **Order Conditions by Frequency:**
   - Most common conditions first
   - Early continue/reject saves processing

2. **Minimize Regex:**
   - Use simple operators when possible
   - Cache regex compilation if repeated

3. **Batch Similar Logic:**
   - Group related conditions in one step
   - Use separate steps for unrelated logic

---

## Automated Testing Script

```javascript
// test_if_then_else.js

const testCases = [
  {
    name: 'Age > 65',
    input: { patient: { age: 70 } },
    expected: { _metadata: { priority: 'high' } }
  },
  {
    name: 'Age <= 65',
    input: { patient: { age: 45 } },
    expected: { _metadata: {} }
  }
];

async function runTests() {
  for (const test of testCases) {
    const result = await executeStep(ifThenElseStep, test.input);

    if (JSON.stringify(result._metadata) === JSON.stringify(test.expected._metadata)) {
      console.log(`✅ PASS: ${test.name}`);
    } else {
      console.log(`❌ FAIL: ${test.name}`);
      console.log('Expected:', test.expected);
      console.log('Got:', result);
    }
  }
}

runTests();
```

---

## Success Criteria

### UI Testing
- ✅ Can drag If-Then-Else to any layer
- ✅ Properties panel opens correctly
- ✅ Condition builder displays all fields
- ✅ Action builder displays all actions
- ✅ Can add/delete conditions
- ✅ Can save configuration
- ✅ Help modal shows examples

### Functional Testing
- ✅ All operators work correctly
- ✅ Cross-field comparison works
- ✅ All 9 actions execute properly
- ✅ Multiple conditions execute in sequence
- ✅ Error handling works (invalid JSON, missing fields)
- ✅ Logs show correct execution flow

### Integration Testing
- ✅ Works with other pipeline steps
- ✅ Metadata flows to downstream steps
- ✅ Routing directs to correct destination
- ✅ Rejection stops pipeline correctly
- ✅ End-to-end pipeline executes successfully

---

## Common Issues & Solutions

| Issue | Solution |
|-------|----------|
| UI not loading | Check browser console, verify IfThenElseBuilder.js loaded |
| Condition never true | Verify field path, check data structure |
| Metadata not setting | Verify JSON syntax, check for trailing commas |
| Cross-field fails | Ensure checkbox checked, verify both fields exist |
| Performance slow | Reduce regex usage, order conditions by frequency |

---

## Next Steps After Testing

1. ✅ Verify all test cases pass
2. ✅ Review execution logs for errors
3. ✅ Test with production-like data
4. ✅ Document any edge cases discovered
5. ✅ Share findings with team

---

**Testing Complete!** 🎉

If all tests pass, the If-Then-Else step is ready for production use.

---

**Version:** 1.0
**Last Updated:** December 28, 2025
