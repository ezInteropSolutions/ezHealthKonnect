package cdafhir_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	cdaSchema "ezhealthkonnect/cda"
	cdadocument "ezhealthkonnect/cda/document"
	cdafhir "ezhealthkonnect/services/cda_fhir"
	"ezhealthkonnect/services/cda_fhir/assembly"
	"ezhealthkonnect/services/cda_fhir/assembly/rules"
)

func TestZZZRealDoc_VitalSigns_NewSamples(t *testing.T) {
	paths := []string{
		"/host_downloads/CCD_05_15_2026.xml",
		"/host_downloads/CCD_06_24_20267.xml",
		"/host_downloads/Continuity of Care Document (3).xml",
	}
	for _, p := range paths {
		t.Run(p, func(t *testing.T) {
			if _, err := os.Stat(p); err != nil {
				t.Skipf("file not mounted: %v", err)
			}

			_, thisFile, _, _ := runtime.Caller(0)
			repoRoot := filepath.Join(filepath.Dir(thisFile), "..", "..")
			schemaDir := filepath.Join(repoRoot, "cda", "schemas")
			loader, err := cdaSchema.NewCDASchemaLoader(schemaDir)
			if err != nil {
				t.Fatalf("loading schema: %v", err)
			}
			parser := cdadocument.NewCDAParser(loader)
			rawXML, err := os.ReadFile(p)
			if err != nil {
				t.Fatalf("reading: %v", err)
			}
			doc, err := parser.ParseFromRawXML(string(rawXML))
			if err != nil {
				t.Fatalf("parsing: %v", err)
			}
			sec, ok := doc.SectionsByKey["vitalSigns"]
			if !ok {
				fmt.Printf("=== %s: no vitalSigns section ===\n", p)
				return
			}

			wrapper := map[string]interface{}{
				"sectionsByKey": map[string]interface{}{
					"vitalSigns": map[string]interface{}{"entries": sec.Entries},
				},
			}
			encoded, _ := json.Marshal(wrapper)
			var documentMap map[string]interface{}
			json.Unmarshal(encoded, &documentMap)

			engine := cdafhir.NewDeclarativeEngine()
			resources, errs := engine.BuildResources(documentMap, cdafhir.VitalSignsMappingRules()[0])
			fmt.Printf("=== %s: pre-assembly %d errors, %d resources ===\n", p, len(errs), len(resources))
			for i, e := range errs {
				fmt.Printf("  err[%d]: %v\n", i, e)
			}
			for _, r := range resources {
				fmt.Printf("  resourceType=%v keys=%d\n", r["resourceType"], len(r))
			}

			ctx := &assembly.AssemblyContext{
				Resources:      resources,
				DedupRedirects: make(map[string]string),
				Removed:        make(map[string]bool),
			}
			ruleEngine := assembly.NewDefaultRuleEngine()
			ruleEngine.Register(rules.NewBPPanelSynthesisRule())
			if err := ruleEngine.Run(ctx); err != nil {
				t.Fatalf("assembly run: %v", err)
			}
			fmt.Printf("=== %s: post-assembly %d resources ===\n", p, len(ctx.Resources))
			b, _ := json.MarshalIndent(ctx.Resources, "  ", "  ")
			fmt.Printf("  resources:\n  %s\n", string(b))
		})
	}
}
