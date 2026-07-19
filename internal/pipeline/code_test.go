package pipeline

import "testing"

func TestExtractCode(t *testing.T) {
	cases := []struct {
		name     string
		wantCode string
		wantOK   bool
	}{
		// Plain codes.
		{"IPZZ-729.mp4", "IPZZ-729", true},
		{"DASS-996.mkv", "DASS-996", true},

		// Trailing alpha variant markers are stripped.
		{"DASS-996-AI.mp4", "DASS-996", true},
		{"FNS-222-UC.mp4", "FNS-222", true},
		{"START-001-CH.mp4", "START-001", true},
		{"DASS-996-UC-AI.mp4", "DASS-996", true}, // stacked markers

		// Glued quality markers.
		{"FNS-222HD.mp4", "FNS-222", true},
		{"FNS-222FHD.mkv", "FNS-222", true},

		// False-positive guards: must NOT yield a code (route to review).
		{"HHD800.COM-DASS-996-AI.mp4", "", false}, // release-site prefix
		{"DASS-996-CD1.mp4", "", false},           // digit-bearing tail = multi-part, left for review
		{"vacation-clip.mp4", "", false},          // no code at all
		{"trailer.mp4", "", false},
	}

	for _, tc := range cases {
		gotCode, gotOK := ExtractCode(tc.name)
		if gotCode != tc.wantCode || gotOK != tc.wantOK {
			t.Errorf("ExtractCode(%q) = (%q, %v), want (%q, %v)",
				tc.name, gotCode, gotOK, tc.wantCode, tc.wantOK)
		}
	}
}
