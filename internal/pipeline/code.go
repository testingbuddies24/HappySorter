package pipeline

import (
	"path/filepath"
	"regexp"
	"strings"
)

// codeRegex matches a normalised filename that is exactly a JAV code
// (SPEC.md § F3): 2-5 letters/digits, optional hyphen, 2-5 digits.
var codeRegex = regexp.MustCompile(`^([A-Z0-9]{2,5})-?(\d{2,5})$`)

// trailingVariant matches a hyphen-separated, purely-alphabetic variant marker
// at the end of the name (e.g. "-AI", "-UC", "-CH", "-C"). Stripped in a loop
// so stacked markers clear ("-UC-AI"). Alpha-only on purpose: digit-bearing
// tails like "-CD1" or "-2" denote multi-part files and must survive, and the
// code's own trailing number is digits so it is never eaten (SPEC.md § F3).
var trailingVariant = regexp.MustCompile(`-[A-Z]{1,4}$`)

// gluedSuffixes are hyphen-less quality markers stripped from the end of the
// normalised name (the hyphenated forms are handled by trailingVariant).
var gluedSuffixes = []string{"FHD", "HD"}

// trailingBracket matches a square-bracketed group at the end of the name —
// release dates ("[2026-07-03]"), quality tags ("[1080P]"). Stripped in the
// same loop as variant markers so stacked tags clear. End-anchored on
// purpose: leading site tags like "[HHD800.COM]DASS-996" must keep failing
// the anchored code regex below. Parens are deliberately NOT included here:
// "(1)"/"(2)" is ambiguous with multi-part releases, and trailingVariant's
// digit-bearing-tail exclusion (see above) exists specifically to leave
// those for manual review rather than guess.
var trailingBracket = regexp.MustCompile(`\s*\[[^\]]*\]\s*$`)

// ExtractCode normalises path's filename and attempts to pull a JAV code
// out of it. Returns the canonical "PREFIX-NUMBER" form and true on match.
func ExtractCode(path string) (string, bool) {
	name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	name = strings.ToUpper(strings.TrimSpace(name))

	// Strip trailing variant/quality/bracket markers until the name stops
	// shrinking. The regex stays fully anchored below, so anything that is
	// not a bare code after this (e.g. "HHD800.COM-DASS-996") correctly
	// falls through to review rather than yielding a false code.
	for {
		stripped := trailingBracket.ReplaceAllString(name, "")
		stripped = strings.TrimSpace(stripped)
		stripped = trailingVariant.ReplaceAllString(stripped, "")
		for _, suf := range gluedSuffixes {
			stripped = strings.TrimSuffix(stripped, suf)
		}
		stripped = strings.TrimSpace(stripped)
		if stripped == name {
			break
		}
		name = stripped
	}

	m := codeRegex.FindStringSubmatch(name)
	if m == nil {
		return "", false
	}
	return m[1] + "-" + m[2], true
}
