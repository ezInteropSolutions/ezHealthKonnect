// Package rules provides built-in AssemblyRule implementations.
package rules

import (
	"ezhealthkonnect/services/cda_fhir/assembly"
	mappinglog "ezhealthkonnect/services/cda_fhir/mapping_log"
)

// DeduplicationRule eliminates FHIR resources that represent the same clinical
// entity. Duplicates are identified by matching HL7 CDAII keys (root + extension)
// embedded in the "_cdaIds" internal field by the individual mappers.
//
// When a duplicate is found:
//   - ctx.DedupRedirects["ResourceType/dup-id"] = "ResourceType/survivor-id"
//   - ctx.Removed["ResourceType/dup-id"] = true
//   - An AssemblyEvent is appended to ctx.Log
//
// The rule does NOT modify resources or rewrite references. Reference rewriting
// is done in a single pass by AssembleEntriesWithRedirects (bundle_assembler.go)
// which pre-seeds its shortToUUID lookup with the dedup redirect map.
type DeduplicationRule struct{}

// NewDeduplicationRule returns a ready-to-register DeduplicationRule.
func NewDeduplicationRule() *DeduplicationRule { return &DeduplicationRule{} }

// Name implements AssemblyRule.
func (r *DeduplicationRule) Name() string { return "deduplication" }

// Apply scans all resources for duplicates and populates ctx.DedupRedirects and
// ctx.Removed. Runs in O(n) over the resource list.
func (r *DeduplicationRule) Apply(ctx *assembly.AssemblyContext) error {
	reg := assembly.NewInMemoryResourceRegistry()

	for _, res := range ctx.Resources {
		rt, _ := res["resourceType"].(string)
		id, _ := res["id"].(string)
		if rt == "" || id == "" {
			continue
		}

		keys := assembly.ExtractIdentityKeys(res)
		if len(keys) == 0 {
			// No CDA identity embedded — resource cannot participate in dedup.
			continue
		}

		thisRef := rt + "/" + id

		if existing, matchKey := reg.FindDuplicate(rt, keys); existing != nil {
			// Duplicate detected.
			survivorRT, _ := existing["resourceType"].(string)
			survivorID, _ := existing["id"].(string)
			survivorRef := survivorRT + "/" + survivorID

			ctx.DedupRedirects[thisRef] = survivorRef
			ctx.Removed[thisRef] = true

			if ctx.Log != nil {
				ctx.Log.AddAssemblyEvent(mappinglog.AssemblyEvent{
					Rule:         r.Name(),
					Action:       "deduplicated",
					ResourceType: rt,
					IDs:          []string{id, survivorID},
					SurvivorID:   survivorID,
					Detail:       "matched on " + matchKey,
				})
			}
		} else {
			// First occurrence — register as survivor.
			reg.Register(rt, keys, res)
		}
	}

	return nil
}
