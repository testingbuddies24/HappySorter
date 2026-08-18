package config

import "testing"

// A config.yaml saved before the FC2 sources existed carries the classic
// five-source list; after migration it must also carry missav and javmenu,
// enabled, without disturbing what was already there.
func TestMigrateNewSources(t *testing.T) {
	cfg := &Config{Sources: []SourceConfig{
		{Name: "s1", Enabled: false, Priority: 1, QPS: 1.0},
		{Name: "ideapocket", Enabled: false, Priority: 2, QPS: 1.0},
		{Name: "javbus", Enabled: true, Priority: 3, QPS: 1.0},
		{Name: "javdb", Enabled: true, Priority: 4, QPS: 1.0},
		{Name: "javlibrary", Enabled: false, Priority: 5, QPS: 0.5},
	}}

	migrateNewSources(cfg)

	if len(cfg.Sources) != 7 {
		t.Fatalf("len(Sources) = %d, want 7", len(cfg.Sources))
	}
	for _, name := range []string{"missav", "javmenu"} {
		var sc SourceConfig
		for _, s := range cfg.Sources {
			if s.Name == name {
				sc = s
			}
		}
		if sc.Name == "" {
			t.Fatalf("source %q not appended by migration", name)
		}
		if !sc.Enabled {
			t.Errorf("%s.Enabled = false, want true", name)
		}
	}

	// Migration must be idempotent: a second pass (e.g. config reloaded in
	// the same process) must not duplicate the entries.
	migrateNewSources(cfg)
	if len(cfg.Sources) != 7 {
		t.Errorf("len(Sources) after second migration = %d, want 7", len(cfg.Sources))
	}

	// A source the user disabled must stay disabled across migrations.
	cfg.Sources[5].Enabled = false // missav
	migrateNewSources(cfg)
	if cfg.Sources[5].Enabled {
		t.Error("missav re-enabled by migration after user disabled it")
	}
}
