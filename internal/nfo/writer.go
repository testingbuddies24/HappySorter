// Package nfo writes Kodi/Jellyfin-schema movie.nfo files
// (docs/ARCHITECTURE.md § 2.8, § 5).
package nfo

import (
	"encoding/xml"
	"fmt"
	"os"

	"github.com/testingbuddies24/HappySorter/internal/scraper"
)

type actor struct {
	Name string `xml:"name"`
}

type uniqueID struct {
	Type    string `xml:"type,attr"`
	Default bool   `xml:"default,attr"`
	Value   string `xml:",chardata"`
}

type movie struct {
	XMLName       xml.Name `xml:"movie"`
	Title         string   `xml:"title"`
	OriginalTitle string   `xml:"originaltitle"`
	Studio        string   `xml:"studio,omitempty"`
	Year          int      `xml:"year,omitempty"`
	Premiered     string   `xml:"premiered,omitempty"`
	Plot          string   `xml:"plot"`
	Runtime       int      `xml:"runtime,omitempty"`
	Director      string   `xml:"director,omitempty"`
	Poster        string   `xml:"poster,omitempty"`
	Fanart        string   `xml:"fanart,omitempty"`
	Actors        []actor  `xml:"actor"`
	Genres        []string `xml:"genre,omitempty"`
	Tags          []string `xml:"tag,omitempty"`
	Maker         string   `xml:"maker,omitempty"`
	Num           string   `xml:"num"`
	Release       string   `xml:"release,omitempty"`
	UniqueID      uniqueID `xml:"uniqueid"`
}

// Artwork names the image sidecar files (relative to the .nfo) that the
// organiser wrote, so the NFO can reference them explicitly. Empty fields
// are omitted from the output.
type Artwork struct {
	Poster string
	Fanart string
}

// Write emits a Kodi/Jellyfin-schema movie.nfo for m at path. art carries the
// filenames of the poster/fanart sidecars the organiser produced alongside it.
func Write(path string, m *scraper.Metadata, art Artwork) error {
	// Prefix the title with the code (e.g. "[MIDA-678]Title") so the code is
	// visible in Jellyfin's library grid, matching the common JAV convention.
	title := "[" + m.Code + "]" + m.Title

	doc := movie{
		Title:         title,
		OriginalTitle: title,
		Studio:        m.Studio,
		Year:          m.Year,
		Premiered:     m.ReleaseDate,
		Plot:          m.Plot,
		Runtime:       m.Runtime,
		Director:      m.Director,
		Poster:        art.Poster,
		Fanart:        art.Fanart,
		Genres:        m.Genres,
		Tags:          m.Genres, // Jellyfin JAV setups mirror genres as tags
		Maker:         m.Studio,
		Num:           m.Code,
		Release:       m.ReleaseDate,
		UniqueID:      uniqueID{Type: "jav", Default: true, Value: m.Code},
	}
	for _, name := range m.Actresses {
		doc.Actors = append(doc.Actors, actor{Name: name})
	}

	out, err := xml.MarshalIndent(doc, "", "  ")
	if err != nil {
		return fmt.Errorf("marshalling nfo: %w", err)
	}

	data := append([]byte(xml.Header), out...)
	return os.WriteFile(path, data, 0o644)
}
