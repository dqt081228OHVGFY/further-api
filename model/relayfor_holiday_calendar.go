package model

import (
	_ "embed"
	"encoding/json"
	"strconv"
)

// RelayForHolidayMinYear and RelayForHolidayMaxYear bound the verified calendar.
// Unsupported years must not be extrapolated from an approximate lunar cycle.
const (
	RelayForHolidayMinYear = 2025
	RelayForHolidayMaxYear = 2050
)

//go:embed relayfor_holiday_calendar.json
var relayForHolidayCalendarJSON []byte

type relayForHolidayCalendarData struct {
	MinYear    int                          `json:"minYear"`
	MaxYear    int                          `json:"maxYear"`
	TimeZone   string                       `json:"timeZone"`
	Source     string                       `json:"source"`
	References []string                     `json:"references"`
	Dates      map[string]map[string]string `json:"dates"`
}

var relayForHolidayCalendar = func() relayForHolidayCalendarData {
	var calendar relayForHolidayCalendarData
	if err := json.Unmarshal(relayForHolidayCalendarJSON, &calendar); err != nil {
		panic("relayfor: invalid embedded holiday calendar: " + err.Error())
	}
	if calendar.MinYear != RelayForHolidayMinYear || calendar.MaxYear != RelayForHolidayMaxYear {
		panic("relayfor: embedded holiday calendar bounds do not match supported years")
	}
	return calendar
}()

// RelayForHolidayDate returns the Asia/Shanghai civil date of a lunar festival
// within gregorianYear. Supported keys are duanwu, qixi, mid_autumn, chongyang,
// new_year_eve, and the fixed Gregorian national_day (October 1).
// Leap lunar months are excluded. New Year's Eve is the day
// before the actual Lunar New Year in the requested Gregorian year, so its
// lunar day can be 29 or 30. Unknown keys and unsupported years return false.
//
// The bounded data is generated using Intl's Chinese calendar and cross-checked
// against Hong Kong Observatory conversion tables for every supported year.
func RelayForHolidayDate(key string, gregorianYear int) (string, bool) {
	if gregorianYear < RelayForHolidayMinYear || gregorianYear > RelayForHolidayMaxYear {
		return "", false
	}
	if key == "national_day" {
		return strconv.Itoa(gregorianYear) + "-10-01", true
	}
	date, ok := relayForHolidayCalendar.Dates[strconv.Itoa(gregorianYear)][key]
	return date, ok
}

// RelayForHolidayDates returns a copy of five verified lunar dates plus the
// fixed Gregorian National Day in
// gregorianYear. Unsupported years return an empty map. The caller may safely
// modify the returned map without changing the embedded calendar.
func RelayForHolidayDates(gregorianYear int) map[string]string {
	dates := make(map[string]string)
	if gregorianYear < RelayForHolidayMinYear || gregorianYear > RelayForHolidayMaxYear {
		return dates
	}
	for key, date := range relayForHolidayCalendar.Dates[strconv.Itoa(gregorianYear)] {
		dates[key] = date
	}
	dates["national_day"] = strconv.Itoa(gregorianYear) + "-10-01"
	return dates
}
