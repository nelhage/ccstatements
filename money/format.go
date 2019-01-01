package money

import "fmt"

func FormatCents(amt int64) string {
	signum := ""
	if amt < 0 {
		signum = "-"
		amt = -amt
	}
	return fmt.Sprintf("%s%d.%02d", signum, amt/100, amt%100)
}
