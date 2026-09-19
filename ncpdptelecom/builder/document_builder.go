// ncpdptelecom/builder/document_builder.go
// BuildTransmission is the write-direction mirror of
// ncpdptelecom.ParseTransmission: ONE generic walk over the canonical JSON
// shape, driven entirely by schema data — no per-segment or
// per-transaction-code Go function.
package builder

import (
	"fmt"
	"strings"

	"ezhealthkonnect/ncpdptelecom"
)

const (
	fieldSeparator   = "\x1C" // FS
	segmentSeparator = "\x1E" // RS
	groupSeparator   = "\x1D" // GS
)

// BuildTransmission builds a complete D.0 transmission for the given
// transaction code and direction from canonical header/transmissionGroup/
// transactionGroups maps — the same shape ncpdptelecom.ParseTransmission
// produces, so a parsed transmission round-trips through
// BuildTransmission(spec, code, direction, result.Header,
// result.TransmissionGroup, result.TransactionGroups).
func BuildTransmission(
	spec *ncpdptelecom.TelecomSpecDef,
	code, direction string,
	header map[string]interface{},
	transmissionGroup map[string]interface{},
	transactionGroups []interface{},
) (string, error) {
	if spec == nil {
		return "", fmt.Errorf("ncpdptelecom: nil spec")
	}
	tx := spec.Transactions[ncpdptelecom.TransactionKey(code, direction)]
	if tx == nil {
		return "", fmt.Errorf("ncpdptelecom: unknown transaction %q direction %q", code, direction)
	}

	var sb strings.Builder
	if err := writeHeader(&sb, spec.HeaderFields, header); err != nil {
		return "", err
	}

	for _, segKey := range tx.TransmissionGroupSegments {
		segData, present := transmissionGroup[segKey]
		if !present {
			continue
		}
		segMap, ok := segData.(map[string]interface{})
		if !ok {
			continue
		}
		writeSegment(&sb, spec.Segments[segKey], segMap)
	}

	for i, rawCluster := range transactionGroups {
		cluster, ok := rawCluster.(map[string]interface{})
		if !ok {
			continue
		}
		for _, segKey := range tx.TransactionGroupSegments {
			segData, present := cluster[segKey]
			if !present {
				continue
			}
			segMap, ok := segData.(map[string]interface{})
			if !ok {
				continue
			}
			writeSegment(&sb, spec.Segments[segKey], segMap)
		}
		if i < len(transactionGroups)-1 {
			sb.WriteString(groupSeparator)
		}
	}

	return sb.String(), nil
}

func writeHeader(sb *strings.Builder, fields []ncpdptelecom.TelecomFieldDef, header map[string]interface{}) error {
	for _, f := range fields {
		v, present := header[f.Key]
		if !present {
			sb.WriteString(strings.Repeat(" ", f.Width))
			continue
		}
		raw, err := ncpdptelecom.Format(f.DataType, v, f.Places, f.Width)
		if err != nil {
			return fmt.Errorf("ncpdptelecom: formatting header field %q: %w", f.Key, err)
		}
		if len(raw) > f.Width {
			raw = raw[:f.Width]
		} else if len(raw) < f.Width {
			raw = raw + strings.Repeat(" ", f.Width-len(raw))
		}
		sb.WriteString(raw)
	}
	return nil
}

func writeSegment(sb *strings.Builder, segDef *ncpdptelecom.TelecomSegmentDef, data map[string]interface{}) {
	if segDef == nil {
		return
	}
	sb.WriteString(segmentSeparator)
	sb.WriteString(fieldSeparator)
	sb.WriteString("AM")
	sb.WriteString(segDef.Identifier)
	for _, f := range segDef.Fields {
		v, present := data[f.Key]
		if !present {
			continue
		}
		raw, err := ncpdptelecom.Format(f.DataType, v, f.Places, f.Width)
		if err != nil {
			continue // a single unformattable field is skipped rather than failing the whole transmission
		}
		sb.WriteString(fieldSeparator)
		sb.WriteString(f.FieldID)
		sb.WriteString(raw)
	}
}
