// Tests for unit conversion (SPEC-20260310-008).
package convert_test

import (
	"strings"
	"testing"

	"github.com/dinav2/vida/internal/convert"
)

// --- IsConversion detection ---

func TestIsConversion_Match(t *testing.T) {
	cases := []string{
		"5 cm to in",
		"5cm to in",
		"100 kg to lbs",
		"32 c to f",
		"1 GB to MB",
		"60 mph to km/h",
		"1 day to hours",
		"5 cm → in",
	}
	for _, tc := range cases {
		if !convert.IsConversion(tc) {
			t.Errorf("IsConversion(%q) = false, want true", tc)
		}
	}
}

func TestIsConversion_NoMatch(t *testing.T) {
	cases := []string{
		"hello",
		"2 + 2",
		"firefox",
		"5 cm",        // no target unit
		"to inches",   // no number
		":translate x", // command mode
	}
	for _, tc := range cases {
		if convert.IsConversion(tc) {
			t.Errorf("IsConversion(%q) = true, want false", tc)
		}
	}
}

// --- Convert correctness (SCN-01 through SCN-16) ---

func TestConvert_CmToIn(t *testing.T) { // SCN-01
	assertConvert(t, "5 cm to in", "1.97")
}

func TestConvert_KmToMiles(t *testing.T) { // SCN-02
	assertConvert(t, "1 km to miles", "0.62")
}

func TestConvert_KgToLbs(t *testing.T) { // SCN-03
	assertConvert(t, "70 kg to lbs", "154.32")
}

func TestConvert_CelsiusToFahrenheit(t *testing.T) { // SCN-04
	assertConvert(t, "32 c to f", "89.6")
}

func TestConvert_KelvinToCelsius(t *testing.T) { // SCN-05
	assertConvert(t, "0 k to c", "-273.15")
}

func TestConvert_FahrenheitToCelsius(t *testing.T) { // SCN-06
	assertConvert(t, "212 f to c", "100")
}

func TestConvert_GBtoMB(t *testing.T) { // SCN-07
	assertConvert(t, "1 GB to MB", "1024")
}

func TestConvert_MphToKph(t *testing.T) { // SCN-08
	assertConvert(t, "60 mph to km/h", "96.56")
}

func TestConvert_NoSpaceBetweenNumberAndUnit(t *testing.T) { // SCN-09
	assertConvert(t, "5cm to in", "1.97")
}

func TestConvert_AcreToM2(t *testing.T) { // SCN-10
	assertConvert(t, "1 acre to m2", "4046.86")
}

func TestConvert_MlToCups(t *testing.T) { // SCN-11
	assertConvert(t, "500 ml to cups", "2.11")
}

func TestConvert_DayToHours(t *testing.T) { // SCN-12
	assertConvert(t, "1 day to hours", "24")
}

func TestConvert_UnknownInput_NoResult(t *testing.T) { // SCN-13
	if convert.IsConversion("hello") {
		t.Error("IsConversion(\"hello\") should be false")
	}
}

func TestConvert_IncompatibleFamilies(t *testing.T) { // SCN-14
	_, ok := convert.Convert("5 km to kg")
	if ok {
		t.Error("Convert(\"5 km to kg\") should fail — incompatible families")
	}
}

func TestConvert_UnknownUnits(t *testing.T) { // SCN-15
	_, ok := convert.Convert("5 xyz to abc")
	if ok {
		t.Error("Convert(\"5 xyz to abc\") should fail — unknown units")
	}
}

// --- Display format checks ---

func TestConvert_TemperatureDegreesSymbol(t *testing.T) {
	result, ok := convert.Convert("32 c to f")
	if !ok {
		t.Fatal("convert failed")
	}
	if !strings.Contains(result, "°") {
		t.Errorf("temperature result %q missing degree symbol", result)
	}
}

func TestConvert_ArrowSeparator(t *testing.T) {
	result, ok := convert.Convert("5 cm to in")
	if !ok {
		t.Fatal("convert failed")
	}
	if !strings.Contains(result, "→") {
		t.Errorf("result %q missing → separator", result)
	}
}

func TestConvert_TrailingZerosTrimmed(t *testing.T) {
	result, ok := convert.Convert("1 km to m")
	if !ok {
		t.Fatal("convert failed")
	}
	// 1 km = 1000 m — should display as "1000" not "1000.000000"
	if strings.Contains(result, ".") {
		t.Errorf("result %q should not contain decimal for whole number", result)
	}
}

func TestConvert_AliasesWork(t *testing.T) {
	pairs := [][2]string{
		{"1 kilometer to miles", "0.62"},
		{"1 pound to kg", "0.45"},
		{"1 foot to cm", "30.48"},
		{"1 litre to ml", "1000"},
	}
	for _, p := range pairs {
		assertConvert(t, p[0], p[1])
	}
}

// assertConvert checks that Convert(input) succeeds and result contains want.
func assertConvert(t *testing.T, input, want string) {
	t.Helper()
	result, ok := convert.Convert(input)
	if !ok {
		t.Errorf("Convert(%q) failed (returned false)", input)
		return
	}
	if !strings.Contains(result, want) {
		t.Errorf("Convert(%q) = %q, want it to contain %q", input, result, want)
	}
}
