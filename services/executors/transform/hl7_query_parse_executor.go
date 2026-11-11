// services/executors/transform/hl7_query_parse_executor.go
// HL7QueryParseExecutor — pipeline step type "hl7.query_parse".
//
// Reads an HL7 v2 query (QBP, QCN) from the received message text and turns
// it into what answering it needs: the header values the reply echoes, the
// query tag and QPD segment, the identifier and "what domains returned" of a
// PIX query, and the search parameters of a demographics query, as one
// URL-encoded query string for an API call. Problems (a missing or unknown
// value) are listed with their HL7 error location and code, so the reply can
// report them in ERR instead of the step failing.
//
// Nothing here is specific to one query: which fields hold what, which
// "@SEG.F.C" query parameters map to which API search parameters, and how
// assigning authorities map to identifier systems are all configuration.
//
// Config keys:
//
//	rawPath           — the received message text (default "raw", then
//	                    "message.raw")
//	tagField          — the query tag (default "QPD.2"; "QID.1" for QCN)
//	namespaces        — {HL7 namespace ID: identifier system} for assigning
//	                    authorities sent by namespace only; a value that is an
//	                    OID or UUID becomes urn:oid: / urn:uuid:
//	identifierField   — a field holding one CX patient identifier ("QPD.3")
//	identifierParam   — the query parameter it is sent as, "system|value"
//	domainsField      — a field of repeating CX "what domains returned"
//	                    ("QPD.4" for PIX, "QPD.8" for PDQ)
//	domainsParam      — the query parameter each domain's system is sent as
//	parametersField   — a field of repeating "@SEG.F.C^value" parameters
//	parameters        — []{field: "PID.5.1.1", param: "family", transform?,
//	                    valueMap?, domainField?: "PID.3.4"}; a parameter with
//	                    domainField is an identifier sent as "system|value",
//	                    its system read from the parameters under domainField
//	countParam, defaultCount, quantityField ("RCP.2", "<n>^RD")
//	continuationField — the continuation pointer ("DSC.1")
//
// Output (step_output): message_type, trigger_event, control_id, version,
// sending_application, sending_facility, receiving_application,
// receiving_facility, query_name, query_tag (each the encoded field, for
// echoing), qpd_segment, continuation, quantity, identifier{value,
// namespace, universal_id, universal_id_type, system}, domains[]{repetition,
// namespace, universal_id, universal_id_type, system}, domain_systems[],
// domain_count, parameters[]{field, param, value}, query, problems[]{location,
// code, text}, problem_count, has_problems.
package transform

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"strconv"
	"strings"
	"time"

	"ezhealthkonnect/models"
	"ezhealthkonnect/services/executors"
)

// HL7 table 0357 error codes the problems use.
const (
	hl7ErrRequiredMissing = "101^Required field missing^HL70357"
	hl7ErrDataType        = "102^Data type error^HL70357"
	hl7ErrUnknownKey      = "204^Unknown key identifier^HL70357"
	hl7ErrInternal        = "207^Application internal error^HL70357"
)

// HL7QueryParseExecutor reads an HL7 v2 query; see the file comment.
type HL7QueryParseExecutor struct {
	*executors.BaseExecutor
}

// NewHL7QueryParseExecutor constructs the executor.
func NewHL7QueryParseExecutor() *HL7QueryParseExecutor {
	return &HL7QueryParseExecutor{
		BaseExecutor: executors.NewBaseExecutor("hl7.query_parse", models.ExecutorMetadata{
			Name:        "HL7 v2 Query Reader",
			Description: "Reads an HL7 v2 query (QBP/QCN): its tag, identifiers, domains and search parameters, for an API lookup and the reply",
			Version:     "1.0.0",
			Author:      "ezHealthKonnect",
			Category:    "HL7 Transform",
		}),
	}
}

type queryParameter struct {
	Field       string            `json:"field"`
	Param       string            `json:"param"`
	Transform   string            `json:"transform,omitempty"`
	ValueMap    map[string]string `json:"valueMap,omitempty"`
	DomainField string            `json:"domainField,omitempty"`
}

type queryParseConfig struct {
	RawPath           string            `json:"rawPath"`
	TagField          string            `json:"tagField"`
	Namespaces        map[string]string `json:"namespaces"`
	IdentifierField   string            `json:"identifierField"`
	IdentifierParam   string            `json:"identifierParam"`
	DomainsField      string            `json:"domainsField"`
	DomainsParam      string            `json:"domainsParam"`
	ParametersField   string            `json:"parametersField"`
	Parameters        []queryParameter  `json:"parameters"`
	CountParam        string            `json:"countParam"`
	DefaultCount      int               `json:"defaultCount"`
	QuantityField     string            `json:"quantityField"`
	ContinuationField string            `json:"continuationField"`
}

// givenParameter is one "@SEG.F.C^value" repetition as received: its
// repetition number, field name and encoded value.
type givenParameter struct {
	rep   int
	field string
	raw   string
}

// queryProblem is one thing wrong with the query, as the reply's ERR
// reports it.
type queryProblem struct {
	location, code, text string
}

// Execute reads the query; see the file comment.
func (e *HL7QueryParseExecutor) Execute(ctx context.Context, step *models.TransformationStep, inputData map[string]interface{}) (map[string]interface{}, error) {
	start := time.Now()
	if err := e.PreExecute(ctx, step); err != nil {
		return nil, err
	}
	cfg := queryParseConfig{TagField: "QPD.2"}
	if step.Config != nil {
		raw, _ := json.Marshal(step.Config)
		if err := json.Unmarshal(raw, &cfg); err != nil {
			return nil, fmt.Errorf("hl7.query_parse: invalid config: %w", err)
		}
	}

	rawText := ""
	for _, p := range []string{cfg.RawPath, "raw", "message.raw"} {
		if p == "" {
			continue
		}
		if s, ok := executors.GetNestedValue(inputData, p).(string); ok && s != "" {
			rawText = s
			break
		}
	}
	msg, ok := readHL7Text(rawText)
	if !ok {
		return nil, fmt.Errorf("hl7.query_parse: no HL7 v2 message found (rawPath %q)", cfg.RawPath)
	}

	q := &queryReader{cfg: cfg, msg: msg, values: url.Values{}}
	out := q.read()

	outputData := make(map[string]interface{}, len(inputData))
	for k, v := range inputData {
		outputData[k] = v
	}
	log.Printf("  ✅ [hl7.query_parse] %s^%s tag %q: %d parameter(s), %d problem(s) in %dms",
		out["message_type"], out["trigger_event"], out["query_tag"], len(q.params), len(q.problems), time.Since(start).Milliseconds())
	e.SetStepOutputWithDetails(outputData, out, map[string]interface{}{
		"duration_ms": time.Since(start).Milliseconds(),
		"success":     true,
	})
	return outputData, nil
}

// queryReader holds one read's state.
type queryReader struct {
	cfg      queryParseConfig
	msg      hl7Text
	values   url.Values
	params   []interface{}
	problems []queryProblem
}

func (q *queryReader) problem(location, code, text string) {
	q.problems = append(q.problems, queryProblem{location, code, text})
}

func (q *queryReader) read() map[string]interface{} {
	msh := q.msg.segment("MSH", 1)
	msgType := field(msh, 9)
	out := map[string]interface{}{
		"message_type":          unescapeHL7(component(msgType, 1)),
		"trigger_event":         unescapeHL7(component(msgType, 2)),
		"control_id":            field(msh, 10),
		"version":               field(msh, 12),
		"sending_application":   field(msh, 3),
		"sending_facility":      field(msh, 4),
		"receiving_application": field(msh, 5),
		"receiving_facility":    field(msh, 6),
		"query_name":            field(q.msg.segment("QPD", 1), 1),
		"qpd_segment":           q.msg.segment("QPD", 1),
	}
	if tag, ok := q.msg.fieldAt(q.cfg.TagField); ok {
		out["query_tag"] = tag
	}
	if q.cfg.ContinuationField != "" {
		c, _ := q.msg.fieldAt(q.cfg.ContinuationField)
		out["continuation"] = unescapeHL7(component(c, 1))
	}

	if q.cfg.IdentifierField != "" {
		out["identifier"] = q.readIdentifier()
	}
	if q.cfg.DomainsField != "" {
		domains, systems := q.readDomains()
		out["domains"] = domains
		out["domain_systems"] = systems
		out["domain_count"] = len(systems)
	}
	if q.cfg.ParametersField != "" {
		q.readParameters()
		out["parameters"] = q.params
	}
	quantity := q.readQuantity()
	out["quantity"] = quantity
	if q.cfg.CountParam != "" && quantity > 0 {
		q.values.Set(q.cfg.CountParam, strconv.Itoa(quantity))
	}
	out["query"] = q.values.Encode()

	problems := make([]interface{}, len(q.problems))
	for i, p := range q.problems {
		problems[i] = map[string]interface{}{"location": p.location, "code": p.code, "text": p.text}
	}
	out["problems"] = problems
	out["problem_count"] = len(problems)
	out["has_problems"] = len(problems) > 0
	return out
}

// location is an ERR-2 error location (ERL) in the field named by path,
// e.g. "QPD^1^3^1^4".
func location(path string, more ...int) string {
	seg, num, _ := splitFieldPath(path)
	s := seg + "^1^" + strconv.Itoa(num)
	for _, n := range more {
		s += "^" + strconv.Itoa(n)
	}
	return s
}

// readIdentifier reads the CX at identifierField: ID number and assigning
// authority.
func (q *queryReader) readIdentifier() map[string]interface{} {
	f, _ := q.msg.fieldAt(q.cfg.IdentifierField)
	cx := ""
	if reps := repetitions(f); len(reps) > 0 {
		cx = reps[0]
	}
	value := unescapeHL7(component(cx, 1))
	id := q.authority(component(cx, 4))
	id["value"] = value
	switch {
	case value == "":
		q.problem(location(q.cfg.IdentifierField, 1, 1), hl7ErrRequiredMissing, "The patient identifier has no ID number.")
	case id["system"] == "":
		q.problem(location(q.cfg.IdentifierField, 1, 4), hl7ErrUnknownKey, "The identifier's assigning authority isn't known.")
	case q.cfg.IdentifierParam != "":
		q.values.Add(q.cfg.IdentifierParam, id["system"].(string)+"|"+value)
	}
	return id
}

// readDomains reads the repeating "what domains returned" field.
func (q *queryReader) readDomains() ([]interface{}, []interface{}) {
	f, _ := q.msg.fieldAt(q.cfg.DomainsField)
	domains, systems := []interface{}{}, []interface{}{}
	for i, cx := range repetitions(f) {
		hd := component(cx, 4)
		if strings.Trim(hd, "&") == "" {
			continue
		}
		d := q.authority(hd)
		d["repetition"] = i + 1
		domains = append(domains, d)
		system, _ := d["system"].(string)
		if system == "" {
			q.problem(location(q.cfg.DomainsField, i+1), hl7ErrUnknownKey, "A requested domain isn't known.")
			continue
		}
		systems = append(systems, system)
		if q.cfg.DomainsParam != "" {
			q.values.Add(q.cfg.DomainsParam, system)
		}
	}
	return domains, systems
}

// authority resolves an HD (namespace&universal ID&type, encoded) to its
// identifier system: from the universal ID when given, else from the
// namespaces table. An unresolvable one has system "".
func (q *queryReader) authority(hd string) map[string]interface{} {
	ns := unescapeHL7(subcomponent(hd, 1))
	uid := unescapeHL7(subcomponent(hd, 2))
	uidType := unescapeHL7(subcomponent(hd, 3))
	return q.resolveAuthority(ns, uid, uidType)
}

func (q *queryReader) resolveAuthority(ns, uid, uidType string) map[string]interface{} {
	system := systemFromUniversalID(uid, uidType)
	if system == "" && ns != "" {
		if mapped, ok := q.cfg.Namespaces[ns]; ok {
			system = systemFromUniversalID(mapped, "")
		}
	}
	d := map[string]interface{}{"namespace": ns, "universal_id": uid, "universal_id_type": uidType, "system": system}
	if system != "" {
		if uid == "" {
			d["universal_id"], d["universal_id_type"] = universalID(system)
		}
	}
	return d
}

// readParameters reads "@SEG.F.C^value" repetitions into search parameters.
func (q *queryReader) readParameters() {
	f, _ := q.msg.fieldAt(q.cfg.ParametersField)
	var all []givenParameter
	for i, rep := range repetitions(f) {
		name := strings.ToUpper(strings.TrimPrefix(strings.TrimSpace(unescapeHL7(component(rep, 1))), "@"))
		if name == "" {
			continue
		}
		all = append(all, givenParameter{i + 1, name, component(rep, 2)})
	}
	if len(all) == 0 {
		q.problem(location(q.cfg.ParametersField), hl7ErrRequiredMissing, "The query has no search parameters.")
		return
	}

	byField := map[string]queryParameter{}
	var domainFields []string
	for _, p := range q.cfg.Parameters {
		byField[strings.ToUpper(p.Field)] = p
		if p.DomainField != "" {
			domainFields = append(domainFields, strings.ToUpper(p.DomainField))
		}
	}
	inDomain := func(name, domain string) bool { return name == domain || strings.HasPrefix(name, domain+".") }
	isDomainPart := func(name string) bool {
		for _, d := range domainFields {
			if inDomain(name, d) {
				return true
			}
		}
		return false
	}

	for _, g := range all {
		p, known := byField[g.field]
		switch {
		case !known && isDomainPart(g.field):
			continue // read with its identifier below
		case !known:
			q.problem(location(q.cfg.ParametersField, g.rep), hl7ErrInternal, "Searching by @"+g.field+" isn't supported.")
			continue
		}
		value := unescapeHL7(g.raw)
		if p.DomainField != "" {
			system := q.parameterDomain(all, strings.ToUpper(p.DomainField))
			if system == "" {
				q.problem(location(q.cfg.ParametersField, g.rep), hl7ErrUnknownKey, "@"+g.field+" needs a known assigning authority (@"+p.DomainField+").")
				continue
			}
			value = system + "|" + value
		} else {
			value = strings.TrimRight(strings.TrimSpace(value), "*")
			if p.Transform != "" {
				v, ok, known := applyHL7Transform(p.Transform, value)
				if !known || !ok {
					q.problem(location(q.cfg.ParametersField, g.rep), hl7ErrDataType, "@"+g.field+" has a value that can't be read.")
					continue
				}
				value = v
			}
			value = mapValue(value, p.ValueMap)
		}
		if value == "" {
			continue
		}
		q.values.Add(p.Param, value)
		q.params = append(q.params, map[string]interface{}{"field": g.field, "param": p.Param, "value": value})
	}
}

// parameterDomain resolves the assigning authority given by the parameters
// under domain: "@PID.3.4^NS&1.2.3&ISO" or "@PID.3.4.1^NS", "@PID.3.4.2^…".
func (q *queryReader) parameterDomain(all []givenParameter, domain string) string {
	var ns, uid, uidType string
	for _, g := range all {
		switch g.field {
		case domain:
			ns = unescapeHL7(subcomponent(g.raw, 1))
			uid = unescapeHL7(subcomponent(g.raw, 2))
			uidType = unescapeHL7(subcomponent(g.raw, 3))
		case domain + ".1":
			ns = unescapeHL7(g.raw)
		case domain + ".2":
			uid = unescapeHL7(g.raw)
		case domain + ".3":
			uidType = unescapeHL7(g.raw)
		}
	}
	system, _ := q.resolveAuthority(ns, uid, uidType)["system"].(string)
	return system
}

// readQuantity reads the quantity limit ("<n>^RD"): 0 when absent, else
// defaultCount when unreadable.
func (q *queryReader) readQuantity() int {
	n := q.cfg.DefaultCount
	if q.cfg.QuantityField == "" {
		return n
	}
	f, _ := q.msg.fieldAt(q.cfg.QuantityField)
	amount, unit := component(f, 1), strings.ToUpper(component(f, 2))
	if amount == "" || (unit != "" && unit != "RD") {
		return n
	}
	if v, err := strconv.Atoi(strings.TrimSpace(amount)); err == nil && v > 0 {
		return v
	}
	return n
}

// Validate checks the field paths.
func (e *HL7QueryParseExecutor) Validate(step *models.TransformationStep) error {
	for _, key := range []string{"tagField", "identifierField", "domainsField", "parametersField", "quantityField", "continuationField"} {
		if s, _ := step.Config[key].(string); s != "" {
			if _, _, ok := splitFieldPath(s); !ok {
				return fmt.Errorf("hl7.query_parse: %s %q is not a field such as QPD.3", key, s)
			}
		}
	}
	return nil
}

// GetOutputVariables declares the outputs for the field picker.
func (e *HL7QueryParseExecutor) GetOutputVariables(step *models.TransformationStep) []models.VariableDefinition {
	v := func(name, path, typ, desc string) models.VariableDefinition {
		return models.VariableDefinition{Name: name, Path: path, DataType: typ, Description: desc, Category: "HL7 Transform"}
	}
	return []models.VariableDefinition{
		v("Query Tag", "query_tag", "string", "The query tag, to echo in QAK-1"),
		v("Control ID", "control_id", "string", "The query's MSH-10, to echo in MSA-2"),
		v("QPD Segment", "qpd_segment", "string", "The query's QPD segment, to echo"),
		v("Query", "query", "string", "The URL-encoded API query built from the query"),
		v("Problems", "problems", "array", "Problems found: location, code, text"),
		v("Has Problems", "has_problems", "boolean", "Whether any problem was found"),
		v("Continuation", "continuation", "string", "The continuation pointer (DSC-1)"),
	}
}
