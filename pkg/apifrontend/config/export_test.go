package config

import "github.com/go-logr/logr"

// FileWatcherLoggerForTest returns the logger stored in a FileWatcher.
func FileWatcherLoggerForTest(fw *FileWatcher) logr.Logger {
	return fw.logger
}
