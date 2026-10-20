// services/ai/schema_digest_hl7.go
//
// HL7 v2 digester: one document per message structure (its segments and
// groups in order, with optionality/repetition) and one per distinct segment
// (its fields with data type, optionality, repetition, table and component
// names). Segments are shared by many message files, so each is emitted once.
package ai

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

type hl7v2Digester struct {
	version      string
	seenSegments map[string]bool
}

func newHL7v2Digester(version string) *hl7v2Digester {
	return &hl7v2Digester{version: version, seenSegments: map[string]bool{}}
}

// hl7SchemaFile is one message structure file (schemas/hl7/<version>/<MSG>.gz).
type hl7SchemaFile struct {
	MessageType string        `json:"messageType"`
	Description *string       `json:"description"`
	Structure   orderedHL7Nodes `json:"structure"`
}

// hl7SchemaNode is a segment or a group inside a message structure.
type hl7SchemaNode struct {
	Name        string                    `json:"-"`
	Description *string                   `json:"description"`
	Usage       string                    `json:"usage"`
	Repeat      string                    `json:"repeat"`
	Type        string                    `json:"type"` // "segment" | "group"
	Fields      map[string]hl7SchemaField `json:"fields"`
	Segments    orderedHL7Nodes           `json:"segments"`
}

// hl7SchemaField is one field; components are decoded one level deep only.
type hl7SchemaField struct {
	Name         string                       `json:"name"`
	DataType     string                       `json:"dataType"`
	DataTypeName string                       `json:"dataTypeName"`
	Usage        string                       `json:"usage"`
	Repeat       string                       `json:"repeat"`
	Description  *string                      `json:"description"`
	TableID      interface{}                  `json:"tableId"`
	TableName    *string                      `json:"tableName"`
	Components   map[string]hl7SchemaComponent `json:"components"`
}

type hl7SchemaComponent struct {
	Name     string `json:"name"`
	DataType string `json:"dataType"`
}

// orderedHL7Nodes decodes a JSON object of nodes keeping key order — the
// files' "sequence" values are inconsistent (strings, numbers, blanks), so the
// object order is the only reliable segment order.
type orderedHL7Nodes []hl7SchemaNode

func (o *orderedHL7Nodes) UnmarshalJSON(b []byte) error {
	if bytes.Equal(bytes.TrimSpace(b), []byte("null")) {
		return nil
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	if t, err := dec.Token(); err != nil || t != json.Delim('{') {
		return fmt.Errorf("structure: expected object")
	}
	for dec.More() {
		t, err := dec.Token()
		if err != nil {
			return err
		}
		key, _ := t.(string)
		var node hl7SchemaNode
		if err := dec.Decode(&node); err != nil {
			return fmt.Errorf("structure %s: %w", key, err)
		}
		node.Name = key
		*o = append(*o, node)
	}
	return nil
}

func (d *hl7v2Digester) DigestFile(name string, content []byte) ([]SchemaDigestDoc, error) {
	var f hl7SchemaFile
	if err := json.Unmarshal(content, &f); err != nil {
		return nil, err
	}
	msgType := f.MessageType
	if msgType == "" {
		msgType = name
	}

	var docs []SchemaDigestDoc
	var b strings.Builder
	fmt.Fprintf(&b, "HL7 v%s message structure %s", d.version, msgType)
	if f.Description != nil && strings.TrimSpace(*f.Description) != "" {
		fmt.Fprintf(&b, " — %s", firstSentence(*f.Description, 200))
	}
	b.WriteString("\nSegments in order (R = required, O = optional; * = repeating):\n")
	d.writeStructure(&b, f.Structure, 0)
	docs = append(docs, SchemaDigestDoc{Ref: "message:" + msgType, Text: b.String()})

	d.collectSegments(f.Structure, &docs)
	return docs, nil
}

func (d *hl7v2Digester) writeStructure(b *strings.Builder, nodes orderedHL7Nodes, depth int) {
	indent := strings.Repeat("  ", depth)
	for _, n := range nodes {
		if n.Type == "group" {
			fmt.Fprintf(b, "%s%s group [%s]\n", indent, n.Name, hl7Cardinality(n.Usage, n.Repeat))
			d.writeStructure(b, n.Segments, depth+1)
			continue
		}
		line := fmt.Sprintf("%s%s [%s]", indent, n.Name, hl7Cardinality(n.Usage, n.Repeat))
		if n.Description != nil && *n.Description != "" {
			line += " " + firstSentence(*n.Description, 90)
		}
		b.WriteString(line + "\n")
	}
}

func (d *hl7v2Digester) collectSegments(nodes orderedHL7Nodes, docs *[]SchemaDigestDoc) {
	for _, n := range nodes {
		if n.Type == "group" {
			d.collectSegments(n.Segments, docs)
			continue
		}
		if d.seenSegments[n.Name] || len(n.Fields) == 0 {
			continue
		}
		d.seenSegments[n.Name] = true
		*docs = append(*docs, SchemaDigestDoc{Ref: "segment:" + n.Name, Text: d.segmentText(n)})
	}
}

func (d *hl7v2Digester) segmentText(n hl7SchemaNode) string {
	var b strings.Builder
	fmt.Fprintf(&b, "HL7 v%s segment %s", d.version, n.Name)
	if n.Description != nil && *n.Description != "" {
		fmt.Fprintf(&b, " — %s", firstSentence(*n.Description, 300))
	}
	b.WriteString("\nFields:\n")
	for _, key := range sortedHL7Keys(n.Fields) {
		f := n.Fields[key]
		fmt.Fprintf(&b, "%s %s (%s", key, f.Name, f.DataType)
		if f.DataTypeName != "" && f.DataTypeName != f.Name {
			fmt.Fprintf(&b, " %s", f.DataTypeName)
		}
		fmt.Fprintf(&b, "; %s)", hl7Cardinality(f.Usage, f.Repeat))
		if f.TableName != nil && *f.TableName != "" {
			fmt.Fprintf(&b, " table %v %s", f.TableID, *f.TableName)
		} else if f.TableID != nil {
			fmt.Fprintf(&b, " table %v", f.TableID)
		}
		if f.Description != nil && *f.Description != "" {
			fmt.Fprintf(&b, " — %s", firstSentence(*f.Description, 140))
		}
		if len(f.Components) > 0 {
			var parts []string
			for _, ck := range sortedHL7Keys(f.Components) {
				c := f.Components[ck]
				parts = append(parts, fmt.Sprintf("%s %s (%s)", ck, c.Name, c.DataType))
			}
			b.WriteString("\n    components: " + strings.Join(parts, "; "))
		}
		b.WriteString("\n")
	}
	return b.String()
}

// hl7Cardinality renders usage + repeat, e.g. "R", "O, *", "O, up to 3".
func hl7Cardinality(usage, repeat string) string {
	u := strings.TrimSpace(usage)
	switch u {
	case "R", "O", "C", "B", "X", "W", "RE":
	case "":
		u = "O"
	}
	switch strings.TrimSpace(repeat) {
	case "", "1", "0":
		return u
	case "*", "Y", "unbounded":
		return u + ", *"
	default:
		return u + ", up to " + repeat
	}
}

// sortedHL7Keys orders "PID.2" < "PID.10" (numeric on each dotted part).
func sortedHL7Keys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return hl7KeyLess(keys[i], keys[j]) })
	return keys
}

func hl7KeyLess(a, b string) bool {
	pa, pb := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < len(pa) && i < len(pb); i++ {
		na, ea := strconv.Atoi(pa[i])
		nb, eb := strconv.Atoi(pb[i])
		if ea == nil && eb == nil {
			if na != nb {
				return na < nb
			}
			continue
		}
		if pa[i] != pb[i] {
			return pa[i] < pb[i]
		}
	}
	return len(pa) < len(pb)
}
