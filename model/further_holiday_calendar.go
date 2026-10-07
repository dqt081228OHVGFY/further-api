package model

import (
	_ "embed"
	"encoding/json"
	"strconv"
)

// FurtherHolidayMinYear and FurtherHolidayMaxYear bound the verified calendar.
// Unsupported years must not be extrapolated from an approximate lunar cycle.
const (
	FurtherHolidayMinYear = 2025
	FurtherHolidayMaxYear = 2050
)

//go:embed further_holiday_calendar.json
var furtherHolidayCalendarJSON []byte

type furtherHolidayCalendarData struct {
	MinYear    int                          `json:"minYear"`
	MaxYear    int                          `json:"maxYear"`
	TimeZone   string                       `json:"timeZone"`
	Source     string                       `json:"source"`
	References []string                     `json:"references"`
	Dates      map[string]map[string]string `json:"dates"`
}

var furtherHolidayCalendar = func() furtherHolidayCalendarData {
	var calendar furtherHolidayCalendarData
	if err := json.Unmarshal(furtherHolidayCalendarJSON, &calendar); err != nil {
		panic("further: invalid embedded holiday calendar: " + err.Error())
	}
	if calendar.MinYear != FurtherHolidayMinYear || calendar.MaxYear != FurtherHolidayMaxYear {
		panic("further: embedded holiday calendar bounds do not match supported years")
	}
	return calendar
}()

// FurtherHolidayDate returns the Asia/Shanghai civil date of a lunar festival
// within gregorianYear. Supported keys are duanwu, qixi, mid_autumn, chongyang,
// new_year_eve, and the fixed Gregorian national_day (October 1).
// Leap lunar months are excluded. New Year's Eve is the day
// before the actual Lunar New Year in the requested Gregorian year, so its
// lunar day can be 29 or 30. Unknown keys and unsupported years return false.
//
// The bounded data is generated using Intl's Chinese calendar and cross-checked
// against Hong Kong Observatory conversion tables for every supported year.
func FurtherHolidayDate(key string, gregorianYear int) (string, bool) {
	if gregorianYear < FurtherHolidayMinYear || gregorianYear > FurtherHolidayMaxYear {
		return "", false
	}
	if key == "national_day" {
		return strconv.Itoa(gregorianYear) + "-10-01", true
	}
	date, ok := furtherHolidayCalendar.Dates[strconv.Itoa(gregorianYear)][key]
	return date, ok
}

// FurtherHolidayDates returns a copy of five verified lunar dates plus the
// fixed Gregorian National Day in
// gregorianYear. Unsupported years return an empty map. The caller may safely
// modify the returned map without changing the embedded calendar.
func FurtherHolidayDates(gregorianYear int) map[string]string {
	dates := make(map[string]string)
	if gregorianYear < FurtherHolidayMinYear || gregorianYear > FurtherHolidayMaxYear {
		return dates
	}
	for key, date := range furtherHolidayCalendar.Dates[strconv.Itoa(gregorianYear)] {
		dates[key] = date
	}
	dates["national_day"] = strconv.Itoa(gregorianYear) + "-10-01"
	return dates
}
