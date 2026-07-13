package pipeline

import (
	"path/filepath"
	"strings"
)

var videoExtensions = map[string]bool{
	".mp4": true, ".mkv": true, ".avi": true, ".wmv": true,
	".mov": true, ".flv": true, ".rmvb": true, ".ts": true,
}

var junkExtensions = map[string]bool{
	".url": true, ".txt": true, ".html": true, ".part": true, ".torrent": true,
}

// junkPatterns are matched as substrings against the lowercased filename
// (SPEC.md § F2).
var junkPatterns = []string{"sample", "trailer", "preview", "字幕"}

const minVideoBytes = 50 * 1024 * 1024 // 50 MB size floor, SPEC.md § F2

// FilterResult is the rubbish filter's verdict on one file.
type FilterResult struct {
	Accepted bool
	Reason   string // populated when Accepted is false
}

// Filter applies the rubbish-filter rules from SPEC.md § F2. size is the
// file's size in bytes.
func Filter(path string, size int64) FilterResult {
	if size == 0 {
		return FilterResult{Reason: "empty file"}
	}

	ext := strings.ToLower(filepath.Ext(path))
	if junkExtensions[ext] {
		return FilterResult{Reason: "junk extension " + ext}
	}
	if !videoExtensions[ext] {
		return FilterResult{Reason: "extension not in video allow-list: " + ext}
	}
	if size < minVideoBytes {
		return FilterResult{Reason: "below 50 MB size floor"}
	}

	name := strings.ToLower(filepath.Base(path))
	for _, p := range junkPatterns {
		if strings.Contains(name, strings.ToLower(p)) {
			return FilterResult{Reason: "filename matches junk pattern: " + p}
		}
	}

	return FilterResult{Accepted: true}
}
