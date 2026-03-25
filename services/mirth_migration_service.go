package services

import (
	"context"
	"database/sql"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"log"
	"regexp"
	"strconv"
	"strings"
	"time"

	"ezhealthkonnect/models"

	"github.com/google/uuid"
)

// MaxMirthXMLBytes is the maximum size of a Mirth channel XML accepted by the service.
const MaxMirthXMLBytes = 10 * 1024 * 1024 // 10 MB

// MirthMigrationService parses Mirth Connect channel XML exports and converts
// them into ezHealthKonnect interfaces, pipelines, and steps.
type MirthMigrationService struct {
	db *sql.DB
}

func NewMirthMigrationService(db *sql.DB) *MirthMigrationService {
	return &MirthMigrationService{db: db}
}

// ============================================================================
// Public API
// ============================================================================

// Preview parses raw Mirth channel XML and returns a MigrationPreview without
// writing anything to the database.
func (s *MirthMigrationService) Preview(xmlData []byte) (*models.MigrationPreview, error) {
	if len(xmlData) > MaxMirthXMLBytes {
		return nil, fmt.Errorf("XML exceeds maximum allowed size of %d MB", MaxMirthXMLBytes/1024/1024)
	}
	ch, err := parseMirthXML(xmlData)
	if err != nil {
		return nil, fmt.Errorf("invalid Mirth XML: %w", err)
	}
	return buildPreview(ch), nil
}

// History returns recent migration history entries for the given limit.
func (s *MirthMigrationService) History(ctx context.Context, limit int) ([]models.MirthMigrationHistoryEntry, error) {
	if limit <= 0 || limit > 100 {
		limit = 25
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT h.id, h.interface_id, h.channel_id, h.channel_name, h.mirth_version,
		       h.steps_created, h.steps_skipped, h.migrated_by, h.created_at,
		       i.name AS interface_name
		FROM   mirth_migration_history h
		LEFT   JOIN interfaces i ON i.id = h.interface_id
		ORDER  BY h.created_at DESC
		LIMIT  $1`, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query migration history: %w", err)
	}
	defer rows.Close()

	var entries []models.MirthMigrationHistoryEntry
	for rows.Next() {
		var e models.MirthMigrationHistoryEntry
		var iid, cid, mv, mb, iname sql.NullString
		if err := rows.Scan(
			&e.ID, &iid, &cid, &e.ChannelName, &mv,
			&e.StepsCreated, &e.StepsSkipped, &mb, &e.CreatedAt, &iname,
		); err != nil {
			continue
		}
		e.InterfaceID = iid.String
		e.ChannelID = cid.String
		e.MirthVersion = mv.String
		e.MigratedBy = mb.String
		e.InterfaceName = iname.String
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

// Import parses the raw channel XML string, creates the interface, pipeline,
// and steps in the database, and returns the result.
func (s *MirthMigrationService) Import(ctx context.Context, req *models.MigrationImportRequest) (*models.MigrationImportResult, error) {
	if len(req.ChannelXML) > MaxMirthXMLBytes {
		return nil, fmt.Errorf("XML exceeds maximum allowed size of %d MB", MaxMirthXMLBytes/1024/1024)
	}

	ch, err := parseMirthXML([]byte(req.ChannelXML))
	if err != nil {
		return nil, fmt.Errorf("invalid Mirth XML: %w", err)
	}

	preview := buildPreview(ch)

	interfaceName := req.InterfaceName
	if interfaceName == "" {
		interfaceName = ch.Name
	}
	description := req.Description
	if description == "" {
		description = fmt.Sprintf("Migrated from Mirth Connect (channel: %s, version: %s)", ch.Name, ch.Version)
	}

	// Build skip set
	skipSet := make(map[int]bool, len(req.SkipStepIndices))
	for _, i := range req.SkipStepIndices {
		skipSet[i] = true
	}

	return s.writeToDatabase(ctx, ch, preview, interfaceName, description, req.UserID, skipSet)
}

// ============================================================================
// XML Parsing
// ============================================================================

func parseMirthXML(data []byte) (*models.MirthChannelXML, error) {
	var ch models.MirthChannelXML
	if err := xml.Unmarshal(data, &ch); err != nil {
		return nil, err
	}
	if ch.Name == "" {
		return nil, fmt.Errorf("channel has no name — may not be a valid Mirth channel export")
	}
	return &ch, nil
}

// ============================================================================
// Preview Builder
// ============================================================================

func buildPreview(ch *models.MirthChannelXML) *models.MigrationPreview {
	source := mapConnector(ch.SourceConnector, "inbound")

	destinations := make([]models.MappedConnector, 0, len(ch.DestConnectors))
	for _, d := range ch.DestConnectors {
		destinations = append(destinations, mapConnector(d, "outbound"))
	}

	steps := analyzeSteps(
		parseStepsFromInner(ch.SourceConnector.Transformer.Inner),
		parseRulesFromInner(ch.SourceConnector.Filter.Inner),
	)

	summary := buildSummary(source, destinations, steps)

	return &models.MigrationPreview{
		ChannelID:    ch.ID,
		ChannelName:  ch.Name,
		Description:  ch.Description,
		MirthVersion: ch.Version,
		Enabled:      ch.Enabled,
		Source:       source,
		Destinations: destinations,
		Steps:        steps,
		Summary:      summary,
	}
}

func buildSummary(source models.MappedConnector, dests []models.MappedConnector, steps []models.TransformerStepAnalysis) models.MigrationSummary {
	s := models.MigrationSummary{TotalSteps: len(steps)}
	for _, st := range steps {
		switch st.Conversion {
		case models.ConversionAuto:
			s.AutoMapped++
		case models.ConversionAI:
			s.AutoMapped++ // AI-rewritten counts as mapped (user already reviewed)
		default:
			s.NeedsReview++
		}
	}
	if source.Status == models.MirthMappingAuto {
		s.AutoMapped++
	} else if source.Status == models.MirthMappingUnsupported {
		s.Unsupported++
	}
	for _, d := range dests {
		if d.Status == models.MirthMappingAuto {
			s.AutoMapped++
		} else if d.Status == models.MirthMappingUnsupported {
			s.Unsupported++
		}
	}
	return s
}

// ============================================================================
// Connector Mapping
// ============================================================================

// transportMap maps Mirth transport names to ezHealthKonnect connector types.
// direction is "inbound" or "outbound".
var inboundTransportMap = map[string]string{
	"TCP Listener":          "tcp_mllp_inbound",
	"HTTP Listener":         "http_rest_inbound",
	"File Reader":           "file_listener",
	"Database Reader":       "postgresql_inbound",
	"SFTP Listener":         "sftp_inbound",
	"FTP Listener":          "ftp_inbound",
	"JMS Listener":          "rabbitmq_inbound",
	"Kafka Consumer":        "kafka_inbound",
	"Web Service Listener":  "http_rest_inbound",
	"Amazon SQS":            "aws_s3_inbound",
	"JavaScript Reader":     "http_rest_inbound", // JS-based polling — closest equivalent
}

var outboundTransportMap = map[string]string{
	"TCP Sender":         "tcp_mllp_outbound",
	"HTTP Sender":        "http_outbound",
	"File Writer":        "file_writer",
	"Database Writer":    "postgresql_outbound",
	"SFTP Sender":        "sftp_outbound",
	"FTP Sender":         "ftp_outbound",
	"JMS Writer":         "rabbitmq_outbound",
	"Kafka Producer":     "kafka_outbound",
	"Web Service Sender": "http_outbound",
	"JavaScript Writer":  "file_writer", // JS-based output — approximate
}

// internalTransports are Mirth-internal connectors with no external equivalent.
var internalTransports = map[string]bool{
	"Channel Reader": true,
	"Channel Writer": true,
	"VM Listener":    true,
	"VM Sender":      true,
}

// unsupportedTransports are outbound transports with no ezHK connector equivalent.
var unsupportedTransports = map[string]string{
	"Document Writer":  "PDF/document generation — implement via script step with HTML template",
	"SMTP Sender":      "Email delivery — no SMTP connector in ezHealthKonnect; use webhook/HTTP outbound",
	"Email Sender":     "Email delivery — no SMTP connector in ezHealthKonnect; use webhook/HTTP outbound",
}

// classToConnectorMap maps Mirth Java class names to ezHK connector types.
// Used as fallback when the transport name string is not recognised.
var classToConnectorMap = map[string]string{
	"com.mirth.connect.connectors.tcp.TcpReceiverProperties":         "tcp_mllp_inbound",
	"com.mirth.connect.connectors.tcp.TcpDispatcherProperties":       "tcp_mllp_outbound",
	"com.mirth.connect.connectors.file.FileReceiverProperties":       "file_listener",
	"com.mirth.connect.connectors.file.FileDispatcherProperties":     "file_writer",
	"com.mirth.connect.connectors.jdbc.DatabaseReceiverProperties":   "postgresql_inbound",
	"com.mirth.connect.connectors.jdbc.DatabaseDispatcherProperties": "postgresql_outbound",
	"com.mirth.connect.connectors.ftp.FtpReceiverProperties":         "ftp_inbound",
	"com.mirth.connect.connectors.ftp.FtpDispatcherProperties":       "ftp_outbound",
	"com.mirth.connect.connectors.sftp.SftpReceiverProperties":       "sftp_inbound",
	"com.mirth.connect.connectors.sftp.SftpDispatcherProperties":     "sftp_outbound",
	"com.mirth.connect.connectors.http.HttpReceiverProperties":       "http_rest_inbound",
	"com.mirth.connect.connectors.http.HttpDispatcherProperties":     "http_outbound",
	"com.mirth.connect.connectors.ws.WebServiceReceiverProperties":   "http_rest_inbound",
	"com.mirth.connect.connectors.ws.WebServiceDispatcherProperties": "http_outbound",
}

// classUnsupported maps Java class names that have no ezHK connector equivalent.
var classUnsupported = map[string]string{
	"com.mirth.connect.connectors.smtp.SmtpDispatcherProperties":    "Email (SMTP) delivery — no SMTP connector in ezHealthKonnect; use HTTP outbound",
	"com.mirth.connect.connectors.doc.DocumentDispatcherProperties": "PDF/document generation — no native equivalent; implement as script step",
	"com.mirth.connect.connectors.js.JavaScriptDispatcherProperties": "JavaScript Dispatcher — convert logic to an enrichment.script step",
	"com.mirth.connect.connectors.js.JavaScriptReceiverProperties":   "JavaScript Reader — convert logic to an enrichment.script step",
}

// classInternal maps Java class names for internal Mirth VM connectors.
var classInternal = map[string]bool{
	"com.mirth.connect.connectors.vm.VmReceiverProperties":  true,
	"com.mirth.connect.connectors.vm.VmDispatcherProperties": true,
}

func mapConnector(c models.MirthConnectorXML, direction string) models.MappedConnector {
	transport := c.TransportName
	inner := c.Properties.Inner
	class := c.Properties.Class

	result := models.MappedConnector{
		MirthTransport: transport,
		MirthClass:     class,
		Config:         make(map[string]interface{}),
	}

	// 1. Internal channel-chaining connectors
	if internalTransports[transport] || classInternal[class] {
		result.EzHKConnectorType = ""
		result.Status = models.MirthMappingUnsupported
		result.Note = "Internal Mirth channel chaining — no external equivalent in ezHealthKonnect"
		return result
	}

	// 2. Unsupported transports (SMTP, Document, etc.)
	if note, bad := unsupportedTransports[transport]; bad {
		result.EzHKConnectorType = ""
		result.Status = models.MirthMappingUnsupported
		result.Note = note
		return result
	}
	if note, bad := classUnsupported[class]; bad {
		result.EzHKConnectorType = ""
		result.Status = models.MirthMappingUnsupported
		result.Note = note
		return result
	}

	// 3. Transport-name lookup
	var mapped string
	var ok bool
	if direction == "inbound" {
		mapped, ok = inboundTransportMap[transport]
	} else {
		mapped, ok = outboundTransportMap[transport]
	}

	// 4. Class-based fallback (handles localisation differences and version quirks)
	if !ok {
		mapped, ok = classToConnectorMap[class]
	}

	if !ok {
		result.EzHKConnectorType = "tcp_mllp_inbound" // safest default
		result.Status = models.MirthMappingReview
		result.Note = fmt.Sprintf("Unknown transport '%s' (class: %s) — defaulted to tcp_mllp_inbound, please review", transport, class)
		return result
	}

	result.EzHKConnectorType = mapped
	result.Config = extractConnectorConfig(transport, class, inner)
	result.Status = models.MirthMappingAuto
	return result
}

// extractConnectorConfig extracts key config fields from the raw Mirth properties XML.
func extractConnectorConfig(transport, class string, inner []byte) map[string]interface{} {
	cfg := make(map[string]interface{})

	switch transport {
	case "TCP Listener":
		host := xmlVal(inner, "host")
		if host == "" {
			host = "0.0.0.0"
		}
		cfg["host"] = host
		cfg["port"] = clampPort(xmlValInt(inner, "port", 6661), 6661)
		if xmlVal(inner, "mllpMode") == "true" {
			cfg["mllpMode"] = true
		}
		if mc := xmlValInt(inner, "maxConnections", 0); mc > 0 {
			cfg["maxConnections"] = mc
		}

	case "TCP Sender":
		cfg["host"] = xmlVal(inner, "remoteAddress")
		cfg["port"] = clampPort(xmlValInt(inner, "remotePort", 2575), 2575)

	case "HTTP Listener":
		host := xmlVal(inner, "host")
		if host == "" {
			host = "0.0.0.0"
		}
		cfg["host"] = host
		cfg["port"] = clampPort(xmlValInt(inner, "port", 8080), 8080)
		if p := xmlVal(inner, "contextPath"); p != "" {
			cfg["path"] = p
		}

	case "HTTP Sender", "Web Service Sender":
		cfg["endpoint"] = xmlVal(inner, "host") // Mirth uses "host" for the URL in HTTP sender
		cfg["method"] = xmlValDefault(inner, "method", "POST")

	case "File Reader":
		cfg["directory"] = xmlVal(inner, "host") // Mirth uses "host" for directory path
		cfg["pattern"] = xmlValDefault(inner, "fileFilter", "*.*")
		if freq := xmlValInt(inner, "pollingFrequency", 0); freq > 0 {
			cfg["pollIntervalMs"] = freq
		}

	case "File Writer":
		cfg["directory"] = xmlVal(inner, "host")
		cfg["filename"] = xmlValDefault(inner, "outputPattern", "${originalFilename}")

	case "Database Reader":
		cfg["url"] = xmlVal(inner, "url") // JDBC URL — user will need to convert
		cfg["query"] = xmlVal(inner, "select")

	case "Database Writer":
		cfg["url"] = xmlVal(inner, "url")
		cfg["statement"] = xmlVal(inner, "insert")

	case "SFTP Listener", "SFTP Sender", "FTP Listener", "FTP Sender":
		cfg["host"] = xmlVal(inner, "host")
		cfg["port"] = xmlValInt(inner, "port", 22)
		cfg["username"] = xmlVal(inner, "username")
		cfg["path"] = xmlValDefault(inner, "path", "/")
	}

	return cfg
}

// ============================================================================
// Step / Filter Parsing  (handles both Mirth 3.x <elements> and legacy <steps>)
// ============================================================================

// xmlDecodeEntities replaces the five predefined XML entities with their characters.
func xmlDecodeEntities(s string) string {
	s = strings.ReplaceAll(s, "&apos;", "'")
	s = strings.ReplaceAll(s, "&quot;", "\"")
	s = strings.ReplaceAll(s, "&amp;", "&")
	s = strings.ReplaceAll(s, "&lt;", "<")
	s = strings.ReplaceAll(s, "&gt;", ">")
	return s
}

// parseStepsFromInner extracts transformer steps from the raw inner XML of a
// <transformer> element, supporting both the modern <elements> format (Mirth 3.x,
// where each child element is named after its Java class) and the legacy
// <steps><step> format used in earlier Mirth versions.
func parseStepsFromInner(inner []byte) []models.MirthStepXML {
	if len(inner) == 0 {
		return nil
	}

	// ── Modern format: <elements><com.mirth...Step>...</com.mirth...Step></elements> ──
	elRe := regexp.MustCompile(`(?s)<elements[^>]*>(.*?)</elements>`)
	if m := elRe.FindSubmatch(inner); m != nil {
		return parseElementChildren(m[1])
	}

	// ── Legacy format: <steps><step>...</step></steps> ──
	stRe := regexp.MustCompile(`(?s)<steps[^>]*>(.*?)</steps>`)
	if m := stRe.FindSubmatch(inner); m != nil {
		var wrapper struct {
			Steps []models.MirthStepXML `xml:"step"`
		}
		if err := xml.Unmarshal(append([]byte("<steps>"), append(m[1], []byte("</steps>")...)...), &wrapper); err == nil {
			return wrapper.Steps
		}
	}
	return nil
}

// parseElementChildren parses the children of a Mirth 3.x <elements> block.
// Each child is a Java-class-named element; we derive the step type from the
// last component of the class name (e.g. "MessageBuilderStep").
//
// NOTE: Go RE2 does not support backreferences (\1), so we cannot use a single
// regex to match open+close tags. Instead we find opening tags first, then
// build per-tag regexes using the exact tag name.
func parseElementChildren(elementsBody []byte) []models.MirthStepXML {
	// Find all Java class opening tags (com.* or org.*)
	openTagRe := regexp.MustCompile(`<((?:com|org)\.[a-zA-Z0-9_.]+)(?:\s[^>]*)?>`)
	tagMatches := openTagRe.FindAllSubmatch(elementsBody, -1)

	seqRe     := regexp.MustCompile(`(?s)<sequenceNumber>(\d+)</sequenceNumber>`)
	nameRe    := regexp.MustCompile(`(?s)<name>(.*?)</name>`)
	enabledRe := regexp.MustCompile(`(?s)<enabled>(true|false)</enabled>`)

	seen  := make(map[string]bool)
	steps := make([]models.MirthStepXML, 0, len(tagMatches))

	for _, tm := range tagMatches {
		tag := strings.TrimSpace(string(tm[1]))
		if tag == "" || seen[tag] {
			continue
		}
		seen[tag] = true

		// Build a regex that finds every instance of this element.
		// Use Compile (not MustCompile) so a bad tag can't panic the handler.
		qTag   := regexp.QuoteMeta(tag)
		elemRe, compErr := regexp.Compile(`(?s)<` + qTag + `(?:\s[^>]*)?>` + `(.*?)</` + qTag + `>`)
		if compErr != nil {
			continue
		}
		bodies := elemRe.FindAllSubmatch(elementsBody, -1)

		// Derive step type from last segment of the class name (e.g. "MessageBuilderStep")
		parts    := strings.Split(tag, ".")
		stepType := parts[len(parts)-1]

		for _, bm := range bodies {
			body := bm[1]

			// Skip steps explicitly disabled in Mirth
			if em := enabledRe.FindSubmatch(body); em != nil && string(em[1]) == "false" {
				continue
			}

			seq := 0
			if sm := seqRe.FindSubmatch(body); sm != nil {
				seq, _ = strconv.Atoi(string(sm[1]))
			}
			name := ""
			if nm := nameRe.FindSubmatch(body); nm != nil {
				name = xmlDecodeEntities(strings.TrimSpace(string(nm[1])))
			}

			steps = append(steps, models.MirthStepXML{
				SequenceNumber: seq,
				Name:           name,
				Type:           stepType,
				Inner:          body,
			})
		}
	}

	// Sort by sequence number
	for i := 1; i < len(steps); i++ {
		for j := i; j > 0 && steps[j].SequenceNumber < steps[j-1].SequenceNumber; j-- {
			steps[j], steps[j-1] = steps[j-1], steps[j]
		}
	}
	return steps
}

// parseRulesFromInner extracts filter rules from the raw inner XML of a <filter>
// element, supporting both <elements> (Mirth 3.x) and <rules><rule> formats.
func parseRulesFromInner(inner []byte) []models.MirthRuleXML {
	if len(inner) == 0 {
		return nil
	}

	// Modern format: <elements>
	elRe := regexp.MustCompile(`(?s)<elements[^>]*>(.*?)</elements>`)
	if m := elRe.FindSubmatch(inner); m != nil {
		steps := parseElementChildren(m[1])
		rules := make([]models.MirthRuleXML, len(steps))
		for i, s := range steps {
			rules[i] = models.MirthRuleXML{
				SequenceNumber: s.SequenceNumber,
				Name:           s.Name,
				Type:           s.Type,
				Inner:          s.Inner,
			}
		}
		return rules
	}

	// Legacy format: <rules><rule>
	rRe := regexp.MustCompile(`(?s)<rules[^>]*>(.*?)</rules>`)
	if m := rRe.FindSubmatch(inner); m != nil {
		var wrapper struct {
			Rules []models.MirthRuleXML `xml:"rule"`
		}
		if err := xml.Unmarshal(append([]byte("<rules>"), append(m[1], []byte("</rules>")...)...), &wrapper); err == nil {
			return wrapper.Rules
		}
	}
	return nil
}

// ============================================================================
// Step / Filter Analysis
// ============================================================================

func analyzeSteps(steps []models.MirthStepXML, rules []models.MirthRuleXML) []models.TransformerStepAnalysis {
	result := make([]models.TransformerStepAnalysis, 0, len(rules)+len(steps))

	// ---- Filter rules --------------------------------------------------------
	for _, r := range rules {
		script := xmlDecodeEntities(extractScript(r.Inner))
		a := models.TransformerStepAnalysis{
			Sequence:       r.SequenceNumber,
			Name:           r.Name,
			MirthType:      r.Type,
			IsFilterRule:   true,
		}

		switch r.Type {
		case "RuleBuilderRule":
			// Declarative rule: extract Variable/Condition/Values and auto-describe
			desc := extractRuleBuilderConditions(r.Inner)
			a.SuggestedType  = "control.if_then_else"
			a.Script         = fmt.Sprintf("// Rule Builder filter: %s\n// Wire as a Conditional step in the pipeline builder", desc)
			a.Conversion     = models.ConversionAuto
			a.AutoConverted  = true
			a.Note           = fmt.Sprintf("Rule Builder: %s", desc)

		case "IteratorRule":
			// Iterator filter — loops over repeating segments; no direct ezHK equivalent
			a.SuggestedType  = "enrichment.script"
			a.Script         = rewriteMirthScript(script)
			a.OriginalScript = script
			a.Conversion     = models.ConversionManual
			a.Note           = "Iterator filter rule — loops over repeating segments; convert to a script step. Use 'Rewrite with AI'."

		case "JavaScriptRule":
			// JavaScript filter rule
			rewritten := rewriteMirthScript(script)
			a.Script         = rewritten
			a.OriginalScript = script
			if isTrivialMSHCheck(script) {
				a.SuggestedType = "control.if_then_else"
				a.Conversion    = models.ConversionManual
				a.Note          = "Simple message-type filter — wire as a Conditional step checking message_type field"
			} else {
				a.SuggestedType = "enrichment.script"
				a.Conversion    = models.ConversionManual
				a.Note          = "JavaScript filter — partially rewritten. Use 'Rewrite with AI' for full conversion."
			}

		default:
			// Legacy or unknown rule type
			rewritten := rewriteMirthScript(script)
			a.Script         = rewritten
			a.OriginalScript = script
			if isTrivialMSHCheck(script) {
				a.SuggestedType = "control.if_then_else"
				a.Conversion    = models.ConversionManual
				a.Note          = "Simple message-type filter — wire as a Conditional step checking message_type field"
			} else {
				a.SuggestedType = "enrichment.script"
				a.Conversion    = models.ConversionManual
				a.Note          = "Complex filter — converted to script step. Use 'Rewrite with AI' to adapt."
			}
		}

		result = append(result, a)
	}

	// ---- Transformer steps ---------------------------------------------------
	for _, st := range steps {
		script := extractScript(st.Inner)
		a := models.TransformerStepAnalysis{
			Sequence:  st.SequenceNumber,
			Name:      st.Name,
			MirthType: st.Type,
		}

		switch st.Type {

		case "MessageBuilderStep":
			// ── AUTO-CONVERT: Mirth 3.x MessageBuilderStep ────────────────────
			// Sets an outbound HL7 field to a literal value or expression.
			// <messageSegment>  = target HL7 path (bracket notation)
			// <mapping>         = value or expression to assign
			msgSegRe := regexp.MustCompile(`(?s)<messageSegment>(.*?)</messageSegment>`)
			mappingRe := regexp.MustCompile(`(?s)<mapping>(.*?)</mapping>`)

			targetRaw := ""
			if sm := msgSegRe.FindSubmatch(st.Inner); sm != nil {
				targetRaw = xmlDecodeEntities(string(sm[1]))
			}
			valueRaw := ""
			if vm := mappingRe.FindSubmatch(st.Inner); vm != nil {
				valueRaw = xmlDecodeEntities(string(vm[1]))
			}

			target := convertMirthPath(targetRaw)
			// Strip surrounding quotes from literal string values ("Lab7" → Lab7)
			value := strings.Trim(strings.TrimSpace(valueRaw), "\"'")
			if value == "" {
				value = valueRaw
			}

			// Generate a simple script: SetFieldValue or direct assignment
			generatedScript := fmt.Sprintf("// Auto-converted from Mirth MessageBuilderStep\n// Sets %s = %s\ndata['%s'] = %s;",
				target, valueRaw, target, func() string {
					trimmed := strings.TrimSpace(valueRaw)
					// If it's a quoted string literal, keep as JS string
					if strings.HasPrefix(trimmed, "\"") || strings.HasPrefix(trimmed, "'") {
						return fmt.Sprintf("'%s'", value)
					}
					// Otherwise it's an expression — rewrite Mirth API calls
					return rewriteMirthScript(trimmed)
				}())

			a.SuggestedType = "enrichment.script"
			a.Script        = generatedScript
			a.Conversion    = models.ConversionAuto
			a.AutoConverted = true
			a.Note          = fmt.Sprintf("MessageBuilder: sets %s → auto-generated script", target)

		case "MapperStep":
			// ── AUTO-CONVERT: Mirth 3.x MapperStep (one field per step) ──────
			// Modern format: <variable>, <mapping>, <defaultValue>, <scope> directly in the step.
			fm := extractSingleMapperStep(st.Inner)
			if fm != nil {
				a.SuggestedType = "field_mapping"
				a.FieldMappings = []models.MirthFieldMapping{*fm}
				a.Conversion    = models.ConversionAuto
				a.AutoConverted = true
				a.Note          = fmt.Sprintf("Mapper: %s ← %s", fm.Target, fm.Source)
				a.Script        = buildFieldMappingScript([]models.MirthFieldMapping{*fm})
			} else {
				a.SuggestedType  = "enrichment.script"
				a.Script         = rewriteMirthScript(script)
				a.OriginalScript = script
				a.Conversion     = models.ConversionManual
				a.Note           = "MapperStep with no parseable variable/mapping — review manually"
			}

		case "Mapper":
			// ── AUTO-CONVERT: Legacy Mirth Mapper (multiple mappings in one step) ───
			mappings := extractMapperMappings(st.Inner)
			if len(mappings) > 0 {
				a.SuggestedType  = "field_mapping"
				a.FieldMappings  = mappings
				a.Conversion     = models.ConversionAuto
				a.AutoConverted  = true
				a.Note           = fmt.Sprintf("%d field mapping(s) auto-converted from Mirth Mapper", len(mappings))
				a.Script         = buildFieldMappingScript(mappings)
			} else {
				a.SuggestedType  = "enrichment.script"
				a.Script         = rewriteMirthScript(script)
				a.OriginalScript = script
				a.Conversion     = models.ConversionManual
				a.Note           = "Mapper step with no parseable mappings — review manually"
			}

		case "IteratorStep":
			// ── NEEDS REVIEW: Mirth Iterator iterates over repeating segments ─
			// ezHK doesn't have a native iterator step; the inner logic needs to
			// be rewritten as a script that loops over the segment array.
			a.SuggestedType  = "enrichment.script"
			a.Script         = rewriteMirthScript(script)
			a.OriginalScript = script
			a.Conversion     = models.ConversionManual
			a.Note           = "Iterator step — loops over repeating segments (OBX, NK1, AL1, etc.). Rewrite as a script that iterates data.enhancedSegments. Use 'Rewrite with AI'."

		case "JavaScript", "JavaScriptStep":
			// ── SYNTAX REWRITE: Custom JS transformer ────────────────────────
			rewritten := rewriteMirthScript(script)
			a.SuggestedType  = "enrichment.script"
			a.Script         = rewritten
			a.OriginalScript = script
			a.Conversion     = models.ConversionManual
			a.AutoConverted  = false
			a.Note           = "Mirth API calls partially rewritten. Use 'Rewrite with AI' for a full semantic conversion."

		case "XSLT":
			a.SuggestedType  = "enrichment.script"
			a.Script         = script
			a.Conversion     = models.ConversionManual
			a.Note           = "XSLT — no native equivalent. Use 'Rewrite with AI' to convert logic to JS."

		case "HL7 to XML Serializer", "XML to HL7 Serializer":
			a.SuggestedType  = "enrichment.script"
			a.Script         = ""
			a.Conversion     = models.ConversionManual
			a.Note           = "Serializer step — ezHealthKonnect parses HL7 automatically. This step is likely redundant; review before importing."

		default:
			a.SuggestedType  = "enrichment.script"
			a.Script         = rewriteMirthScript(script)
			a.OriginalScript = script
			a.Conversion     = models.ConversionManual
			a.Note           = fmt.Sprintf("Unknown step type '%s' — placed as script step for review", st.Type)
		}

		result = append(result, a)
	}

	return result
}

// buildFieldMappingScript generates a human-readable JS summary of auto-converted
// Mapper mappings, so the script block in the UI is meaningful even for Mapper steps.
func buildFieldMappingScript(mappings []models.MirthFieldMapping) string {
	lines := []string{"// Auto-converted Mirth Mapper — field_mapping step config below"}
	for _, m := range mappings {
		line := fmt.Sprintf("// %s  ←  %s", m.Target, m.Source)
		if m.DefaultValue != "" {
			line += fmt.Sprintf("  (default: %q)", m.DefaultValue)
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

// isTrivialMSHCheck detects simple message type filter patterns like:
// msg['MSH']['MSH.9']['MSH.9.1'].toString() == 'ADT'
func isTrivialMSHCheck(script string) bool {
	s := strings.TrimSpace(script)
	return strings.Contains(s, "MSH.9") && (strings.Contains(s, "==") || strings.Contains(s, "!="))
}

// ============================================================================
// Mapper Step Parsing
// ============================================================================

// extractMapperMappings parses a Mirth Mapper step's inner XML and returns
// the list of field-to-path mappings it contains.
//
// Mirth Mapper inner XML structure:
//
//	<entry>
//	  <string>Mappings</string>
//	  <list>
//	    <...Mapping>
//	      <variable>patientId</variable>
//	      <mapping>msg['PID']['PID.3']['PID.3.1'].toString()</mapping>
//	      <defaultValue></defaultValue>
//	      <scope>LOCAL</scope>
//	    </...Mapping>
//	  </list>
//	</entry>
func extractMapperMappings(inner []byte) []models.MirthFieldMapping {
	// Pull the full <list>...</list> block that sits after "Mappings"
	listRe := regexp.MustCompile(`(?s)<string>Mappings</string>\s*<list>(.*?)</list>`)
	listMatch := listRe.FindSubmatch(inner)
	if len(listMatch) < 2 {
		return nil
	}
	listContent := listMatch[1]

	// Each mapping entry is a single XML element (class name varies by Mirth version)
	// We grab all <variable> + <mapping> + <defaultValue> + <scope> quads.
	varRe   := regexp.MustCompile(`(?s)<variable>(.*?)</variable>`)
	pathRe  := regexp.MustCompile(`(?s)<mapping>(.*?)</mapping>`)
	defRe   := regexp.MustCompile(`(?s)<defaultValue>(.*?)</defaultValue>`)
	scopeRe := regexp.MustCompile(`(?s)<scope>(.*?)</scope>`)

	vars    := varRe.FindAllSubmatch(listContent, -1)
	paths   := pathRe.FindAllSubmatch(listContent, -1)
	defs    := defRe.FindAllSubmatch(listContent, -1)
	scopes  := scopeRe.FindAllSubmatch(listContent, -1)

	count := len(vars)
	if len(paths) < count {
		count = len(paths)
	}

	mappings := make([]models.MirthFieldMapping, 0, count)
	for i := 0; i < count; i++ {
		mirthPath := strings.TrimSpace(string(paths[i][1]))
		ezHKPath  := convertMirthPath(mirthPath)

		fm := models.MirthFieldMapping{
			Source:    ezHKPath,
			Target:    strings.TrimSpace(string(vars[i][1])),
			MirthPath: mirthPath,
		}
		if i < len(defs) {
			fm.DefaultValue = strings.TrimSpace(string(defs[i][1]))
		}
		if i < len(scopes) {
			fm.Scope = strings.TrimSpace(string(scopes[i][1]))
		}
		mappings = append(mappings, fm)
	}
	return mappings
}

// extractSingleMapperStep parses a Mirth 3.x MapperStep inner XML that contains
// exactly one <variable>/<mapping> pair (the modern per-step format).
func extractSingleMapperStep(inner []byte) *models.MirthFieldMapping {
	varRe   := regexp.MustCompile(`(?s)<variable>(.*?)</variable>`)
	mapRe   := regexp.MustCompile(`(?s)<mapping>(.*?)</mapping>`)
	defRe   := regexp.MustCompile(`(?s)<defaultValue>(.*?)</defaultValue>`)
	scopeRe := regexp.MustCompile(`(?s)<scope>(.*?)</scope>`)

	varM := varRe.FindSubmatch(inner)
	mapM := mapRe.FindSubmatch(inner)
	if varM == nil || mapM == nil {
		return nil
	}

	variable := strings.TrimSpace(string(varM[1]))
	mapping  := xmlDecodeEntities(strings.TrimSpace(string(mapM[1])))
	if variable == "" || mapping == "" {
		return nil
	}

	fm := &models.MirthFieldMapping{
		Source:    convertMirthPath(mapping),
		Target:    variable,
		MirthPath: mapping,
	}
	if m := defRe.FindSubmatch(inner); m != nil {
		fm.DefaultValue = strings.TrimSpace(string(m[1]))
	}
	fm.Scope = "LOCAL"
	if m := scopeRe.FindSubmatch(inner); m != nil {
		if s := strings.TrimSpace(string(m[1])); s != "" {
			fm.Scope = s
		}
	}
	return fm
}

// extractRuleBuilderConditions summarises a Mirth RuleBuilderRule's declarative
// conditions as a human-readable string for the migration preview.
func extractRuleBuilderConditions(inner []byte) string {
	// The data map contains alternating <string> key/value pairs.
	// Keys we care about: "Variable", "Condition", "Values"
	varRe  := regexp.MustCompile(`(?s)<string>Variable</string>\s*<string>(.*?)</string>`)
	condRe := regexp.MustCompile(`(?s)<string>Condition</string>\s*<string>(.*?)</string>`)
	valRe  := regexp.MustCompile(`(?s)<string>Values</string>.*?<list>(.*?)</list>`)
	strRe  := regexp.MustCompile(`(?s)<string>(.*?)</string>`)

	variable := ""
	if m := varRe.FindSubmatch(inner); m != nil {
		variable = xmlDecodeEntities(strings.TrimSpace(string(m[1])))
	}
	condition := ""
	if m := condRe.FindSubmatch(inner); m != nil {
		condition = strings.TrimSpace(string(m[1]))
	}
	var values []string
	if m := valRe.FindSubmatch(inner); m != nil {
		for _, sm := range strRe.FindAllSubmatch(m[1], -1) {
			if v := strings.TrimSpace(string(sm[1])); v != "" {
				values = append(values, v)
			}
		}
	}

	if variable == "" {
		return "declarative rule (no variable found)"
	}
	if len(values) > 0 {
		return fmt.Sprintf("%s %s [%s]", variable, condition, strings.Join(values, ", "))
	}
	return fmt.Sprintf("%s %s", variable, condition)
}

// convertMirthPath converts a Mirth bracket-notation path to an ezHK dot-notation path.
//
// Examples:
//
//	msg['PID']['PID.3']['PID.3.1'].toString()  →  PID.3.1
//	msg['MSH']['MSH.9']                         →  MSH.9
//	msg['PID']['PID.5'][0]['PID.5.1']           →  PID.5.1
//	tmp['someVar']                              →  someVar   (tmp → context var)
func convertMirthPath(mirthExpr string) string {
	// Strip method calls like .toString(), .trim(), etc.
	methodRe := regexp.MustCompile(`\.\w+\(\)$`)
	clean := methodRe.ReplaceAllString(strings.TrimSpace(mirthExpr), "")

	// Extract all bracket keys: ['KEY'] or ["KEY"]
	keyRe := regexp.MustCompile(`\[['"]([^'"]+)['"]\]`)
	matches := keyRe.FindAllStringSubmatch(clean, -1)
	if len(matches) == 0 {
		return mirthExpr // can't parse, return as-is
	}

	// The deepest key that contains a dot is the ezHK field path (e.g. PID.3.1)
	// If no dotted key exists, use the last key.
	for i := len(matches) - 1; i >= 0; i-- {
		key := matches[i][1]
		if strings.Contains(key, ".") {
			return key
		}
	}
	// Last key (e.g. tmp['varName'])
	return matches[len(matches)-1][1]
}

// rewriteMirthScript does a best-effort syntactic replacement of Mirth-specific
// API calls with their ezHK equivalents, making the script a closer starting
// point for the LLM (or for users who don't use the AI rewrite).
//
// This is NOT a full semantic translation — it handles only the most common
// patterns. The LLM rewrite covers everything else.
func rewriteMirthScript(script string) string {
	s := script

	// msg['SEG']['SEG.N']['SEG.N.M'] → GetFieldValue(data, 'SEG.N.M')
	msgDeepRe := regexp.MustCompile(`msg\['([A-Z0-9]+)'\]\['([A-Z0-9]+\.[0-9]+)'\]\['([A-Z0-9]+\.[0-9]+\.[0-9]+)'\]`)
	s = msgDeepRe.ReplaceAllStringFunc(s, func(m string) string {
		parts := msgDeepRe.FindStringSubmatch(m)
		if len(parts) < 4 {
			return m
		}
		return fmt.Sprintf("GetFieldValue(data, '%s')", parts[3])
	})

	// msg['SEG']['SEG.N'] → GetFieldValue(data, 'SEG.N')
	msgShallowRe := regexp.MustCompile(`msg\['([A-Z0-9]+)'\]\['([A-Z0-9]+\.[0-9]+)'\]`)
	s = msgShallowRe.ReplaceAllStringFunc(s, func(m string) string {
		parts := msgShallowRe.FindStringSubmatch(m)
		if len(parts) < 3 {
			return m
		}
		return fmt.Sprintf("GetFieldValue(data, '%s')", parts[2])
	})

	// tmp['varName'] = value  →  data['varName'] = value
	s = regexp.MustCompile(`\btmp\['`).ReplaceAllString(s, "data['")

	// channelMap.put('key', value)  →  data['key'] = value  (approximate)
	channelMapRe := regexp.MustCompile(`channelMap\.put\('([^']+)',\s*([^)]+)\)`)
	s = channelMapRe.ReplaceAllString(s, "data['$1'] = $2")

	// globalMap.put / globalChannelMap.put — flag as unsupported
	s = regexp.MustCompile(`\b(globalMap|globalChannelMap)\.put\(`).ReplaceAllString(s,
		"/* NOTE: globalMap not available in ezHK — use data[] or context variables */ $1.put(")

	// $re('table', key) — lookup tables not supported
	s = regexp.MustCompile(`\$re\(`).ReplaceAllString(s,
		"/* NOTE: $re() lookup tables not available — implement via database enrichment step */ $re(")

	// $('varName') / $c('varName') — channel/connector map shorthand
	s = regexp.MustCompile(`\$c?\('([^']+)'\)`).ReplaceAllString(s, "data['$1']")

	// E4X: for each (seg in msg..SEGMENT) — not valid in standard JS
	s = regexp.MustCompile(`\bfor\s+each\s*\(`).ReplaceAllString(s,
		"/* NOTE: E4X 'for each' is not standard JS — rewrite as: for (const seg of (data.enhancedSegments?.SEGMENT?.fields ?? [])) */ for each (")

	// E4X: msg..SEGMENT — descendant access shorthand
	s = regexp.MustCompile(`\bmsg\.\.([A-Z][A-Z0-9]+)\b`).ReplaceAllString(s,
		"/* E4X descendant — use data.enhancedSegments.$1 */ msg..$1")

	// createSegmentAfter / addSegment / deleteSegment helpers
	s = regexp.MustCompile(`\bcreateSegmentAfter\(`).ReplaceAllString(s,
		"/* NOTE: createSegmentAfter() not available — use SetFieldValue to add segment fields */ createSegmentAfter(")
	s = regexp.MustCompile(`\bdeleteSegment\(`).ReplaceAllString(s,
		"/* NOTE: deleteSegment() not available — manipulate data.enhancedSegments directly */ deleteSegment(")

	// connectorMessage.getRawData() → data._raw
	s = strings.ReplaceAll(s, "connectorMessage.getRawData()", "data._raw")

	// logger.* → console.*
	s = regexp.MustCompile(`\blogger\.(info|warn|error|debug)\(`).ReplaceAllString(s, "console.$1(")

	return s
}

// ============================================================================
// Database Write
// ============================================================================

func (s *MirthMigrationService) writeToDatabase(
	ctx context.Context,
	ch *models.MirthChannelXML,
	preview *models.MigrationPreview,
	name, description, userID string,
	skipSet map[int]bool,
) (*models.MigrationImportResult, error) {
	warnings := []string{}

	// Derive source_type and target_type from connector mappings
	sourceType := preview.Source.EzHKConnectorType
	if sourceType == "" {
		sourceType = "unknown"
	}
	targetType := "unknown"
	if len(preview.Destinations) > 0 {
		targetType = preview.Destinations[0].EzHKConnectorType
		if targetType == "" {
			targetType = "unknown"
		}
	}

	// Determine message_type from filter rules if detectable
	messageType := detectMessageType(ch)

	now := time.Now()
	interfaceID := uuid.New().String()

	// ---- 1. Insert interface ----
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO interfaces (
			id, name, description, source_type, target_type, message_type,
			status, is_active, version, created_at, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,'draft',false,1,$7,$8)`,
		interfaceID, name, description, sourceType, targetType, messageType, now, now,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create interface: %w", err)
	}

	// ---- 2. Insert pipeline ----
	pipelineID := uuid.New().String()
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO transformation_pipelines (
			id, interface_id, message_type, pipeline_name, enabled, version, created_at, updated_at
		) VALUES ($1,$2,$3,$4,false,1,$5,$6)`,
		pipelineID, interfaceID, messageType,
		fmt.Sprintf("%s Pipeline (Migrated)", name),
		now, now,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create pipeline: %w", err)
	}

	// ---- 3. Insert steps ----
	// Use original Mirth sequence numbers (×10 for spacing) so relative order is preserved
	// even when steps are skipped.
	created := 0
	skipped := 0
	for i, step := range preview.Steps {
		if skipSet[i] {
			skipped++
			continue
		}

		// Preserve original Mirth sequence; multiply by 10 to leave room for manual insertions.
		// Minimum sequence is 10 to avoid collision with connector.inbound (seq 5).
		seq := (step.Sequence + 1) * 10

		stepID := uuid.New().String()
		config := map[string]interface{}{
			"script": addMirthHeader(step.Script, step.Name, step.MirthType),
		}
		configJSON := marshalJSON(config)

		_, err = s.db.ExecContext(ctx, `
			INSERT INTO transformation_steps (
				id, pipeline_id, step_name, step_type, sequence, required,
				timeout_ms, enabled, config, script_type, script_content,
				on_error_strategy, execution_mode, parent_step_id, container_zone,
				created_at, updated_at
			) VALUES ($1,$2,$3,$4,$5,false,30000,true,$6,'javascript',$7,'suppress','sequential',NULL,'pre',$8,$9)`,
			stepID, pipelineID,
			fmt.Sprintf("[Mirth] %s", step.Name),
			step.SuggestedType,
			seq,
			configJSON,
			step.Script,
			now, now,
		)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("step %d (%s): %v", i, step.Name, err))
			continue
		}
		created++
	}

	// ---- 4. Log migration in history table ----
	if _, auditErr := s.db.ExecContext(ctx, `
		INSERT INTO mirth_migration_history (
			id, interface_id, channel_id, channel_name, mirth_version,
			steps_created, steps_skipped, migrated_by, created_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
		uuid.New().String(), interfaceID, ch.ID, ch.Name, ch.Version,
		created, skipped, userID, now,
	); auditErr != nil {
		// Non-fatal: interface was created successfully. Warn but don't rollback.
		log.Printf("⚠️  mirth_migration: audit log insert failed (interface %s): %v", interfaceID, auditErr)
		warnings = append(warnings, "audit log entry could not be written: "+auditErr.Error())
	}

	return &models.MigrationImportResult{
		InterfaceID:   interfaceID,
		PipelineID:    pipelineID,
		InterfaceName: name,
		StepsCreated:  created,
		StepsSkipped:  skipped,
		Warnings:      warnings,
	}, nil
}

// ============================================================================
// Helpers
// ============================================================================

// xmlVal extracts the first text value of a given XML tag from raw inner XML bytes.
func xmlVal(inner []byte, tag string) string {
	re := regexp.MustCompile(`(?s)<` + regexp.QuoteMeta(tag) + `[^>]*>(.*?)</` + regexp.QuoteMeta(tag) + `>`)
	m := re.FindSubmatch(inner)
	if len(m) > 1 {
		return strings.TrimSpace(string(m[1]))
	}
	return ""
}

func xmlValDefault(inner []byte, tag, def string) string {
	v := xmlVal(inner, tag)
	if v == "" {
		return def
	}
	return v
}

func xmlValInt(inner []byte, tag string, def int) int {
	v := xmlVal(inner, tag)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}

// clampPort ensures a port number is within valid TCP range 1–65535.
func clampPort(p, def int) int {
	if p < 1 || p > 65535 {
		return def
	}
	return p
}

// extractScript pulls the JavaScript content out of a Mirth <data class="map"> block.
// Mirth stores step scripts as key/value pairs of <string> tags:
//
//	<entry><string>Script</string><string>var x = ...</string></entry>
func extractScript(inner []byte) string {
	re := regexp.MustCompile(`(?s)<string>Script</string>\s*<string>(.*?)</string>`)
	m := re.FindSubmatch(inner)
	if len(m) > 1 {
		return strings.TrimSpace(string(m[1]))
	}
	// Fallback: look for any <script> tag
	re2 := regexp.MustCompile(`(?s)<script[^>]*>(.*?)</script>`)
	m2 := re2.FindSubmatch(inner)
	if len(m2) > 1 {
		return strings.TrimSpace(string(m2[1]))
	}
	return ""
}

// detectMessageType tries to infer the HL7 message type from source filter rules.
// Returns "*" (all) if it cannot determine one.
func detectMessageType(ch *models.MirthChannelXML) string {
	for _, rule := range parseRulesFromInner(ch.SourceConnector.Filter.Inner) {
		script := xmlDecodeEntities(extractScript(rule.Inner))
		if strings.Contains(script, "MSH.9") {
			re := regexp.MustCompile(`==\s*['"]([A-Z0-9\^]+)['"]`)
			m := re.FindStringSubmatch(script)
			if len(m) > 1 {
				return m[1]
			}
		}
	}
	return "*"
}

// addMirthHeader prepends a comment block to ported Mirth JS to help users understand
// what it was originally.
func addMirthHeader(script, stepName, stepType string) string {
	if script == "" {
		return ""
	}
	header := fmt.Sprintf(
		"/* ============================================================\n"+
			" * Migrated from Mirth Connect\n"+
			" * Step: %s (%s)\n"+
			" *\n"+
			" * NOTE: Mirth proprietary APIs (msg[], tmp[], $re(), etc.)\n"+
			" * are NOT available here. Adapt the logic below to use\n"+
			" * ezHealthKonnect field paths (e.g. PID.3, MSH.9.1).\n"+
			" * ============================================================ */\n",
		stepName, stepType,
	)
	return header + script
}

// marshalJSON marshals a value to JSON bytes (returns empty object on error).
func marshalJSON(v interface{}) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		return []byte("{}")
	}
	return b
}
