package httpheaders

import "strings"

func splitEtagList(value string) []string {
	tags := make([]string, 0, 1)
	var b strings.Builder
	inQuotes := false

	for _, r := range value {
		switch {
		case r == '"':
			inQuotes = !inQuotes
			b.WriteRune(r)
		case r == ',' && !inQuotes:
			if s := strings.TrimSpace(b.String()); s != "" {
				tags = append(tags, s)
			}
			b.Reset()
		default:
			b.WriteRune(r)
		}
	}
	if s := strings.TrimSpace(b.String()); s != "" {
		tags = append(tags, s)
	}

	return tags
}

func etagsMatch(etag1, etag2 string) bool {
	if etag1 == etag2 {
		return true
	}
	if len(etag1) > 2 && etag1[:2] == "W/" {
		return etag1[2:] == etag2
	}
	if len(etag2) > 2 && etag2[:2] == "W/" {
		return etag1 == etag2[2:]
	}
	return false
}

func EtagMatchesAny(etag string, headerValues []string) bool {
	for _, line := range headerValues {
		for _, tag := range splitEtagList(line) {
			if tag == "*" {
				return true
			}
			if etag != "" && etagsMatch(etag, tag) {
				return true
			}
		}
	}
	return false
}
