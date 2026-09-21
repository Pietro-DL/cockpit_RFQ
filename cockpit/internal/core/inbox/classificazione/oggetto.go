package classificazione

import (
	"regexp"
	"strings"
)

var rePrefissi = regexp.MustCompile(`(?i)^((re|r|fw|fwd|i|tr|aw|wg)\s*:\s*)+`)

// OggettoPulito toglie i prefissi RE:/FW:/I: ripetuti dall'oggetto della mail.
func OggettoPulito(oggetto string) string {
	return strings.TrimSpace(rePrefissi.ReplaceAllString(strings.TrimSpace(oggetto), ""))
}
