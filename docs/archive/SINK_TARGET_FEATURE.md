# Sink Target Feature - November 3, 2025

## Overview
Added **Sink** as a new target connectivity type that stores received messages in the database **without routing to any external system**. Perfect for testing, debugging, archiving, and development environments.

## What is a Sink?

A **sink** is a message endpoint that receives and stores messages but does not forward them to any destination.

### Use Cases

1. **Testing & Debugging** - Test message reception without needing a live destination
2. **Message Archiving** - Compliance and audit trail requirements  
3. **Quality Assurance** - Validate messages meet specifications
4. **Development Environments** - Safe environment without downstream impacts

## Configuration Options

1. **Enable detailed logging** (default: checked)
2. **Validate message format** (default: unchecked)
3. **Message Retention Period** (default: 30 days)
4. **Generate ACK/NACK responses** (default: checked)

## Files Modified

- InterfaceConfigComponents.js (lines 1142-1244)
- interface-config-manager.js (lines 369-469)
- interfaces.html (cache busters updated)

## Testing

1. Create new interface with Target Connectivity: Sink
2. Verify sink configuration panel appears
3. Save interface successfully
4. Check console for targetType: "sink"

