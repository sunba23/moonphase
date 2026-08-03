package catalog

import (
	"fmt"
	"strconv"
	"strings"
)

// ParseAngle turns a "25°"/"40°"-style string into its degree value. A
// missing trailing "°" is tolerated.
func ParseAngle(s string) (int, error) {
	trimmed := strings.TrimSuffix(s, "°")

	angle, err := strconv.Atoi(trimmed)
	if err != nil {
		return 0, fmt.Errorf("catalog: invalid angle %q: %w", s, err)
	}

	return angle, nil
}
