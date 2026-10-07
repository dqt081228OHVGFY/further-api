package model

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestFurtherHolidayCalendarKnownDates(t *testing.T) {
	// Hong Kong Observatory's 2025/2026 Gregorian-Lunar Conversion Tables.
	expected := map[int]map[string]string{
		2025: {"duanwu": "2025-05-31", "qixi": "2025-08-29", "mid_autumn": "2025-10-06", "chongyang": "2025-10-29", "new_year_eve": "2025-01-28"},
		2026: {"duanwu": "2026-06-19", "qixi": "2026-08-19", "mid_autumn": "2026-09-25", "chongyang": "2026-10-18", "new_year_eve": "2026-02-16"},
	}
	for year, dates := range expected {
		dates["national_day"] = strconv.Itoa(year) + "-10-01"
		if actual := FurtherHolidayDates(year); !reflect.DeepEqual(actual, dates) {
			t.Errorf("year %d: got %v, want %v", year, actual, dates)
		}
		for key, expectedDate := range dates {
			if actual, ok := FurtherHolidayDate(key, year); !ok || actual != expectedDate {
				t.Errorf("%d/%s: got (%q, %v), want (%q, true)", year, key, actual, ok, expectedDate)
			}
		}
	}
}

func TestFurtherHolidayCalendarNewYearEveUsesGregorianYear(t *testing.T) {
	// HKO's "No Lunar New Year's Eve on the 30th Day for Five Consecutive
	// Years" gives the actual first lunar day for 2025–2034. Eve is one day
	// earlier, including the short twelfth lunar months in 2025–2029.
	expected := map[int]string{
		2025: "2025-01-28", 2026: "2026-02-16", 2027: "2027-02-05",
		2028: "2028-01-25", 2029: "2029-02-12", 2030: "2030-02-02",
		2031: "2031-01-22", 2032: "2032-02-10", 2033: "2033-01-30",
		2034: "2034-02-18",
	}
	for year, expectedDate := range expected {
		actual, ok := FurtherHolidayDate("new_year_eve", year)
		if !ok || actual != expectedDate {
			t.Errorf("Eve in %d: got (%q, %v), want %q", year, actual, ok, expectedDate)
		}
	}
}

func TestFurtherHolidayCalendarExcludesLeapMonths(t *testing.T) {
	// These years repeat lunar month 5 or 7. The festival occurs in the
	// ordinary month, never again in the later intercalary month.
	cases := []struct {
		year      int
		key       string
		regular   string
		leapMonth string
	}{
		{2028, "duanwu", "2028-05-28", "2028-06-27"},
		{2039, "duanwu", "2039-05-27", "2039-06-26"},
		{2044, "qixi", "2044-07-31", "2044-08-29"},
		{2047, "duanwu", "2047-05-29", "2047-06-27"},
	}
	for _, test := range cases {
		actual, ok := FurtherHolidayDate(test.key, test.year)
		if !ok || actual != test.regular || actual == test.leapMonth {
			t.Errorf("%d/%s: got (%q, %v), want regular %q, excluding %q", test.year, test.key, actual, ok, test.regular, test.leapMonth)
		}
	}
}

func TestFurtherHolidayCalendarBoundsAndCompleteness(t *testing.T) {
	keys := []string{"duanwu", "qixi", "mid_autumn", "chongyang", "new_year_eve", "national_day"}
	if len(furtherHolidayCalendar.Dates) != FurtherHolidayMaxYear-FurtherHolidayMinYear+1 {
		t.Fatalf("unexpected year count: %d", len(furtherHolidayCalendar.Dates))
	}
	for year := FurtherHolidayMinYear; year <= FurtherHolidayMaxYear; year++ {
		dates := FurtherHolidayDates(year)
		if len(dates) != len(keys) {
			t.Errorf("%d has %d festivals, want %d", year, len(dates), len(keys))
		}
		for _, key := range keys {
			date, ok := FurtherHolidayDate(key, year)
			parsed, err := time.Parse(time.DateOnly, date)
			if !ok || err != nil || parsed.Year() != year || dates[key] != date {
				t.Errorf("invalid %d/%s date %q (%v)", year, key, date, err)
			}
		}
	}
	for _, year := range []int{-1, 0, 2024, 2051, 9999} {
		if dates := FurtherHolidayDates(year); dates == nil || len(dates) != 0 {
			t.Errorf("unsupported year %d: want an empty map, got %v", year, dates)
		}
		if date, ok := FurtherHolidayDate("duanwu", year); date != "" || ok {
			t.Errorf("unsupported year %d: got (%q, %v)", year, date, ok)
		}
	}
	for _, key := range []string{"", "unknown", "DUANWU", " duanwu", "new_year"} {
		if date, ok := FurtherHolidayDate(key, 2026); date != "" || ok {
			t.Errorf("unknown key %q: got (%q, %v)", key, date, ok)
		}
	}
}

func TestFurtherHolidayCalendarReturnsIndependentMaps(t *testing.T) {
	first := FurtherHolidayDates(2026)
	second := FurtherHolidayDates(2026)
	first["mid_autumn"] = "2026-01-01"
	delete(first, "duanwu")
	first["unknown"] = "2026-01-01"
	if date, ok := FurtherHolidayDate("mid_autumn", 2026); !ok || date != "2026-09-25" {
		t.Fatalf("caller mutated calendar: (%q, %v)", date, ok)
	}
	if !reflect.DeepEqual(second, FurtherHolidayDates(2026)) {
		t.Fatal("returned maps share mutable state")
	}
}

func TestFurtherHolidayCalendarBrowserDataMatchesBackend(t *testing.T) {
	_, testFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("unable to locate calendar test source")
	}
	asset := filepath.Join(filepath.Dir(testFile), "..", "further-ui", "assets", "lunar-schedule.js")
	contents, err := os.ReadFile(asset)
	if err != nil {
		t.Fatal(err)
	}
	_, data, found := strings.Cut(string(contents), "const schedule = ")
	if !found {
		t.Fatal("browser asset does not contain generated schedule")
	}
	data, _, found = strings.Cut(data, ";\n")
	if !found {
		t.Fatal("browser schedule terminator is missing")
	}
	var browser furtherHolidayCalendarData
	if err := json.Unmarshal([]byte(data), &browser); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(browser, furtherHolidayCalendar) {
		t.Fatal("browser lunar schedule differs from embedded backend calendar")
	}
	if browser.TimeZone != "Asia/Shanghai" || browser.Source == "" || len(browser.References) == 0 {
		t.Fatal("calendar timezone or source metadata is missing")
	}
	for year := FurtherHolidayMinYear; year <= FurtherHolidayMaxYear; year++ {
		lunarDates := FurtherHolidayDates(year)
		delete(lunarDates, "national_day") // Solar holidays never alter the verified lunar dataset.
		if !reflect.DeepEqual(browser.Dates[strconv.Itoa(year)], lunarDates) {
			t.Fatalf("browser dates differ for year %d", year)
		}
	}
}
