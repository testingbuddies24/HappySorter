package pipeline

import (
	"path/filepath"
	"regexp"
	"strings"
)

// codeRegex matches a normalised filename that is exactly a JAV code
// (SPEC.md § F3): 2-5 letters/digits, optional hyphen, 2-5 digits.
var codeRegex = regexp.MustCompile(`^([A-Z0-9]{2,5})-?(\d{2,5})$`)

// releaseSuffixes are stripped from the end of the normalised name before
// matching (SPEC.md § F3).
var releaseSuffixes = []string{"-CH", "-UC", "-JP", "-HD", "-FHD", "HD", "FHD"}

// ExtractCode normalises path's filename and attempts to pull a JAV code
// out of it. Returns the canonical "PREFIX-NUMBER" form and true on match.
func ExtractCode(path string) (string, bool) {
	name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	name = strings.ToUpper(strings.TrimSpace(name))

	for _, suf := range releaseSuffixes {
		if trimmed := strings.TrimSuffix(name, suf); trimmed != name {
			name = trimmed
			break
		}
	}

	m := codeRegex.FindStringSubmatch(name)
	if m == nil {
		return "", false
	}
	return m[1] + "-" + m[2], true
}
