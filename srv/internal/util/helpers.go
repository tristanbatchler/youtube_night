package util

import (
	"strings"
	"time"
)

// If returns trueVal if condition is true, falseVal otherwise
// This is helpful for conditional HTML/template attributes
func If(condition bool, trueVal, falseVal string) string {
	if condition {
		return trueVal
	}
	return falseVal
}

// TruncateString truncates a string to the specified length with an ellipsis (...)
func TruncateString(s string, length int) string {
	if len(s) <= length {
		return s
	}
	return s[:length] + "..."
}

// Contains checks if a slice of strings contains a specific string
func Contains(slice []string, str string) bool {
	for _, item := range slice {
		if item == str {
			return true
		}
	}
	return false
}

// JoinStrings joins a slice of strings into a single string separated by the given delimiter
func JoinStrings(slice []string, delimiter string) string {
	return strings.Join(slice, delimiter)
}

// SplitString splits a string into a slice of strings by the given delimiter
func SplitString(s, delimiter string) []string {
	return strings.Split(s, delimiter)
}

// HasuraDateTimeFormat is the layout for parsing and formatting Hasura's timestamp with time zone
const HasuraDateTimeFormat = "2006-01-02T15:04:05.999999999Z07:00"

// ParseHasuraDateTime parses a string into a Time value using Hasura's timestamp with time zone format
func ParseHasuraDateTime(value string) (time.Time, error) {
	return time.Parse(HasuraDateTimeFormat, value)
}

// FormatHasuraDateTime formats a Time value into a string using Hasura's timestamp with time zone format
func FormatHasuraDateTime(value time.Time) string {
	return value.Format(HasuraDateTimeFormat)
}
