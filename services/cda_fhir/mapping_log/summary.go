package mappinglog

// Summary is the compact view returned in HTTP responses (alongside the FHIR bundle).
// The full MappingLog (with Sections + Assembly detail) is written to MongoDB async.
type Summary struct {
	TotalResources    int   `json:"totalResources"`
	TotalTimeMs       int64 `json:"totalTimeMs"`
	MappingTimeMs     int64 `json:"mappingTimeMs"`
	AssemblyTimeMs    int64 `json:"assemblyTimeMs"`
	SerialTimeMs      int64 `json:"serialTimeMs"`
	DeduplicatedCount int   `json:"deduplicatedCount"`
	SynthesizedCount  int   `json:"synthesizedCount"`
}
