package enrichment

import (
	"bytes"
	"context"
	"ezhealthkonnect/models"
	"fmt"

	"github.com/extrame/xls"
	"github.com/xuri/excelize/v2"
)

// ===============================================================
// XLSX FORMAT PARSER
// ===============================================================

// XlsxFormatParser handles modern Excel (.xlsx) files using the excelize library.
type XlsxFormatParser struct{}

func init() { RegisterFormatParser(&XlsxFormatParser{}) }

func (p *XlsxFormatParser) Format() string      { return "xlsx" }
func (p *XlsxFormatParser) Extensions() []string { return []string{".xlsx"} }

// MagicBytes: ZIP signature PK\x03\x04 (xlsx is a ZIP container)
func (p *XlsxFormatParser) MagicBytes() [][]byte {
	return [][]byte{{0x50, 0x4B, 0x03, 0x04}}
}

func (p *XlsxFormatParser) Parse(
	ctx context.Context,
	data []byte,
	config *models.FileParserConfig,
) ([]map[string]interface{}, []string, error) {
	f, err := excelize.OpenReader(bytes.NewReader(data))
	if err != nil {
		return nil, nil, fmt.Errorf("xlsx parse error: %w", err)
	}
	defer f.Close()

	// Determine which sheet to parse
	sheetName := config.SheetName
	if sheetName == "" {
		sheetList := f.GetSheetList()
		if len(sheetList) == 0 {
			return nil, nil, fmt.Errorf("xlsx file contains no sheets")
		}
		if config.SheetIndex > 0 && config.SheetIndex < len(sheetList) {
			sheetName = sheetList[config.SheetIndex]
		} else {
			sheetName = sheetList[0]
		}
	}

	allRows, err := f.GetRows(sheetName)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read xlsx sheet '%s': %w", sheetName, err)
	}

	// Apply MaxRecords cap before building record maps
	if config.MaxRecords > 0 {
		dataStart := config.SkipRows
		if config.HasHeader {
			dataStart++
		}
		maxEnd := dataStart + config.MaxRecords
		if maxEnd < len(allRows) {
			allRows = allRows[:maxEnd]
		}
	}

	return buildRecords(allRows, config)
}

// ===============================================================
// XLS FORMAT PARSER
// ===============================================================

// XlsFormatParser handles legacy Excel (.xls) files using the extrame/xls library.
type XlsFormatParser struct{}

func init() { RegisterFormatParser(&XlsFormatParser{}) }

func (p *XlsFormatParser) Format() string      { return "xls" }
func (p *XlsFormatParser) Extensions() []string { return []string{".xls"} }

// MagicBytes: OLE2 Compound Binary format (D0 CF 11 E0 A1 B1 1A E1)
func (p *XlsFormatParser) MagicBytes() [][]byte {
	return [][]byte{{0xD0, 0xCF, 0x11, 0xE0}}
}

func (p *XlsFormatParser) Parse(
	ctx context.Context,
	data []byte,
	config *models.FileParserConfig,
) ([]map[string]interface{}, []string, error) {
	wb, err := xls.OpenReader(bytes.NewReader(data), "utf-8")
	if err != nil {
		return nil, nil, fmt.Errorf("xls parse error: %w", err)
	}

	numSheets := wb.NumSheets()
	if numSheets == 0 {
		return nil, nil, fmt.Errorf("xls file contains no sheets")
	}

	// Find the target sheet
	var sheet *xls.WorkSheet
	if config.SheetName != "" {
		for i := 0; i < numSheets; i++ {
			s := wb.GetSheet(i)
			if s != nil && s.Name == config.SheetName {
				sheet = s
				break
			}
		}
		if sheet == nil {
			return nil, nil, fmt.Errorf("sheet '%s' not found in xls file", config.SheetName)
		}
	} else if config.SheetIndex > 0 && config.SheetIndex < numSheets {
		sheet = wb.GetSheet(config.SheetIndex)
	} else {
		sheet = wb.GetSheet(0)
	}

	if sheet == nil {
		return nil, nil, fmt.Errorf("failed to read sheet from xls file")
	}

	// Read all rows into a 2D string slice (same shape as excelize output)
	maxRow := int(sheet.MaxRow)
	var allRows [][]string

	for i := 0; i <= maxRow; i++ {
		row := sheet.Row(i)
		if row == nil {
			continue
		}
		lastCol := row.LastCol()
		rowData := make([]string, lastCol)
		for j := 0; j < lastCol; j++ {
			rowData[j] = row.Col(j)
		}
		allRows = append(allRows, rowData)

		// Early exit when MaxRecords reached (account for skip/header rows)
		if config.MaxRecords > 0 {
			dataStart := config.SkipRows
			if config.HasHeader {
				dataStart++
			}
			if len(allRows) >= dataStart+config.MaxRecords {
				break
			}
		}
	}

	return buildRecords(allRows, config)
}
