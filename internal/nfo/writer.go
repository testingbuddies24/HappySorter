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
	Plot          string   `xml:"plot"`
	Runtime       int      `xml:"runtime,omitempty"`
	Premiered     string   `xml:"premiered,omitempty"`
	Year          int      `xml:"year,omitempty"`
	Studio        string   `xml:"studio,omitempty"`
	Director      string   `xml:"director,omitempty"`
	Genres        []string `xml:"genre,omitempty"`
	Actors        []actor  `xml:"actor"`
	UniqueID      uniqueID `xml:"uniqueid"`
}

// Write emits a Kodi-schema movie.nfo for m at path.
func Write(path string, m *scraper.Metadata) error {
	doc := movie{
		Title:         m.Title,
		OriginalTitle: m.Title,
		Plot:          m.Plot,
		Runtime:       m.Runtime,
		Premiered:     m.ReleaseDate,
		Year:          m.Year,
		Studio:        m.Studio,
		Director:      m.Director,
		Genres:        m.Genres,
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
