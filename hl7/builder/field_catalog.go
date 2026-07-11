// hl7/builder/field_catalog.go
//
// Public, read-only catalog of every field/component key hl7.build can write
// for one segment — sourced live from hl7.RealHL7Schema, the SAME compiled
// schema hl7.ParseWithRealSchema already reads. This is the single source of
// truth the hl7.build pipeline step's no-code field-mapping UI is built from
// (via a small API endpoint), mirroring cda/builder/canonical_field_catalog.go
// and fhir/builder/canonical_field_catalog.go's identical role for their own
// formats — so a user configuring a mapping only ever sees field keys that
// genuinely exist in the schema, never a stale or invented vocabulary.
package builder

import (
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"ezhealthkonnect/hl7"
)

// CanonicalFieldInfo describes one field/component key available as an
// hl7.build mapping target.
type CanonicalFieldInfo struct {
	Key      string `json:"key"`
	Label    string `json:"label"`
	DataType string `json:"dataType,omitempty"`
}

// SegmentFieldCatalog returns every field/component key hl7.build can write
// for one segment (e.g. "PID"), fully-qualified the same way
// hl7/real_schema_parser.go's schema JSON already keys them ("PID.5",
// "PID.5.1", ...) — the same convention Segment.Set expects. Returns nil
// when segmentName isn't defined in schema.
func SegmentFieldCatalog(schema *hl7.RealHL7Schema, segmentName string) []CanonicalFieldInfo {
	if schema == nil {
		return nil
	}
	segDef, ok := schema.Segments[segmentName]
	if !ok {
		return nil
	}

	fieldKeys := make([]string, 0, len(segDef.Fields))
	for k := range segDef.Fields {
		fieldKeys = append(fieldKeys, k)
	}
	sortFieldKeys(fieldKeys)

	out := make([]CanonicalFieldInfo, 0, len(segDef.Fields)*2)
	for _, fieldKey := range fieldKeys {
		f := segDef.Fields[fieldKey]
		out = append(out, CanonicalFieldInfo{Key: fieldKey, Label: f.Name, DataType: f.DataType})

		compKeys := make([]string, 0, len(f.Components))
		for ck := range f.Components {
			compKeys = append(compKeys, ck)
		}
		sortFieldKeys(compKeys)
		for _, compKey := range compKeys {
			c := f.Components[compKey]
			out = append(out, CanonicalFieldInfo{Key: compKey, Label: c.Name, DataType: c.DataType})
		}
	}
	return out
}

// SegmentNames returns every segment name defined in schema, for the
// hl7.build step's "add segment" picker.
func SegmentNames(schema *hl7.RealHL7Schema) []string {
	if schema == nil {
		return nil
	}
	names := make([]string, 0, len(schema.SegmentOrder))
	seen := make(map[string]bool, len(schema.SegmentOrder))
	for _, name := range schema.SegmentOrder {
		if !seen[name] {
			seen[name] = true
			names = append(names, name)
		}
	}
	return names
}

// MessageTypeInfo describes one available message-type/trigger-event
// combination for one version, for the hl7.build step's message-type picker.
type MessageTypeInfo struct {
	MessageType  string `json:"messageType"`
	TriggerEvent string `json:"triggerEvent"`
}

// MessageTypeCatalog lists every "{MessageType}_{TriggerEvent}.gz" schema
// file under schemaDir/v{version}/ — read-only directory enumeration, the
// same file-naming convention hl7/real_schema_parser.go's own (unexported)
// scanForSchemaFiles relies on, just exposed here for the hl7.build step's
// no-code UI. Returns nil if the version directory doesn't exist.
func MessageTypeCatalog(schemaDir, version string) []MessageTypeInfo {
	v := strings.TrimSpace(version)
	if !strings.HasPrefix(v, "v") && !strings.HasPrefix(v, "V") {
		v = "v" + v
	}
	entries, err := os.ReadDir(filepath.Join(schemaDir, v))
	if err != nil {
		return nil
	}

	out := make([]MessageTypeInfo, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".gz") {
			continue
		}
		name := strings.TrimSuffix(entry.Name(), ".gz")
		parts := strings.SplitN(name, "_", 2)
		if len(parts) != 2 {
			continue
		}
		out = append(out, MessageTypeInfo{MessageType: parts[0], TriggerEvent: parts[1]})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].MessageType != out[j].MessageType {
			return out[i].MessageType < out[j].MessageType
		}
		return out[i].TriggerEvent < out[j].TriggerEvent
	})
	return out
}

// sortFieldKeys orders fully-qualified keys ("PID.5", "PID.10", "PID.5.1")
// numerically by their dot-separated position segments, not lexicographically
// — a plain string sort would place "PID.10" before "PID.2".
func sortFieldKeys(keys []string) {
	sort.Slice(keys, func(i, j int) bool {
		return lessNumericParts(numericParts(keys[i]), numericParts(keys[j]))
	})
}

// lessNumericParts reports whether a sorts before b, comparing element-wise
// as integers (a shorter, otherwise-equal prefix sorts first, e.g. "PID.5"
// before "PID.5.1").
func lessNumericParts(a, b []int) bool {
	for i := 0; i < len(a) && i < len(b); i++ {
		if a[i] != b[i] {
			return a[i] < b[i]
		}
	}
	return len(a) < len(b)
}

// numericParts extracts every dot-separated numeric segment from a
// fully-qualified key (e.g. "PID.5.1" -> [5, 1]), skipping the non-numeric
// segment name itself.
func numericParts(key string) []int {
	parts := strings.Split(key, ".")
	nums := make([]int, 0, len(parts))
	for _, p := range parts {
		if n, err := strconv.Atoi(p); err == nil {
			nums = append(nums, n)
		}
	}
	return nums
}
