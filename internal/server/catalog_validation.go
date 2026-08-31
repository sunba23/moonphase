package server

import "github.com/sunba23/moonphase/internal/catalog"

func gradeValid(grades []string, g string) bool {
	for _, x := range grades {
		if x == g {
			return true
		}
	}
	return false
}

func boardValid(boards []catalog.BoardEdition, holdsetup int16) bool {
	for _, b := range boards {
		if b.Holdsetup == holdsetup {
			return true
		}
	}
	return false
}

func angleValid(angles []int16, angle int16) bool {
	for _, a := range angles {
		if a == angle {
			return true
		}
	}
	return false
}
