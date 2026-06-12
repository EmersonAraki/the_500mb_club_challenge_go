package load

import (
	"strconv"
	"time"
)

// ms renders a duration in milliseconds with two decimals (e.g. "2.39ms").
func ms(d time.Duration) string {
	return strconv.FormatFloat(float64(d)/float64(time.Millisecond), 'f', 2, 64) + "ms"
}

func itoa(n int) string { return strconv.Itoa(n) }

func ftoa(f float64) string { return strconv.FormatFloat(f, 'f', 2, 64) }
