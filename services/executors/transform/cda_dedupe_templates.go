// services/executors/transform/cda_dedupe_templates.go
// OOB identity-matching rules for cda.dedupe — one DedupeIdentityRule per
// supported CDA section, defining the composite CDA path(s) (relative to one
// section entry) that determine whether two entries represent the SAME
// real-world clinical fact, as opposed to remove_duplicates' generic
// full-record-or-configured-field hash: CDA entries need paths several
// entryRelationships[typeCode=X] hops deep (e.g. an allergy's substance
// code), which only the CDA-specific path grammar (ResolveCDAPath,
// cda_path_resolver.go) can resolve — remove_duplicates' generic
// GetFieldValue cannot reach them.
//
// Key paths were chosen per-section based on what actually distinguishes two
// GENUINELY duplicate entries from two DIFFERENT clinical facts that merely
// share a code:
//   - Allergies: substance code alone — an allergy to penicillin is the same
//     allergy no matter when it was recorded; there is no legitimate
//     "second, different" penicillin allergy.
//   - Medications/Immunizations/Procedures/VitalSigns/Results: code + date —
//     the same drug/vaccine/test on two DIFFERENT dates is two real,
//     distinct clinical events, not a duplicate.
//   - Problems: code + onset date, same reasoning as above.
//   - Encounters: the CDA <id> (root+extension) itself, matching the SAME
//     identity convention declarative_oob_rules.go already established for
//     merging Encounter resources (EmbedCDAIdentity) — encounters are
//     canonically identified by their own id, not by content.
//   - SocialHistory: observation type + recorded value — e.g. two identical
//     "smoking status: never smoked" entries are a duplicate; a changed
//     answer on a later visit is not (within-document scope, this mostly
//     guards against accidental double-entry).
//
// V1 scope: same entry-level-direct-paths-only limitation as
// cda_csv_templates.go — no nested Scope-then-KeyPath two-tier resolution.
package transform

import "sort"

// DedupeIdentityRule is the OOB composite identity key for one CDA section:
// entries whose KeyPaths ALL resolve to equal values are considered
// duplicates of each other.
type DedupeIdentityRule struct {
	SectionKey string
	KeyPaths   []string
}

// cdaDedupeIdentityRules is the OOB registry, keyed by section key — the
// dedup-identity analogue of cdaCSVSectionTemplates (cda_csv_templates.go).
var cdaDedupeIdentityRules = map[string]DedupeIdentityRule{
	"medications": {
		SectionKey: "medications",
		KeyPaths: []string{
			"consumable.manufacturedProduct.manufacturedMaterial.code.code",
			"effectiveTime.low.value",
		},
	},
	"allergiesAndIntolerances": {
		SectionKey: "allergiesAndIntolerances",
		KeyPaths: []string{
			"entryRelationships[typeCode=SUBJ].entry.participants[typeCode=CSM].participantRole.playingEntity.code.code",
		},
	},
	"problems": {
		SectionKey: "problems",
		KeyPaths: []string{
			"entryRelationships[typeCode=SUBJ].entry.value.code",
			"entryRelationships[typeCode=SUBJ].entry.effectiveTime.low.value",
		},
	},
	"vitalSigns": {
		SectionKey: "vitalSigns",
		KeyPaths: []string{
			"code.code",
			"effectiveTime.value.value",
		},
	},
	"immunizations": {
		SectionKey: "immunizations",
		KeyPaths: []string{
			"consumable.manufacturedProduct.manufacturedMaterial.code.code",
			"effectiveTime.value.value",
		},
	},
	"procedures": {
		SectionKey: "procedures",
		KeyPaths: []string{
			"code.code",
			"effectiveTime.value.value",
		},
	},
	"encounters": {
		SectionKey: "encounters",
		KeyPaths: []string{
			"id[0].root",
			"id[0].extension",
		},
	},
	"socialHistory": {
		SectionKey: "socialHistory",
		KeyPaths: []string{
			"code.code",
			"value.code.code",
		},
	},
	"results": {
		SectionKey: "results",
		KeyPaths: []string{
			"code.code",
			"effectiveTime.value.value",
		},
	},
}

// SupportedCDADedupeSections returns the sorted list of section keys with an
// OOB identity rule — used by the executor's default "dedupe every section
// we have a rule for" behavior and by the Documentation tab.
func SupportedCDADedupeSections() []string {
	keys := make([]string, 0, len(cdaDedupeIdentityRules))
	for k := range cdaDedupeIdentityRules {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// DedupeFieldOption is one selectable identity-matching field for a section in
// the cda.dedupe step's no-code field picker — a friendly Name + the CDA Path
// it resolves (same path a user would otherwise have had to type by hand into
// overrides.<key>.keyPaths), plus whether it's part of the OOB identity rule.
type DedupeFieldOption struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	Default bool   `json:"default"`
}

// DedupeSectionOption is one section's full field-picker option list.
type DedupeSectionOption struct {
	SectionKey string               `json:"sectionKey"`
	Fields     []DedupeFieldOption  `json:"fields"`
}

// DedupeSectionCatalog returns every section cda.dedupe can operate on, with
// its available identity-matching fields and which are pre-selected by the OOB
// rule (cdaDedupeIdentityRules) — for the step config UI's checkbox picker.
//
// Sourced live from CSVSectionCatalog() (services/executors/transform/
// cda_section_to_csv_executor.go) for the field list, since that catalog
// already carries a friendly Name + resolved CDA Path per field for every
// section this codebase knows about — reusing it here means the picker can
// never drift out of sync with a hand-maintained list, and it automatically
// covers every section the CSV catalog ever gains in the future. Sections
// marked NarrativeOnly are excluded: a narrative section has no repeatable
// <entry> elements for cda.dedupe to compare, only one block of prose text.
//
// A field counts as "Default" (pre-checked) when its Path exactly matches one
// of cdaDedupeIdentityRules[sectionKey].KeyPaths — verified by hand against
// every OOB section that the two catalogs' path strings agree exactly.
func DedupeSectionCatalog() []DedupeSectionOption {
	csvSections := CSVSectionCatalog()
	out := make([]DedupeSectionOption, 0, len(csvSections))
	for _, sec := range csvSections {
		if sec.NarrativeOnly {
			continue
		}
		defaultPaths := map[string]bool{}
		if rule, ok := cdaDedupeIdentityRules[sec.SectionKey]; ok {
			for _, p := range rule.KeyPaths {
				defaultPaths[p] = true
			}
		}
		fields := make([]DedupeFieldOption, 0, len(sec.Columns))
		for _, col := range sec.Columns {
			fields = append(fields, DedupeFieldOption{
				Name:    col.Name,
				Path:    col.Path,
				Default: defaultPaths[col.Path],
			})
		}
		out = append(out, DedupeSectionOption{SectionKey: sec.SectionKey, Fields: fields})
	}
	return out
}
