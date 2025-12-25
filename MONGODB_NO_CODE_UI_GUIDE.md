# MongoDB No-Code UI Guide

## 🎉 What Changed?

MongoDB database enrichment is now **fully no-code** with visual builders instead of JSON editing!

### Before (Technical - Required JSON Knowledge)
```
Filter (JSON): { "mrn": "{PID.3}" }
Projection (JSON): { "mrn": 1, "firstName": 1, "lastName": 1 }
```

### After (No-Code - Visual Builders)
- ✅ **Filter Builder**: Click-to-add conditions with dropdowns
- ✅ **Projection Builder**: Checkbox field selector

---

## 🧪 How to Test MongoDB (Step-by-Step)

### 1. Open Pipeline Builder
Navigate to: **http://localhost:3000/pipeline-builder.html**

### 2. Create or Select an Interface
- Click "Create Interface" or select an existing one
- Click "Configure Pipeline"

### 3. Add Database Enrichment Step
1. Click **"+ Add Step"**
2. Select **Pre-Processing** → **Database Enrichment**

### 4. Configure MongoDB Connection

**Database Type**: Select **MongoDB** from dropdown

**Connection Details**:
- **Host**: `mongodb`
- **Port**: `27017`
- **Database**: `ezhealthkonnect`
- **Username**: `ezhealth_user`
- **Password**: `secure_password_change_me`

**Collection Name**: `patients`

---

## 🔍 MongoDB Filter Builder (Visual Query Builder)

### What is a Filter?
The filter tells MongoDB which documents (records) to find. Like a WHERE clause in SQL.

### How to Use It

1. **Click "Add Condition"** - Adds a new filter row

2. **Field Name**: Enter the MongoDB field to query
   - Example: `mrn` (Medical Record Number)
   - Uses autocomplete with common fields

3. **Operator**: Select from dropdown
   - **Equals (=)**: Exact match - `mrn equals "MRN123456"`
   - **Contains**: Text search - `name contains "John"`
   - **Starts With**: Prefix match - `mrn starts with "MRN"`
   - **Greater Than (>)**: Numeric comparison - `age > 18`
   - **In List**: Match multiple values - `status in ["active", "pending"]`
   - **Exists**: Check if field has a value

4. **Value**: Enter the value to search for
   - **Static value**: `MRN123456`
   - **HL7 field placeholder**: `{PID.3}` (uses patient ID from incoming HL7 message)

### Example Configuration

**Find patient by MRN from HL7 message**:
```
Field Name: mrn
Operator: Equals (=)
Value: {PID.3}
```

**Find active adult patients**:
```
Field Name: status     Operator: Equals      Value: active
Field Name: age        Operator: Greater (>) Value: 18
```

### Live Preview
The **Generated Filter (Preview)** box shows the MongoDB JSON automatically:
```json
{
  "mrn": "{PID.3}"
}
```

---

## 📋 MongoDB Projection Builder (Field Selector)

### What is a Projection?
The projection controls which fields to return from MongoDB. Like SELECT columns in SQL.

### How to Use It

1. **Select Mode**:
   - **Include selected fields** (return only these)
   - **Exclude selected fields** (return everything except these)

2. **Check Fields to Include/Exclude**:
   - ✅ `mrn` - Medical Record Number
   - ✅ `firstName` - First Name
   - ✅ `lastName` - Last Name
   - ✅ `dateOfBirth` - Date of Birth
   - ✅ `gender` - Gender
   - ✅ `phone` - Phone Number
   - ✅ `insurance` - Insurance (Full Object)
   - ✅ `allergies` - Allergies (Array)

3. **Add Custom Fields**: Type field name and click "Add"

4. **Select All/Deselect All**: Click "Select All Common Fields" button

### Example Configuration

**Return only basic patient info**:
```
Mode: Include selected fields
Checked: ✅ mrn, firstName, lastName, dateOfBirth
```

**Return everything except sensitive fields**:
```
Mode: Exclude selected fields
Checked: ✅ ssn, insurance.memberId
```

### Leave Empty
If you don't check any fields, MongoDB returns **all fields** (like `SELECT *` in SQL)

---

## 🎯 Complete Example: Patient Lookup by MRN

### Configuration

**Database Type**: MongoDB
**Host**: mongodb
**Port**: 27017
**Database**: ezhealthkonnect
**Username**: ezhealth_user
**Password**: secure_password_change_me
**Collection**: patients

**Filter Conditions**:
- Field: `mrn`
- Operator: Equals (=)
- Value: `{PID.3}`

**Select Fields** (Projection):
- ✅ mrn
- ✅ firstName
- ✅ lastName
- ✅ phone
- ✅ insurance

### What Happens When HL7 Message is Processed

1. **HL7 message arrives** with `PID.3 = MRN123456`

2. **Filter is built** by replacing `{PID.3}` with actual value:
   ```json
   { "mrn": "MRN123456" }
   ```

3. **MongoDB query executes**:
   ```javascript
   db.patients.findOne(
     { "mrn": "MRN123456" },
     { "mrn": 1, "firstName": 1, "lastName": 1, "phone": 1, "insurance": 1 }
   )
   ```

4. **Result is returned**:
   ```json
   {
     "mrn": "MRN123456",
     "firstName": "John",
     "lastName": "Doe",
     "phone": "555-1234",
     "insurance": {
       "provider": "Blue Cross",
       "memberId": "BC-12345678"
     }
   }
   ```

5. **Data is added to message** at `enriched.database` path

---

## 🧪 Testing Your Configuration

### Test Data Available in MongoDB

```javascript
// Patient 1
{
  mrn: "MRN123456",
  firstName: "John",
  lastName: "Doe",
  dateOfBirth: "1980-05-15",
  gender: "M",
  phone: "555-1234",
  insurance: { provider: "Blue Cross" },
  allergies: ["Penicillin", "Peanuts"],
  chronicConditions: ["Hypertension", "Type 2 Diabetes"]
}

// Patient 2
{
  mrn: "MRN789012",
  firstName: "Jane",
  lastName: "Smith",
  dateOfBirth: "1975-08-22",
  gender: "F",
  phone: "555-5678",
  insurance: { provider: "Aetna" }
}

// Patient 3
{
  mrn: "MRN345678",
  firstName: "Robert",
  lastName: "Johnson",
  dateOfBirth: "1992-03-10",
  gender: "M",
  phone: "555-9012",
  insurance: { provider: "UnitedHealthcare" }
}
```

### Send Test HL7 Message

Create an HL7 message with one of the test MRNs:
```
MSH|^~\&|SENDING_APP|SENDING_FAC|RECEIVING_APP|RECEIVING_FAC|20250101120000||ADT^A01|MSG123|P|2.5
PID|1||MRN123456^^^MRN||Doe^John||19800515|M
```

The enrichment step will query MongoDB and add John Doe's data to the message.

---

## 💡 Key Benefits of No-Code UI

1. **No JSON Knowledge Required** - Visual builders eliminate syntax errors
2. **Autocomplete** - Suggests common field names
3. **Live Preview** - See generated MongoDB query instantly
4. **Validation** - Operator dropdowns prevent invalid queries
5. **HL7 Field Integration** - Use `{PID.3}` syntax for dynamic values
6. **Click-to-Build** - Add conditions with buttons, not typing

---

## 🔄 Comparison: SQL vs MongoDB in UI

### SQL Databases (PostgreSQL, MySQL)
- **Query**: Text area with SQL syntax
- **Parameters**: Visual builder for `$1, $2` placeholders
- **Query Tester**: Run test query before saving

### MongoDB
- **Filter**: Visual condition builder (no JSON typing!)
- **Projection**: Checkbox field selector
- **Query Tester**: Coming soon!

---

## ❓ FAQ

### Q: Can I still write JSON manually?
**A:** No - the JSON textareas have been replaced with visual builders for better UX.

### Q: What if my field isn't in the autocomplete list?
**A:** Just type it! Autocomplete is for convenience, not restriction. For projection, use "Add Custom Field" button.

### Q: How do I query nested fields?
**A:** Use dot notation in the field name:
```
Field: insurance.provider
Operator: Equals
Value: Blue Cross
```

### Q: Can I add multiple conditions (AND logic)?
**A:** Yes! Click "Add Condition" multiple times. All conditions are combined with AND.

### Q: What about OR logic?
**A:** Currently not supported in the UI builder. The visual builder uses AND logic between all conditions.

### Q: How do I use HL7 field values?
**A:** Use curly braces syntax: `{PID.3}` for patient ID, `{PID.5}` for patient name, etc.

---

## 🚀 Next Steps

1. **Try it!** Open http://localhost:3000/pipeline-builder.html
2. **Create a test interface**
3. **Add Database Enrichment step with MongoDB**
4. **Use the visual builders** to configure your query
5. **Send a test HL7 message** with MRN123456

---

## 📸 Visual Guide (What You'll See)

### MongoDB Filter Builder UI
```
┌─────────────────────────────────────────────────────┐
│ 🔍 Filter Conditions               [+ Add Condition]│
├─────────────────────────────────────────────────────┤
│ Field Name    │ Operator      │ Value      │ Actions│
│ mrn           │ Equals (=) ▼  │ {PID.3}    │ [🗑️]   │
├─────────────────────────────────────────────────────┤
│ Generated Filter (Preview)                          │
│ {                                                   │
│   "mrn": "{PID.3}"                                  │
│ }                                                   │
└─────────────────────────────────────────────────────┘
```

### MongoDB Projection Builder UI
```
┌─────────────────────────────────────────────────────┐
│ 📋 Select Fields to Return  [Select All Common...]  │
├─────────────────────────────────────────────────────┤
│ ⚪ Include selected fields (return only these)      │
│ ⚪ Exclude selected fields (return all except...)   │
├─────────────────────────────────────────────────────┤
│ ┌─────────┐ ┌─────────┐ ┌─────────┐                │
│ │☑️ mrn    │ │☑️ first │ │☑️ last  │                │
│ │MRN      │ │Name     │ │Name     │                │
│ └─────────┘ └─────────┘ └─────────┘                │
├─────────────────────────────────────────────────────┤
│ Add Custom Field: [________] [+ Add]                │
├─────────────────────────────────────────────────────┤
│ Generated Projection (Preview)                      │
│ {                                                   │
│   "mrn": 1, "firstName": 1, "lastName": 1           │
│ }                                                   │
└─────────────────────────────────────────────────────┘
```

---

**Happy Testing! 🎉**

MongoDB enrichment is now as easy as clicking checkboxes!
