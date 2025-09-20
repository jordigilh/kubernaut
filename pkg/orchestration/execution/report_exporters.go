package execution

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/jordigilh/kubernaut/pkg/intelligence/patterns"
	adaptive "github.com/jordigilh/kubernaut/pkg/orchestration/adaptive"
	"github.com/sirupsen/logrus"
)

// ReportExporter provides export capabilities for various report formats
type ReportExporter struct {
	log *logrus.Logger
}

// ExportFormat defines supported export formats
type ExportFormat string

const (
	ExportFormatCSV  ExportFormat = "csv"
	ExportFormatHTML ExportFormat = "html"
	ExportFormatJSON ExportFormat = "json"
	ExportFormatPDF  ExportFormat = "pdf"
)

// ExportOptions configures the export behavior
type ExportOptions struct {
	Format        ExportFormat           `json:"format"`
	IncludeCharts bool                   `json:"include_charts"`
	IncludeRaw    bool                   `json:"include_raw_data"`
	Filters       map[string]string      `json:"filters"`
	CustomFields  []string               `json:"custom_fields"`
	Template      string                 `json:"template,omitempty"`
	Metadata      map[string]interface{} `json:"metadata"`
}

// ExportResult contains the result of an export operation
type ExportResult struct {
	Format     ExportFormat `json:"format"`
	Data       []byte       `json:"data"`
	Filename   string       `json:"filename"`
	MIMEType   string       `json:"mime_type"`
	Size       int64        `json:"size"`
	ExportedAt time.Time    `json:"exported_at"`
}

// CSVExportable defines the interface for CSV export
type CSVExportable interface {
	ToCSVHeaders() []string
	ToCSVRow() []string
}

// PatternDiscoveryCSVRow represents a pattern discovery result as CSV row
type PatternDiscoveryCSVRow struct {
	PatternID        string  `json:"pattern_id"`
	PatternName      string  `json:"pattern_name"`
	PatternType      string  `json:"pattern_type"`
	Confidence       float64 `json:"confidence"`
	Frequency        int     `json:"frequency"`
	SuccessRate      float64 `json:"success_rate"`
	DiscoveredAt     string  `json:"discovered_at"`
	LastUsed         string  `json:"last_used"`
	AverageExecution string  `json:"average_execution_time"`
	ResourceSavings  float64 `json:"resource_savings"`
	ReliabilityScore float64 `json:"reliability_score"`
}

// QualityMetricsCSVRow represents quality metrics as CSV row
type QualityMetricsCSVRow struct {
	MetricName  string  `json:"metric_name"`
	Value       float64 `json:"value"`
	Threshold   float64 `json:"threshold"`
	Status      string  `json:"status"`
	LastUpdated string  `json:"last_updated"`
	Category    string  `json:"category"`
}

// NewReportExporter creates a new report exporter
func NewReportExporter(log *logrus.Logger) *ReportExporter {
	return &ReportExporter{
		log: log,
	}
}

// ExportPatternAnalysisReport exports a pattern analysis report in the specified format
func (re *ReportExporter) ExportPatternAnalysisReport(report *patterns.PatternAnalysisResult, options *ExportOptions) (*ExportResult, error) {
	switch options.Format {
	case ExportFormatCSV:
		return re.exportPatternAnalysisToCSV(report, options)
	case ExportFormatHTML:
		return re.exportPatternAnalysisToHTML(report, options)
	case ExportFormatJSON:
		return re.exportPatternAnalysisToJSON(report, options)
	default:
		return nil, fmt.Errorf("unsupported export format: %s", options.Format)
	}
}

// ExportMaintainabilityReport exports a maintainability report
func (re *ReportExporter) ExportMaintainabilityReport(report *adaptive.MaintainabilityReport, options *ExportOptions) (*ExportResult, error) {
	switch options.Format {
	case ExportFormatCSV:
		return re.exportMaintainabilityToCSV(report, options)
	case ExportFormatHTML:
		return re.exportMaintainabilityToHTML(report, options)
	case ExportFormatJSON:
		return re.exportMaintainabilityToJSON(report, options)
	default:
		return nil, fmt.Errorf("unsupported export format: %s", options.Format)
	}
}

// ExportConfigurationReport exports a configuration report
func (re *ReportExporter) ExportConfigurationReport(report *adaptive.ConfigurationReport, options *ExportOptions) (*ExportResult, error) {
	switch options.Format {
	case ExportFormatCSV:
		return re.exportConfigurationToCSV(report, options)
	case ExportFormatHTML:
		return re.exportConfigurationToHTML(report, options)
	case ExportFormatJSON:
		return re.exportConfigurationToJSON(report, options)
	default:
		return nil, fmt.Errorf("unsupported export format: %s", options.Format)
	}
}

// CSV Export Methods

func (re *ReportExporter) exportPatternAnalysisToCSV(report *patterns.PatternAnalysisResult, options *ExportOptions) (*ExportResult, error) {
	var buf bytes.Buffer
	writer := csv.NewWriter(&buf)
	defer writer.Flush()

	// Write header
	headers := []string{
		"Pattern ID", "Pattern Name", "Pattern Type", "Confidence", "Frequency",
		"Success Rate", "Discovered At", "Last Used", "Average Execution Time",
		"Resource Savings", "Reliability Score",
	}
	if err := writer.Write(headers); err != nil {
		return nil, fmt.Errorf("failed to write CSV headers: %w", err)
	}

	// Write data rows
	for _, pattern := range report.Patterns {
		row := []string{
			pattern.ID,
			pattern.Description, // Using description as name
			string(pattern.Type),
			fmt.Sprintf("%.3f", pattern.Confidence),
			"N/A",                                   // Frequency not available
			"N/A",                                   // Support not available
			"N/A",                                   // DiscoveredAt not available
			"N/A",                                   // LastSeen not available
			"N/A",                                   // AverageExecutionTime not available
			fmt.Sprintf("%.2f", 0.0),                // Placeholder for resource savings
			fmt.Sprintf("%.3f", pattern.Confidence), // Using confidence as reliability proxy
		}
		if err := writer.Write(row); err != nil {
			return nil, fmt.Errorf("failed to write CSV row: %w", err)
		}
	}

	result := &ExportResult{
		Format:     ExportFormatCSV,
		Data:       buf.Bytes(),
		Filename:   fmt.Sprintf("pattern_analysis_%s.csv", time.Now().Format("20060102_150405")),
		MIMEType:   "text/csv",
		Size:       int64(buf.Len()),
		ExportedAt: time.Now(),
	}

	re.log.WithFields(logrus.Fields{
		"format":   options.Format,
		"patterns": len(report.Patterns),
		"size":     result.Size,
	}).Info("Pattern analysis report exported to CSV")

	return result, nil
}

func (re *ReportExporter) exportMaintainabilityToCSV(report *adaptive.MaintainabilityReport, options *ExportOptions) (*ExportResult, error) {
	var buf bytes.Buffer
	writer := csv.NewWriter(&buf)
	defer writer.Flush()

	// Write summary header with custom fields from options
	summaryHeaders := []string{
		"Report Type", "Package Path", "Overall Score", "Total Files", "Total Lines",
		"Total Functions", "Generated At",
	}

	// Add custom fields from options if specified
	if options != nil && len(options.CustomFields) > 0 {
		summaryHeaders = append(summaryHeaders, options.CustomFields...)
	}
	if err := writer.Write(summaryHeaders); err != nil {
		return nil, fmt.Errorf("failed to write summary headers: %w", err)
	}

	// Write summary row
	summaryRow := []string{
		"Maintainability Summary",
		report.PackagePath,
		fmt.Sprintf("%.1f", report.OverallScore),
		strconv.Itoa(report.TotalFiles),
		strconv.Itoa(report.TotalLines),
		strconv.Itoa(report.TotalFunctions),
		report.GeneratedAt.Format(time.RFC3339),
	}
	if err := writer.Write(summaryRow); err != nil {
		return nil, fmt.Errorf("failed to write summary row: %w", err)
	}

	// Add empty row separator
	_ = writer.Write([]string{})

	// Write file analysis header
	fileHeaders := []string{
		"File Path", "Line Count", "Function Count", "Complexity Score",
		"Maintainability Index", "Issues Count", "Test Coverage",
	}
	if err := writer.Write(fileHeaders); err != nil {
		return nil, fmt.Errorf("failed to write file headers: %w", err)
	}

	// Write file analysis rows with optional filtering
	for _, file := range report.FileAnalysis {
		// Apply filters from options if specified
		if options != nil && options.Filters != nil {
			if minComplexity, exists := options.Filters["min_complexity"]; exists {
				if threshold, err := strconv.ParseFloat(minComplexity, 64); err == nil {
					if file.ComplexityScore < threshold {
						continue // Skip files below complexity threshold
					}
				}
			}
			if filePattern, exists := options.Filters["file_pattern"]; exists {
				if matched, _ := filepath.Match(filePattern, file.FilePath); !matched {
					continue // Skip files not matching pattern
				}
			}
		}

		row := []string{
			file.FilePath,
			strconv.Itoa(file.LineCount),
			strconv.Itoa(file.FunctionCount),
			fmt.Sprintf("%.2f", file.ComplexityScore),
			fmt.Sprintf("%.2f", file.MaintainabilityIndex),
			strconv.Itoa(len(file.Issues)),
			fmt.Sprintf("%.2f", file.TestCoverage),
		}
		if err := writer.Write(row); err != nil {
			return nil, fmt.Errorf("failed to write file row: %w", err)
		}
	}

	result := &ExportResult{
		Format:     ExportFormatCSV,
		Data:       buf.Bytes(),
		Filename:   fmt.Sprintf("maintainability_report_%s.csv", time.Now().Format("20060102_150405")),
		MIMEType:   "text/csv",
		Size:       int64(buf.Len()),
		ExportedAt: time.Now(),
	}

	return result, nil
}

func (re *ReportExporter) exportConfigurationToCSV(report *adaptive.ConfigurationReport, options *ExportOptions) (*ExportResult, error) {
	var buf bytes.Buffer
	writer := csv.NewWriter(&buf)
	defer writer.Flush()

	// Write configuration validation results
	headers := []string{
		"Field", "Value", "Rule", "Message", "Severity", "Context",
	}
	if err := writer.Write(headers); err != nil {
		return nil, fmt.Errorf("failed to write headers: %w", err)
	}

	// Write errors and warnings
	for _, err := range append(report.Errors, report.Warnings...) {
		contextStr := ""
		if len(err.Context) > 0 {
			parts := make([]string, 0)
			for k, v := range err.Context {
				parts = append(parts, fmt.Sprintf("%s=%s", k, v))
			}
			contextStr = strings.Join(parts, ";")
		}

		row := []string{
			err.Field,
			fmt.Sprintf("%v", err.Value),
			err.Rule,
			err.Message,
			string(err.Severity),
			contextStr,
		}
		if err := writer.Write(row); err != nil {
			return nil, fmt.Errorf("failed to write configuration row: %w", err)
		}
	}

	result := &ExportResult{
		Format:     ExportFormatCSV,
		Data:       buf.Bytes(),
		Filename:   fmt.Sprintf("configuration_report_%s.csv", time.Now().Format("20060102_150405")),
		MIMEType:   "text/csv",
		Size:       int64(buf.Len()),
		ExportedAt: time.Now(),
	}

	return result, nil
}

// HTML Export Methods

func (re *ReportExporter) exportPatternAnalysisToHTML(report *patterns.PatternAnalysisResult, options *ExportOptions) (*ExportResult, error) {
	tmpl := `
<!DOCTYPE html>
<html>
<head>
    <title>Pattern Analysis Report</title>
    <style>
        body { font-family: Arial, sans-serif; margin: 40px; background-color: #f5f5f5; }
        .container { max-width: 1200px; margin: 0 auto; background: white; padding: 30px; border-radius: 8px; box-shadow: 0 2px 10px rgba(0,0,0,0.1); }
        .header { text-align: center; margin-bottom: 40px; }
        .header h1 { color: #2c3e50; margin-bottom: 10px; }
        .metadata { background: #ecf0f1; padding: 20px; border-radius: 5px; margin-bottom: 30px; }
        .metadata-item { display: inline-block; margin-right: 30px; margin-bottom: 10px; }
        .metadata-label { font-weight: bold; color: #34495e; }
        .patterns-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(400px, 1fr)); gap: 20px; }
        .pattern-card { border: 1px solid #ddd; border-radius: 8px; padding: 20px; background: white; transition: transform 0.2s; }
        .pattern-card:hover { transform: translateY(-2px); box-shadow: 0 4px 15px rgba(0,0,0,0.1); }
        .pattern-header { border-bottom: 2px solid #3498db; padding-bottom: 10px; margin-bottom: 15px; }
        .pattern-title { margin: 0; color: #2c3e50; font-size: 1.2em; }
        .pattern-type { color: #7f8c8d; font-size: 0.9em; margin: 5px 0 0 0; }
        .pattern-metrics { display: grid; grid-template-columns: 1fr 1fr; gap: 10px; margin-top: 15px; }
        .metric { text-align: center; padding: 10px; background: #f8f9fa; border-radius: 4px; }
        .metric-value { font-size: 1.4em; font-weight: bold; color: #2980b9; }
        .metric-label { font-size: 0.85em; color: #7f8c8d; text-transform: uppercase; }
        .confidence-high { color: #27ae60; }
        .confidence-medium { color: #f39c12; }
        .confidence-low { color: #e74c3c; }
        .summary { background: linear-gradient(135deg, #667eea 0%, #764ba2 100%); color: white; padding: 25px; border-radius: 8px; margin-bottom: 30px; text-align: center; }
        .summary h2 { margin: 0 0 15px 0; }
        .summary-stats { display: grid; grid-template-columns: repeat(auto-fit, minmax(150px, 1fr)); gap: 20px; margin-top: 20px; }
        .summary-stat { text-align: center; }
        .summary-stat-value { font-size: 2em; font-weight: bold; margin-bottom: 5px; }
        .summary-stat-label { font-size: 0.9em; opacity: 0.9; }
    </style>
</head>
<body>
    <div class="container">
        <div class="header">
            <h1>🔍 Pattern Analysis Report</h1>
            <p>Comprehensive analysis of discovered workflow patterns</p>
        </div>

        <div class="summary">
            <h2>Analysis Summary</h2>
            <div class="summary-stats">
                <div class="summary-stat">
                    <div class="summary-stat-value">{{.TotalPatterns}}</div>
                    <div class="summary-stat-label">Patterns Discovered</div>
                </div>
                <div class="summary-stat">
                    <div class="summary-stat-value">{{printf "%.1f%%" .AverageConfidence}}</div>
                    <div class="summary-stat-label">Average Confidence</div>
                </div>
                <div class="summary-stat">
                    <div class="summary-stat-value">{{.AnalysisTime}}</div>
                    <div class="summary-stat-label">Analysis Time</div>
                </div>
            </div>
        </div>

        <div class="metadata">
            <div class="metadata-item">
                <span class="metadata-label">Request ID:</span> {{.RequestID}}
            </div>
            <div class="metadata-item">
                <span class="metadata-label">Analysis Type:</span> Pattern Discovery
            </div>
            <div class="metadata-item">
                <span class="metadata-label">Generated:</span> {{.GeneratedAt}}
            </div>
            <div class="metadata-item">
                <span class="metadata-label">Data Points:</span> {{.DataPointsAnalyzed}}
            </div>
        </div>

        <h2 style="color: #2c3e50; border-bottom: 3px solid #3498db; padding-bottom: 10px;">Discovered Patterns</h2>

        <div class="patterns-grid">
        {{range .Patterns}}
            <div class="pattern-card">
                <div class="pattern-header">
                    <h3 class="pattern-title">{{.Name}}</h3>
                    <p class="pattern-type">{{.Type}} Pattern</p>
                </div>

                <div class="pattern-metrics">
                    <div class="metric">
                        <div class="metric-value {{if ge .Confidence 0.8}}confidence-high{{else if ge .Confidence 0.6}}confidence-medium{{else}}confidence-low{{end}}">
                            {{printf "%.1f%%" (.Confidence | mul 100)}}
                        </div>
                        <div class="metric-label">Confidence</div>
                    </div>
                    <div class="metric">
                        <div class="metric-value">{{printf "%.1f%%" (.SuccessRate | mul 100)}}</div>
                        <div class="metric-label">Success Rate</div>
                    </div>
                    <div class="metric">
                        <div class="metric-value">{{.Frequency}}</div>
                        <div class="metric-label">Frequency</div>
                    </div>
                    <div class="metric">
                        <div class="metric-value">{{.AverageExecutionTime}}</div>
                        <div class="metric-label">Avg Time</div>
                    </div>
                </div>

                {{if .Description}}
                <div style="margin-top: 15px; padding: 10px; background: #f8f9fa; border-left: 4px solid #3498db; font-size: 0.9em; color: #555;">
                    {{.Description}}
                </div>
                {{end}}
            </div>
        {{end}}
        </div>

        <div style="text-align: center; margin-top: 40px; padding-top: 20px; border-top: 1px solid #ddd; color: #7f8c8d; font-size: 0.9em;">
            <p>Generated by Kubernaut Pattern Discovery Engine • {{.GeneratedAt}}</p>
        </div>
    </div>
</body>
</html>`

	// Prepare template data
	data := struct {
		*patterns.PatternAnalysisResult
		TotalPatterns      int
		AverageConfidence  float64
		GeneratedAt        string
		DataPointsAnalyzed int
	}{
		PatternAnalysisResult: report,
		TotalPatterns:         len(report.Patterns),
		GeneratedAt:           time.Now().Format("January 2, 2006 at 3:04 PM"),
		DataPointsAnalyzed:    100, // Placeholder
	}

	if len(report.Patterns) > 0 {
		total := 0.0
		for _, pattern := range report.Patterns {
			total += pattern.Confidence
		}
		data.AverageConfidence = (total / float64(len(report.Patterns))) * 100
	}

	// Add multiplication function to template
	t := template.Must(template.New("report").Funcs(template.FuncMap{
		"mul": func(a, b float64) float64 { return a * b },
	}).Parse(tmpl))

	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return nil, fmt.Errorf("failed to execute HTML template: %w", err)
	}

	result := &ExportResult{
		Format:     ExportFormatHTML,
		Data:       buf.Bytes(),
		Filename:   fmt.Sprintf("pattern_analysis_%s.html", time.Now().Format("20060102_150405")),
		MIMEType:   "text/html",
		Size:       int64(buf.Len()),
		ExportedAt: time.Now(),
	}

	return result, nil
}

func (re *ReportExporter) exportMaintainabilityToHTML(report *adaptive.MaintainabilityReport, options *ExportOptions) (*ExportResult, error) {
	// Simplified HTML export for maintainability report
	html := fmt.Sprintf(`
<!DOCTYPE html>
<html>
<head>
    <title>Maintainability Report</title>
    <style>
        body { font-family: Arial, sans-serif; margin: 40px; }
        .header { background: #f5f5f5; padding: 20px; border-radius: 5px; }
        .score { font-size: 2em; color: %s; }
        .metrics { display: grid; grid-template-columns: repeat(auto-fit, minmax(300px, 1fr)); gap: 20px; margin: 20px 0; }
        .metric-card { background: #fff; border: 1px solid #ddd; padding: 15px; border-radius: 5px; }
    </style>
</head>
<body>
    <div class="header">
        <h1>Maintainability Report</h1>
        <p>Package: %s</p>
        <p>Generated: %s</p>
        <p class="score">Overall Score: %.1f/100</p>
    </div>

    <div class="metrics">
        <div class="metric-card">
            <h3>Code Metrics</h3>
            <p>Total Files: %d</p>
            <p>Total Lines: %d</p>
            <p>Total Functions: %d</p>
        </div>
    </div>
</body>
</html>`,
		re.getScoreColor(report.OverallScore),
		report.PackagePath,
		report.GeneratedAt.Format(time.RFC3339),
		report.OverallScore,
		report.TotalFiles,
		report.TotalLines,
		report.TotalFunctions)

	result := &ExportResult{
		Format:     ExportFormatHTML,
		Data:       []byte(html),
		Filename:   fmt.Sprintf("maintainability_report_%s.html", time.Now().Format("20060102_150405")),
		MIMEType:   "text/html",
		Size:       int64(len(html)),
		ExportedAt: time.Now(),
	}

	return result, nil
}

func (re *ReportExporter) exportConfigurationToHTML(report *adaptive.ConfigurationReport, options *ExportOptions) (*ExportResult, error) {
	// Simplified HTML export for configuration report
	statusColor := "#28a745"
	if !report.ValidationPassed {
		statusColor = "#dc3545"
	}

	html := fmt.Sprintf(`
<!DOCTYPE html>
<html>
<head>
    <title>Configuration Report</title>
    <style>
        body { font-family: Arial, sans-serif; margin: 40px; }
        .header { background: #f5f5f5; padding: 20px; border-radius: 5px; }
        .status { font-size: 1.5em; color: %s; }
        .issues { margin-top: 20px; }
        .issue { background: #fff3cd; padding: 10px; margin: 10px 0; border-radius: 3px; }
    </style>
</head>
<body>
    <div class="header">
        <h1>Configuration Report</h1>
        <p>Profile: %s</p>
        <p class="status">Status: %s</p>
    </div>

    <div class="issues">
        <h2>Issues (%d errors, %d warnings)</h2>
    </div>
</body>
</html>`,
		statusColor,
		report.Profile,
		map[bool]string{true: "Valid", false: "Invalid"}[report.ValidationPassed],
		len(report.Errors),
		len(report.Warnings))

	result := &ExportResult{
		Format:     ExportFormatHTML,
		Data:       []byte(html),
		Filename:   fmt.Sprintf("configuration_report_%s.html", time.Now().Format("20060102_150405")),
		MIMEType:   "text/html",
		Size:       int64(len(html)),
		ExportedAt: time.Now(),
	}

	return result, nil
}

// JSON Export Methods (simplified - would use actual JSON marshaling)

func (re *ReportExporter) exportPatternAnalysisToJSON(report *patterns.PatternAnalysisResult, options *ExportOptions) (*ExportResult, error) {
	// For JSON, we can directly marshal the report
	// This is a placeholder implementation
	data := fmt.Sprintf(`{"patterns_count": %d, "request_id": "%s"}`, len(report.Patterns), report.RequestID)

	result := &ExportResult{
		Format:     ExportFormatJSON,
		Data:       []byte(data),
		Filename:   fmt.Sprintf("pattern_analysis_%s.json", time.Now().Format("20060102_150405")),
		MIMEType:   "application/json",
		Size:       int64(len(data)),
		ExportedAt: time.Now(),
	}

	return result, nil
}

func (re *ReportExporter) exportMaintainabilityToJSON(report *adaptive.MaintainabilityReport, options *ExportOptions) (*ExportResult, error) {
	data := fmt.Sprintf(`{"overall_score": %.1f, "total_files": %d}`, report.OverallScore, report.TotalFiles)

	result := &ExportResult{
		Format:     ExportFormatJSON,
		Data:       []byte(data),
		Filename:   fmt.Sprintf("maintainability_report_%s.json", time.Now().Format("20060102_150405")),
		MIMEType:   "application/json",
		Size:       int64(len(data)),
		ExportedAt: time.Now(),
	}

	return result, nil
}

func (re *ReportExporter) exportConfigurationToJSON(report *adaptive.ConfigurationReport, options *ExportOptions) (*ExportResult, error) {
	data := fmt.Sprintf(`{"validation_passed": %t, "errors": %d}`, report.ValidationPassed, len(report.Errors))

	result := &ExportResult{
		Format:     ExportFormatJSON,
		Data:       []byte(data),
		Filename:   fmt.Sprintf("configuration_report_%s.json", time.Now().Format("20060102_150405")),
		MIMEType:   "application/json",
		Size:       int64(len(data)),
		ExportedAt: time.Now(),
	}

	return result, nil
}

// WriteToFile writes export result to a file
func (re *ReportExporter) WriteToFile(result *ExportResult, outputPath string) error {
	// Validate inputs first to prevent nil pointer dereference
	if result == nil {
		return fmt.Errorf("export result cannot be nil")
	}

	re.log.WithFields(logrus.Fields{
		"output_path": outputPath,
		"format":      result.Format,
		"size_bytes":  len(result.Data),
	}).Debug("Writing export result to file")

	if outputPath == "" {
		return fmt.Errorf("output path cannot be empty")
	}

	if len(result.Data) == 0 {
		return fmt.Errorf("export result data is empty")
	}

	// Create directory structure if it doesn't exist
	if err := re.ensureDirectoryExists(outputPath); err != nil {
		return fmt.Errorf("failed to create directory structure: %w", err)
	}

	// Write data to file with appropriate permissions
	if err := os.WriteFile(outputPath, result.Data, 0644); err != nil {
		return fmt.Errorf("failed to write file %s: %w", outputPath, err)
	}

	// Log successful write
	re.log.WithFields(logrus.Fields{
		"output_path": outputPath,
		"format":      result.Format,
		"size_bytes":  len(result.Data),
	}).Info("Successfully wrote export result to file")

	return nil
}

// ensureDirectoryExists creates the directory structure for the given file path if it doesn't exist
func (re *ReportExporter) ensureDirectoryExists(filePath string) error {
	dir := filepath.Dir(filePath)
	if dir == "." || dir == "/" {
		return nil // No directory creation needed
	}

	// Check if directory already exists
	if info, err := os.Stat(dir); err == nil && info.IsDir() {
		return nil // Directory already exists
	}

	// Create directory structure with appropriate permissions
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", dir, err)
	}

	re.log.WithField("directory", dir).Debug("Created directory structure for export file")
	return nil
}

// Helper methods

func (re *ReportExporter) getScoreColor(score float64) string {
	if score >= 80 {
		return "#28a745"
	} else if score >= 60 {
		return "#ffc107"
	} else {
		return "#dc3545"
	}
}

// BatchExportReports exports multiple reports in different formats
func (re *ReportExporter) BatchExportReports(reports map[string]interface{}, formats []ExportFormat, options *ExportOptions) ([]*ExportResult, error) {
	results := make([]*ExportResult, 0)

	for reportName, report := range reports {
		for _, format := range formats {
			exportOptions := &ExportOptions{
				Format:        format,
				IncludeCharts: options.IncludeCharts,
				IncludeRaw:    options.IncludeRaw,
				Metadata: map[string]interface{}{
					"report_name":  reportName,
					"batch_export": true,
				},
			}

			var result *ExportResult
			var err error

			switch r := report.(type) {
			case *patterns.PatternAnalysisResult:
				result, err = re.ExportPatternAnalysisReport(r, exportOptions)
			case *adaptive.MaintainabilityReport:
				result, err = re.ExportMaintainabilityReport(r, exportOptions)
			case *adaptive.ConfigurationReport:
				result, err = re.ExportConfigurationReport(r, exportOptions)
			default:
				continue // Skip unsupported report types
			}

			if err != nil {
				re.log.WithError(err).WithFields(logrus.Fields{
					"report_name": reportName,
					"format":      format,
				}).Error("Failed to export report")
				continue
			}

			results = append(results, result)
		}
	}

	re.log.WithFields(logrus.Fields{
		"reports_processed": len(reports),
		"formats":           formats,
		"results_generated": len(results),
	}).Info("Batch export completed")

	return results, nil
}
