package pipeline

import (
	"path/filepath"
	"regexp"
	"strings"
)

// domainSubstr matches a dotted hostname-shaped run (e.g. "NYAP2P.COM",
// "HHD800.COM", "MASEX.TV") anywhere in the name, so it can be blanked out
// before tokenizing. Applied as a substring replace on the whole name
// (not a per-token match) because tokenizing itself splits on ".", which
// would otherwise break "HHD800.COM" into "HHD800"/"COM" before this ever
// gets a chance to recognise it as one unit. Deliberately excludes "-" from
// the character class: "FJIN-148-UNCENSORED-NYAP2P.COM" must only lose the
// "NYAP2P.COM" run, not the code's own hyphen-joined prefix along with it.
var domainSubstr = regexp.MustCompile(`[A-Z0-9]+(?:\.[A-Z0-9]+)+`)

// separator splits a normalised name into tokens on anything that isn't a
// letter or digit — hyphens, underscores, spaces, brackets, dots, CJK text.
var separator = regexp.MustCompile(`[^A-Z0-9]+`)

// prefixToken is a bare label token: 2-5 letters/digits containing at least
// one letter (SPEC.md § F3). The letter requirement rejects date fragments
// like "2026" or "07" from ever pairing with a following number as a code.
var prefixToken = regexp.MustCompile(`^[A-Z0-9]{2,5}$`)

// numTail matches the code's number token, with an optional glued
// alpha-only quality/variant marker ("049", "049CH", "222HD"). The marker is
// discarded — the code is normalised without it.
var numTail = regexp.MustCompile(`^(\d{2,5})(?:[A-Z]{1,4})?$`)

// gluedCode matches a whole token that is a label and number fused with no
// separator ("IPZZ729", "FNS222HD") — letters-then-digits only, so the split
// point is never ambiguous.
var gluedCode = regexp.MustCompile(`^([A-Z]{2,5})(\d{2,5})(?:[A-Z]{1,4})?$`)

// partToken matches a multi-part/disc marker token following a matched code
// ("1", "2", "CD1", "DVD2"). Digit-terminated on purpose: single-letter or
// "4K"-style tokens are quality/variant markers, not parts.
var partToken = regexp.MustCompile(`^(?:CD|DVD|PART)?\d{1,2}$`)

// fc2Num matches an FC2 PPV numeric id (6-8 digits).
var fc2Num = regexp.MustCompile(`^\d{6,8}$`)

// ExtractCode normalises path's filename and attempts to pull a JAV code out
// of it, tokenizing on any non-alphanumeric run and scanning for the first
// token (or token pair) that looks like a code. This tolerates surrounding
// noise — release-site prefixes/suffixes, quality tags, uncensored/CH
// markers, watermark domains — that a fully-anchored match would reject
// outright. If the filename itself yields nothing, falls back to the same
// scan against the immediate parent folder name, since torrents commonly
// carry the clean code there even when the video file itself doesn't
// (e.g. "rlmp-005/masex.tv@rlmp-005.mp4").
//
// Returns the canonical "PREFIX-NUMBER" code, an optional part marker
// ("1", "CD1", ...) for multi-part releases, and true on match.
func ExtractCode(path string) (code, part string, ok bool) {
	name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	if code, part, ok := extractFromName(name); ok {
		return code, part, ok
	}

	parentBase := filepath.Base(filepath.Dir(path))
	parentName := strings.TrimSuffix(parentBase, filepath.Ext(parentBase))
	return extractFromName(parentName)
}

func extractFromName(name string) (code, part string, ok bool) {
	name = strings.ToUpper(strings.TrimSpace(name))
	if i := strings.LastIndex(name, "@"); i >= 0 {
		name = name[i+1:]
	}
	name = domainSubstr.ReplaceAllString(name, " ")

	var tokens []string
	for _, t := range separator.Split(name, -1) {
		if t != "" {
			tokens = append(tokens, t)
		}
	}

	for i, t := range tokens {
		if t == "FC2" {
			j := i + 1
			if j < len(tokens) && tokens[j] == "PPV" {
				j++
			}
			if j < len(tokens) && fc2Num.MatchString(tokens[j]) {
				return "FC2-PPV-" + tokens[j], "", true
			}
			continue
		}

		if prefixToken.MatchString(t) && containsLetter(t) && i+1 < len(tokens) {
			if m := numTail.FindStringSubmatch(tokens[i+1]); m != nil {
				part := ""
				if i+2 < len(tokens) && partToken.MatchString(tokens[i+2]) {
					part = tokens[i+2]
				}
				return t + "-" + m[1], part, true
			}
		}

		if m := gluedCode.FindStringSubmatch(t); m != nil {
			return m[1] + "-" + m[2], "", true
		}
	}

	return "", "", false
}

func containsLetter(s string) bool {
	for _, r := range s {
		if r >= 'A' && r <= 'Z' {
			return true
		}
	}
	return false
}
