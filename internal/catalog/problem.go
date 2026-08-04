package catalog

import (
	"fmt"
	"strconv"
	"time"
)

// sourceTimeLayout matches the MoonBoard export's timestamp format, which
// carries no timezone designator and a variable-precision fractional second
// (e.g. "2016-03-01T10:00:27" or "2026-06-25T21:58:25.013") -- not valid
// RFC3339, so it can't use time.Time's default JSON unmarshaling.
const sourceTimeLayout = "2006-01-02T15:04:05.999999999"

// SourceTime unmarshals the export's timezone-less timestamps, treating them
// as UTC since the source carries no timezone information.
type SourceTime time.Time

func (t *SourceTime) UnmarshalJSON(data []byte) error {
	s, err := strconv.Unquote(string(data))
	if err != nil {
		return fmt.Errorf("catalog: unquote time %s: %w", data, err)
	}

	parsed, err := time.ParseInLocation(sourceTimeLayout, s, time.UTC)
	if err != nil {
		return fmt.Errorf("catalog: parse time %q: %w", s, err)
	}

	*t = SourceTime(parsed)
	return nil
}

// Time returns the underlying time.Time.
func (t SourceTime) Time() time.Time { return time.Time(t) }

// ExportFile is the top-level shape of one static MoonBoard board-edition
// JSON export.
type ExportFile struct {
	Holdsetup int       `json:"holdsetup"`
	Count     int       `json:"count"`
	Problems  []Problem `json:"problems"`
}

type Problem struct {
	ID             int             `json:"id"`
	Name           string          `json:"name"`
	DateInserted   *SourceTime     `json:"dateInserted"`
	DateUpdated    *SourceTime     `json:"dateUpdated"`
	DateDeleted    *SourceTime     `json:"dateDeleted"`
	Holdsetup      int             `json:"holdsetup"`
	Active         bool            `json:"Active"`
	ClimbMethod    string          `json:"climbMethod"`
	SetbyID        string          `json:"setbyId"`
	Setter         string          `json:"setter"`
	Moves          string          `json:"moves"`
	Coordinates    *string         `json:"coordinates"`
	Holdsets       string          `json:"holdsets"`
	BetaVideos     int             `json:"betaVideos"`
	Configurations []Configuration `json:"configurations"`
}

type Configuration struct {
	APIID                int         `json:"apiId"`
	Grade                string      `json:"grade"`
	UserGrade            string      `json:"userGrade"`
	UserRating           int         `json:"userRating"`
	DateUpdated          *SourceTime `json:"dateUpdated"`
	DateDeleted          *SourceTime `json:"dateDeleted"`
	IsBenchmark          bool        `json:"isBenchmark"`
	IsCompetitionProblem bool        `json:"isCompetitionProblem"`
	IsPrimary            bool        `json:"isPrimary"`
	Comment              *string     `json:"comment"`
	Repeats              int         `json:"repeats"`
	Configuration        string      `json:"configuration"`
	PrimaryAngle         string      `json:"primaryAngle"`
}

// ShouldIngest reports whether a problem qualifies for ingestion: active and
// not soft-deleted.
func ShouldIngest(p Problem) bool {
	return p.Active && p.DateDeleted == nil
}
