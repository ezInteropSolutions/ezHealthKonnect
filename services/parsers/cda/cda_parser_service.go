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
	"fmt"
	"os"
	"runtime"
	"strconv"
	"sync"
	"time"

	cdaSchema "ezhealthkonnect/cda"
	"ezhealthkonnect/models"
	"ezhealthkonnect/uscdi"
	"github.com/beevik/etree"
)

// CDAParserService parses CDA/CCD XML documents and returns a schema-enriched
// ParserResult with USCDI-aligned field names and parallel section processing.
type CDAParserService struct {
	schemaLoader *cdaSchema.CDASchemaLoader
	vocabulary   *uscdi.USCDIVocabulary
	normalizer   *cdaSchema.C32TemplateNormalizer
	headerParser *CDAHeaderParser
	registry     *SectionRegistry
	maxWorkers   int
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
		headerParser: NewCDAHeaderParser(),
		registry:     registry,
		maxWorkers:   resolveSectionWorkers(),
	}
}

// Format returns the canonical format string for the ParserRegistry.
func (s *CDAParserService) Format() string {
	return string(models.FormatCCDA) // "ccda"
}

// Parse is the main entry point. It accepts raw CDA/CCD XML and returns a
// fully populated ParserResult. Returns Success=false on fatal parse errors.
func (s *CDAParserService) Parse(raw string) *models.ParserResult {
	start := time.Now()

	result := &models.ParserResult{
		Format:         models.FormatCCDA,
		EnhancedFields: make(map[string]*models.EnhancedField),
		FieldOrder:     []string{},
	}

	// 1. Parse XML
	doc := etree.NewDocument()
	if err := doc.ReadFromString(raw); err != nil {
		result.Success = false
		result.Error = fmt.Sprintf("cda: XML parse error: %v", err)
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

	// 4. Extract document header
	headerResult := s.headerParser.Parse(root)

	// 5. Process sections in parallel
	sectionResults := s.processSectionsParallel(root)

	// 6. Assemble ParsedJSON
	parsed := s.assembleJSON(raw, profile, normResult.Substitutions, headerResult, sectionResults)
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
				proc, ok := s.registry.Get(job.sectionKey)
				if !ok {
					// No processor — emit a minimal result preserving narrative HTML.
					resultsCh <- s.fallbackResult(job.sectionEl, job.sectionKey)
					continue
				}
				resultsCh <- proc.Process(job.sectionEl, job.schema)
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

func (s *CDAParserService) assembleJSON(
	raw string,
	profile cdaSchema.CDAProfile,
	substitutions int,
	header *ParseResult,
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

		// Attach schema metadata if available
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
		"header":       map[string]interface{}{
			"patient":  header.Patient,
			"document": header.Document,
			"author":   header.Author,
		},
		"sections": sectionsMap,
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
	return NewCDAParserService(loader, vocab, normalizer, DefaultSectionRegistry()), nil
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
