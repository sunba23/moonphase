package server

import (
	"testing"

	"github.com/sunba23/moonphase/internal/catalog"
)

func TestGradeValid_Found(t *testing.T) {
	if !gradeValid([]string{"6B", "6C", "7A"}, "6C") {
		t.Fatal("expected grade to be valid")
	}
}

func TestGradeValid_NotFound(t *testing.T) {
	if gradeValid([]string{"6B", "6C", "7A"}, "9A") {
		t.Fatal("expected grade to be invalid")
	}
}

func TestBoardValid_Found(t *testing.T) {
	boards := []catalog.BoardEdition{{Holdsetup: 1, Name: "2016"}, {Holdsetup: 2, Name: "2019"}}
	if !boardValid(boards, 2) {
		t.Fatal("expected board to be valid")
	}
}

func TestBoardValid_NotFound(t *testing.T) {
	boards := []catalog.BoardEdition{{Holdsetup: 1, Name: "2016"}, {Holdsetup: 2, Name: "2019"}}
	if boardValid(boards, 99) {
		t.Fatal("expected board to be invalid")
	}
}

func TestAngleValid_Found(t *testing.T) {
	if !angleValid([]int16{25, 40}, 40) {
		t.Fatal("expected angle to be valid")
	}
}

func TestAngleValid_NotFound(t *testing.T) {
	if angleValid([]int16{25, 40}, 30) {
		t.Fatal("expected angle to be invalid")
	}
}
