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
	Key       string `json:"key"`
	Label     string `json:"label"`
	DataType  string `json:"dataType,omitempty"`
	Required  bool   `json:"required"`  // Usage == "R" for this field/component itself
	CanRepeat bool   `json:"canRepeat"` // Repeat == "*" for this field/component itself
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
		out = append(out, CanonicalFieldInfo{
			Key: fieldKey, Label: f.Name, DataType: f.DataType,
			Required: f.Usage == "R", CanRepeat: f.Repeat == "*",
		})

		compKeys := make([]string, 0, len(f.Components))
		for ck := range f.Components {
			compKeys = append(compKeys, ck)
		}
		sortFieldKeys(compKeys)
		for _, compKey := range compKeys {
			c := f.Components[compKey]
			out = append(out, CanonicalFieldInfo{
				Key: compKey, Label: c.Name, DataType: c.DataType,
				Required: c.Usage == "R", CanRepeat: c.Repeat == "*",
			})
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

// AvailableVersions lists every HL7 version directory under schemaDir
// (e.g. "v2.5.1" -> "2.5.1"), for the hl7.build step's version picker —
// replacing what used to be a free-text version input with a schema-backed
// choice, the same "only ever show what genuinely exists" discipline
// MessageTypeCatalog/SegmentNames/SegmentFieldCatalog already follow.
func AvailableVersions(schemaDir string) []string {
	entries, err := os.ReadDir(schemaDir)
	if err != nil {
		return nil
	}
	out := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasPrefix(name, "v") || strings.HasPrefix(name, "V") {
			name = name[1:]
		}
		if name != "" {
			out = append(out, name)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return lessNumericParts(numericParts(out[i]), numericParts(out[j]))
	})
	return out
}

// SchemaTreeInfo is the JSON-serializable, cardinality-annotated view of one
// hl7.SchemaNode exposed to the hl7.build step's UI — see SchemaTree for how
// Required/CanRepeat are computed cumulatively down the tree.
type SchemaTreeInfo struct {
	Name      string            `json:"name"`
	Kind      string            `json:"kind"` // "segment" | "group"
	Usage     string            `json:"usage,omitempty"`
	Repeat    string            `json:"repeat,omitempty"`
	Required  bool              `json:"required"`  // true only if this node AND every ancestor group are usage="R"
	CanRepeat bool              `json:"canRepeat"` // true if this node's own repeat="*", OR its DIRECT enclosing group's does
	Children  []*SchemaTreeInfo `json:"children,omitempty"`
}

// SchemaTree returns schema's group/segment tree annotated with Required
// (cumulative down the FULL ancestor chain) and CanRepeat (only the
// IMMEDIATE enclosing group), computed top-down:
//
//   - Required is a transitive AND: a segment nested inside an optional group
//     is not truly required even if its own usage says "R" (e.g. ORU_R01's
//     PID sits inside the optional "PATIENT" group) — skipping any ancestor
//     makes everything beneath it unreachable, at any depth.
//   - CanRepeat is deliberately NOT transitive — only the direct parent
//     group's own repeat flag counts, not any grandparent's. ORU_R01's
//     outer "PATIENT RESULT" group repeats (batched multi-patient results),
//     but that does NOT make its grandchild PID repeatable: PID's own
//     immediate parent, "PATIENT", has repeat="1" (exactly one patient per
//     PATIENT RESULT occurrence). A grandparent-level repeat represents a
//     fundamentally different kind of repetition (a whole batched sub-block)
//     than "multiple instances of this one segment in a row", which is the
//     only thing hl7.build's "repeating" cardinality + rowsPath mechanism
//     actually models — conflating the two let PID incorrectly offer
//     "Repeating" in the UI. OBX under the (immediate parent) "OBSERVATION"
//     group is the case this SHOULD catch, and still does: OBSERVATION's own
//     repeat="*" is one level up from OBX, not two.
func SchemaTree(schema *hl7.RealHL7Schema) []*SchemaTreeInfo {
	if schema == nil {
		return nil
	}
	return annotateTree(schema.GroupTree, true, false)
}

// parentCanRepeat is the IMMEDIATE parent's own repeat=="*" flag (not
// cumulative) — recursion passes each node's own ownRepeats down to its
// children, never the child's already-OR'd canRepeat, so inheritance stops
// after exactly one level.
func annotateTree(nodes []*hl7.SchemaNode, ancestorRequired, parentCanRepeat bool) []*SchemaTreeInfo {
	out := make([]*SchemaTreeInfo, 0, len(nodes))
	for _, n := range nodes {
		required := ancestorRequired && n.Usage == "R"
		ownRepeats := n.Repeat == "*"
		canRepeat := parentCanRepeat || ownRepeats
		info := &SchemaTreeInfo{
			Name: n.Name, Kind: n.Kind, Usage: n.Usage, Repeat: n.Repeat,
			Required: required, CanRepeat: canRepeat,
		}
		if len(n.Children) > 0 {
			info.Children = annotateTree(n.Children, required, ownRepeats)
		}
		out = append(out, info)
	}
	return out
}

// RequiredSpine walks SchemaTree collecting every segment along the
// mandatory path — segments a user configuring hl7.build from scratch should
// see pre-populated rather than having to remember to add. MSH is excluded:
// it's structurally auto-populated by hl7/builder.BuildMessage from top-level
// config, never a generic segment in the step's segments list.
func RequiredSpine(schema *hl7.RealHL7Schema) []string {
	var out []string
	var walk func(nodes []*SchemaTreeInfo)
	walk = func(nodes []*SchemaTreeInfo) {
		for _, n := range nodes {
			if n.Kind == "segment" && n.Required && n.Name != "MSH" {
				out = append(out, n.Name)
			}
			if len(n.Children) > 0 {
				walk(n.Children)
			}
		}
	}
	walk(SchemaTree(schema))
	return out
}

// hl7Slot is one segment occurrence in a schema's flattened document order —
// the unit NextAllowedSegments matches against. Group wrapper nodes
// themselves don't appear, only their leaf segments, in preorder.
type hl7Slot struct {
	Name      string
	Required  bool
	CanRepeat bool
}

func flattenSlots(nodes []*SchemaTreeInfo) []hl7Slot {
	var out []hl7Slot
	for _, n := range nodes {
		if n.Kind == "segment" {
			out = append(out, hl7Slot{Name: n.Name, Required: n.Required, CanRepeat: n.CanRepeat})
		}
		if len(n.Children) > 0 {
			out = append(out, flattenSlots(n.Children)...)
		}
	}
	return out
}

// NextAllowedSegments computes which segment names are grammatically valid
// to add next in the hl7.build step's "Add Segment" picker, given
// addedSegments already configured (in order, MSH excluded — handled
// separately). Returns allowed (the default picker list) and all (every
// segment in the schema, for the picker's explicit "add anyway
// (non-standard)" override).
//
// This is a deliberately conservative, forward-only guardrail, not a full
// HL7 conformance validator: it greedily advances a cursor through the
// schema's flattened segment order and never backtracks, so an
// unusual-but-valid configuration built via the override may leave it more
// permissive than a strict profile checker would be. That's an acceptable
// trade-off since "allowed" is only ever a convenience default — "all" is
// always one click away — and it keeps the guardrail's one implementation
// (this function) simple enough to reason about and test, rather than
// reimplementing full regular-grammar derivative matching for a UI hint.
func NextAllowedSegments(schema *hl7.RealHL7Schema, addedSegments []string) (allowed []string, all []string) {
	slots := make([]hl7Slot, 0)
	for _, s := range flattenSlots(SchemaTree(schema)) {
		if s.Name != "MSH" {
			slots = append(slots, s)
		}
	}

	seenAll := make(map[string]bool, len(slots))
	all = make([]string, 0, len(slots))
	for _, s := range slots {
		if !seenAll[s.Name] {
			seenAll[s.Name] = true
			all = append(all, s.Name)
		}
	}

	cursor := 0
	for _, name := range addedSegments {
		if name == "" || name == "MSH" {
			continue
		}
		// Repeat-in-place: re-adding the segment the cursor just passed
		// doesn't advance further, so a run of repeats (OBX, OBX, OBX) all
		// resolve to the same slot instead of searching past it.
		if cursor > 0 && slots[cursor-1].Name == name && slots[cursor-1].CanRepeat {
			continue
		}
		matched := -1
		for i := cursor; i < len(slots); i++ {
			if slots[i].Name == name {
				matched = i
				break
			}
		}
		if matched == -1 {
			// Added via the "non-standard" override, or a repeat of a
			// segment that can't repeat — don't move the cursor for it;
			// best-effort, never hard-fail on something we can't place.
			continue
		}
		cursor = matched + 1
	}

	seen := make(map[string]bool)
	allowed = make([]string, 0)
	// Always allow another repeat of whatever segment the cursor just passed.
	if cursor > 0 && slots[cursor-1].CanRepeat {
		allowed = append(allowed, slots[cursor-1].Name)
		seen[slots[cursor-1].Name] = true
	}
	for i := cursor; i < len(slots); i++ {
		if !seen[slots[i].Name] {
			seen[slots[i].Name] = true
			allowed = append(allowed, slots[i].Name)
		}
		if slots[i].Required {
			break
		}
	}

	return allowed, all
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
