// services/cda_coverage/report.go
//
// Diffs a BuildInventory(WithGranularity) result against a
// CDACoverageTracker's touched-key set and persists one cda_coverage_audits
// row per destination (V207 migration). Structural location only in the
// persisted report — section key, title, entry index, element path — never
// a clinical value; see this package's inventory.go doc comment and the
// V207 migration header for why.
package cdacoverage

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"ezhealthkonnect/models"
)

// currentSchemaVersion increments only when Report's persisted shape gains
// a field older code/UI must branch on. 1 = entry-only (every report before
// element-level tracking existed). 2 = element-capable: MissedItem MAY carry
// ElementsFound/ElementsTotal/MissedElements, but a version-2 report is not
// guaranteed to have them on every item (an interface with granularity
// "entry" still produces version-2 reports with no element detail at all --
// the version marks CODE capability, not per-report content). 3 = clinical/
// raw split: ElementsFound/ElementsTotal now mean CLINICAL-only counts
// (administrative/structural noise excluded, see element_classifier.go) --
// a real meaning change for those two field names, not just an addition --
// ElementsFoundAll/ElementsTotalAll carry the unfiltered ground-truth pair,
// and each ElementGap is tagged Administrative so a UI can offer a "show
// everything" toggle without a second fetch. Reports persisted before this
// version have ElementsFound/Total meaning the RAW count, not clinical --
// don't compare across versions without accounting for that. 4 = element-
// weighted headline: CategoryStat.ElementsFound/ElementsTotal (summed
// across that category's entries) and Report.ElementCoveragePct (document-
// wide) are new, additive fields -- a UI should prefer ElementCoveragePct
// over OverallCoveragePct whenever it's non-nil, since entry-level alone
// can read a misleading 100% while every entry still has real element gaps
// (an entry only needs ONE field read to count as entry-level "found").
// Reports persisted before this version have no such field to prefer and
// should keep using OverallCoveragePct exactly as before. 5 = USCDI v3
// class bridging: CategoryStat.USCDIClasses is a new, additive field — the
// real, ONC-verified USCDI v3 class set inventory.go's uscdiClassesForSection
// resolves this category's SectionKey to (cda/schemas/uscdi_v3.json via
// uscdi.USCDIVocabulary), distinct from Category itself (which stays
// CDASectionDef.USCDIClass, an unverified display label — see this
// package's own plan doc on why the two must never be conflated). Absent/
// empty on any report built without a vocabulary, and on every report
// persisted before this version — a UI should treat "no USCDIClasses" as
// "not yet in the USCDI v3 vocabulary," never as "zero USCDI coverage."
const currentSchemaVersion = 5

// ElementGap is one specific element within an entry that nothing
// downstream ever read — never a clinical value, only where it lives.
type ElementGap struct {
	// Path is the element's location relative to its entry, in
	// services/executors/cda_element_translation.go's index-every-segment
	// convention (e.g. "effectiveTime[0]", "performer[0].assignedEntity.
	// addr[0]").
	Path string `json:"path"`
	// Label is a best-effort human-readable name mechanically derived from
	// Path's last segment (see elementLabel) — no curated friendly-name
	// table in v1; see this feature's plan doc for why that's deferred.
	Label string `json:"label,omitempty"`
	// Administrative marks CDA-infrastructure/provenance noise (see
	// element_classifier.go) rather than a genuine clinical field — the
	// default UI view filters these out; a "show everything" toggle can
	// reveal them from this same persisted list without a second fetch.
	Administrative bool `json:"administrative,omitempty"`
}

// MissedItem is one section entry with something to report — either the
// WHOLE entry was never touched (the original, entry-level-only meaning:
// ElementsFound/ElementsTotal/MissedElements all zero/empty), or the entry
// itself WAS touched but one or more of its own elements individually
// weren't (element granularity only: ElementsTotal > 0).
type MissedItem struct {
	SectionKey   string `json:"sectionKey,omitempty"`
	SectionTitle string `json:"sectionTitle,omitempty"`
	EntryIndex   int    `json:"entryIndex"`

	// ElementsFound/ElementsTotal are CLINICAL-only counts (administrative/
	// structural elements excluded, see element_classifier.go) — what the UI
	// shows by default. 0 when this interface hasn't opted into element
	// granularity (or this entry had no clinical element items at all).
	ElementsFound int `json:"elementsFound,omitempty"`
	ElementsTotal int `json:"elementsTotal,omitempty"`

	// ElementsFoundAll/ElementsTotalAll are the unfiltered ground-truth
	// pair (every element, no classification applied) — preserves this
	// feature's original "nothing hidden" guarantee for a UI "show all"
	// toggle. Equal to ElementsFound/ElementsTotal when this entry has no
	// administrative/structural elements at all.
	ElementsFoundAll int `json:"elementsFoundAll,omitempty"`
	ElementsTotalAll int `json:"elementsTotalAll,omitempty"`

	// MissedElements is the FULL raw list — clinical AND administrative,
	// each tagged via ElementGap.Administrative. Filtering to the default
	// clinical-only view is a client-side concern, not a server-side data
	// loss (the classifier is a display heuristic, not ground-truth
	// deletion).
	MissedElements []ElementGap `json:"missedElements,omitempty"`
}

// CategoryStat is one clinical/administrative category's coverage summary.
// Found/Total are always ENTRY-level counts (unchanged meaning regardless of
// granularity: "was this entry touched by ANY rule at all") — kept for
// backward compatibility with entry-only reports/consumers, but a category
// can read Found==Total (100%) here while every one of its entries still
// has real element-level gaps, since an entry only needs ONE field read to
// count as "found." ElementsFound/ElementsTotal (the SUM of every entry's
// own clinical element counts in this category — element_classifier.go)
// are the more precise signal once element granularity is active, and
// should drive any headline percentage the UI shows in preference to
// Found/Total whenever ElementsTotal > 0.
type CategoryStat struct {
	Category string `json:"category"`
	Found    int    `json:"found"`
	Total    int    `json:"total"`
	// ElementsFound/ElementsTotal are 0 when no entry in this category has
	// any element-level data (entry-granularity interface) — omitted from
	// JSON in that case so an entry-only consumer sees exactly the shape it
	// always has.
	ElementsFound int          `json:"elementsFound,omitempty"`
	ElementsTotal int          `json:"elementsTotal,omitempty"`
	Missed        []MissedItem `json:"missed"`
	// USCDIClasses is the real USCDI v3 class set every InventoryItem in
	// this category resolved to (deduped/unioned — see
	// inventory.go's uscdiClassesForSection), omitted from JSON when empty
	// so a report built without a vocabulary, or a category with no
	// vocabulary entries yet, reads identically to a pre-version-5 report.
	USCDIClasses []string `json:"uscdiClasses,omitempty"`
}

// Report is the full coverage result for one message/destination pair.
type Report struct {
	OverallCoveragePct float64 `json:"-"`
	// ElementCoveragePct is the document-wide clinical element completeness
	// (sum of every entry's own ElementsFound/ElementsTotal across every
	// category) — nil when NO entry anywhere in the document has any
	// element-level data (this interface is on entry granularity), in
	// which case a UI should fall back to OverallCoveragePct. Present
	// (non-nil) whenever element granularity produced any data at all, even
	// if some individual categories/entries didn't — see CategoryStat's own
	// doc comment for why this, not OverallCoveragePct, is the number a UI
	// should prefer to show as the headline once it exists: entry-level
	// alone can read a misleading 100% while real gaps are visible in every
	// section below it.
	ElementCoveragePct *float64       `json:"elementCoveragePct,omitempty"`
	SchemaVersion      int            `json:"schemaVersion"`
	Categories         []CategoryStat `json:"categories"`
}

// entryGroupKey identifies one entry across an inventory built with mixed
// entry-level and element-level items.
type entryGroupKey struct {
	category   string
	sectionKey string
	entryIndex int
}

// entryGroup accumulates one entry's own touched status plus every element
// item nested under it, in first-seen order (so a group's own
// MissedElements list reads in the same order BuildInventoryWithGranularity
// produced them, itself sorted -- see walkEntryElements). Clinical and raw
// (*All) counters are tracked side by side -- see element_classifier.go and
// currentSchemaVersion's v3 doc comment.
type entryGroup struct {
	sectionKey, sectionTitle string
	entryIndex               int
	entryTouched             bool
	elementTotal             int
	elementFound             int
	elementTotalAll          int
	elementFoundAll          int
	missedElements           []ElementGap
	// uscdiClasses is this entry's own SectionKey's resolved USCDI class set
	// (identical for every item sharing this group — see
	// inventory.go's uscdiClassesForSection) — carried on the group so
	// BuildReport's category loop can union it across every group in that
	// category without re-resolving anything.
	uscdiClasses []string
}

// BuildReport groups inventory by category (and, within a category, by
// entry — see entryGroup) and marks each item found/missed against touched
// (a CDACoverageTracker.Snapshot() result — see cda_coverage_tracker.go).
// Deterministic order: category and entry order both follow inventory's own
// first-seen order, so the report reads the same way run to run for the
// same document shape.
func BuildReport(inventory []InventoryItem, touched map[string]struct{}) *Report {
	var categoryOrder []string
	groupsByCategory := make(map[string][]*entryGroup)
	groupIndex := make(map[entryGroupKey]*entryGroup)

	totalEntries := 0
	totalEntriesFound := 0

	for _, item := range inventory {
		if _, seen := groupsByCategory[item.Category]; !seen {
			categoryOrder = append(categoryOrder, item.Category)
		}
		gk := entryGroupKey{category: item.Category, sectionKey: item.SectionKey, entryIndex: item.EntryIndex}
		g, exists := groupIndex[gk]
		if !exists {
			g = &entryGroup{sectionKey: item.SectionKey, sectionTitle: item.SectionTitle, entryIndex: item.EntryIndex, uscdiClasses: item.USCDIClasses}
			groupIndex[gk] = g
			groupsByCategory[item.Category] = append(groupsByCategory[item.Category], g)
		}

		key := item.TrackingKey()
		isTouched := key != "" && isCovered(key, touched)

		if item.ElementPath == "" {
			// The entry-level item itself -- exactly the original
			// pre-element-granularity accounting, unchanged.
			g.entryTouched = isTouched
			totalEntries++
			if isTouched {
				totalEntriesFound++
			}
			continue
		}

		isAdmin := isAdministrativeElementPath(item.ElementPath)
		g.elementTotalAll++
		if isTouched {
			g.elementFoundAll++
		}
		if !isAdmin {
			g.elementTotal++
			if isTouched {
				g.elementFound++
			}
		}
		if !isTouched {
			g.missedElements = append(g.missedElements, ElementGap{
				Path:           item.ElementPath,
				Label:          elementLabel(item.ElementPath),
				Administrative: isAdmin,
			})
		}
	}

	docElementsFound, docElementsTotal := 0, 0

	categories := make([]CategoryStat, 0, len(categoryOrder))
	for _, cat := range categoryOrder {
		stat := CategoryStat{Category: cat, Missed: []MissedItem{}}
		uscdiClassSeen := make(map[string]bool)
		for _, g := range groupsByCategory[cat] {
			stat.Total++
			if g.entryTouched {
				stat.Found++
			}
			stat.ElementsFound += g.elementFound
			stat.ElementsTotal += g.elementTotal
			// Union across every group in this category, deduped — normally
			// every group shares the identical class set already (all from
			// the same SectionKey), but Category is what groups items here,
			// not SectionKey, so this stays correct even in the rare case
			// two differently-keyed sections share one Category label.
			for _, class := range g.uscdiClasses {
				if !uscdiClassSeen[class] {
					uscdiClassSeen[class] = true
					stat.USCDIClasses = append(stat.USCDIClasses, class)
				}
			}
			// Surfaced when the whole entry was never touched (original
			// meaning) OR the entry WAS touched but has its own element
			// gaps (new: a "partially missed" entry) -- an entry that's
			// both fully touched AND has no element items at all (no
			// granularity opt-in, or every element found) reports nothing.
			if !g.entryTouched || len(g.missedElements) > 0 {
				stat.Missed = append(stat.Missed, MissedItem{
					SectionKey:       g.sectionKey,
					SectionTitle:     g.sectionTitle,
					EntryIndex:       g.entryIndex,
					ElementsFound:    g.elementFound,
					ElementsTotal:    g.elementTotal,
					ElementsFoundAll: g.elementFoundAll,
					ElementsTotalAll: g.elementTotalAll,
					MissedElements:   g.missedElements,
				})
			}
		}
		sort.Strings(stat.USCDIClasses)
		docElementsFound += stat.ElementsFound
		docElementsTotal += stat.ElementsTotal
		categories = append(categories, stat)
	}

	overallPct := 100.0
	if totalEntries > 0 {
		overallPct = (float64(totalEntriesFound) / float64(totalEntries)) * 100
	}

	var elementPct *float64
	if docElementsTotal > 0 {
		pct := (float64(docElementsFound) / float64(docElementsTotal)) * 100
		elementPct = &pct
	}

	return &Report{
		OverallCoveragePct: overallPct,
		ElementCoveragePct: elementPct,
		SchemaVersion:      currentSchemaVersion,
		Categories:         categories,
	}
}

// isCovered reports whether key, or any ancestor prefix of it (split on
// "."), is in touched. An element-level key's ancestor being touched means
// a mapping rule read that ancestor as one opaque node (e.g. a
// cda_timerange_to_onset-style transform reading "effectiveTime" whole) --
// which implicitly covers everything nested inside it, even though nothing
// recorded the deeper child keys individually. Entry-level lookups (no "/"
// in key, see InventoryItem.TrackingKey) have no ancestors to roll up to and
// behave exactly as a plain map lookup.
func isCovered(key string, touched map[string]struct{}) bool {
	if _, ok := touched[key]; ok {
		return true
	}
	slash := strings.IndexByte(key, '/')
	if slash < 0 {
		return false
	}
	prefix, elementPath := key[:slash+1], key[slash+1:]
	segments := strings.Split(elementPath, ".")
	for i := len(segments) - 1; i > 0; i-- {
		ancestor := prefix + strings.Join(segments[:i], ".")
		if _, ok := touched[ancestor]; ok {
			return true
		}
	}
	return false
}

// elementLabel mechanically derives a human-readable label from an element
// path's last segment -- e.g. "performer[0].assignedEntity.addr[0]" ->
// "Addr". No curated friendly-name table in v1 (see this feature's plan doc
// for why that's deliberately deferred); title-cased is closer to a display
// error message that outright means nothing without a document, but still
// concretely better than the raw path alone for scanability.
func elementLabel(path string) string {
	last := path
	if dot := strings.LastIndexByte(path, '.'); dot >= 0 {
		last = path[dot+1:]
	}
	if br := strings.IndexByte(last, '['); br >= 0 {
		last = last[:br]
	}
	if last == "" {
		return ""
	}
	return strings.ToUpper(last[:1]) + last[1:]
}

// HasGaps reports whether this report contains at least one missed item —
// the gate the frontend Journey tab uses to decide whether a "Coverage
// Audit" step is worth showing at all (a fully-covered document renders
// nothing, matching the "Dedup Suppressions" step's own only-when-non-empty
// convention).
func (r *Report) HasGaps() bool {
	for _, c := range r.Categories {
		if len(c.Missed) > 0 {
			return true
		}
	}
	return false
}

// SaveReport persists one cda_coverage_audits row for this (message,
// destination) pair.
func SaveReport(ctx context.Context, db *sql.DB, job models.CoverageAuditJob, report *Report) error {
	if db == nil {
		return fmt.Errorf("cda_coverage: SaveReport: nil db")
	}
	categoryStatsJSON, err := json.Marshal(report)
	if err != nil {
		return fmt.Errorf("cda_coverage: marshal category_stats: %w", err)
	}

	const q = `
		INSERT INTO cda_coverage_audits
			(interface_id, message_id, step_id, step_name, connector_type,
			 destination, delivery_outcome, overall_coverage_pct, category_stats)
		VALUES ($1, $2, NULLIF($3, '')::uuid, $4, $5, $6, $7, $8, $9)
	`
	_, err = db.ExecContext(ctx, q,
		job.InterfaceID, job.MessageID, job.StepID, job.StepName, job.ConnectorType,
		job.Destination, job.Outcome, report.OverallCoveragePct, categoryStatsJSON,
	)
	if err != nil {
		return fmt.Errorf("cda_coverage: insert cda_coverage_audits: %w", err)
	}
	return nil
}
