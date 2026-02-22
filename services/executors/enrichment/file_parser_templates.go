package enrichment

import "ezhealthkonnect/models"

// ===============================================================
// FILE PARSER OOB TEMPLATES
// ===============================================================
// Pre-built column definitions for common healthcare and financial
// fixed-width file formats. Enables no-code parsing — users just
// pick a template and the columns are auto-populated.

// FileParserTemplate defines a reusable fixed-width format template
type FileParserTemplate struct {
	Name           string             `json:"name"`
	Description    string             `json:"description"`
	Category       string             `json:"category"`
	Format         string             `json:"format"` // always "fixed_width" for now
	Columns        []models.ColumnDef `json:"columns"`
	SkipRows       int                `json:"skipRows"`
	HasHeader      bool               `json:"hasHeader"`
	Confidence     string             `json:"confidence"`     // "high", "medium", "low"
	ConfidenceNote string             `json:"confidenceNote"` // shown in UI when template is selected
}

// TemplateInfo is a UI-facing summary returned by the templates API.
// Includes full column definitions so one API call populates both the
// dropdown and the column preview panel.
type TemplateInfo struct {
	Key            string             `json:"key"`
	Name           string             `json:"name"`
	Description    string             `json:"description"`
	Category       string             `json:"category"`
	ColumnCount    int                `json:"columnCount"`
	Confidence     string             `json:"confidence"`
	ConfidenceNote string             `json:"confidenceNote"`
	Columns        []models.ColumnDef `json:"columns"`
}

// templateRegistry holds all OOB templates indexed by key
var templateRegistry = map[string]FileParserTemplate{

	// ===============================================================
	// CMS CCLF (Claims and Claims Line Feed) Templates
	// Source: CMS CCLF Information Packet — positions cross-validated
	// against production processing tools (joewallace7/cclf_processing,
	// tuva-health/medicare_cclf_connector). CMS updates spec annually.
	// ===============================================================

	"cclf1": {
		Name:           "CCLF1 - Part A Claims Header",
		Description:    "CMS Claims and Claims Line Feed file type 1 — Part A institutional claims header (facility/inpatient)",
		Category:       "CMS/CCLF",
		Format:         "fixed_width",
		HasHeader:      false,
		SkipRows:       0,
		Confidence:     "high",
		ConfidenceNote: "26 fields verified against CMS CCLF Information Packet as used in production processing tools. Spec is updated annually by CMS — verify against your exact version.",
		Columns: []models.ColumnDef{
			{Name: "CUR_CLM_UNIQ_ID", Start: 1, Length: 13},
			{Name: "PRVDR_OSCAR_NUM", Start: 14, Length: 6},
			{Name: "BENE_MBI_ID", Start: 20, Length: 11},
			{Name: "BENE_HIC_NUM", Start: 31, Length: 11},
			{Name: "CLM_TYPE_CD", Start: 42, Length: 2},
			{Name: "CLM_FROM_DT", Start: 44, Length: 10},
			{Name: "CLM_THRU_DT", Start: 54, Length: 10},
			{Name: "CLM_BILL_FAC_TYPE_CD", Start: 64, Length: 1},
			{Name: "CLM_BILL_CLSFCTN_CD", Start: 65, Length: 1},
			{Name: "PRNCPL_DGNS_CD", Start: 66, Length: 7},
			{Name: "ADMTG_DGNS_CD", Start: 73, Length: 7},
			{Name: "CLM_MDCR_NPMT_RSN_CD", Start: 80, Length: 2},
			{Name: "CLM_PMT_AMT", Start: 82, Length: 17},
			{Name: "CLM_NCH_PRMRY_PYR_CD", Start: 99, Length: 1},
			{Name: "PRVDR_FAC_FIPS_ST_CD", Start: 100, Length: 2},
			{Name: "BENE_PTNT_STUS_CD", Start: 102, Length: 2},
			{Name: "DGNS_DRG_CD", Start: 104, Length: 4},
			{Name: "CLM_OP_SRVC_TYPE_CD", Start: 108, Length: 1},
			{Name: "FAC_PRVDR_NPI_NUM", Start: 109, Length: 10},
			{Name: "OPRTG_PRVDR_NPI_NUM", Start: 119, Length: 10},
			{Name: "ATNDG_PRVDR_NPI_NUM", Start: 129, Length: 10},
			{Name: "OTHR_PRVDR_NPI_NUM", Start: 139, Length: 10},
			{Name: "CLM_ADJSMT_TYPE_CD", Start: 149, Length: 2},
			{Name: "CLM_EFCTV_DT", Start: 151, Length: 10},
			{Name: "CLM_IDR_LD_DT", Start: 161, Length: 10},
			{Name: "BENE_EQTBL_BIC_HICN_NUM", Start: 171, Length: 11},
		},
	},

	"cclf2": {
		Name:           "CCLF2 - Part A Revenue Center",
		Description:    "CMS CCLF file type 2 — Part A institutional claims revenue center detail",
		Category:       "CMS/CCLF",
		Format:         "fixed_width",
		HasHeader:      false,
		SkipRows:       0,
		Confidence:     "high",
		ConfidenceNote: "22 fields verified against CMS CCLF Information Packet as used in production processing tools. Record length: 179 characters. Spec is updated annually by CMS — verify against your exact version.",
		Columns: []models.ColumnDef{
			{Name: "CUR_CLM_UNIQ_ID", Start: 1, Length: 13},
			{Name: "CLM_LINE_NUM", Start: 14, Length: 10},
			{Name: "BENE_MBI_ID", Start: 24, Length: 11},
			{Name: "BENE_HIC_NUM", Start: 35, Length: 11},
			{Name: "CLM_TYPE_CD", Start: 46, Length: 2},
			{Name: "CLM_LINE_FROM_DT", Start: 48, Length: 10},
			{Name: "CLM_LINE_THRU_DT", Start: 58, Length: 10},
			{Name: "CLM_LINE_PROD_REV_CTR_CD", Start: 68, Length: 4},
			{Name: "CLM_LINE_INSTNL_REV_CTR_DT", Start: 72, Length: 10},
			{Name: "CLM_LINE_HCPCS_CD", Start: 82, Length: 5},
			{Name: "BENE_EQTBL_BIC_HICN_NUM", Start: 87, Length: 11},
			{Name: "PRVDR_OSCAR_NUM", Start: 98, Length: 6},
			{Name: "CLM_FROM_DT", Start: 104, Length: 10},
			{Name: "CLM_THRU_DT", Start: 114, Length: 10},
			{Name: "CLM_LINE_SRVC_UNIT_QTY", Start: 124, Length: 24},
			{Name: "CLM_LINE_CVRD_PD_AMT", Start: 148, Length: 17},
			{Name: "HCPCS_1_MDFR_CD", Start: 165, Length: 2},
			{Name: "HCPCS_2_MDFR_CD", Start: 167, Length: 2},
			{Name: "HCPCS_3_MDFR_CD", Start: 169, Length: 2},
			{Name: "HCPCS_4_MDFR_CD", Start: 171, Length: 2},
			{Name: "HCPCS_5_MDFR_CD", Start: 173, Length: 2},
			{Name: "CLM_REV_APC_HIPPS_CD", Start: 175, Length: 5},
		},
	},

	"cclf5": {
		Name:           "CCLF5 - Part B Physician",
		Description:    "CMS CCLF file type 5 — Part B physician/supplier claims",
		Category:       "CMS/CCLF",
		Format:         "fixed_width",
		HasHeader:      false,
		SkipRows:       0,
		Confidence:     "high",
		ConfidenceNote: "49 fields verified against CMS CCLF Information Packet as used in production processing tools. Record length: 363 characters. Includes 12 diagnosis codes and 5 HCPCS modifiers. Spec is updated annually by CMS — verify against your exact version.",
		Columns: []models.ColumnDef{
			{Name: "CUR_CLM_UNIQ_ID", Start: 1, Length: 13},
			{Name: "CLM_LINE_NUM", Start: 14, Length: 10},
			{Name: "BENE_MBI_ID", Start: 24, Length: 11},
			{Name: "BENE_HIC_NUM", Start: 35, Length: 11},
			{Name: "CLM_TYPE_CD", Start: 46, Length: 2},
			{Name: "CLM_FROM_DT", Start: 48, Length: 10},
			{Name: "CLM_THRU_DT", Start: 58, Length: 10},
			{Name: "RNDRG_PRVDR_TYPE_CD", Start: 68, Length: 3},
			{Name: "RNDRG_PRVDR_FIPS_ST_CD", Start: 71, Length: 2},
			{Name: "CLM_PRVDR_SPCLTY_CD", Start: 73, Length: 2},
			{Name: "CLM_FED_TYPE_SRVC_CD", Start: 75, Length: 1},
			{Name: "CLM_POS_CD", Start: 76, Length: 2},
			{Name: "CLM_LINE_FROM_DT", Start: 78, Length: 10},
			{Name: "CLM_LINE_THRU_DT", Start: 88, Length: 10},
			{Name: "CLM_LINE_HCPCS_CD", Start: 98, Length: 5},
			{Name: "CLM_LINE_CVRD_PD_AMT", Start: 103, Length: 15},
			{Name: "CLM_LINE_PRMRY_PYR_CD", Start: 118, Length: 1},
			{Name: "CLM_LINE_DGNS_CD", Start: 119, Length: 7},
			{Name: "CLM_RNDRG_PRVDR_TAX_NUM", Start: 126, Length: 10},
			{Name: "RNDRG_PRVDR_NPI_NUM", Start: 136, Length: 10},
			{Name: "CLM_CARR_PMT_DNL_CD", Start: 146, Length: 2},
			{Name: "CLM_PRCSG_IND_CD", Start: 148, Length: 2},
			{Name: "CLM_ADJSMT_TYPE_CD", Start: 150, Length: 2},
			{Name: "CLM_EFCTV_DT", Start: 152, Length: 10},
			{Name: "CLM_IDR_LD_DT", Start: 162, Length: 10},
			{Name: "CLM_CNTL_NUM", Start: 172, Length: 40},
			{Name: "BENE_EQTBL_BIC_HICN_NUM", Start: 212, Length: 11},
			{Name: "CLM_LINE_ALOWD_CHRG_AMT", Start: 223, Length: 17},
			{Name: "CLM_LINE_SRVC_UNIT_QTY", Start: 240, Length: 24},
			{Name: "HCPCS_1_MDFR_CD", Start: 264, Length: 2},
			{Name: "HCPCS_2_MDFR_CD", Start: 266, Length: 2},
			{Name: "HCPCS_3_MDFR_CD", Start: 268, Length: 2},
			{Name: "HCPCS_4_MDFR_CD", Start: 270, Length: 2},
			{Name: "HCPCS_5_MDFR_CD", Start: 272, Length: 2},
			{Name: "CLM_DISP_CD", Start: 274, Length: 2},
			{Name: "CLM_DGNS_1_CD", Start: 276, Length: 7},
			{Name: "CLM_DGNS_2_CD", Start: 283, Length: 7},
			{Name: "CLM_DGNS_3_CD", Start: 290, Length: 7},
			{Name: "CLM_DGNS_4_CD", Start: 297, Length: 7},
			{Name: "CLM_DGNS_5_CD", Start: 304, Length: 7},
			{Name: "CLM_DGNS_6_CD", Start: 311, Length: 7},
			{Name: "CLM_DGNS_7_CD", Start: 318, Length: 7},
			{Name: "CLM_DGNS_8_CD", Start: 325, Length: 7},
			{Name: "DGNS_PRCDR_ICD_IND", Start: 332, Length: 1},
			{Name: "CLM_DGNS_9_CD", Start: 333, Length: 7},
			{Name: "CLM_DGNS_10_CD", Start: 340, Length: 7},
			{Name: "CLM_DGNS_11_CD", Start: 347, Length: 7},
			{Name: "CLM_DGNS_12_CD", Start: 354, Length: 7},
			{Name: "HCPCS_BETOS_CD", Start: 361, Length: 3},
		},
	},

	"cclf8": {
		Name:           "CCLF8 - Beneficiary Demographics",
		Description:    "CMS CCLF file type 8 — Medicare beneficiary demographic and enrollment data",
		Category:       "CMS/CCLF",
		Format:         "fixed_width",
		HasHeader:      false,
		SkipRows:       0,
		Confidence:     "high",
		ConfidenceNote: "31 fields verified against CMS CCLF Information Packet as used in production processing tools. Record length: 549 characters. Includes name, DOB, address (6 lines), geographic data, and enrollment dates. Spec is updated annually by CMS — verify against your exact version.",
		Columns: []models.ColumnDef{
			{Name: "BENE_MBI_ID", Start: 1, Length: 11},
			{Name: "BENE_HIC_NUM", Start: 12, Length: 11},
			{Name: "BENE_FIPS_STATE_CD", Start: 23, Length: 2},
			{Name: "BENE_FIPS_CNTY_CD", Start: 25, Length: 3},
			{Name: "BENE_ZIP_CD", Start: 28, Length: 5},
			{Name: "BENE_DOB", Start: 33, Length: 10},
			{Name: "BENE_SEX_CD", Start: 43, Length: 1},
			{Name: "BENE_RACE_CD", Start: 44, Length: 1},
			{Name: "BENE_AGE", Start: 45, Length: 3},
			{Name: "BENE_MDCR_STUS_CD", Start: 48, Length: 2},
			{Name: "BENE_DUAL_STUS_CD", Start: 50, Length: 2},
			{Name: "BENE_DEATH_DT", Start: 52, Length: 10},
			{Name: "BENE_RNG_BGN_DT", Start: 62, Length: 10},
			{Name: "BENE_RNG_END_DT", Start: 72, Length: 10},
			{Name: "BENE_1ST_NAME", Start: 82, Length: 30},
			{Name: "BENE_MIDL_NAME", Start: 112, Length: 15},
			{Name: "BENE_LAST_NAME", Start: 127, Length: 40},
			{Name: "BENE_ORGNL_ENTLMT_RSN_CD", Start: 167, Length: 1},
			{Name: "BENE_ENTLMT_BUYIN_IND", Start: 168, Length: 1},
			{Name: "BENE_PART_A_ENRLMT_BGN_DT", Start: 169, Length: 10},
			{Name: "BENE_PART_B_ENRLMT_BGN_DT", Start: 179, Length: 10},
			{Name: "BENE_LINE_1_ADR", Start: 189, Length: 45},
			{Name: "BENE_LINE_2_ADR", Start: 234, Length: 45},
			{Name: "BENE_LINE_3_ADR", Start: 279, Length: 40},
			{Name: "BENE_LINE_4_ADR", Start: 319, Length: 40},
			{Name: "BENE_LINE_5_ADR", Start: 359, Length: 40},
			{Name: "BENE_LINE_6_ADR", Start: 399, Length: 40},
			{Name: "GEO_ZIP_PLC_NAME", Start: 439, Length: 100},
			{Name: "GEO_USPS_STATE_CD", Start: 539, Length: 2},
			{Name: "GEO_ZIP5_CD", Start: 541, Length: 5},
			{Name: "GEO_ZIP4_CD", Start: 546, Length: 4},
		},
	},

	// ===============================================================
	// NACHA/ACH Payment File Templates
	// Source: NACHA Operating Rules (2024)
	// ===============================================================

	"nacha_file_header": {
		Name:           "NACHA File Header (Type 1)",
		Description:    "ACH file header record — identifies the originating institution",
		Category:       "NACHA/ACH",
		Format:         "fixed_width",
		HasHeader:      false,
		SkipRows:       0,
		Confidence:     "high",
		ConfidenceNote: "Follows NACHA Operating Rules. All records are exactly 94 characters. Field positions verified against published spec.",
		Columns: []models.ColumnDef{
			{Name: "RECORD_TYPE_CODE", Start: 1, Length: 1},
			{Name: "PRIORITY_CODE", Start: 2, Length: 2},
			{Name: "IMMEDIATE_DEST", Start: 4, Length: 10},
			{Name: "IMMEDIATE_ORIGIN", Start: 14, Length: 10},
			{Name: "FILE_CREATION_DATE", Start: 24, Length: 6},
			{Name: "FILE_CREATION_TIME", Start: 30, Length: 4},
			{Name: "FILE_ID_MODIFIER", Start: 34, Length: 1},
			{Name: "RECORD_SIZE", Start: 35, Length: 3},
			{Name: "BLOCKING_FACTOR", Start: 38, Length: 2},
			{Name: "FORMAT_CODE", Start: 40, Length: 1},
			{Name: "IMMEDIATE_DEST_NAME", Start: 41, Length: 23},
			{Name: "IMMEDIATE_ORIGIN_NAME", Start: 64, Length: 23},
			{Name: "REFERENCE_CODE", Start: 87, Length: 8},
		},
	},

	"nacha_batch_header": {
		Name:           "NACHA Batch Header (Type 5)",
		Description:    "ACH batch header record — groups related entries",
		Category:       "NACHA/ACH",
		Format:         "fixed_width",
		HasHeader:      false,
		SkipRows:       0,
		Confidence:     "high",
		ConfidenceNote: "Follows NACHA Operating Rules. All records are exactly 94 characters. Field positions verified against published spec.",
		Columns: []models.ColumnDef{
			{Name: "RECORD_TYPE_CODE", Start: 1, Length: 1},
			{Name: "SERVICE_CLASS_CODE", Start: 2, Length: 3},
			{Name: "COMPANY_NAME", Start: 5, Length: 16},
			{Name: "COMPANY_DISCRETIONARY_DATA", Start: 21, Length: 20},
			{Name: "COMPANY_ID", Start: 41, Length: 10},
			{Name: "STANDARD_ENTRY_CLASS", Start: 51, Length: 3},
			{Name: "COMPANY_ENTRY_DESCRIPTION", Start: 54, Length: 10},
			{Name: "COMPANY_DESCRIPTIVE_DATE", Start: 64, Length: 6},
			{Name: "EFFECTIVE_ENTRY_DATE", Start: 70, Length: 6},
			{Name: "SETTLEMENT_DATE", Start: 76, Length: 3},
			{Name: "ORIGINATOR_STATUS_CODE", Start: 79, Length: 1},
			{Name: "ORIGINATING_DFI_ID", Start: 80, Length: 8},
			{Name: "BATCH_NUMBER", Start: 88, Length: 7},
		},
	},

	"nacha_entry": {
		Name:           "NACHA Entry Detail (Type 6)",
		Description:    "ACH entry detail record — individual payment/debit transaction",
		Category:       "NACHA/ACH",
		Format:         "fixed_width",
		HasHeader:      false,
		SkipRows:       0,
		Confidence:     "high",
		ConfidenceNote: "Follows NACHA Operating Rules. All records are exactly 94 characters. Field positions verified against published spec.",
		Columns: []models.ColumnDef{
			{Name: "RECORD_TYPE_CODE", Start: 1, Length: 1},
			{Name: "TRANSACTION_CODE", Start: 2, Length: 2},
			{Name: "RECEIVING_DFI_ID", Start: 4, Length: 8},
			{Name: "CHECK_DIGIT", Start: 12, Length: 1},
			{Name: "DFI_ACCOUNT_NUMBER", Start: 13, Length: 17},
			{Name: "AMOUNT", Start: 30, Length: 10},
			{Name: "INDIVIDUAL_ID_NUMBER", Start: 40, Length: 15},
			{Name: "INDIVIDUAL_NAME", Start: 55, Length: 22},
			{Name: "DISCRETIONARY_DATA", Start: 77, Length: 2},
			{Name: "ADDENDA_RECORD_IND", Start: 79, Length: 1},
			{Name: "TRACE_NUMBER", Start: 80, Length: 15},
		},
	},

	// ===============================================================
	// ERA/Remittance Templates
	// ===============================================================

	"era_835_header": {
		Name:           "ERA 835 Payment Header",
		Description:    "Electronic Remittance Advice payment/check level — fixed-width variant used by some clearinghouses",
		Category:       "ERA/Remittance",
		Format:         "fixed_width",
		HasHeader:      false,
		SkipRows:       0,
		Confidence:     "low",
		ConfidenceNote: "ERA 835 is an X12 EDI format — not standard fixed-width. These columns represent a clearinghouse-specific flat-file export variant. Verify positions against your clearinghouse's proprietary layout spec.",
		Columns: []models.ColumnDef{
			{Name: "RECORD_TYPE", Start: 1, Length: 3},
			{Name: "PAYER_ID", Start: 4, Length: 10},
			{Name: "PAYER_NAME", Start: 14, Length: 35},
			{Name: "CHECK_NUMBER", Start: 49, Length: 20},
			{Name: "CHECK_DATE", Start: 69, Length: 10},
			{Name: "CHECK_AMOUNT", Start: 79, Length: 12},
			{Name: "PAYEE_NPI", Start: 91, Length: 10},
			{Name: "PAYEE_NAME", Start: 101, Length: 35},
			{Name: "PAYMENT_METHOD", Start: 136, Length: 3},
		},
	},

	"era_835_claim": {
		Name:           "ERA 835 Claim Detail",
		Description:    "Electronic Remittance Advice claim-level detail — fixed-width variant",
		Category:       "ERA/Remittance",
		Format:         "fixed_width",
		HasHeader:      false,
		SkipRows:       0,
		Confidence:     "low",
		ConfidenceNote: "ERA 835 is an X12 EDI format — not standard fixed-width. These columns represent a clearinghouse-specific flat-file export variant. Verify positions against your clearinghouse's proprietary layout spec.",
		Columns: []models.ColumnDef{
			{Name: "RECORD_TYPE", Start: 1, Length: 3},
			{Name: "PATIENT_CONTROL_NUM", Start: 4, Length: 20},
			{Name: "CLAIM_STATUS", Start: 24, Length: 2},
			{Name: "TOTAL_CHARGE_AMT", Start: 26, Length: 12},
			{Name: "PAYMENT_AMT", Start: 38, Length: 12},
			{Name: "PATIENT_RESP_AMT", Start: 50, Length: 12},
			{Name: "CLAIM_FILING_IND", Start: 62, Length: 2},
			{Name: "PAYER_CLM_CTRL_NUM", Start: 64, Length: 20},
			{Name: "FACILITY_TYPE_CD", Start: 84, Length: 2},
			{Name: "CLAIM_FREQ_CD", Start: 86, Length: 1},
			{Name: "DRG_CD", Start: 87, Length: 4},
			{Name: "CLAIM_DRG_AMT", Start: 91, Length: 12},
		},
	},

	// ===============================================================
	// Eligibility / Enrollment File Templates
	// ===============================================================

	"eligibility_834": {
		Name:           "834 Enrollment - Member Record",
		Description:    "Healthcare enrollment/eligibility member-level record — fixed-width format used by many payers",
		Category:       "Enrollment",
		Format:         "fixed_width",
		HasHeader:      false,
		SkipRows:       0,
		Confidence:     "low",
		ConfidenceNote: "X12 834 is an EDI format — not standard fixed-width. These columns represent a payer-specific proprietary layout. Verify positions against your payer's specification before using in production.",
		Columns: []models.ColumnDef{
			{Name: "RECORD_TYPE", Start: 1, Length: 2},
			{Name: "MEMBER_ID", Start: 3, Length: 20},
			{Name: "SSN", Start: 23, Length: 9},
			{Name: "LAST_NAME", Start: 32, Length: 35},
			{Name: "FIRST_NAME", Start: 67, Length: 25},
			{Name: "MIDDLE_NAME", Start: 92, Length: 15},
			{Name: "DOB", Start: 107, Length: 10},
			{Name: "GENDER", Start: 117, Length: 1},
			{Name: "ADDRESS_LINE1", Start: 118, Length: 55},
			{Name: "CITY", Start: 173, Length: 30},
			{Name: "STATE", Start: 203, Length: 2},
			{Name: "ZIP_CODE", Start: 205, Length: 10},
			{Name: "COVERAGE_START_DT", Start: 215, Length: 10},
			{Name: "COVERAGE_END_DT", Start: 225, Length: 10},
			{Name: "PLAN_CODE", Start: 235, Length: 10},
			{Name: "GROUP_NUMBER", Start: 245, Length: 15},
		},
	},
}

// GetTemplate retrieves a template by its key
func GetTemplate(name string) (*FileParserTemplate, bool) {
	tmpl, ok := templateRegistry[name]
	if !ok {
		return nil, false
	}
	return &tmpl, true
}

// GetTemplateList returns all templates with full column definitions (for UI)
func GetTemplateList() []TemplateInfo {
	list := make([]TemplateInfo, 0, len(templateRegistry))
	for key, tmpl := range templateRegistry {
		list = append(list, TemplateInfo{
			Key:            key,
			Name:           tmpl.Name,
			Description:    tmpl.Description,
			Category:       tmpl.Category,
			ColumnCount:    len(tmpl.Columns),
			Confidence:     tmpl.Confidence,
			ConfidenceNote: tmpl.ConfidenceNote,
			Columns:        tmpl.Columns,
		})
	}
	return list
}

// GetTemplatesByCategory returns templates grouped by category with full column definitions
func GetTemplatesByCategory() map[string][]TemplateInfo {
	result := make(map[string][]TemplateInfo)
	for key, tmpl := range templateRegistry {
		info := TemplateInfo{
			Key:            key,
			Name:           tmpl.Name,
			Description:    tmpl.Description,
			Category:       tmpl.Category,
			ColumnCount:    len(tmpl.Columns),
			Confidence:     tmpl.Confidence,
			ConfidenceNote: tmpl.ConfidenceNote,
			Columns:        tmpl.Columns,
		}
		result[tmpl.Category] = append(result[tmpl.Category], info)
	}
	return result
}
