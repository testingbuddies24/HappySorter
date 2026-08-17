package pipeline

import "testing"

func TestFilter(t *testing.T) {
	const validSize = 60 * 1024 * 1024

	cases := []struct {
		name       string
		path       string
		size       int64
		wantAccept bool
		wantReason string
	}{
		{"empty file", "SSIS-001.mp4", 0, false, "empty file"},
		{"junk extension", "readme.txt", validSize, false, "junk extension .txt"},
		{"disallowed extension", "cover.jpg", validSize, false, "extension not in video allow-list: .jpg"},
		{"below size floor", "SSIS-001.mp4", 10 * 1024 * 1024, false, "below 50 MB size floor"},
		{"sample pattern", "SSIS-001-sample.mp4", validSize, false, "filename matches junk pattern: sample"},
		{"spaced ad clip", "苍 老 师 强 力 推 荐.mp4", validSize, false, "filename matches junk pattern: 苍老师"},
		{"glued ad clip", "苍老师强力推荐.mp4", validSize, false, "filename matches junk pattern: 苍老师"},
		{"real video accepted", "SSIS-001.mp4", validSize, true, ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Filter(tc.path, tc.size)
			if got.Accepted != tc.wantAccept {
				t.Errorf("Filter(%q, %d).Accepted = %v, want %v (reason: %q)", tc.path, tc.size, got.Accepted, tc.wantAccept, got.Reason)
			}
			if !tc.wantAccept && got.Reason != tc.wantReason {
				t.Errorf("Filter(%q, %d).Reason = %q, want %q", tc.path, tc.size, got.Reason, tc.wantReason)
			}
		})
	}
}
