package assembly

import "ezhealthkonnect/services/cda_fhir/mapping_log"

// AssemblyRule is an operation applied to the full set of mapped FHIR resources
// after all section goroutines have completed. Rules run serially in registration
// order; each rule may modify AssemblyContext to pass information to later rules.
type AssemblyRule interface {
	// Name returns the stable rule identifier used in RuleConfig lookups and log events.
	Name() string
	// Apply executes the rule. Returning a non-nil error does NOT abort the pipeline —
	// the engine logs the error and continues with the next rule.
	Apply(ctx *AssemblyContext) error
}

// AssemblyContext is the shared state threaded through all assembly rules. It is
// not thread-safe — rules run serially in the post-goroutine serial phase.
type AssemblyContext struct {
	// Resources is the flat list of all FHIR resources produced by every section
	// mapper (including header Patient/Practitioner/Organization). Rules read from
	// this slice; they do not modify it directly — use Removed to mark removal.
	Resources []map[string]interface{}

	// DedupRedirects maps "ResourceType/dup-id" → "ResourceType/survivor-id".
	// Populated by DeduplicationRule; consumed by AssembleEntriesWithRedirects in
	// fhir/r4/bundle_assembler.go to rewrite references in one pass.
	DedupRedirects map[string]string

	// Removed marks resources that should be excluded from the final bundle.
	// Key: "ResourceType/id". Set by DeduplicationRule (and optionally others).
	Removed map[string]bool

	// Log accumulates assembly events for inclusion in the MappingLog.
	Log *mappinglog.LogBuilder

	// Config carries user-level overrides for the assembly layer.
	Config AssemblyConfig
}

// AssemblyConfig controls the assembly layer and its individual rules.
// Embedded in CDAToFHIRConfig. Zero value = everything enabled (correct Go idiom:
// Disabled:false means NOT disabled = enabled).
type AssemblyConfig struct {
	// Disabled skips the entire assembly layer when true.
	Disabled bool `json:"disabled,omitempty"`

	// Rules carries per-rule overrides keyed by AssemblyRule.Name().
	Rules map[string]RuleConfig `json:"rules,omitempty"`

	// DeepLineage instructs assembly rules to capture per-resource CDA provenance
	// (section + entry index + HL7 II identity) for every participant resource —
	// survivor and discarded/merged alike — into AssemblyEvent.Lineage. Identical
	// for every rule, so it is a direct flag rather than a per-rule RuleConfig
	// parameter. Set once per pipeline run from the interface's debug_logging/
	// log_level flag (see services.ResolveInterfaceDebugLevel). False by default —
	// zero extra work/storage when off.
	DeepLineage bool `json:"deepLineage,omitempty"`
}

// RuleConfig is the per-rule user override.
type RuleConfig struct {
	Disabled   bool                   `json:"disabled,omitempty"`
	Parameters map[string]interface{} `json:"parameters,omitempty"`
}

// IsRuleDisabled returns true when the named rule has been explicitly disabled
// in AssemblyConfig.Rules.
func (c *AssemblyConfig) IsRuleDisabled(ruleName string) bool {
	if rc, ok := c.Rules[ruleName]; ok {
		return rc.Disabled
	}
	return false
}

// RuleParameters returns the Parameters map for the named rule, or nil.
func (c *AssemblyConfig) RuleParameters(ruleName string) map[string]interface{} {
	if rc, ok := c.Rules[ruleName]; ok {
		return rc.Parameters
	}
	return nil
}
