package common

import (
	"fmt"
	"math"
	"strings"
	"time"
)

// IST is UTC+05:30.
var IST = time.FixedZone("IST", 330*60)

const day = 24 * time.Hour

// ISTShift mirrors api-server getClientTimeFromUtcTime / getIndianTime: t + 330 minutes, still in UTC.
func ISTShift(t time.Time) time.Time { return t.UTC().Add(330 * time.Minute) }

// UTCMidnight mirrors setTimeZeroForDate (setUTCHours(0,0,0,0)).
func UTCMidnight(t time.Time) time.Time {
	t = t.UTC()
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

// StartOfDayUTC mirrors setTimeStartOfDay with the Node process in UTC.
func StartOfDayUTC(t time.Time) time.Time { return UTCMidnight(t) }

// EndOfDayUTC mirrors setTimeEndOfDay / moment().endOf('day') in UTC: 23:59:59.999.
func EndOfDayUTC(t time.Time) time.Time { return UTCMidnight(t).Add(day - time.Millisecond) }

// CalendarDaysDifference mirrors calculateDaysDifference: abs whole days between UTC midnights.
func CalendarDaysDifference(a, b time.Time) int {
	d := UTCMidnight(b).Sub(UTCMidnight(a)).Hours() / 24
	return int(math.Abs(d))
}

// ISTDate returns midnight IST of t's IST calendar day (in the IST zone).
func ISTDate(t time.Time) time.Time {
	l := t.In(IST)
	return time.Date(l.Year(), l.Month(), l.Day(), 0, 0, 0, 0, IST)
}

// ISTDateString returns YYYY-MM-DD in IST.
func ISTDateString(t time.Time) string { return t.In(IST).Format("2006-01-02") }

// ISTDaysBetween mirrors logearn istDaysBetween: max(0, round((istDate(b)-istDate(a))/1d)).
func ISTDaysBetween(a, b time.Time) int {
	d := math.Round(ISTDate(b).Sub(ISTDate(a)).Hours() / 24)
	if d < 0 {
		return 0
	}
	return int(d)
}

// ISTDayAnchor mirrors logearn istDayAnchor: the UTC instant of 00:00 IST on t's IST day.
func ISTDayAnchor(t time.Time) time.Time { return ISTDate(t).UTC() }

// ISTStartOfDay is the lifeline-schedule IST startOf('day') (same instant as ISTDayAnchor).
func ISTStartOfDay(t time.Time) time.Time { return ISTDayAnchor(t) }

// ParseYMD parses YYYY-MM-DD as UTC midnight.
func ParseYMD(s string) (time.Time, error) { return time.Parse("2006-01-02", s) }

// ParseYMDIST parses YYYY-MM-DD as IST midnight.
func ParseYMDIST(s string) (time.Time, error) { return time.ParseInLocation("2006-01-02", s, IST) }

// FormatMoment renders t with a moment.js layout limited to the tokens used in BAH:
// YYYY, MMMM, MMM, MM, DD, D, Do.
func FormatMoment(t time.Time, layout string) string {
	var b strings.Builder
	for i := 0; i < len(layout); {
		switch {
		case strings.HasPrefix(layout[i:], "YYYY"):
			b.WriteString(fmt.Sprintf("%04d", t.Year()))
			i += 4
		case strings.HasPrefix(layout[i:], "MMMM"):
			b.WriteString(t.Month().String())
			i += 4
		case strings.HasPrefix(layout[i:], "MMM"):
			b.WriteString(t.Month().String()[:3])
			i += 3
		case strings.HasPrefix(layout[i:], "MM"):
			b.WriteString(fmt.Sprintf("%02d", int(t.Month())))
			i += 2
		case strings.HasPrefix(layout[i:], "DD"):
			b.WriteString(fmt.Sprintf("%02d", t.Day()))
			i += 2
		case strings.HasPrefix(layout[i:], "Do"):
			b.WriteString(ordinal(t.Day()))
			i += 2
		case layout[i] == 'D':
			b.WriteString(fmt.Sprintf("%d", t.Day()))
			i++
		default:
			b.WriteByte(layout[i])
			i++
		}
	}
	return b.String()
}

func ordinal(n int) string {
	suffix := "th"
	switch {
	case n%100 >= 11 && n%100 <= 13:
	case n%10 == 1:
		suffix = "st"
	case n%10 == 2:
		suffix = "nd"
	case n%10 == 3:
		suffix = "rd"
	}
	return fmt.Sprintf("%d%s", n, suffix)
}

// JSDateString mirrors JavaScript Date.prototype.toString() for a UTC process:
// "Thu Sep 18 2026 18:30:00 GMT+0000 (Coordinated Universal Time)".
func JSDateString(t time.Time) string {
	t = t.UTC()
	return t.Format("Mon Jan 02 2006 15:04:05") + " GMT+0000 (Coordinated Universal Time)"
}

// DayText returns "day" or "days".
func DayText(n int) string { return Plural(n, "day", "days") }

// Plural picks one for n==1 else many.
func Plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}
