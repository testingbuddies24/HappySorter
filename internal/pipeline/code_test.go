package pipeline

import "testing"

func TestExtractCode(t *testing.T) {
	cases := []struct {
		name     string
		wantCode string
		wantPart string
		wantOK   bool
	}{
		// Plain codes.
		{"IPZZ-729.mp4", "IPZZ-729", "", true},
		{"DASS-996.mkv", "DASS-996", "", true},

		// Trailing alpha variant markers no longer need stripping — the code
		// token is matched before the marker is ever consulted.
		{"DASS-996-AI.mp4", "DASS-996", "", true},
		{"FNS-222-UC.mp4", "FNS-222", "", true},
		{"START-001-CH.mp4", "START-001", "", true},
		{"DASS-996-UC-AI.mp4", "DASS-996", "", true},

		// Glued quality markers.
		{"FNS-222HD.mp4", "FNS-222", "", true},
		{"FNS-222FHD.mkv", "FNS-222", "", true},

		// Bracketed suffixes.
		{"CAWD-991 [2026-07-03].mp4", "CAWD-991", "", true},
		{"CAWD-991 [1080P] [2026-07-03].mp4", "CAWD-991", "", true},
		{"DASS-996-AI [2026-07-03].mp4", "DASS-996", "", true},

		// Previously false-positive guards, now conscious reversals: these
		// ARE the real code, and rejecting them was the user-reported pain.
		{"HHD800.COM-DASS-996-AI.mp4", "DASS-996", "", true},
		{"[HHD800.COM] DASS-996.mp4", "DASS-996", "", true},
		{"DASS-996-CD1.mp4", "DASS-996", "CD1", true},
		{"DASS-996-CD1 [2026-07-03].mp4", "DASS-996", "CD1", true},
		{"CAWD-991 (1).mp4", "CAWD-991", "1", true},

		// Still no code at all.
		{"vacation-clip.mp4", "", "", false},
		{"trailer.mp4", "", "", false},
		{"2026-07-03.mp4", "", "", false},

		// --- Real production filenames from the 2026-08-17 log review ---

		// Suffix-after-code: uncensored/CH/4K markers, with and without a
		// trailing site-domain token.
		{"ofes-049ch.mp4", "OFES-049", "", true},
		{"START-459_CH-nyap2p.com.mp4", "START-459", "", true},
		{"SONE-982_CH-nyap2p.com.mp4", "SONE-982", "", true},
		{"FJIN-148-uncensored-nyap2p.com.mp4", "FJIN-148", "", true},
		{"DLDSS-369-uncensored-nyap2p.com.mp4", "DLDSS-369", "", true},
		{"UMD-1013-uncensored-nyap2p.com.mp4", "UMD-1013", "", true},
		{"EMP-004_4K.mp4", "EMP-004", "", true},

		// Multi-part suffixes.
		{"ftkd-040-1.mp4", "FTKD-040", "1", true},
		{"ftkd-040-2.mp4", "FTKD-040", "2", true},
		{"masex.tv@ftkd-040-1.mp4", "FTKD-040", "1", true},
		{"masex.tv@ftkd-040-2.mp4", "FTKD-040", "2", true},

		// Site watermark prefix.
		{"masex.tv@rlmp-005.mp4", "RLMP-005", "", true},
		{"masex.tv@gara-025.mp4", "GARA-025", "", true},
		{"masex.tv@start-603.mp4", "START-603", "", true},
		{"masex.tv@dal-011.mp4", "DAL-011", "", true},

		// FC2 format.
		{"FC2-PPV-4947342.mp4", "FC2-PPV-4947342", "", true},
		{"FC2-PPV-4939725.mp4", "FC2-PPV-4939725", "", true},
		{"FC2-PPV-4940400.mp4", "FC2-PPV-4940400", "", true},
		{"FC2-PPV-4948229.mp4", "FC2-PPV-4948229", "", true},
		{"FC2-PPV-4948798.mp4", "FC2-PPV-4948798", "", true},
		{"FC2-PPV-4940524.mp4", "FC2-PPV-4940524", "", true},

		// Genuinely uncoded — correct to leave unmatched.
		{"苍 老 师 强 力 推 荐.mp4", "", "", false},
		{"頂級NTR綠帽情侶【混血cc】最新8.3號作品.mp4", "", "", false},
	}

	for _, tc := range cases {
		gotCode, gotPart, gotOK := ExtractCode(tc.name)
		if gotCode != tc.wantCode || gotPart != tc.wantPart || gotOK != tc.wantOK {
			t.Errorf("ExtractCode(%q) = (%q, %q, %v), want (%q, %q, %v)",
				tc.name, gotCode, gotPart, gotOK, tc.wantCode, tc.wantPart, tc.wantOK)
		}
	}
}

func TestExtractCodeParentFolderFallback(t *testing.T) {
	cases := []struct {
		path     string
		wantCode string
		wantPart string
		wantOK   bool
	}{
		// Real filename carries no code, but the parent folder does.
		{"/download/rlmp-005/苍 老 师 强 力 推 荐.mp4", "RLMP-005", "", true},
		{"/download/gara-025/苍 老 师 强 力 推 荐.mp4", "GARA-025", "", true},
		{"/download/FC2-PPV-4947342/FC2-PPV-4947342.mp4", "FC2-PPV-4947342", "", true},

		// Neither filename nor parent folder carries a code.
		{"/download/0726 (5)/video.mp4", "", "", false},
		{"/download/0811 (3)/浙江新婚小少婦被完美調教.mp4", "", "", false},

		// Filename alone already matches — fallback never needed, and must
		// not be reached even if the parent folder happens to look coded.
		{"/download/RLMP-005/OFES-049.mp4", "OFES-049", "", true},
	}

	for _, tc := range cases {
		gotCode, gotPart, gotOK := ExtractCode(tc.path)
		if gotCode != tc.wantCode || gotPart != tc.wantPart || gotOK != tc.wantOK {
			t.Errorf("ExtractCode(%q) = (%q, %q, %v), want (%q, %q, %v)",
				tc.path, gotCode, gotPart, gotOK, tc.wantCode, tc.wantPart, tc.wantOK)
		}
	}
}
