// ============================================================================
// NO-CODE VARIABLE ACCESS EXAMPLE
// ============================================================================
// Instead of:  input.enriched.field_mapping.riskWeights  (confusing nested structure)
// Use:         $vars["field_mapping.risk_weights"]       (flat, simple, IntelliSense-ready)
// ============================================================================

// OPTION 1: Bracket notation (works for all variable names)
var riskWeights = $vars["field_mapping.risk_weights"];

// OPTION 2: Dot notation (cleaner, but only works if step names have no special chars)
// var riskWeights = $vars.field_mapping.risk_weights;

// Validate data exists
if (!riskWeights || !riskWeights.weights) {
    return {
        error: "risk_weights not found",
        risk_score: 0,
        risk_level: "unknown"
    };
}

// Get patient data from HL7 message (using helper functions)
var dateOfBirth = getHL7Field("PID.7");
var patientAge = calculateAge(dateOfBirth);
var patientName = getHL7Field("PID.5.1") + " " + getHL7Field("PID.5.2");

// Calculate risk score
var riskScore = 0;
var factors = [];

// Age risk
if (patientAge > 65) {
    riskScore += riskWeights.weights.ageOver65 || 0;
    factors.push("Age over 65");
}

// NOTE: Database enrichment would be accessed like:
// var chronicConditions = $vars["database_enrichment_sqlserver.chronic_conditions"] || 0;
// var smokingStatus = $vars["database_enrichment_sqlserver.smoking_status"] || "";

// For now, using placeholder values since we don't have these in the database
var chronicConditions = 0;
var smokingStatus = "";

// Chronic conditions risk
if (chronicConditions > 0) {
    riskScore += chronicConditions * (riskWeights.weights.chronicCondition || 0);
    factors.push(chronicConditions + " chronic conditions");
}

// Smoking status risk
if (smokingStatus === "current") {
    riskScore += riskWeights.weights.currentSmoker || 0;
    factors.push("Current smoker");
}

// Determine risk level
var riskLevel = "low";
if (riskScore >= (riskWeights.thresholds.highRisk || 8)) {
    riskLevel = "high";
} else if (riskScore >= (riskWeights.thresholds.moderateRisk || 5)) {
    riskLevel = "moderate";
}

// Return calculated data (FLAT structure, snake_case keys - matches our standard)
return {
    risk_score: riskScore,
    risk_level: riskLevel,
    risk_factors: factors,
    patient_name: patientName,
    patient_age: patientAge,
    chronic_conditions: chronicConditions,
    smoking_status: smokingStatus,
    calculated_at: new Date().toISOString()
};
