package entry

import (
	pkgPath "path"
	"strings"
)

const EmptyPath = " "

// Processor handles path manipulation for DynamoDB key/path decomposition.
type Processor struct{}

func NewProcessor() *Processor {
	return &Processor{}
}

// Prefixes returns all parent "folders" for a given key.
// e.g. for "foo/bar/baz" returns ["foo", "foo/bar"].
func (p *Processor) Prefixes(s string) []string {
	components := strings.Split(s, "/")
	var result []string
	for i := 1; i < len(components); i++ {
		result = append(result, strings.Join(components[:i], "/"))
	}
	return result
}

func (p *Processor) UnescapeEmptyPath(s string) string {
	if s == EmptyPath {
		return ""
	}
	return s
}

func (p *Processor) EscapeEmptyPath(s string) string {
	if s == "" {
		return EmptyPath
	}
	return s
}

// PathWithoutKey returns the "directory" portion of a key (everything before the last "/").
func (p *Processor) PathWithoutKey(key string) string {
	if strings.Contains(key, "/") {
		return pkgPath.Dir(key)
	}
	return EmptyPath
}

// BaseKey returns the last component of a key.
func (p *Processor) BaseKey(key string) string {
	return pkgPath.Base(key)
}

// Concat reconstructs a full key from its path and key components.
func (p *Processor) Concat(path, key string) string {
	unp := p.UnescapeEmptyPath(path)
	if unp == "" {
		return key
	}
	fullKey := pkgPath.Join(path, key)
	if strings.HasSuffix(key, "/") {
		fullKey += "/"
	}
	return fullKey
}

func (p *Processor) NormalizeKey(key string) string {
	return pkgPath.Join("/", strings.TrimSpace(key))
}
