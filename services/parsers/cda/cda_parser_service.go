// services/parsers/cda/cda_parser_service.go
// CDAParserService orchestrates full CDA/CCD document parsing.
//
// Processing flow:
//   1. Parse raw XML → etree.Document
//   2. Detect profile (C-CDA 2.1 / C32 / HITSP)
//   3. Normalise C32/HITSP template OIDs if needed
//   4. Extract document header (patient demographics + metadata)
//   5. Process CDA sections in parallel (MAX_CDA_SECTION_WORKERS workers)
//   6. Assemble ParserResult with USCDI-keyed ParsedJSON + EnhancedFields
//
// Implements the services/parsers MessageParser interface so it can be
// registered in the ParserRegistry alongside the HL7 and FHIR parsers.

package cdaparser

import (
	"bytes"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	cdaSchema "ezhealthkonnect/cda"
	cdadocument "ezhealthkonnect/cda/document"
	"ezhealthkonnect/models"
	"ezhealthkonnect/uscdi"
	"github.com/beevik/etree"
)

const (
	defaultMaxDocSizeMB = 50
	hardCapDocSizeMB    = 200
)

// CDAParserService parses CDA/CCD XML documents and returns a schema-enriched
// ParserResult with USCDI-aligned field names and parallel section processing.
type CDAParserService struct {
	schemaLoader *cdaSchema.CDASchemaLoader
	vocabulary   *uscdi.USCDIVocabulary
	normalizer   *cdaSchema.C32TemplateNormalizer
	registry     *SectionRegistry
	typedParser  *cdadocument.CDAParser // Sprint B typed document model
	maxWorkers   int
	maxDocSizeMB int // 0 = use default (50 MB)
}

// NewCDAParserService constructs a service with all dependencies injected.
// Pass DefaultSectionRegistry() for the standard 9 OOB section processors.
func NewCDAParserService(
	loader *cdaSchema.CDASchemaLoader,
	vocab *uscdi.USCDIVocabulary,
	normalizer *cdaSchema.C32TemplateNormalizer,
	registry *SectionRegistry,
) *CDAParserService {
	return &CDAParserService{
		schemaLoader: loader,
		vocabulary:   vocab,
		normalizer:   normalizer,
		registry:     registry,
		typedParser:  cdadocument.NewCDAParser(loader),
		maxWorkers:   resolveSectionWorkers(),
		maxDocSizeMB: resolveMaxDocSizeMB(),
	}
}

// WithMaxDocSizeMB overrides the document size limit at construction time.
// Values above hardCapDocSizeMB (200 MB) are silently clamped.
func (s *CDAParserService) WithMaxDocSizeMB(mb int) *CDAParserService {
	if mb <= 0 {
		mb = defaultMaxDocSizeMB
	}
	if mb > hardCapDocSizeMB {
		mb = hardCapDocSizeMB
	}
	s.maxDocSizeMB = mb
	return s
}

// Format returns the canonical format string for the ParserRegistry.
func (s *CDAParserService) Format() string {
	return string(models.FormatCCDA) // "ccda"
}

// Parse is the main entry point. It accepts raw CDA/CCD XML and returns a
// fully populated ParserResult. Returns Success=false on fatal parse errors.
//
// Documents larger than maxDocSizeMB use streaming XML parsing via
// etree.Document.ReadFrom(io.Reader) to avoid loading the full string into
// memory twice. The hard cap (200 MB default) rejects oversized documents
// immediately with a descriptive error.
func (s *CDAParserService) Parse(raw string) *models.ParserResult {
	start := time.Now()

	result := &models.ParserResult{
		Format:         models.FormatCCDA,
		EnhancedFields: make(map[string]*models.EnhancedField),
		FieldOrder:     []string{},
	}

	// Size gate — reject documents above the hard cap.
	limit := s.maxDocSizeMB
	if limit <= 0 {
		limit = defaultMaxDocSizeMB
	}
	hardCap := hardCapDocSizeMB
	docBytes := len(raw)
	if docBytes > hardCap*1024*1024 {
		result.Success = false
		result.Error = fmt.Sprintf(
			"cda: document size %d MB exceeds hard cap of %d MB",
			docBytes/(1024*1024), hardCap,
		)
		return result
	}

	// 1. Parse XML — streaming path for large documents (> maxDocSizeMB).
	doc := etree.NewDocument()
	var parseErr error
	if docBytes > limit*1024*1024 {
		// Use streaming reader to avoid doubling memory footprint.
		_, parseErr = doc.ReadFrom(bytes.NewReader([]byte(raw)))
	} else {
		parseErr = doc.ReadFromString(raw)
	}
	if parseErr != nil {
		result.Success = false
		result.Error = fmt.Sprintf("cda: XML parse error: %v", parseErr)
		return result
	}

	root := doc.Root()
	if root == nil {
		result.Success = false
		result.Error = "cda: empty document"
		return result
	}

	// 2 & 3. Detect profile + normalise C32/HITSP
	normResult, err := s.normalizer.Normalize(raw)
	if err != nil {
		result.Success = false
		result.Error = fmt.Sprintf("cda: normalisation error: %v", err)
		return result
	}

	profile := normResult.OriginalProfile
	workingXML := normResult.NormalizedXML

	// If normalisation rewrote OIDs, re-parse to get the updated DOM.
	if normResult.Substitutions > 0 {
		doc = etree.NewDocument()
		if err := doc.ReadFromString(workingXML); err != nil {
			result.Success = false
			result.Error = fmt.Sprintf("cda: re-parse after normalisation failed: %v", err)
			return result
		}
		root = doc.Root()
	}

	// 4. Build the generic, schema-agnostic XML->JSON mirror FIRST, directly
	// off the raw etree root, before any CDA-specific interpretation happens.
	// This has zero knowledge of CDA/RIM semantics — it cannot have a
	// "missing field" bug because it enumerates nothing — making it the
	// authoritative, guaranteed-complete representation of the source
	// document. Everything below (the typed CDADocument tree, "sections",
	// "header") is a curated VIEW built for semantic/FHIR-mapping
	// convenience; it is best-effort, not the fidelity guarantee. See
	// cda/document/generic_xml.go for the full rationale.
	genericXML := cdadocument.GenericXMLToJSON(root)

	// 5. Build typed document model (Sprint B/D).
	// Moved before section processing so the typed header can produce the JSON header map.
	typedDoc := s.typedParser.ParseDocument(root, workingXML)
	result.TypedDocument = typedDoc

	// 6. Process sections in parallel
	sectionResults := s.processSectionsParallel(root)

	// 7. Assemble ParsedJSON — header map derived from typed CDADocument (Sprint D).
	// This removes the XPath-based CDAHeaderParser dependency entirely.
	headerMap := headerToJSON(typedDoc)
	parsed := s.assembleJSON(raw, profile, normResult.Substitutions, headerMap, sectionResults)

	// "xml" is the primary, complete representation (see step 4 above) —
	// assigned first to reflect that it is the source of truth, not an
	// afterthought alongside the curated views below.
	parsed["xml"] = genericXML

	// Phase 3: also expose the full typed tree under "document" — classCode,
	// moodCode, negationInd, entryRelationship semantics, repeating elements,
	// translations, etc. that the curated "sections"/"header" maps above never
	// carry (they're driven by a fixed per-section field whitelist). This is
	// what makes the document addressable by GetFieldValue's "document.*" CDA
	// path form (services/executors/field_utils.go) and by any generic
	// pipeline step, not just the cda.to_fhir mapper's privileged typed path.
	// Like "sections"/"header", this remains a curated, best-effort VIEW —
	// "xml" above is the one with a completeness guarantee.
	parsed["document"] = typedDoc

	result.ParsedJSON = parsed

	// 7. Populate EnhancedFields for the field picker
	s.populateEnhancedFields(result, sectionResults)

	// Metadata
	result.Success = true
	result.ParsingTime = time.Since(start)
	result.Metadata = models.ParserMetadata{
		DetectedVersion: string(profile),
		ParsedAt:        time.Now(),
		MessageType:     "CCD",
	}

	return result
}

// =====================================
// Parallel section processing
// =====================================

type sectionJob struct {
	sectionEl *etree.Element
	sectionKey string
	schema     *cdaSchema.CDASectionDef
}

func (s *CDAParserService) processSectionsParallel(root *etree.Element) []*SectionResult {
	// Collect all <section> elements from the structured body.
	var sectionEls []*etree.Element
	for _, el := range root.FindElements("//section") {
		sectionEls = append(sectionEls, el)
	}

	if len(sectionEls) == 0 {
		return nil
	}

	jobs := make(chan sectionJob, len(sectionEls))
	resultsCh := make(chan *SectionResult, len(sectionEls))

	// Start workers.
	workers := s.maxWorkers
	if workers > len(sectionEls) {
		workers = len(sectionEls)
	}

	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for job := range jobs {
				resultsCh <- s.processOneSectionSafe(job)
			}
		}()
	}

	// Dispatch jobs.
	for _, el := range sectionEls {
		key, schema := s.resolveSectionKey(el)
		if key == "" {
			continue // Skip sections we cannot identify
		}
		jobs <- sectionJob{sectionEl: el, sectionKey: key, schema: schema}
	}
	close(jobs)

	wg.Wait()
	close(resultsCh)

	var results []*SectionResult
	for r := range resultsCh {
		results = append(results, r)
	}
	return results
}

// processOneSectionSafe runs the registered processor for job and recovers any
// panic (e.g. unsupported etree XPath predicate) so a single bad section never
// crashes the whole goroutine pool or the Go process.
func (s *CDAParserService) processOneSectionSafe(job sectionJob) (res *SectionResult) {
	defer func() {
		if r := recover(); r != nil {
			res = &SectionResult{
				SectionKey: job.sectionKey,
				Entries:    []map[string]interface{}{},
				Error:      fmt.Sprintf("section processor panic (recovered): %v", r),
			}
		}
	}()
	proc, ok := s.registry.Get(job.sectionKey)
	if !ok {
		return s.fallbackResult(job.sectionEl, job.sectionKey)
	}
	return proc.Process(job.sectionEl, job.schema)
}

// resolveSectionKey determines the section key from a <section> element.
// It checks templateId first, then the section/code LOINC code.
func (s *CDAParserService) resolveSectionKey(el *etree.Element) (string, *cdaSchema.CDASectionDef) {
	// Try templateId elements
	for _, tid := range el.SelectElements("templateId") {
		oid := tid.SelectAttrValue("root", "")
		if sec := s.schemaLoader.GetSectionByTemplateID(oid); sec != nil {
			return sec.Key, sec
		}
	}

	// Try LOINC code on <code> child
	codeEl := el.FindElement("code")
	if codeEl != nil {
		loinc := codeEl.SelectAttrValue("code", "")
		if sec := s.schemaLoader.GetSectionByLOINC(loinc); sec != nil {
			return sec.Key, sec
		}
	}

	return "", nil
}

func (s *CDAParserService) fallbackResult(el *etree.Element, key string) *SectionResult {
	b := &baseSectionProcessor{}
	return &SectionResult{
		SectionKey:    key,
		NarrativeHTML: b.extractNarrativeHTML(el),
		Entries:       []map[string]interface{}{},
	}
}

// =====================================
// Result assembly
// =====================================

// assembleJSON builds the parsedCDA map[string]interface{} stored on ParserResult.ParsedJSON.
// headerMap is now supplied by headerToJSON() from the typed CDADocument (Sprint D).
func (s *CDAParserService) assembleJSON(
	raw string,
	profile cdaSchema.CDAProfile,
	substitutions int,
	headerMap map[string]interface{},
	sections []*SectionResult,
) map[string]interface{} {

	sectionsMap := make(map[string]interface{})
	for _, sec := range sections {
		if sec == nil {
			continue
		}
		entry := map[string]interface{}{
			"entries":    sec.Entries,
			"entryCount": sec.EntryCount,
		}
		if sec.NarrativeHTML != "" {
			entry["narrativeHTML"] = sec.NarrativeHTML
		}
		if sec.Error != "" {
			entry["error"] = sec.Error
		}
		if schemaDef := s.schemaLoader.GetSection(sec.SectionKey); schemaDef != nil {
			entry["loincCode"] = schemaDef.LOINCCode
			entry["displayName"] = schemaDef.DisplayName
			entry["uscdiClass"] = schemaDef.USCDIClass
		}
		sectionsMap[sec.SectionKey] = entry
	}

	parsed := map[string]interface{}{
		"_format":      string(models.FormatCCDA),
		"cdaProfile":   string(profile),
		"uscdiVersion": string(uscdi.USCDIv3),
		"raw":          raw,
		"header":       headerMap,
		"sections":     sectionsMap,
	}
	if substitutions > 0 {
		parsed["normalizedFrom"] = string(profile)
		parsed["templateSubstitutions"] = substitutions
	}
	return parsed
}

// populateEnhancedFields creates one EnhancedField per section key so the
// field picker can enumerate available CDA sections.
func (s *CDAParserService) populateEnhancedFields(result *models.ParserResult, sections []*SectionResult) {
	for _, sec := range sections {
		if sec == nil {
			continue
		}
		schemaDef := s.schemaLoader.GetSection(sec.SectionKey)
		name := sec.SectionKey
		desc := ""
		if schemaDef != nil {
			name = schemaDef.DisplayName
			desc = schemaDef.USCDIClass
		}

		field := &models.EnhancedField{
			Path:        sec.SectionKey,
			Name:        name,
			Description: desc,
			HasValue:    sec.EntryCount > 0,
			SchemaFound: schemaDef != nil,
		}
		result.EnhancedFields[sec.SectionKey] = field
		result.FieldOrder = append(result.FieldOrder, sec.SectionKey)
	}
}

// =====================================
// Smart field search
// =====================================

// SearchFields returns USCDI-labeled SearchResult entries for a free-text query,
// enriched with CDA XPath and conformance. Used by the smart search API when
// messageType=CCD so the field picker shows clinical labels instead of raw paths.
func (s *CDAParserService) SearchFields(query string, maxResults int) []*uscdi.SearchResult {
	results := s.vocabulary.Search(query, "cda", maxResults)
	resolver := NewCDAPathResolver(s.schemaLoader, s.vocabulary)
	for _, r := range results {
		fhirPath, xPath, conformance := resolver.ResolveShortPath(r.ShortPath)
		if r.FHIRPath == "" && fhirPath != "" {
			r.FHIRPath = fhirPath
		}
		if xPath != "" {
			r.CDAXPath = xPath
		}
		if conformance != "" {
			r.Conformance = conformance
		}
	}
	return results
}

// =====================================
// Factory constructor (convenience)
// =====================================

// NewFromSchemaDir builds a fully wired CDAParserService from a schema directory
// path, using DefaultSectionRegistry (all 9 OOB processors).
func NewFromSchemaDir(schemaDir string) (*CDAParserService, error) {
	loader, err := cdaSchema.NewCDASchemaLoader(schemaDir)
	if err != nil {
		return nil, fmt.Errorf("cda: schema loader init: %w", err)
	}

	uscdiPath := schemaDir + "/uscdi_v3.json"
	vocab, err := uscdi.NewUSCDIVocabulary(uscdiPath)
	if err != nil {
		return nil, fmt.Errorf("cda: uscdi vocabulary init: %w", err)
	}

	normalizer := cdaSchema.NewC32TemplateNormalizer(loader)
	return NewCDAParserService(loader, vocab, normalizer, DefaultSectionRegistry(loader)), nil
}

// =====================================
// services.MessageParser interface methods
// =====================================

// GetSupportedFormat satisfies the services.MessageParser interface used by ParserFactory.
func (s *CDAParserService) GetSupportedFormat() models.MessageFormat {
	return models.FormatCCDA
}

// ValidateStructure checks whether the raw content looks like valid CDA XML.
func (s *CDAParserService) ValidateStructure(rawContent string) (*models.ValidationResult, error) {
	result := &models.ValidationResult{Warnings: []string{}, Errors: []string{}}

	if len(rawContent) < 20 {
		result.Errors = append(result.Errors, "Content too short to be valid CDA XML")
		return result, nil
	}

	if !containsStr(rawContent, "ClinicalDocument") {
		result.Errors = append(result.Errors, "Missing ClinicalDocument root element")
		return result, nil
	}

	doc := etree.NewDocument()
	if err := doc.ReadFromString(rawContent); err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("XML parse error: %v", err))
		return result, nil
	}

	result.IsValid = true
	return result, nil
}

func containsStr(s, sub string) bool {
	if len(sub) > len(s) {
		return false
	}
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// =====================================
// Worker count
// =====================================

func resolveSectionWorkers() int {
	if val := os.Getenv("MAX_CDA_SECTION_WORKERS"); val != "" {
		if n, err := strconv.Atoi(val); err == nil && n > 0 {
			return n
		}
	}
	if n := runtime.NumCPU(); n > 0 {
		return n
	}
	return 4
}

func resolveMaxDocSizeMB() int {
	if val := os.Getenv("CDA_MAX_DOCUMENT_SIZE_MB"); val != "" {
		if n, err := strconv.Atoi(val); err == nil && n > 0 {
			if n > hardCapDocSizeMB {
				n = hardCapDocSizeMB
			}
			return n
		}
	}
	return defaultMaxDocSizeMB
}

// =====================================
// Typed-header → JSON conversion (Sprint D)
// =====================================

// headerToJSON converts a CDAHeader (from the typed document model) into the
// map[string]interface{} shape expected by assembleJSON() and the legacy Map() path.
// Keys match those historically produced by CDAHeaderParser so all downstream
// consumers (generic_mapper.go legacy path, UI field picker) continue to work.
func headerToJSON(doc *cdadocument.CDADocument) map[string]interface{} {
	if doc == nil {
		return map[string]interface{}{
			"patient":  map[string]interface{}{},
			"document": map[string]interface{}{},
			"author":   map[string]interface{}{},
		}
	}
	h := doc.Header
	return map[string]interface{}{
		"patient":              patientToLegacyJSON(h.Patient),
		"document":             documentToJSON(h),
		"author":               authorToJSON(h),
		"informants":           informantsToJSON(h),
		"documentationOf":      documentOfToJSON(h),
		"encompassingEncounter": encompassingEncounterToJSON(h),
	}
}

// patientToLegacyJSON maps CDAPatient fields to the same key names that
// CDAHeaderParser.parsePatient() produced, so the legacy buildLegacyPatient()
// function in generic_mapper.go continues to work without modification.
func patientToLegacyJSON(p cdadocument.CDAPatient) map[string]interface{} {
	m := make(map[string]interface{})

	// Names — use="L" (legal) preferred
	for _, n := range p.Names {
		if n.Use == "L" || n.Use == "" {
			if len(n.Given) > 0 {
				m["firstName"] = n.Given[0]
				if len(n.Given) > 1 {
					m["middleName"] = n.Given[1]
				}
			}
			m["lastName"] = n.Family
			break
		}
	}
	if _, hasFirst := m["firstName"]; !hasFirst && len(p.Names) > 0 {
		n := p.Names[0]
		if len(n.Given) > 0 {
			m["firstName"] = n.Given[0]
			if len(n.Given) > 1 {
				m["middleName"] = n.Given[1]
			}
		}
		m["lastName"] = n.Family
	}

	m["dateOfBirth"] = p.BirthDate.Value
	m["sex"] = p.Gender.Code
	m["sexDisplay"] = p.Gender.DisplayName
	m["race"] = p.Race.Code
	m["raceDisplay"] = p.Race.DisplayName
	m["ethnicity"] = p.Ethnicity.Code
	m["ethnicityDisplay"] = p.Ethnicity.DisplayName

	// All <id> elements as [{root, extension}] array
	ids := make([]interface{}, 0, len(p.Ids))
	for _, id := range p.Ids {
		ids = append(ids, map[string]interface{}{
			"root":      id.Root,
			"extension": id.Extension,
		})
	}
	m["ids"] = ids

	// First home/permanent address
	for _, addr := range p.Addresses {
		if addr.NullFlavor != "" {
			continue
		}
		addrMap := make(map[string]interface{})
		if len(addr.StreetLines) > 0 {
			addrMap["street"] = addr.StreetLines[0]
		}
		addrMap["city"] = addr.City
		addrMap["state"] = addr.State
		addrMap["postalCode"] = addr.PostalCode
		addrMap["country"] = addr.Country
		m["address"] = addrMap
		break
	}

	// First home phone
	for _, t := range p.Telecoms {
		if t.Use == "HP" || t.Use == "" {
			m["phone"] = strings.TrimPrefix(t.Value, "tel:")
			break
		}
	}

	// Language
	for _, lang := range p.Languages {
		if lang.PreferenceInd && lang.Code != "" {
			m["preferredLanguage"] = lang.Code
			break
		}
	}

	return m
}

func documentToJSON(h cdadocument.CDAHeader) map[string]interface{} {
	return map[string]interface{}{
		"title":         h.Title,
		"effectiveTime": h.EffectiveTime.Value,
		"versionNumber": h.VersionNumber,
	}
}

func authorToJSON(h cdadocument.CDAHeader) map[string]interface{} {
	if len(h.Authors) == 0 {
		return map[string]interface{}{}
	}
	a := h.Authors[0]
	m := make(map[string]interface{})
	if a.AssignedAuthor.AssignedPerson != nil && len(a.AssignedAuthor.AssignedPerson.Names) > 0 {
		n := a.AssignedAuthor.AssignedPerson.Names[0]
		if len(n.Given) > 0 {
			m["given"] = n.Given[0]
		}
		m["family"] = n.Family
	}
	for _, id := range a.AssignedAuthor.Ids {
		if id.Root == "2.16.840.1.113883.4.6" { // NPI OID
			m["npi"] = id.Extension
			break
		}
	}
	return m
}

// informantsToJSON maps document-level <informant> entries — sources of
// information for the document who are not its author (e.g. a referring
// provider) — to the same flat given/family/npi/orgName shape authorToJSON
// uses, one map per informant.
func informantsToJSON(h cdadocument.CDAHeader) []interface{} {
	informants := make([]interface{}, 0, len(h.Informants))
	for _, inf := range h.Informants {
		m := make(map[string]interface{})
		ae := inf.AssignedEntity
		if ae.AssignedPerson != nil && len(ae.AssignedPerson.Names) > 0 {
			n := ae.AssignedPerson.Names[0]
			if len(n.Given) > 0 {
				m["given"] = n.Given[0]
			}
			m["family"] = n.Family
		}
		for _, id := range ae.Ids {
			if id.Root == "2.16.840.1.113883.4.6" {
				m["npi"] = id.Extension
				break
			}
		}
		if ae.RepresentedOrganization != nil && len(ae.RepresentedOrganization.Names) > 0 {
			m["orgName"] = ae.RepresentedOrganization.Names[0]
		}
		informants = append(informants, m)
	}
	return informants
}

// documentOfToJSON maps documentationOf/serviceEvent — the overall clinical
// service this document documents, and who performed it — to a flat shape.
// Only the first performer is exposed as top-level performerGiven/Family/NPI
// (matching author's single-entity convention); the full list is also
// included as "performers" for completeness.
func documentOfToJSON(h cdadocument.CDAHeader) map[string]interface{} {
	m := map[string]interface{}{}
	se := h.DocumentOf
	if se == nil {
		return m
	}
	if se.EffectiveTime.Low.Value != "" {
		m["effectiveTimeLow"] = se.EffectiveTime.Low.Value
	}
	if se.EffectiveTime.High.Value != "" {
		m["effectiveTimeHigh"] = se.EffectiveTime.High.Value
	}
	performers := make([]interface{}, 0, len(se.Performers))
	for i, p := range se.Performers {
		pm := map[string]interface{}{}
		if p.AssignedEntity.AssignedPerson != nil && len(p.AssignedEntity.AssignedPerson.Names) > 0 {
			n := p.AssignedEntity.AssignedPerson.Names[0]
			if len(n.Given) > 0 {
				pm["given"] = n.Given[0]
				if i == 0 {
					m["performerGiven"] = n.Given[0]
				}
			}
			pm["family"] = n.Family
			if i == 0 {
				m["performerFamily"] = n.Family
			}
		}
		for _, id := range p.AssignedEntity.Ids {
			if id.Root == "2.16.840.1.113883.4.6" {
				pm["npi"] = id.Extension
				if i == 0 {
					m["performerNPI"] = id.Extension
				}
				break
			}
		}
		performers = append(performers, pm)
	}
	m["performers"] = performers
	return m
}

// encompassingEncounterToJSON maps componentOf/encompassingEncounter — the
// specific encounter this document was generated for, distinct from the
// "encounters" SECTION's historical encounter list — to a flat shape.
func encompassingEncounterToJSON(h cdadocument.CDAHeader) map[string]interface{} {
	m := map[string]interface{}{}
	enc := h.EncompassingEncounter
	if enc == nil {
		return m
	}
	if enc.Id.Extension != "" {
		m["id"] = enc.Id.Extension
	}
	if enc.EffectiveTime.Low.Value != "" {
		m["effectiveTimeLow"] = enc.EffectiveTime.Low.Value
	}
	if enc.EffectiveTime.High.Value != "" {
		m["effectiveTimeHigh"] = enc.EffectiveTime.High.Value
	}
	if enc.DischargeDispositionCode.Code != "" {
		m["dischargeDispositionCode"] = enc.DischargeDispositionCode.Code
	}
	if enc.Location != nil {
		hcf := enc.Location.HealthCareFacility
		if hcf.Code.Code != "" {
			m["facilityTypeCode"] = hcf.Code.Code
		}
		if hcf.Location != nil && len(hcf.Location.Names) > 0 {
			m["facilityName"] = hcf.Location.Names[0].Family
		}
		if hcf.ServiceProviderOrganization != nil && len(hcf.ServiceProviderOrganization.Names) > 0 {
			m["facilityOrgName"] = hcf.ServiceProviderOrganization.Names[0]
		}
	}
	return m
}
