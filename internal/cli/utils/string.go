package utils

import (
	"regexp"
	"strings"
)

var firstCamelCaseRegex = regexp.MustCompile("(.)([A-Z][a-z]+)")
var camelCaseRegex = regexp.MustCompile("([a-z0-9])([A-Z])")

func PascalCaseToSnakeCase(str string) string {
	snake := firstCamelCaseRegex.ReplaceAllString(str, "${1}_${2}")
	snake = camelCaseRegex.ReplaceAllString(snake, "${1}_${2}")
	return strings.ToLower(snake)
}
