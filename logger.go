package crud

import (
	"fmt"
	"maps"
	"sort"
	"strings"
)

type Fields map[string]any

type loggerWithFields interface {
	WithFields(Fields) Logger
}

type Logger interface {
	Debug(format string, args ...any)
	Info(format string, args ...any)
	Error(format string, args ...any)
}

var LoggerEnabled = false

type defaultLogger struct {
	fields Fields
}

func (d *defaultLogger) Debug(format string, args ...any) {
	d.log("DEBUG", format, args...)
}

func (d *defaultLogger) Info(format string, args ...any) {
	d.log("INFO", format, args...)
}

func (d *defaultLogger) Error(format string, args ...any) {
	d.log("ERROR", format, args...)
}

func (d *defaultLogger) WithFields(fields Fields) Logger {
	if len(fields) == 0 {
		return d
	}

	merged := make(Fields, len(d.fields)+len(fields))
	maps.Copy(merged, d.fields)
	maps.Copy(merged, fields)

	return &defaultLogger{fields: merged}
}

func (d *defaultLogger) log(level string, format string, args ...any) {
	if !LoggerEnabled {
		return
	}

	message := fmt.Sprintf(format, args...)
	if len(d.fields) == 0 {
		fmt.Printf("[%s] %s\n", level, message)
		return
	}

	fmt.Printf("[%s] %s %s\n", level, message, d.formatFields())
}

func (d *defaultLogger) formatFields() string {
	if len(d.fields) == 0 {
		return ""
	}

	keys := make([]string, 0, len(d.fields))
	for k := range d.fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, fmt.Sprintf("%s=%v", key, d.fields[key]))
	}

	return fmt.Sprintf("{%s}", strings.Join(parts, ", "))
}
