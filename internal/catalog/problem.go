package catalog

import "time"

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
	DateInserted   *time.Time      `json:"dateInserted"`
	DateUpdated    *time.Time      `json:"dateUpdated"`
	DateDeleted    *time.Time      `json:"dateDeleted"`
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
	APIID                int        `json:"apiId"`
	Grade                string     `json:"grade"`
	UserGrade            string     `json:"userGrade"`
	UserRating           int        `json:"userRating"`
	DateUpdated          *time.Time `json:"dateUpdated"`
	DateDeleted          *time.Time `json:"dateDeleted"`
	IsBenchmark          bool       `json:"isBenchmark"`
	IsCompetitionProblem bool       `json:"isCompetitionProblem"`
	IsPrimary            bool       `json:"isPrimary"`
	Comment              *string    `json:"comment"`
	Repeats              int        `json:"repeats"`
	Configuration        string     `json:"configuration"`
	PrimaryAngle         string     `json:"primaryAngle"`
}

// ShouldIngest reports whether a problem qualifies for ingestion: active and
// not soft-deleted.
func ShouldIngest(p Problem) bool {
	return p.Active && p.DateDeleted == nil
}
