package learning

import (
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/jordigilh/kubernaut/pkg/intelligence/shared"
	sharedmath "github.com/jordigilh/kubernaut/pkg/shared/math"
	"github.com/jordigilh/kubernaut/pkg/shared/types"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"

	"github.com/sirupsen/logrus"
	"gonum.org/v1/gonum/stat"
)

// Types now consolidated in pkg/intelligence/shared/types.go

// TimeSeriesAnalyzer analyzes temporal patterns in workflow execution data
type TimeSeriesAnalyzer struct {
	config *PatternDiscoveryConfig
	log    *logrus.Logger
}

// TimeSeriesData represents time-series data points
type TimeSeriesData struct {
	Timestamp time.Time              `json:"timestamp"`
	Values    map[string]float64     `json:"values"`
	Metadata  map[string]interface{} `json:"metadata"`
}

// SeasonalityAnalysis contains seasonality detection results
type SeasonalityAnalysis struct {
	Pattern         string                  `json:"pattern"`  // "daily", "weekly", "monthly"
	Strength        float64                 `json:"strength"` // 0-1, strength of seasonal pattern
	Period          time.Duration           `json:"period"`
	PeakTimes       []sharedtypes.TimeRange `json:"peak_times"`
	ValleyTimes     []sharedtypes.TimeRange `json:"valley_times"`
	SeasonalFactors map[string]float64      `json:"seasonal_factors"`
	Confidence      float64                 `json:"confidence"`
}

// TimeSeriesTrendAnalysis contains trend detection results for time series
type TimeSeriesTrendAnalysis struct {
	Direction    string                `json:"direction"` // "increasing", "decreasing", "stable"
	Slope        float64               `json:"slope"`
	Confidence   float64               `json:"confidence"`
	StartValue   float64               `json:"start_value"`
	EndValue     float64               `json:"end_value"`
	TrendPeriod  sharedtypes.TimeRange `json:"trend_period"`
	Significance float64               `json:"significance"`
}

// AnomalyDetection contains anomaly detection results
type AnomalyDetection struct {
	Anomalies     []*AnomalyPoint     `json:"anomalies"`
	Threshold     float64             `json:"threshold"`
	BaselineStats *BaselineStatistics `json:"baseline_stats"`
	Method        string              `json:"method"`
}

// AnomalyPoint represents a detected anomaly
type AnomalyPoint struct {
	Timestamp   time.Time `json:"timestamp"`
	Value       float64   `json:"value"`
	Expected    float64   `json:"expected"`
	Deviation   float64   `json:"deviation"`
	Severity    string    `json:"severity"`     // "low", "medium", "high", "critical"
	AnomalyType string    `json:"anomaly_type"` // "spike", "dip", "sustained"
}

// BaselineStatistics contains statistical baseline information
type BaselineStatistics struct {
	Mean   float64 `json:"mean"`
	StdDev float64 `json:"std_dev"`
	Median float64 `json:"median"`
	Min    float64 `json:"min"`
	Max    float64 `json:"max"`
	Q1     float64 `json:"q1"`
	Q3     float64 `json:"q3"`
	IQR    float64 `json:"iqr"`
}

// ForecastResult contains prediction results
type ForecastResult struct {
	Predictions        []*ForecastPoint          `json:"predictions"`
	ConfidenceInterval *types.ConfidenceInterval `json:"confidence_interval"`
	Model              string                    `json:"model"`
	Accuracy           float64                   `json:"accuracy"`
	ForecastPeriod     sharedtypes.TimeRange     `json:"forecast_period"`
}

// ForecastPoint represents a forecasted value
type ForecastPoint struct {
	Timestamp       time.Time `json:"timestamp"`
	Value           float64   `json:"value"`
	ConfidenceLower float64   `json:"confidence_lower"`
	ConfidenceUpper float64   `json:"confidence_upper"`
}

// ConfidenceInterval represents confidence bounds for forecasts

// NewTimeSeriesAnalyzer creates a new time series analyzer
func NewTimeSeriesAnalyzer(config *PatternDiscoveryConfig, log *logrus.Logger) *TimeSeriesAnalyzer {
	return &TimeSeriesAnalyzer{
		config: config,
		log:    log,
	}
}

// AnalyzeResourceTrends analyzes resource utilization trends over time
func (tsa *TimeSeriesAnalyzer) AnalyzeResourceTrends(data []*sharedtypes.WorkflowExecutionData) []*shared.ResourceTrendAnalysis {
	tsa.log.WithField("data_points", len(data)).Info("Analyzing resource trends")

	// Group data by resource type
	resourceData := tsa.groupByResourceType(data)

	trends := make([]*shared.ResourceTrendAnalysis, 0)
	for resourceType, executions := range resourceData {
		if len(executions) < 5 {
			continue // Need minimum data points for trend analysis
		}

		trend := tsa.analyzeResourceTypeTrend(resourceType, executions)
		if trend != nil {
			trends = append(trends, trend)
		}
	}

	return trends
}

// DetectTemporalPatterns detects time-based patterns in execution data
func (tsa *TimeSeriesAnalyzer) DetectTemporalPatterns(data []*sharedtypes.WorkflowExecutionData) []*shared.TemporalAnalysis {
	tsa.log.WithField("data_points", len(data)).Info("Detecting temporal patterns")

	if len(data) < 10 {
		return []*shared.TemporalAnalysis{}
	}

	patterns := make([]*shared.TemporalAnalysis, 0)

	// Convert to time series
	timeSeries := tsa.convertToTimeSeries(data)

	// Detect daily patterns
	if dailyPattern := tsa.detectDailyPattern(timeSeries); dailyPattern != nil {
		patterns = append(patterns, dailyPattern)
	}

	// Detect weekly patterns
	if weeklyPattern := tsa.detectWeeklyPattern(timeSeries); weeklyPattern != nil {
		patterns = append(patterns, weeklyPattern)
	}

	// Detect burst patterns
	if burstPattern := tsa.detectBurstPattern(timeSeries); burstPattern != nil {
		patterns = append(patterns, burstPattern)
	}

	return patterns
}

// AnalyzeSeasonality performs seasonality analysis on time series data
func (tsa *TimeSeriesAnalyzer) AnalyzeSeasonality(timeSeries []*TimeSeriesData, metric string) (*SeasonalityAnalysis, error) {
	if len(timeSeries) < 24 { // Need at least 24 hours of data
		return nil, fmt.Errorf("insufficient data for seasonality analysis: %d points", len(timeSeries))
	}

	tsa.log.WithFields(logrus.Fields{
		"metric":      metric,
		"data_points": len(timeSeries),
	}).Info("Analyzing seasonality")

	// Extract values for the specified metric
	values := make([]float64, 0)
	timestamps := make([]time.Time, 0)

	for _, point := range timeSeries {
		if value, exists := point.Values[metric]; exists {
			values = append(values, value)
			timestamps = append(timestamps, point.Timestamp)
		}
	}

	if len(values) < 24 {
		return nil, fmt.Errorf("insufficient metric data: %d points", len(values))
	}

	// Analyze different seasonal patterns
	dailyAnalysis := tsa.analyzeDailySeasonality(values, timestamps)
	weeklyAnalysis := tsa.analyzeWeeklySeasonality(values, timestamps)

	// Choose the strongest pattern
	var bestAnalysis *SeasonalityAnalysis
	if dailyAnalysis.Strength > weeklyAnalysis.Strength {
		bestAnalysis = dailyAnalysis
	} else {
		bestAnalysis = weeklyAnalysis
	}

	return bestAnalysis, nil
}

// DetectTrends identifies trends in time series data
func (tsa *TimeSeriesAnalyzer) DetectTrends(timeSeries []*TimeSeriesData, metric string) (*TimeSeriesTrendAnalysis, error) {
	if len(timeSeries) < 10 {
		return nil, fmt.Errorf("insufficient data for trend analysis: %d points", len(timeSeries))
	}

	// Extract values and timestamps
	values := make([]float64, 0)
	timestamps := make([]time.Time, 0)

	for _, point := range timeSeries {
		if value, exists := point.Values[metric]; exists {
			values = append(values, value)
			timestamps = append(timestamps, point.Timestamp)
		}
	}

	if len(values) < 10 {
		return nil, fmt.Errorf("insufficient metric data: %d points", len(values))
	}

	// Sort by timestamp
	tsa.sortTimeSeriesByTimestamp(values, timestamps)

	// Calculate linear trend using least squares
	n := len(values)
	x := make([]float64, n)
	for i := 0; i < n; i++ {
		x[i] = float64(i) // Use index as x-axis
	}

	// Calculate slope and intercept
	slope, intercept := tsa.calculateLinearRegression(x, values)

	// Determine trend direction and significance
	direction := "stable"
	if math.Abs(slope) > 0.01 { // Minimum threshold for trend detection
		if slope > 0 {
			direction = "increasing"
		} else {
			direction = "decreasing"
		}
	}

	// Calculate R-squared for confidence
	rSquared := tsa.calculateRSquared(x, values, slope, intercept)
	confidence := math.Max(0, rSquared) // Ensure non-negative

	trend := &TimeSeriesTrendAnalysis{
		Direction:    direction,
		Slope:        slope,
		Confidence:   confidence,
		StartValue:   values[0],
		EndValue:     values[n-1],
		TrendPeriod:  sharedtypes.TimeRange{Start: timestamps[0], End: timestamps[n-1]},
		Significance: math.Abs(slope) * confidence,
	}

	return trend, nil
}

// DetectAnomalies identifies anomalous points in time series
func (tsa *TimeSeriesAnalyzer) DetectAnomalies(timeSeries []*TimeSeriesData, metric string) (*AnomalyDetection, error) {
	if len(timeSeries) < 20 {
		return nil, fmt.Errorf("insufficient data for anomaly detection: %d points", len(timeSeries))
	}

	// Extract values
	values := make([]float64, 0)
	timestamps := make([]time.Time, 0)

	for _, point := range timeSeries {
		if value, exists := point.Values[metric]; exists {
			values = append(values, value)
			timestamps = append(timestamps, point.Timestamp)
		}
	}

	if len(values) < 20 {
		return nil, fmt.Errorf("insufficient metric data: %d points", len(values))
	}

	// Calculate baseline statistics
	baseline := tsa.calculateBaselineStats(values)

	// Use IQR method for anomaly detection
	threshold := 1.5 * baseline.IQR
	anomalies := make([]*AnomalyPoint, 0)

	for i, value := range values {
		isAnomaly := false
		severity := "low"
		anomalyType := "normal"

		// Check for outliers using IQR method
		if value < baseline.Q1-threshold || value > baseline.Q3+threshold {
			isAnomaly = true
			deviation := math.Max(math.Abs(value-baseline.Q1), math.Abs(value-baseline.Q3))

			if value > baseline.Q3+threshold {
				anomalyType = "spike"
			} else {
				anomalyType = "dip"
			}

			// Determine severity based on deviation
			if deviation > 3*threshold {
				severity = "critical"
			} else if deviation > 2*threshold {
				severity = "high"
			} else if deviation > threshold {
				severity = "medium"
			}
		}

		// Also check for sudden changes (z-score method)
		zScore := (value - baseline.Mean) / baseline.StdDev
		if math.Abs(zScore) > 3 {
			isAnomaly = true
			if !isAnomaly || severity == "low" {
				if math.Abs(zScore) > 4 {
					severity = "critical"
				} else {
					severity = "high"
				}
			}
		}

		if isAnomaly {
			anomaly := &AnomalyPoint{
				Timestamp:   timestamps[i],
				Value:       value,
				Expected:    baseline.Mean,
				Deviation:   math.Abs(value - baseline.Mean),
				Severity:    severity,
				AnomalyType: anomalyType,
			}
			anomalies = append(anomalies, anomaly)
		}
	}

	detection := &AnomalyDetection{
		Anomalies:     anomalies,
		Threshold:     threshold,
		BaselineStats: baseline,
		Method:        "IQR + Z-Score",
	}

	return detection, nil
}

// ForecastTimeSeries generates forecasts for time series data
func (tsa *TimeSeriesAnalyzer) ForecastTimeSeries(timeSeries []*TimeSeriesData, metric string, forecastHours int) (*ForecastResult, error) {
	if len(timeSeries) < 10 {
		return nil, fmt.Errorf("insufficient data for forecasting: %d points", len(timeSeries))
	}

	// Extract values and timestamps
	values := make([]float64, 0)
	timestamps := make([]time.Time, 0)

	for _, point := range timeSeries {
		if value, exists := point.Values[metric]; exists {
			values = append(values, value)
			timestamps = append(timestamps, point.Timestamp)
		}
	}

	if len(values) < 10 {
		return nil, fmt.Errorf("insufficient metric data: %d points", len(values))
	}

	// Sort by timestamp
	tsa.sortTimeSeriesByTimestamp(values, timestamps)

	// Use simple linear extrapolation for forecasting
	n := len(values)
	x := make([]float64, n)
	for i := 0; i < n; i++ {
		x[i] = float64(i)
	}

	slope, intercept := tsa.calculateLinearRegression(x, values)

	// Generate forecasts
	predictions := make([]*ForecastPoint, 0)
	lastTimestamp := timestamps[n-1]

	for i := 1; i <= forecastHours; i++ {
		futureTimestamp := lastTimestamp.Add(time.Duration(i) * time.Hour)
		futureX := float64(n + i - 1)
		predictedValue := slope*futureX + intercept

		// Calculate confidence bounds (simplified)
		residualStdDev := tsa.calculateResidualStdDev(x, values, slope, intercept)
		confidenceRange := 1.96 * residualStdDev // 95% confidence interval

		prediction := &ForecastPoint{
			Timestamp:       futureTimestamp,
			Value:           predictedValue,
			ConfidenceLower: predictedValue - confidenceRange,
			ConfidenceUpper: predictedValue + confidenceRange,
		}
		predictions = append(predictions, prediction)
	}

	// Calculate model accuracy (R-squared)
	rSquared := tsa.calculateRSquared(x, values, slope, intercept)

	result := &ForecastResult{
		Predictions: predictions,
		Model:       "Linear Regression",
		Accuracy:    math.Max(0, rSquared),
		ForecastPeriod: sharedtypes.TimeRange{
			Start: predictions[0].Timestamp,
			End:   predictions[len(predictions)-1].Timestamp,
		},
	}

	return result, nil
}

// Private helper methods

func convertToPatternTimeRanges(ranges []sharedtypes.TimeRange) []shared.PatternTimeRange {
	result := make([]shared.PatternTimeRange, len(ranges))
	for i, tr := range ranges {
		result[i] = shared.PatternTimeRange{
			Start: tr.Start,
			End:   tr.End,
		}
	}
	return result
}

func (tsa *TimeSeriesAnalyzer) groupByResourceType(data []*sharedtypes.WorkflowExecutionData) map[string][]*sharedtypes.WorkflowExecutionData {
	groups := make(map[string][]*sharedtypes.WorkflowExecutionData)

	for _, execution := range data {
		resourceType := "unknown"
		if execution.WorkflowID != "" {
			resourceType = execution.WorkflowID
		}

		if _, exists := groups[resourceType]; !exists {
			groups[resourceType] = make([]*sharedtypes.WorkflowExecutionData, 0)
		}
		groups[resourceType] = append(groups[resourceType], execution)
	}

	return groups
}

func (tsa *TimeSeriesAnalyzer) analyzeResourceTypeTrend(resourceType string, executions []*sharedtypes.WorkflowExecutionData) *shared.ResourceTrendAnalysis {
	if len(executions) < 5 {
		return nil
	}

	// Convert to time series of success rates
	successRates := make([]float64, 0)

	// Group by time windows (e.g., daily)
	windowSize := 24 * time.Hour
	windows := tsa.groupExecutionsByTimeWindow(executions, windowSize)

	for _, windowExecutions := range windows {
		if len(windowExecutions) == 0 {
			continue
		}

		successful := 0
		for _, exec := range windowExecutions {
			if exec.Success {
				successful++
			}
		}

		successRate := float64(successful) / float64(len(windowExecutions))
		successRates = append(successRates, successRate)
	}

	if len(successRates) < 3 {
		return nil
	}

	// Calculate trend
	x := make([]float64, len(successRates))
	for i := range x {
		x[i] = float64(i)
	}

	slope, _ := tsa.calculateLinearRegression(x, successRates)
	rSquared := tsa.calculateRSquared(x, successRates, slope, 0)

	trend := &shared.ResourceTrendAnalysis{
		ResourceType:   resourceType,
		Confidence:     math.Max(0, rSquared),
		Significance:   math.Abs(slope),
		Occurrences:    len(executions),
		MetricPatterns: make(map[string]*shared.MetricPattern),
	}

	return trend
}

func (tsa *TimeSeriesAnalyzer) convertToTimeSeries(data []*sharedtypes.WorkflowExecutionData) []*TimeSeriesData {
	timeSeries := make([]*TimeSeriesData, 0)

	for _, execution := range data {
		values := make(map[string]float64)

		// Add execution success as binary metric
		if execution.Success {
			values["success"] = 1.0
		} else {
			values["success"] = 0.0
		}
		values["duration_seconds"] = execution.Duration.Seconds()

		// Add resource usage if available in metrics
		if cpuUsage, exists := execution.Metrics["cpu_usage"]; exists {
			values["cpu_usage"] = cpuUsage
		}
		if memUsage, exists := execution.Metrics["memory_usage"]; exists {
			values["memory_usage"] = memUsage
		}
		if netUsage, exists := execution.Metrics["network_usage"]; exists {
			values["network_usage"] = netUsage
		}
		if storageUsage, exists := execution.Metrics["storage_usage"]; exists {
			values["storage_usage"] = storageUsage
		}

		point := &TimeSeriesData{
			Timestamp: execution.Timestamp,
			Values:    values,
			Metadata: map[string]interface{}{
				"execution_id": execution.ExecutionID,
				"workflow_id":  execution.WorkflowID,
			},
		}

		timeSeries = append(timeSeries, point)
	}

	// Sort by timestamp
	sort.Slice(timeSeries, func(i, j int) bool {
		return timeSeries[i].Timestamp.Before(timeSeries[j].Timestamp)
	})

	return timeSeries
}

func (tsa *TimeSeriesAnalyzer) detectDailyPattern(timeSeries []*TimeSeriesData) *shared.TemporalAnalysis {
	if len(timeSeries) < 24 {
		return nil
	}

	// Group by hour of day
	hourlyStats := make(map[int][]float64)

	for _, point := range timeSeries {
		hour := point.Timestamp.Hour()
		if success, exists := point.Values["success"]; exists {
			if _, hourExists := hourlyStats[hour]; !hourExists {
				hourlyStats[hour] = make([]float64, 0)
			}
			hourlyStats[hour] = append(hourlyStats[hour], success)
		}
	}

	if len(hourlyStats) < 12 { // Need data for at least half the day
		return nil
	}

	// Calculate average success rate per hour
	hourlyAverages := make(map[int]float64)
	for hour, values := range hourlyStats {
		if len(values) > 0 {
			hourlyAverages[hour] = stat.Mean(values, nil)
		}
	}

	// Find peak and valley hours
	peakTimes := make([]sharedtypes.TimeRange, 0)
	minSuccessRate := 1.0
	maxSuccessRate := 0.0

	for _, rate := range hourlyAverages {
		if rate < minSuccessRate {
			minSuccessRate = rate
		}
		if rate > maxSuccessRate {
			maxSuccessRate = rate
		}
	}

	// Identify hours with high success rates as peak times
	threshold := minSuccessRate + 0.7*(maxSuccessRate-minSuccessRate)
	for hour, rate := range hourlyAverages {
		if rate >= threshold {
			peakStart := time.Date(2000, 1, 1, hour, 0, 0, 0, time.UTC)
			peakEnd := peakStart.Add(time.Hour)
			peakTimes = append(peakTimes, sharedtypes.TimeRange{Start: peakStart, End: peakEnd})
		}
	}

	// Calculate pattern strength (variance in hourly success rates)
	rates := make([]float64, 0)
	for _, rate := range hourlyAverages {
		rates = append(rates, rate)
	}

	variance := stat.Variance(rates, nil)
	strength := math.Min(variance*4, 1.0) // Scale to 0-1

	analysis := &shared.TemporalAnalysis{
		PatternType:     "daily",
		Confidence:      strength,
		PeakTimes:       convertToPatternTimeRanges(peakTimes),
		SeasonalFactors: make(map[string]float64),
		CycleDuration:   24 * time.Hour,
	}

	// Add seasonal factors
	for hour, rate := range hourlyAverages {
		analysis.SeasonalFactors[fmt.Sprintf("hour_%d", hour)] = rate
	}

	return analysis
}

func (tsa *TimeSeriesAnalyzer) detectWeeklyPattern(timeSeries []*TimeSeriesData) *shared.TemporalAnalysis {
	if len(timeSeries) < 7*24 { // Need at least a week of hourly data
		return nil
	}

	// Group by day of week
	weeklyStats := make(map[time.Weekday][]float64)

	for _, point := range timeSeries {
		weekday := point.Timestamp.Weekday()
		if success, exists := point.Values["success"]; exists {
			if _, dayExists := weeklyStats[weekday]; !dayExists {
				weeklyStats[weekday] = make([]float64, 0)
			}
			weeklyStats[weekday] = append(weeklyStats[weekday], success)
		}
	}

	if len(weeklyStats) < 5 { // Need data for most weekdays
		return nil
	}

	// Calculate average success rate per day
	dailyAverages := make(map[time.Weekday]float64)
	for day, values := range weeklyStats {
		if len(values) > 0 {
			dailyAverages[day] = stat.Mean(values, nil)
		}
	}

	// Calculate pattern strength
	rates := make([]float64, 0)
	for _, rate := range dailyAverages {
		rates = append(rates, rate)
	}

	variance := stat.Variance(rates, nil)
	strength := math.Min(variance*7, 1.0) // Scale to 0-1

	analysis := &shared.TemporalAnalysis{
		PatternType:     "weekly",
		Confidence:      strength,
		SeasonalFactors: make(map[string]float64),
		CycleDuration:   7 * 24 * time.Hour,
	}

	// Add seasonal factors
	for day, rate := range dailyAverages {
		analysis.SeasonalFactors[day.String()] = rate
	}

	return analysis
}

func (tsa *TimeSeriesAnalyzer) detectBurstPattern(timeSeries []*TimeSeriesData) *shared.TemporalAnalysis {
	if len(timeSeries) < 10 {
		return nil
	}

	// Calculate execution frequency in time windows
	windowSize := time.Hour
	windows := make(map[time.Time]int)

	for _, point := range timeSeries {
		windowStart := point.Timestamp.Truncate(windowSize)
		windows[windowStart]++
	}

	// Convert to slice for analysis
	frequencies := make([]float64, 0)
	for _, count := range windows {
		frequencies = append(frequencies, float64(count))
	}

	// Calculate statistics
	mean := stat.Mean(frequencies, nil)
	stdDev := stat.StdDev(frequencies, nil)

	// Detect bursts (frequencies significantly above mean)
	burstThreshold := mean + 2*stdDev
	burstCount := 0

	for _, freq := range frequencies {
		if freq > burstThreshold {
			burstCount++
		}
	}

	// Pattern strength based on burst frequency
	burstRatio := float64(burstCount) / float64(len(frequencies))

	if burstRatio < 0.1 { // Less than 10% burst periods
		return nil
	}

	analysis := &shared.TemporalAnalysis{
		PatternType: "burst",
		Confidence:  burstRatio,
		SeasonalFactors: map[string]float64{
			"burst_threshold": burstThreshold,
			"burst_ratio":     burstRatio,
			"mean_frequency":  mean,
		},
		CycleDuration: windowSize,
	}

	return analysis
}

func (tsa *TimeSeriesAnalyzer) analyzeDailySeasonality(values []float64, timestamps []time.Time) *SeasonalityAnalysis {
	// Group by hour of day
	hourlyData := make(map[int][]float64)

	for i, timestamp := range timestamps {
		hour := timestamp.Hour()
		if _, exists := hourlyData[hour]; !exists {
			hourlyData[hour] = make([]float64, 0)
		}
		hourlyData[hour] = append(hourlyData[hour], values[i])
	}

	// Calculate hourly averages
	hourlyAverages := make([]float64, 24)
	validHours := 0

	for hour := 0; hour < 24; hour++ {
		if data, exists := hourlyData[hour]; exists && len(data) > 0 {
			hourlyAverages[hour] = stat.Mean(data, nil)
			validHours++
		}
	}

	if validHours < 12 {
		return &SeasonalityAnalysis{Pattern: "daily", Strength: 0.0}
	}

	// Calculate seasonality strength
	overallMean := stat.Mean(hourlyAverages, nil)
	variance := 0.0
	for _, avg := range hourlyAverages {
		variance += math.Pow(avg-overallMean, 2)
	}
	variance /= float64(len(hourlyAverages))

	strength := math.Min(variance/overallMean, 1.0)

	return &SeasonalityAnalysis{
		Pattern:    "daily",
		Strength:   strength,
		Period:     24 * time.Hour,
		Confidence: float64(validHours) / 24.0,
	}
}

func (tsa *TimeSeriesAnalyzer) analyzeWeeklySeasonality(values []float64, timestamps []time.Time) *SeasonalityAnalysis {
	// Group by day of week
	weeklyData := make(map[time.Weekday][]float64)

	for i, timestamp := range timestamps {
		weekday := timestamp.Weekday()
		if _, exists := weeklyData[weekday]; !exists {
			weeklyData[weekday] = make([]float64, 0)
		}
		weeklyData[weekday] = append(weeklyData[weekday], values[i])
	}

	// Calculate daily averages
	dailyAverages := make([]float64, 7)
	validDays := 0

	for day := time.Sunday; day <= time.Saturday; day++ {
		if data, exists := weeklyData[day]; exists && len(data) > 0 {
			dailyAverages[int(day)] = stat.Mean(data, nil)
			validDays++
		}
	}

	if validDays < 5 {
		return &SeasonalityAnalysis{Pattern: "weekly", Strength: 0.0}
	}

	// Calculate seasonality strength
	overallMean := stat.Mean(dailyAverages, nil)
	variance := 0.0
	for _, avg := range dailyAverages {
		variance += math.Pow(avg-overallMean, 2)
	}
	variance /= float64(len(dailyAverages))

	strength := math.Min(variance/overallMean, 1.0)

	return &SeasonalityAnalysis{
		Pattern:    "weekly",
		Strength:   strength,
		Period:     7 * 24 * time.Hour,
		Confidence: float64(validDays) / 7.0,
	}
}

func (tsa *TimeSeriesAnalyzer) groupExecutionsByTimeWindow(executions []*sharedtypes.WorkflowExecutionData, windowSize time.Duration) map[time.Time][]*sharedtypes.WorkflowExecutionData {
	windows := make(map[time.Time][]*sharedtypes.WorkflowExecutionData)

	for _, execution := range executions {
		windowStart := execution.Timestamp.Truncate(windowSize)
		if _, exists := windows[windowStart]; !exists {
			windows[windowStart] = make([]*sharedtypes.WorkflowExecutionData, 0)
		}
		windows[windowStart] = append(windows[windowStart], execution)
	}

	return windows
}

func (tsa *TimeSeriesAnalyzer) calculateLinearRegression(x, y []float64) (slope, intercept float64) {
	if len(x) != len(y) || len(x) < 2 {
		return 0, 0
	}

	n := float64(len(x))
	sumX := sharedmath.Mean(x) * n
	sumY := sharedmath.Mean(y) * n
	sumXY := 0.0
	sumXX := 0.0

	for i := 0; i < len(x); i++ {
		sumXY += x[i] * y[i]
		sumXX += x[i] * x[i]
	}

	// Calculate slope and intercept
	slope = (n*sumXY - sumX*sumY) / (n*sumXX - sumX*sumX)
	intercept = (sumY - slope*sumX) / n

	return slope, intercept
}

func (tsa *TimeSeriesAnalyzer) calculateRSquared(x, y []float64, slope, intercept float64) float64 {
	if len(x) != len(y) || len(x) < 2 {
		return 0
	}

	yMean := stat.Mean(y, nil)
	ssRes := 0.0 // Sum of squares of residuals
	ssTot := 0.0 // Total sum of squares

	for i := 0; i < len(x); i++ {
		predicted := slope*x[i] + intercept
		ssRes += math.Pow(y[i]-predicted, 2)
		ssTot += math.Pow(y[i]-yMean, 2)
	}

	if ssTot == 0 {
		return 1.0 // Perfect fit if no variance in y
	}

	rSquared := 1.0 - (ssRes / ssTot)
	return math.Max(0, rSquared) // Ensure non-negative
}

func (tsa *TimeSeriesAnalyzer) calculateResidualStdDev(x, y []float64, slope, intercept float64) float64 {
	if len(x) != len(y) || len(x) < 3 {
		return 1.0 // Default uncertainty
	}

	residuals := make([]float64, len(x))
	for i := 0; i < len(x); i++ {
		predicted := slope*x[i] + intercept
		residuals[i] = y[i] - predicted
	}

	return stat.StdDev(residuals, nil)
}

func (tsa *TimeSeriesAnalyzer) calculateBaselineStats(values []float64) *BaselineStatistics {
	if len(values) == 0 {
		return &BaselineStatistics{}
	}

	// Sort values for percentile calculations
	sorted := make([]float64, len(values))
	copy(sorted, values)
	sort.Float64s(sorted)

	stats := &BaselineStatistics{
		Mean:   stat.Mean(values, nil),
		StdDev: stat.StdDev(values, nil),
		Min:    sorted[0],
		Max:    sorted[len(sorted)-1],
	}

	// Calculate median and quartiles
	n := len(sorted)
	if n%2 == 0 {
		stats.Median = (sorted[n/2-1] + sorted[n/2]) / 2
	} else {
		stats.Median = sorted[n/2]
	}

	// Calculate Q1 and Q3
	if n >= 4 {
		stats.Q1 = sorted[n/4]
		stats.Q3 = sorted[3*n/4]
		stats.IQR = stats.Q3 - stats.Q1
	}

	return stats
}

func (tsa *TimeSeriesAnalyzer) sortTimeSeriesByTimestamp(values []float64, timestamps []time.Time) {
	// Create a slice of indices
	indices := make([]int, len(timestamps))
	for i := range indices {
		indices[i] = i
	}

	// Sort indices by timestamp
	sort.Slice(indices, func(i, j int) bool {
		return timestamps[indices[i]].Before(timestamps[indices[j]])
	})

	// Reorder both slices according to sorted indices
	sortedValues := make([]float64, len(values))
	sortedTimestamps := make([]time.Time, len(timestamps))

	for i, idx := range indices {
		sortedValues[i] = values[idx]
		sortedTimestamps[i] = timestamps[idx]
	}

	// Copy back to original slices
	copy(values, sortedValues)
	copy(timestamps, sortedTimestamps)
}

// Supporting types for time series analysis

type CapacityPattern struct {
	ResourceType        string                  `json:"resource_type"`
	CapacityTrend       string                  `json:"capacity_trend"` // "approaching_limit", "stable", "decreasing"
	UtilizationPeaks    []sharedtypes.TimeRange `json:"utilization_peaks"`
	ScalingTriggers     []float64               `json:"scaling_triggers"` // Threshold values
	PredictedExhaustion *time.Time              `json:"predicted_exhaustion,omitempty"`
	Recommendations     []string                `json:"recommendations"`
}

type ScalingPattern struct {
	TriggerConditions    []string      `json:"trigger_conditions"`
	ScalingFrequency     time.Duration `json:"scaling_frequency"`
	AverageScaleUp       int           `json:"average_scale_up"`
	AverageScaleDown     int           `json:"average_scale_down"`
	OptimalCapacity      int           `json:"optimal_capacity"`
	ScalingEffectiveness float64       `json:"scaling_effectiveness"`
}
