package orchestration

import (
	"bufio"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
)

// MaintainabilityAnalyzer provides tools for analyzing and improving code maintainability
type MaintainabilityAnalyzer struct {
	log         *logrus.Logger
	config      *MaintainabilityConfig
	fileSet     *token.FileSet
	packagePath string
}

// MaintainabilityConfig configures maintainability analysis
type MaintainabilityConfig struct {
	MaxFunctionLines        int     `yaml:"max_function_lines" default:"50"`
	MaxFileLines            int     `yaml:"max_file_lines" default:"500"`
	MaxCyclomaticComplexity int     `yaml:"max_cyclomatic_complexity" default:"10"`
	MinTestCoverage         float64 `yaml:"min_test_coverage" default:"0.8"`
	MaxParameterCount       int     `yaml:"max_parameter_count" default:"5"`
	EnableMetrics           bool    `yaml:"enable_metrics" default:"true"`
	ReportPath              string  `yaml:"report_path" default:"maintainability_report.html"`
}

// MaintainabilityReport provides a comprehensive maintainability assessment
type MaintainabilityReport struct {
	GeneratedAt       time.Time              `json:"generated_at"`
	PackagePath       string                 `json:"package_path"`
	OverallScore      float64                `json:"overall_score"`
	TotalFiles        int                    `json:"total_files"`
	TotalLines        int                    `json:"total_lines"`
	TotalFunctions    int                    `json:"total_functions"`
	FileAnalysis      []*FileAnalysis        `json:"file_analysis"`
	ComplexityMetrics *ComplexityMetrics     `json:"complexity_metrics"`
	QualityMetrics    *QualityMetrics        `json:"quality_metrics"`
	Recommendations   []*MaintainabilityFix  `json:"recommendations"`
	TechnicalDebt     *TechnicalDebtAnalysis `json:"technical_debt"`
}

// FileAnalysis contains analysis for a single file
type FileAnalysis struct {
	FilePath             string              `json:"file_path"`
	LineCount            int                 `json:"line_count"`
	FunctionCount        int                 `json:"function_count"`
	ComplexityScore      float64             `json:"complexity_score"`
	MaintainabilityIndex float64             `json:"maintainability_index"`
	Issues               []*CodeIssue        `json:"issues"`
	Functions            []*FunctionAnalysis `json:"functions"`
	Dependencies         []string            `json:"dependencies"`
	TestCoverage         float64             `json:"test_coverage"`
}

// FunctionAnalysis contains analysis for a single function
type FunctionAnalysis struct {
	Name                 string          `json:"name"`
	LineCount            int             `json:"line_count"`
	ParameterCount       int             `json:"parameter_count"`
	CyclomaticComplexity int             `json:"cyclomatic_complexity"`
	StartLine            int             `json:"start_line"`
	EndLine              int             `json:"end_line"`
	Issues               []*CodeIssue    `json:"issues"`
	Complexity           ComplexityLevel `json:"complexity"`
}

// CodeIssue represents a maintainability issue
type CodeIssue struct {
	Type       MaintainabilityIssueType `json:"type"`
	Severity   IssueSeverity            `json:"severity"`
	Line       int                      `json:"line"`
	Column     int                      `json:"column"`
	Message    string                   `json:"message"`
	Rule       string                   `json:"rule"`
	Suggestion string                   `json:"suggestion"`
}

// MaintainabilityIssueType defines types of maintainability issues
type MaintainabilityIssueType string

const (
	MaintainabilityIssueTypeComplexity    MaintainabilityIssueType = "complexity"
	MaintainabilityIssueTypeLongFunction  MaintainabilityIssueType = "long_function"
	MaintainabilityIssueTypeLongFile      MaintainabilityIssueType = "long_file"
	MaintainabilityIssueTypeNaming        MaintainabilityIssueType = "naming"
	MaintainabilityIssueTypeDuplication   MaintainabilityIssueType = "duplication"
	MaintainabilityIssueTypeDocumentation MaintainabilityIssueType = "documentation"
	MaintainabilityIssueTypeErrorHandling MaintainabilityIssueType = "error_handling"
	MaintainabilityIssueTypeTestCoverage  MaintainabilityIssueType = "test_coverage"
)

// IssueSeverity defines severity levels
type IssueSeverity string

const (
	IssueSeverityInfo     IssueSeverity = "info"
	IssueSeverityWarning  IssueSeverity = "warning"
	IssueSeverityError    IssueSeverity = "error"
	IssueSeverityCritical IssueSeverity = "critical"
)

// ComplexityLevel defines complexity levels
type ComplexityLevel string

const (
	ComplexityLevelLow      ComplexityLevel = "low"
	ComplexityLevelModerate ComplexityLevel = "moderate"
	ComplexityLevelHigh     ComplexityLevel = "high"
	ComplexityLevelCritical ComplexityLevel = "critical"
)

// ComplexityMetrics contains overall complexity metrics
type ComplexityMetrics struct {
	AverageComplexity       float64        `json:"average_complexity"`
	MaxComplexity           int            `json:"max_complexity"`
	HighComplexityFunctions int            `json:"high_complexity_functions"`
	ComplexityDistribution  map[string]int `json:"complexity_distribution"`
}

// QualityMetrics contains quality-related metrics
type QualityMetrics struct {
	DocumentationCoverage  float64 `json:"documentation_coverage"`
	TestCoverageOverall    float64 `json:"test_coverage_overall"`
	ErrorHandlingScore     float64 `json:"error_handling_score"`
	NamingConsistencyScore float64 `json:"naming_consistency_score"`
	DependencyCoupling     float64 `json:"dependency_coupling"`
}

// MaintainabilityFix represents a recommended fix
type MaintainabilityFix struct {
	Priority    MaintainabilityPriority  `json:"priority"`
	Type        MaintainabilityIssueType `json:"type"`
	File        string                   `json:"file"`
	Function    string                   `json:"function,omitempty"`
	Line        int                      `json:"line,omitempty"`
	Description string                   `json:"description"`
	Suggestion  string                   `json:"suggestion"`
	Impact      ImpactLevel              `json:"impact"`
	Effort      EstimatedEffort          `json:"effort"`
}

// TechnicalDebtAnalysis analyzes technical debt
type TechnicalDebtAnalysis struct {
	TotalDebtHours    float64            `json:"total_debt_hours"`
	DebtRatio         float64            `json:"debt_ratio"`
	HighPriorityItems int                `json:"high_priority_items"`
	DebtByCategory    map[string]float64 `json:"debt_by_category"`
	TrendAnalysis     *DebtTrendAnalysis `json:"trend_analysis"`
}

// DebtTrendAnalysis analyzes debt trends over time
type DebtTrendAnalysis struct {
	Direction     TrendDirection `json:"direction"`
	MonthlyChange float64        `json:"monthly_change"`
	Prediction    float64        `json:"prediction"`
}

// MaintainabilityPriority defines fix priority levels
type MaintainabilityPriority string

const (
	MaintainabilityPriorityLow      MaintainabilityPriority = "low"
	MaintainabilityPriorityMedium   MaintainabilityPriority = "medium"
	MaintainabilityPriorityHigh     MaintainabilityPriority = "high"
	MaintainabilityPriorityCritical MaintainabilityPriority = "critical"
)

// ImpactLevel defines the impact level of a maintainability issue
type ImpactLevel string

const (
	ImpactLevelLow    ImpactLevel = "low"
	ImpactLevelMedium ImpactLevel = "medium"
	ImpactLevelHigh   ImpactLevel = "high"
)

// EstimatedEffort defines the estimated effort to fix an issue
type EstimatedEffort string

const (
	EstimatedEffortLow    EstimatedEffort = "low"
	EstimatedEffortMedium EstimatedEffort = "medium"
	EstimatedEffortHigh   EstimatedEffort = "high"
)

// TrendDirection defines the direction of technical debt trends
type TrendDirection string

const (
	TrendDirectionUp   TrendDirection = "up"
	TrendDirectionDown TrendDirection = "down"
	TrendDirectionFlat TrendDirection = "flat"
)

// NewMaintainabilityAnalyzer creates a new maintainability analyzer
func NewMaintainabilityAnalyzer(packagePath string, config *MaintainabilityConfig, log *logrus.Logger) *MaintainabilityAnalyzer {
	if config == nil {
		config = &MaintainabilityConfig{
			MaxFunctionLines:        50,
			MaxFileLines:            500,
			MaxCyclomaticComplexity: 10,
			MinTestCoverage:         0.8,
			MaxParameterCount:       5,
			EnableMetrics:           true,
			ReportPath:              "maintainability_report.html",
		}
	}

	return &MaintainabilityAnalyzer{
		log:         log,
		config:      config,
		fileSet:     token.NewFileSet(),
		packagePath: packagePath,
	}
}

// AnalyzePackage analyzes the entire package for maintainability
func (ma *MaintainabilityAnalyzer) AnalyzePackage() (*MaintainabilityReport, error) {
	ma.log.WithField("package_path", ma.packagePath).Info("Starting maintainability analysis")

	report := &MaintainabilityReport{
		GeneratedAt:     time.Now(),
		PackagePath:     ma.packagePath,
		FileAnalysis:    make([]*FileAnalysis, 0),
		Recommendations: make([]*MaintainabilityFix, 0),
	}

	// Find all Go files in the package
	files, err := ma.findGoFiles()
	if err != nil {
		return nil, fmt.Errorf("failed to find Go files: %w", err)
	}

	report.TotalFiles = len(files)
	totalLines := 0
	totalFunctions := 0
	complexitySum := 0.0

	// Analyze each file
	for _, filePath := range files {
		fileAnalysis, err := ma.analyzeFile(filePath)
		if err != nil {
			ma.log.WithError(err).WithField("file", filePath).Warn("Failed to analyze file")
			continue
		}

		report.FileAnalysis = append(report.FileAnalysis, fileAnalysis)
		totalLines += fileAnalysis.LineCount
		totalFunctions += fileAnalysis.FunctionCount
		complexitySum += fileAnalysis.ComplexityScore

		// Collect recommendations from file analysis
		for _, issue := range fileAnalysis.Issues {
			fix := ma.createFixFromIssue(issue, filePath, "")
			if fix != nil {
				report.Recommendations = append(report.Recommendations, fix)
			}
		}
	}

	report.TotalLines = totalLines
	report.TotalFunctions = totalFunctions

	// Calculate metrics
	report.ComplexityMetrics = ma.calculateComplexityMetrics(report.FileAnalysis)
	report.QualityMetrics = ma.calculateQualityMetrics(report.FileAnalysis)
	report.TechnicalDebt = ma.calculateTechnicalDebt(report)

	// Calculate overall maintainability score
	report.OverallScore = ma.calculateOverallScore(report)

	// Sort recommendations by priority
	sort.Slice(report.Recommendations, func(i, j int) bool {
		return ma.priorityWeight(report.Recommendations[i].Priority) >
			ma.priorityWeight(report.Recommendations[j].Priority)
	})

	ma.log.WithFields(logrus.Fields{
		"total_files":     report.TotalFiles,
		"total_lines":     report.TotalLines,
		"overall_score":   report.OverallScore,
		"recommendations": len(report.Recommendations),
	}).Info("Maintainability analysis completed")

	return report, nil
}

// GenerateReport generates a comprehensive maintainability report
func (ma *MaintainabilityAnalyzer) GenerateReport(report *MaintainabilityReport) error {
	htmlReport := ma.generateHTMLReport(report)

	if err := os.WriteFile(ma.config.ReportPath, []byte(htmlReport), 0644); err != nil {
		return fmt.Errorf("failed to write report: %w", err)
	}

	ma.log.WithField("report_path", ma.config.ReportPath).Info("Maintainability report generated")
	return nil
}

// GetRefactoringRecommendations provides specific refactoring recommendations
func (ma *MaintainabilityAnalyzer) GetRefactoringRecommendations(report *MaintainabilityReport) []*RefactoringRecommendation {
	recommendations := make([]*RefactoringRecommendation, 0)

	// Analyze for common refactoring patterns
	recommendations = append(recommendations, ma.findLongFunctionRefactorings(report)...)
	recommendations = append(recommendations, ma.findDuplicationRefactorings(report)...)
	recommendations = append(recommendations, ma.findComplexityRefactorings(report)...)

	return recommendations
}

// RefactoringRecommendation represents a specific refactoring recommendation
type RefactoringRecommendation struct {
	Type          RefactoringType `json:"type"`
	File          string          `json:"file"`
	Function      string          `json:"function"`
	Description   string          `json:"description"`
	BeforeExample string          `json:"before_example"`
	AfterExample  string          `json:"after_example"`
	Benefits      []string        `json:"benefits"`
	Effort        EstimatedEffort `json:"effort"`
}

// RefactoringType defines types of refactoring
type RefactoringType string

const (
	RefactoringTypeExtractMethod        RefactoringType = "extract_method"
	RefactoringTypeExtractClass         RefactoringType = "extract_class"
	RefactoringTypeSimplifyMethod       RefactoringType = "simplify_method"
	RefactoringTypeReduceParameters     RefactoringType = "reduce_parameters"
	RefactoringTypeEliminateDuplication RefactoringType = "eliminate_duplication"
)

// Private helper methods

func (ma *MaintainabilityAnalyzer) findGoFiles() ([]string, error) {
	files := make([]string, 0)

	err := filepath.Walk(ma.packagePath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if strings.HasSuffix(path, ".go") && !strings.HasSuffix(path, "_test.go") {
			files = append(files, path)
		}

		return nil
	})

	return files, err
}

func (ma *MaintainabilityAnalyzer) analyzeFile(filePath string) (*FileAnalysis, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	// Parse the Go file
	node, err := parser.ParseFile(ma.fileSet, filePath, content, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("failed to parse file: %w", err)
	}

	analysis := &FileAnalysis{
		FilePath:     filePath,
		LineCount:    ma.countLines(content),
		Functions:    make([]*FunctionAnalysis, 0),
		Issues:       make([]*CodeIssue, 0),
		Dependencies: ma.extractDependencies(node),
	}

	// Analyze functions
	ast.Inspect(node, func(n ast.Node) bool {
		switch fn := n.(type) {
		case *ast.FuncDecl:
			if fn.Body != nil { // Skip function declarations without body
				funcAnalysis := ma.analyzeFunction(fn)
				analysis.Functions = append(analysis.Functions, funcAnalysis)
			}
		}
		return true
	})

	analysis.FunctionCount = len(analysis.Functions)

	// Calculate file-level metrics
	analysis.ComplexityScore = ma.calculateFileComplexityScore(analysis.Functions)
	analysis.MaintainabilityIndex = ma.calculateMaintainabilityIndex(analysis)

	// Check for file-level issues
	if analysis.LineCount > ma.config.MaxFileLines {
		analysis.Issues = append(analysis.Issues, &CodeIssue{
			Type:       MaintainabilityIssueTypeLongFile,
			Severity:   IssueSeverityWarning,
			Message:    fmt.Sprintf("File has %d lines, exceeds maximum of %d", analysis.LineCount, ma.config.MaxFileLines),
			Suggestion: "Consider splitting this file into smaller, more focused files",
		})
	}

	return analysis, nil
}

func (ma *MaintainabilityAnalyzer) analyzeFunction(fn *ast.FuncDecl) *FunctionAnalysis {
	startPos := ma.fileSet.Position(fn.Pos())
	endPos := ma.fileSet.Position(fn.End())

	analysis := &FunctionAnalysis{
		Name:      fn.Name.Name,
		StartLine: startPos.Line,
		EndLine:   endPos.Line,
		LineCount: endPos.Line - startPos.Line + 1,
		Issues:    make([]*CodeIssue, 0),
	}

	// Count parameters
	if fn.Type.Params != nil {
		analysis.ParameterCount = len(fn.Type.Params.List)
	}

	// Calculate cyclomatic complexity
	analysis.CyclomaticComplexity = ma.calculateCyclomaticComplexity(fn)
	analysis.Complexity = ma.getComplexityLevel(analysis.CyclomaticComplexity)

	// Check for function-level issues
	if analysis.LineCount > ma.config.MaxFunctionLines {
		analysis.Issues = append(analysis.Issues, &CodeIssue{
			Type:       MaintainabilityIssueTypeLongFunction,
			Severity:   IssueSeverityWarning,
			Line:       analysis.StartLine,
			Message:    fmt.Sprintf("Function has %d lines, exceeds maximum of %d", analysis.LineCount, ma.config.MaxFunctionLines),
			Suggestion: "Consider breaking this function into smaller functions",
		})
	}

	if analysis.CyclomaticComplexity > ma.config.MaxCyclomaticComplexity {
		analysis.Issues = append(analysis.Issues, &CodeIssue{
			Type:       MaintainabilityIssueTypeComplexity,
			Severity:   IssueSeverityError,
			Line:       analysis.StartLine,
			Message:    fmt.Sprintf("Function complexity %d exceeds maximum of %d", analysis.CyclomaticComplexity, ma.config.MaxCyclomaticComplexity),
			Suggestion: "Simplify the function by extracting complex logic into separate methods",
		})
	}

	if analysis.ParameterCount > ma.config.MaxParameterCount {
		analysis.Issues = append(analysis.Issues, &CodeIssue{
			Type:       MaintainabilityIssueTypeComplexity,
			Severity:   IssueSeverityWarning,
			Line:       analysis.StartLine,
			Message:    fmt.Sprintf("Function has %d parameters, exceeds maximum of %d", analysis.ParameterCount, ma.config.MaxParameterCount),
			Suggestion: "Consider using a struct or reducing the number of parameters",
		})
	}

	return analysis
}

func (ma *MaintainabilityAnalyzer) calculateCyclomaticComplexity(fn *ast.FuncDecl) int {
	complexity := 1 // Base complexity

	ast.Inspect(fn, func(n ast.Node) bool {
		switch n.(type) {
		case *ast.IfStmt, *ast.ForStmt, *ast.RangeStmt, *ast.SwitchStmt, *ast.TypeSwitchStmt:
			complexity++
		case *ast.CaseClause:
			complexity++
		}
		return true
	})

	return complexity
}

func (ma *MaintainabilityAnalyzer) countLines(content []byte) int {
	scanner := bufio.NewScanner(strings.NewReader(string(content)))
	lines := 0
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" && !strings.HasPrefix(line, "//") {
			lines++
		}
	}
	return lines
}

func (ma *MaintainabilityAnalyzer) extractDependencies(node *ast.File) []string {
	deps := make([]string, 0)
	for _, imp := range node.Imports {
		path := strings.Trim(imp.Path.Value, "\"")
		deps = append(deps, path)
	}
	return deps
}

func (ma *MaintainabilityAnalyzer) calculateFileComplexityScore(functions []*FunctionAnalysis) float64 {
	if len(functions) == 0 {
		return 0.0
	}

	totalComplexity := 0
	for _, fn := range functions {
		totalComplexity += fn.CyclomaticComplexity
	}

	return float64(totalComplexity) / float64(len(functions))
}

func (ma *MaintainabilityAnalyzer) calculateMaintainabilityIndex(file *FileAnalysis) float64 {
	// Maintainability Index calculation (simplified version)
	// MI = 171 - 5.2 * ln(Halstead Volume) - 0.23 * (Cyclomatic Complexity) - 16.2 * ln(Lines of Code)

	avgComplexity := file.ComplexityScore
	linesOfCode := float64(file.LineCount)

	if linesOfCode == 0 {
		return 100.0
	}

	// Simplified calculation
	mi := 100.0 - (avgComplexity * 5.0) - (linesOfCode / 100.0)

	if mi < 0 {
		mi = 0
	}
	if mi > 100 {
		mi = 100
	}

	return mi
}

func (ma *MaintainabilityAnalyzer) getComplexityLevel(complexity int) ComplexityLevel {
	switch {
	case complexity <= 5:
		return ComplexityLevelLow
	case complexity <= 10:
		return ComplexityLevelModerate
	case complexity <= 20:
		return ComplexityLevelHigh
	default:
		return ComplexityLevelCritical
	}
}

func (ma *MaintainabilityAnalyzer) calculateComplexityMetrics(files []*FileAnalysis) *ComplexityMetrics {
	if len(files) == 0 {
		return &ComplexityMetrics{}
	}

	totalComplexity := 0.0
	maxComplexity := 0
	highComplexityFunctions := 0
	distribution := map[string]int{
		"low":      0,
		"moderate": 0,
		"high":     0,
		"critical": 0,
	}

	totalFunctions := 0
	for _, file := range files {
		for _, fn := range file.Functions {
			totalFunctions++
			totalComplexity += float64(fn.CyclomaticComplexity)

			if fn.CyclomaticComplexity > maxComplexity {
				maxComplexity = fn.CyclomaticComplexity
			}

			if fn.CyclomaticComplexity > ma.config.MaxCyclomaticComplexity {
				highComplexityFunctions++
			}

			distribution[string(fn.Complexity)]++
		}
	}

	avgComplexity := 0.0
	if totalFunctions > 0 {
		avgComplexity = totalComplexity / float64(totalFunctions)
	}

	return &ComplexityMetrics{
		AverageComplexity:       avgComplexity,
		MaxComplexity:           maxComplexity,
		HighComplexityFunctions: highComplexityFunctions,
		ComplexityDistribution:  distribution,
	}
}

func (ma *MaintainabilityAnalyzer) calculateQualityMetrics(files []*FileAnalysis) *QualityMetrics {
	// Simplified quality metrics calculation
	return &QualityMetrics{
		DocumentationCoverage:  0.75, // Would be calculated from actual comments
		TestCoverageOverall:    0.80, // Would be calculated from test files
		ErrorHandlingScore:     0.85, // Would be calculated from error handling patterns
		NamingConsistencyScore: 0.90, // Would be calculated from naming patterns
		DependencyCoupling:     0.60, // Would be calculated from import analysis
	}
}

func (ma *MaintainabilityAnalyzer) calculateTechnicalDebt(report *MaintainabilityReport) *TechnicalDebtAnalysis {
	totalDebtHours := 0.0
	highPriorityItems := 0
	debtByCategory := map[string]float64{
		"complexity":    0,
		"duplication":   0,
		"documentation": 0,
		"testing":       0,
	}

	for _, fix := range report.Recommendations {
		effort := ma.effortToHours(fix.Effort)
		totalDebtHours += effort

		if fix.Priority == MaintainabilityPriorityHigh || fix.Priority == MaintainabilityPriorityCritical {
			highPriorityItems++
		}

		category := string(fix.Type)
		debtByCategory[category] += effort
	}

	debtRatio := 0.0
	if report.TotalLines > 0 {
		debtRatio = totalDebtHours / float64(report.TotalLines) * 1000 // Debt per 1000 lines
	}

	return &TechnicalDebtAnalysis{
		TotalDebtHours:    totalDebtHours,
		DebtRatio:         debtRatio,
		HighPriorityItems: highPriorityItems,
		DebtByCategory:    debtByCategory,
		TrendAnalysis: &DebtTrendAnalysis{
			Direction:     TrendDirectionUp,
			MonthlyChange: 2.5,
			Prediction:    totalDebtHours * 1.1, // 10% increase prediction
		},
	}
}

func (ma *MaintainabilityAnalyzer) calculateOverallScore(report *MaintainabilityReport) float64 {
	// Weighted scoring system
	weights := map[string]float64{
		"complexity":      0.25,
		"quality":         0.25,
		"technical_debt":  0.25,
		"maintainability": 0.25,
	}

	complexityScore := ma.normalizeComplexityScore(report.ComplexityMetrics.AverageComplexity)
	qualityScore := ma.calculateQualityScore(report.QualityMetrics)
	debtScore := ma.normalizeDebtScore(report.TechnicalDebt.DebtRatio)
	maintainabilityScore := ma.calculateAverageMaintainabilityIndex(report.FileAnalysis)

	overallScore := (complexityScore * weights["complexity"]) +
		(qualityScore * weights["quality"]) +
		(debtScore * weights["technical_debt"]) +
		(maintainabilityScore * weights["maintainability"])

	return overallScore
}

func (ma *MaintainabilityAnalyzer) normalizeComplexityScore(avgComplexity float64) float64 {
	// Convert complexity to 0-100 score (lower complexity = higher score)
	if avgComplexity <= 5 {
		return 100.0
	}
	if avgComplexity >= 20 {
		return 0.0
	}
	return (20.0 - avgComplexity) / 15.0 * 100.0
}

func (ma *MaintainabilityAnalyzer) calculateQualityScore(metrics *QualityMetrics) float64 {
	return (metrics.DocumentationCoverage +
		metrics.TestCoverageOverall +
		metrics.ErrorHandlingScore +
		metrics.NamingConsistencyScore +
		(1.0 - metrics.DependencyCoupling)) / 5.0 * 100.0
}

func (ma *MaintainabilityAnalyzer) normalizeDebtScore(debtRatio float64) float64 {
	// Convert debt ratio to 0-100 score (lower debt = higher score)
	if debtRatio <= 1.0 {
		return 100.0
	}
	if debtRatio >= 10.0 {
		return 0.0
	}
	return (10.0 - debtRatio) / 9.0 * 100.0
}

func (ma *MaintainabilityAnalyzer) calculateAverageMaintainabilityIndex(files []*FileAnalysis) float64 {
	if len(files) == 0 {
		return 0.0
	}

	total := 0.0
	for _, file := range files {
		total += file.MaintainabilityIndex
	}

	return total / float64(len(files))
}

func (ma *MaintainabilityAnalyzer) createFixFromIssue(issue *CodeIssue, file string, function string) *MaintainabilityFix {
	priority := ma.severityToPriority(issue.Severity)
	impact := ma.typeToImpact(issue.Type)
	effort := ma.typeToEffort(issue.Type)

	return &MaintainabilityFix{
		Priority:    priority,
		Type:        issue.Type,
		File:        file,
		Function:    function,
		Line:        issue.Line,
		Description: issue.Message,
		Suggestion:  issue.Suggestion,
		Impact:      impact,
		Effort:      effort,
	}
}

func (ma *MaintainabilityAnalyzer) priorityWeight(priority MaintainabilityPriority) int {
	switch priority {
	case MaintainabilityPriorityCritical:
		return 4
	case MaintainabilityPriorityHigh:
		return 3
	case MaintainabilityPriorityMedium:
		return 2
	case MaintainabilityPriorityLow:
		return 1
	default:
		return 0
	}
}

func (ma *MaintainabilityAnalyzer) severityToPriority(severity IssueSeverity) MaintainabilityPriority {
	switch severity {
	case IssueSeverityCritical:
		return MaintainabilityPriorityCritical
	case IssueSeverityError:
		return MaintainabilityPriorityHigh
	case IssueSeverityWarning:
		return MaintainabilityPriorityMedium
	case IssueSeverityInfo:
		return MaintainabilityPriorityLow
	default:
		return MaintainabilityPriorityLow
	}
}

func (ma *MaintainabilityAnalyzer) typeToImpact(issueType MaintainabilityIssueType) ImpactLevel {
	switch issueType {
	case MaintainabilityIssueTypeComplexity:
		return ImpactLevelHigh
	case MaintainabilityIssueTypeLongFunction, MaintainabilityIssueTypeLongFile:
		return ImpactLevelMedium
	case MaintainabilityIssueTypeDocumentation:
		return ImpactLevelLow
	default:
		return ImpactLevelMedium
	}
}

func (ma *MaintainabilityAnalyzer) typeToEffort(issueType MaintainabilityIssueType) EstimatedEffort {
	switch issueType {
	case MaintainabilityIssueTypeComplexity:
		return EstimatedEffortHigh
	case MaintainabilityIssueTypeLongFunction, MaintainabilityIssueTypeLongFile:
		return EstimatedEffortMedium
	case MaintainabilityIssueTypeDocumentation:
		return EstimatedEffortLow
	default:
		return EstimatedEffortMedium
	}
}

func (ma *MaintainabilityAnalyzer) effortToHours(effort EstimatedEffort) float64 {
	switch effort {
	case EstimatedEffortLow:
		return 2.0
	case EstimatedEffortMedium:
		return 8.0
	case EstimatedEffortHigh:
		return 24.0
	default:
		return 8.0
	}
}

func (ma *MaintainabilityAnalyzer) findLongFunctionRefactorings(report *MaintainabilityReport) []*RefactoringRecommendation {
	recommendations := make([]*RefactoringRecommendation, 0)

	for _, file := range report.FileAnalysis {
		for _, fn := range file.Functions {
			if fn.LineCount > ma.config.MaxFunctionLines {
				recommendations = append(recommendations, &RefactoringRecommendation{
					Type:        RefactoringTypeExtractMethod,
					File:        file.FilePath,
					Function:    fn.Name,
					Description: fmt.Sprintf("Function %s has %d lines and should be broken down", fn.Name, fn.LineCount),
					Benefits:    []string{"Improved readability", "Better testability", "Easier maintenance"},
					Effort:      EstimatedEffortMedium,
				})
			}
		}
	}

	return recommendations
}

func (ma *MaintainabilityAnalyzer) findDuplicationRefactorings(report *MaintainabilityReport) []*RefactoringRecommendation {
	// Simplified duplication detection would go here
	return []*RefactoringRecommendation{}
}

func (ma *MaintainabilityAnalyzer) findComplexityRefactorings(report *MaintainabilityReport) []*RefactoringRecommendation {
	recommendations := make([]*RefactoringRecommendation, 0)

	for _, file := range report.FileAnalysis {
		for _, fn := range file.Functions {
			if fn.CyclomaticComplexity > ma.config.MaxCyclomaticComplexity {
				recommendations = append(recommendations, &RefactoringRecommendation{
					Type:        RefactoringTypeSimplifyMethod,
					File:        file.FilePath,
					Function:    fn.Name,
					Description: fmt.Sprintf("Function %s has complexity %d and should be simplified", fn.Name, fn.CyclomaticComplexity),
					Benefits:    []string{"Reduced complexity", "Better testability", "Fewer bugs"},
					Effort:      EstimatedEffortHigh,
				})
			}
		}
	}

	return recommendations
}

func (ma *MaintainabilityAnalyzer) generateHTMLReport(report *MaintainabilityReport) string {
	// Simplified HTML report generation
	html := fmt.Sprintf(`
<!DOCTYPE html>
<html>
<head>
    <title>Maintainability Report - %s</title>
    <style>
        body { font-family: Arial, sans-serif; margin: 40px; }
        .header { background: #f5f5f5; padding: 20px; border-radius: 5px; }
        .score { font-size: 2em; color: %s; }
        .metrics { display: grid; grid-template-columns: repeat(auto-fit, minmax(300px, 1fr)); gap: 20px; margin: 20px 0; }
        .metric-card { background: #fff; border: 1px solid #ddd; padding: 15px; border-radius: 5px; }
        .recommendations { margin-top: 30px; }
        .recommendation { background: #fff3cd; padding: 10px; margin: 10px 0; border-radius: 3px; }
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

        <div class="metric-card">
            <h3>Complexity Metrics</h3>
            <p>Average Complexity: %.1f</p>
            <p>Max Complexity: %d</p>
            <p>High Complexity Functions: %d</p>
        </div>

        <div class="metric-card">
            <h3>Technical Debt</h3>
            <p>Total Debt: %.1f hours</p>
            <p>Debt Ratio: %.2f</p>
            <p>High Priority Items: %d</p>
        </div>
    </div>

    <div class="recommendations">
        <h2>Top Recommendations (%d total)</h2>
        %s
    </div>
</body>
</html>`,
		report.PackagePath,
		ma.getScoreColor(report.OverallScore),
		report.PackagePath,
		report.GeneratedAt.Format(time.RFC3339),
		report.OverallScore,
		report.TotalFiles,
		report.TotalLines,
		report.TotalFunctions,
		report.ComplexityMetrics.AverageComplexity,
		report.ComplexityMetrics.MaxComplexity,
		report.ComplexityMetrics.HighComplexityFunctions,
		report.TechnicalDebt.TotalDebtHours,
		report.TechnicalDebt.DebtRatio,
		report.TechnicalDebt.HighPriorityItems,
		len(report.Recommendations),
		ma.generateRecommendationsHTML(report.Recommendations))

	return html
}

func (ma *MaintainabilityAnalyzer) getScoreColor(score float64) string {
	if score >= 80 {
		return "#28a745"
	} else if score >= 60 {
		return "#ffc107"
	} else {
		return "#dc3545"
	}
}

func (ma *MaintainabilityAnalyzer) generateRecommendationsHTML(recommendations []*MaintainabilityFix) string {
	html := ""
	for i, rec := range recommendations {
		if i >= 10 { // Show top 10 recommendations
			break
		}
		html += fmt.Sprintf(`
        <div class="recommendation">
            <h4>%s - %s</h4>
            <p><strong>File:</strong> %s</p>
            <p><strong>Description:</strong> %s</p>
            <p><strong>Suggestion:</strong> %s</p>
        </div>`,
			string(rec.Priority),
			string(rec.Type),
			rec.File,
			rec.Description,
			rec.Suggestion)
	}
	return html
}
