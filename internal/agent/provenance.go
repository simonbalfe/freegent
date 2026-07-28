package agent

import (
	"net/url"
	"regexp"
	"sort"
	"strings"
)

var templateFieldPattern = regexp.MustCompile(`\{\{\s*([\w.]+)\s*\}\}`)

func fillTemplate(template string, row Row) string {
	return templateFieldPattern.ReplaceAllStringFunc(template, func(token string) string {
		key := strings.TrimSpace(strings.Trim(strings.TrimSuffix(strings.TrimPrefix(token, "{{"), "}}"), " "))
		if value := row[key]; value != "" {
			return value
		}
		return "[MISSING:" + key + "]"
	})
}

type urlLedger struct {
	sourceURLs map[string]string
	seen       map[string]bool
}

func newURLLedger(row Row) urlLedger {
	ledger := urlLedger{sourceURLs: map[string]string{}, seen: map[string]bool{}}
	for _, value := range row {
		ledger.addSeen(value)
	}
	return ledger
}

func (l urlLedger) addSource(rawURL string) {
	if normalized := normalizeURL(rawURL); normalized != "" {
		l.sourceURLs[normalized] = rawURL
		l.seen[normalized] = true
	}
}

func (l urlLedger) addSeen(rawURL string) {
	if normalized := normalizeURL(rawURL); normalized != "" {
		l.seen[normalized] = true
	}
}

func (l urlLedger) permits(rawURL string) bool {
	_, ok := l.seen[normalizeURL(rawURL)]
	return ok
}

func (l urlLedger) sources() []string {
	values := make([]string, 0, len(l.sourceURLs))
	for _, value := range l.sourceURLs {
		values = append(values, value)
	}
	sort.Strings(values)
	return values
}

func normalizeURL(rawURL string) string {
	rawURL = strings.TrimSpace(rawURL)
	if !strings.Contains(rawURL, "://") && !strings.ContainsAny(rawURL, " @/") && strings.Contains(rawURL, ".") {
		rawURL = "https://" + rawURL
	}
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Host == "" {
		return ""
	}
	return strings.ToLower(strings.TrimPrefix(parsed.Host, "www.")) + strings.TrimSuffix(strings.ToLower(parsed.Path), "/")
}
