package usecases

import (
	pkgPath "path"
	"strings"
)

const EmptyPath = " "

type PathUseCase struct{}

func NewPathUseCase() *PathUseCase {
	return &PathUseCase{}
}

// Prefixes is a shared helper function returns all parent 'folders' for a
// given vault key.
// e.g. for 'foo/bar/baz', it returns ['foo', 'foo/bar'].
func (p *PathUseCase) Prefixes(s string) []string {
	components := strings.Split(s, "/")
	var result []string
	for i := 1; i < len(components); i++ {
		result = append(result, strings.Join(components[:i], "/"))
	}
	return result
}

// UnescapeEmptyPath is the opposite of `escapeEmptyPath`.
func (p *PathUseCase) UnescapeEmptyPath(s string) string {
	if s == EmptyPath {
		return ""
	}
	return s
}

// EscapeEmptyPath is used to escape the root key's path
// with a value that can be stored in DynamoDB. DynamoDB
// does not allow values to be empty strings.
func (p *PathUseCase) EscapeEmptyPath(s string) string {
	if s == "" {
		return EmptyPath
	}
	return s
}

// PathWithoutKey recordPathForVaultKey transforms a vault key into
// a value suitable for the `DynamoDBRecord`'s `Path`
// property. This path equals the vault key without
// its last component.
func (p *PathUseCase) PathWithoutKey(key string) string {
	if strings.Contains(key, "/") {
		return pkgPath.Dir(key)
	}
	return EmptyPath
}

// BaseKey recordKeyForVaultKey transforms a vault key into
// a value suitable for the `DynamoDBRecord`'s `Key`
// property. This path equals the vault key's
// last component.
func (p *PathUseCase) BaseKey(key string) string {
	return pkgPath.Base(key)
}

// Concat vaultKey returns the vault key for a given record
// from the DynamoDB table. This is the combination of
// the records Path and Key.
func (p *PathUseCase) Concat(path, key string) string {
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

func (p *PathUseCase) NormalizeKey(key string) string {
	return pkgPath.Join("/", strings.TrimSpace(key))
}
