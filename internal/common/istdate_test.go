package common

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestISTBoundaries(t *testing.T) {
	tm := time.Date(2026, 9, 18, 18, 29, 59, 0, time.UTC) // 23:59:59 IST on the 18th
	assert.Equal(t, "2026-09-18", ISTDateString(tm))
	assert.Equal(t, "2026-09-19", ISTDateString(tm.Add(time.Second)))
	assert.Equal(t, time.Date(2026, 9, 17, 18, 30, 0, 0, time.UTC), ISTDayAnchor(tm))
	assert.Equal(t, 1, ISTDaysBetween(tm, tm.Add(time.Second)))
	assert.Equal(t, 0, ISTDaysBetween(tm.Add(time.Second), tm))
	assert.Equal(t, time.Date(2026, 9, 18, 23, 59, 59, 0, time.UTC), ISTShift(tm))
	assert.Equal(t, 0, CalendarDaysDifference(tm, tm.Add(time.Second)))
	assert.Equal(t, 1, CalendarDaysDifference(tm, tm.Add(6*time.Hour)))
	assert.Equal(t, 2, CalendarDaysDifference(tm.Add(30*time.Hour), tm))
	assert.Equal(t, time.Date(2026, 9, 18, 23, 59, 59, 999000000, time.UTC), EndOfDayUTC(tm))
}

func TestFormatMoment(t *testing.T) {
	d := time.Date(2026, 9, 5, 0, 0, 0, 0, time.UTC)
	assert.Equal(t, "5 September, 2026", FormatMoment(d, "D MMMM, YYYY"))
	assert.Equal(t, "05 Sep 2026", FormatMoment(d, "DD MMM YYYY"))
	assert.Equal(t, "5 Sep, 2026", FormatMoment(d, "D MMM, YYYY"))
	assert.Equal(t, "5th September", FormatMoment(d, "Do MMMM"))
	assert.Equal(t, "September 2026", FormatMoment(d, "MMMM YYYY"))
	assert.Equal(t, "2026-09-05", FormatMoment(d, "YYYY-MM-DD"))
	assert.Equal(t, "22nd", ordinal(22))
	assert.Equal(t, "11th", ordinal(11))
	assert.Equal(t, "3rd", ordinal(3))
}

func TestJSDateString(t *testing.T) {
	assert.Equal(t, "Fri Sep 18 2026 18:30:00 GMT+0000 (Coordinated Universal Time)",
		JSDateString(time.Date(2026, 9, 18, 18, 30, 0, 0, time.UTC)))
	assert.Equal(t, "day", DayText(1))
	assert.Equal(t, "days", DayText(0))
}
