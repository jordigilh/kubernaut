package engine

import "github.com/sirupsen/logrus"

// LogrusAdapter adapts logrus.Logger to the Logger interface
// Following guideline #11: reuse existing code patterns
type LogrusAdapter struct {
	logger *logrus.Logger
}

// NewLogrusAdapter creates a new logger adapter
func NewLogrusAdapter(logger *logrus.Logger) Logger {
	return &LogrusAdapter{logger: logger}
}

// WithField implements Logger interface
func (la *LogrusAdapter) WithField(key string, value interface{}) Logger {
	return &LogrusEntryAdapter{entry: la.logger.WithField(key, value)}
}

// WithFields implements Logger interface
func (la *LogrusAdapter) WithFields(fields map[string]interface{}) Logger {
	return &LogrusEntryAdapter{entry: la.logger.WithFields(fields)}
}

// Debug implements Logger interface
func (la *LogrusAdapter) Debug(args ...interface{}) {
	la.logger.Debug(args...)
}

// Info implements Logger interface
func (la *LogrusAdapter) Info(args ...interface{}) {
	la.logger.Info(args...)
}

// Warn implements Logger interface
func (la *LogrusAdapter) Warn(args ...interface{}) {
	la.logger.Warn(args...)
}

// Error implements Logger interface
func (la *LogrusAdapter) Error(args ...interface{}) {
	la.logger.Error(args...)
}

// WithError implements Logger interface
func (la *LogrusAdapter) WithError(err error) Logger {
	return &LogrusEntryAdapter{entry: la.logger.WithError(err)}
}

// LogrusEntryAdapter adapts logrus.Entry to the Logger interface
type LogrusEntryAdapter struct {
	entry *logrus.Entry
}

// WithField implements Logger interface
func (lea *LogrusEntryAdapter) WithField(key string, value interface{}) Logger {
	return &LogrusEntryAdapter{entry: lea.entry.WithField(key, value)}
}

// WithFields implements Logger interface
func (lea *LogrusEntryAdapter) WithFields(fields map[string]interface{}) Logger {
	return &LogrusEntryAdapter{entry: lea.entry.WithFields(fields)}
}

// Debug implements Logger interface
func (lea *LogrusEntryAdapter) Debug(args ...interface{}) {
	lea.entry.Debug(args...)
}

// Info implements Logger interface
func (lea *LogrusEntryAdapter) Info(args ...interface{}) {
	lea.entry.Info(args...)
}

// Warn implements Logger interface
func (lea *LogrusEntryAdapter) Warn(args ...interface{}) {
	lea.entry.Warn(args...)
}

// Error implements Logger interface
func (lea *LogrusEntryAdapter) Error(args ...interface{}) {
	lea.entry.Error(args...)
}

// WithError implements Logger interface
func (lea *LogrusEntryAdapter) WithError(err error) Logger {
	return &LogrusEntryAdapter{entry: lea.entry.WithError(err)}
}
