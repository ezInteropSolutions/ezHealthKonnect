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
// When a duplicate is found, whichever resource has MORE top-level FHIR fields
// populated (len(map) — the same "is this resource non-empty" proxy
// buildOneResource/buildEmittedSubResource already use elsewhere in this
// codebase) becomes the survivor, regardless of which one was registered first.
// Plain first-wins (this rule's original behavior) silently kept whichever
// resource happened to be built first even when a later one was strictly more
// complete -- found via Care Team: a sparse, nameless Practitioner built from a
// <participant>'s bare NPI registered before a richer one (name, specialty,
// address, phone) built from a sibling <component>'s <performer> for the SAME
// person, and the sparse one won purely by being processed first.
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

			if len(res) > len(existing) {
				// res is the more complete resource -- swap it in as the
				// survivor instead of discarding it. Re-point any earlier
				// redirect that already targeted the old survivor: this
				// rule's own DedupRedirects map is later consumed by
				// AssembleEntriesWithRedirects as a single-hop lookup (no
				// chain resolution -- see that function's own doc comment),
				// so leaving an old redirect pointing at survivorRef after
				// survivorRef itself becomes a removed duplicate would
				// produce an unresolved/broken reference.
				for oldRef, redirectTarget := range ctx.DedupRedirects {
					if redirectTarget == survivorRef {
						ctx.DedupRedirects[oldRef] = thisRef
					}
				}
				reg.Replace(rt, existing, res)
				ctx.DedupRedirects[survivorRef] = thisRef
				ctx.Removed[survivorRef] = true
				delete(ctx.Removed, thisRef)

				if ctx.Log != nil {
					ev := mappinglog.AssemblyEvent{
						Rule:         r.Name(),
						Action:       "deduplicated",
						ResourceType: rt,
						IDs:          []string{survivorID, id},
						SurvivorID:   id,
						Detail:       "matched on " + matchKey + " (later, more complete resource kept)",
					}
					if ctx.Config.DeepLineage {
						ev.Lineage = map[string]mappinglog.ResourceLineage{}
						if l, ok := assembly.ExtractLineage(res); ok {
							ev.Lineage[id] = l
						}
						if l, ok := assembly.ExtractLineage(existing); ok {
							ev.Lineage[survivorID] = l
						}
					}
					ctx.Log.AddAssemblyEvent(ev)
				}
				continue
			}

			ctx.DedupRedirects[thisRef] = survivorRef
			ctx.Removed[thisRef] = true

			if ctx.Log != nil {
				ev := mappinglog.AssemblyEvent{
					Rule:         r.Name(),
					Action:       "deduplicated",
					ResourceType: rt,
					IDs:          []string{id, survivorID},
					SurvivorID:   survivorID,
					Detail:       "matched on " + matchKey,
				}
				if ctx.Config.DeepLineage {
					ev.Lineage = map[string]mappinglog.ResourceLineage{}
					if l, ok := assembly.ExtractLineage(res); ok {
						ev.Lineage[id] = l
					}
					if l, ok := assembly.ExtractLineage(existing); ok {
						ev.Lineage[survivorID] = l
					}
				}
				ctx.Log.AddAssemblyEvent(ev)
			}
		} else {
			// First occurrence — register as survivor.
			reg.Register(rt, keys, res)
		}
	}

	return nil
}
