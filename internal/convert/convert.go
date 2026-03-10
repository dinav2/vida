// Package convert implements offline unit conversion for vida.
// Supports length, weight, temperature, area, volume, speed, data, and time.
// Currency is out of scope (see SPEC-20260310-009).
package convert

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
)

// conversionPattern matches: <number> <unit> (to|in|→) <unit>
var conversionPattern = regexp.MustCompile(
	`(?i)^(\d+(?:\.\d+)?)\s*([^\s]+)\s+(?:to|in|→)\s+(.+?)\s*$`,
)

// IsConversion reports whether s looks like a unit conversion query.
func IsConversion(s string) bool {
	return conversionPattern.MatchString(strings.TrimSpace(s))
}

// Convert evaluates a unit conversion query.
// Returns the formatted display string and true on success.
// Returns "", false if units are unknown, incompatible, or input is malformed.
func Convert(s string) (string, bool) {
	m := conversionPattern.FindStringSubmatch(strings.TrimSpace(s))
	if m == nil {
		return "", false
	}
	val, err := strconv.ParseFloat(m[1], 64)
	if err != nil {
		return "", false
	}
	fromRaw := m[2]
	toRaw := m[3]

	result, fromLabel, toLabel, ok := convertUnits(val, fromRaw, toRaw)
	if !ok {
		return "", false
	}

	resultStr := formatNum(result)
	return fmt.Sprintf("%s %s → %s %s", formatNum(val), fromLabel, resultStr, toLabel), true
}

// formatNum rounds to 2 decimal places and trims trailing zeros.
// Falls back to full precision for values that round to zero but are non-zero.
func formatNum(v float64) string {
	rounded := math.Round(v*100) / 100
	if rounded == 0 && v != 0 {
		// Very small number — use full precision to avoid showing "0"
		s := strconv.FormatFloat(v, 'f', -1, 64)
		return s
	}
	s := strconv.FormatFloat(rounded, 'f', 2, 64)
	if strings.Contains(s, ".") {
		s = strings.TrimRight(s, "0")
		s = strings.TrimRight(s, ".")
	}
	return s
}

// ---------- unit tables ----------

type unit struct {
	family    string
	canonical string  // display name
	toBase    float64 // multiply to convert to base unit; 0 = special (temperature)
}

// unitAliases maps lowercase input tokens to their unit descriptor.
var unitAliases = map[string]unit{
	// --- length (base: m) ---
	"mm": {"length", "mm", 0.001}, "millimeter": {"length", "mm", 0.001}, "millimeters": {"length", "mm", 0.001},
	"cm": {"length", "cm", 0.01}, "centimeter": {"length", "cm", 0.01}, "centimeters": {"length", "cm", 0.01},
	"m":  {"length", "m", 1}, "meter": {"length", "m", 1}, "meters": {"length", "m", 1}, "metre": {"length", "m", 1}, "metres": {"length", "m", 1},
	"km": {"length", "km", 1000}, "kilometer": {"length", "km", 1000}, "kilometers": {"length", "km", 1000}, "kilometre": {"length", "km", 1000}, "kilometres": {"length", "km", 1000},
	"in": {"length", "in", 0.0254}, "inch": {"length", "in", 0.0254}, "inches": {"length", "in", 0.0254},
	"ft": {"length", "ft", 0.3048}, "foot": {"length", "ft", 0.3048}, "feet": {"length", "ft", 0.3048},
	"yd": {"length", "yd", 0.9144}, "yard": {"length", "yd", 0.9144}, "yards": {"length", "yd", 0.9144},
	"mi": {"length", "mi", 1609.344}, "mile": {"length", "mi", 1609.344}, "miles": {"length", "mi", 1609.344},

	// --- weight (base: g) ---
	"mg": {"weight", "mg", 0.001}, "milligram": {"weight", "mg", 0.001}, "milligrams": {"weight", "mg", 0.001},
	"g":  {"weight", "g", 1}, "gram": {"weight", "g", 1}, "grams": {"weight", "g", 1},
	"kg": {"weight", "kg", 1000}, "kilogram": {"weight", "kg", 1000}, "kilograms": {"weight", "kg", 1000},
	"t":  {"weight", "t", 1_000_000}, "tonne": {"weight", "t", 1_000_000}, "tonnes": {"weight", "t", 1_000_000},
	"oz": {"weight", "oz", 28.3495}, "ounce": {"weight", "oz", 28.3495}, "ounces": {"weight", "oz", 28.3495},
	"lb": {"weight", "lb", 453.592}, "lbs": {"weight", "lb", 453.592}, "pound": {"weight", "lb", 453.592}, "pounds": {"weight", "lb", 453.592},

	// --- temperature (special: toBase=0, handled separately) ---
	"c": {"temperature", "°C", 0}, "°c": {"temperature", "°C", 0}, "celsius": {"temperature", "°C", 0},
	"f": {"temperature", "°F", 0}, "°f": {"temperature", "°F", 0}, "fahrenheit": {"temperature", "°F", 0},
	"k": {"temperature", "K", 0}, "kelvin": {"temperature", "K", 0},

	// --- area (base: m²) ---
	"mm2": {"area", "mm²", 0.000001}, "mm²": {"area", "mm²", 0.000001},
	"cm2": {"area", "cm²", 0.0001}, "cm²": {"area", "cm²", 0.0001},
	"m2":  {"area", "m²", 1}, "m²": {"area", "m²", 1},
	"km2": {"area", "km²", 1_000_000}, "km²": {"area", "km²", 1_000_000},
	"in2": {"area", "in²", 0.00064516}, "in²": {"area", "in²", 0.00064516},
	"ft2": {"area", "ft²", 0.092903}, "ft²": {"area", "ft²", 0.092903},
	"acre": {"area", "acre", 4046.86}, "acres": {"area", "acre", 4046.86},
	"ha": {"area", "ha", 10000}, "hectare": {"area", "ha", 10000}, "hectares": {"area", "ha", 10000},

	// --- volume (base: ml) ---
	"ml":     {"volume", "ml", 1}, "milliliter": {"volume", "ml", 1}, "millilitre": {"volume", "ml", 1},
	"l":      {"volume", "l", 1000}, "litre": {"volume", "l", 1000}, "liter": {"volume", "l", 1000}, "litres": {"volume", "l", 1000}, "liters": {"volume", "l", 1000},
	"floz":   {"volume", "fl oz", 29.5735}, "fl-oz": {"volume", "fl oz", 29.5735},
	"cup":    {"volume", "cups", 236.588}, "cups": {"volume", "cups", 236.588},
	"pt":     {"volume", "pt", 473.176}, "pint": {"volume", "pt", 473.176}, "pints": {"volume", "pt", 473.176},
	"qt":     {"volume", "qt", 946.353}, "quart": {"volume", "qt", 946.353}, "quarts": {"volume", "qt", 946.353},
	"gal":    {"volume", "gal", 3785.41}, "gallon": {"volume", "gal", 3785.41}, "gallons": {"volume", "gal", 3785.41},

	// --- speed (base: m/s) ---
	"m/s":  {"speed", "m/s", 1}, "ms": {"speed", "m/s", 1},
	"km/h": {"speed", "km/h", 0.277778}, "kph": {"speed", "km/h", 0.277778},
	"mph":  {"speed", "mph", 0.44704},
	"knot": {"speed", "knot", 0.514444}, "knots": {"speed", "knot", 0.514444},

	// --- data (base: B, base-2) ---
	"b":  {"data", "B", 1}, "byte": {"data", "B", 1}, "bytes": {"data", "B", 1},
	"kb": {"data", "KB", 1024}, "kilobyte": {"data", "KB", 1024}, "kilobytes": {"data", "KB", 1024},
	"mb": {"data", "MB", 1048576}, "megabyte": {"data", "MB", 1048576}, "megabytes": {"data", "MB", 1048576},
	"gb": {"data", "GB", 1073741824}, "gigabyte": {"data", "GB", 1073741824}, "gigabytes": {"data", "GB", 1073741824},
	"tb": {"data", "TB", 1099511627776}, "terabyte": {"data", "TB", 1099511627776}, "terabytes": {"data", "TB", 1099511627776},
	"pb": {"data", "PB", 1125899906842624}, "petabyte": {"data", "PB", 1125899906842624}, "petabytes": {"data", "PB", 1125899906842624},

	// --- time (base: s) ---
	"s": {"time", "s", 1}, "sec": {"time", "s", 1}, "second": {"time", "s", 1}, "seconds": {"time", "s", 1},
	"min": {"time", "min", 60}, "minute": {"time", "min", 60}, "minutes": {"time", "min", 60},
	"h": {"time", "h", 3600}, "hr": {"time", "h", 3600}, "hour": {"time", "h", 3600}, "hours": {"time", "h", 3600},
	"d": {"time", "d", 86400}, "day": {"time", "d", 86400}, "days": {"time", "d", 86400},
	"wk": {"time", "wk", 604800}, "week": {"time", "wk", 604800}, "weeks": {"time", "wk", 604800},
}

// convertUnits converts val from fromRaw to toRaw.
// Returns result, fromLabel, toLabel, ok.
func convertUnits(val float64, fromRaw, toRaw string) (float64, string, string, bool) {
	from, ok := unitAliases[strings.ToLower(fromRaw)]
	if !ok {
		return 0, "", "", false
	}
	to, ok := unitAliases[strings.ToLower(toRaw)]
	if !ok {
		return 0, "", "", false
	}
	if from.family != to.family {
		return 0, "", "", false
	}

	if from.family == "temperature" {
		result, ok := convertTemperature(val, from.canonical, to.canonical)
		return result, from.canonical, to.canonical, ok
	}

	// Linear: convert to base then to target
	base := val * from.toBase
	result := base / to.toBase
	return result, from.canonical, to.canonical, true
}

// convertTemperature handles the non-linear temperature conversions.
func convertTemperature(val float64, from, to string) (float64, bool) {
	// Normalise to Celsius first
	var celsius float64
	switch from {
	case "°C":
		celsius = val
	case "°F":
		celsius = (val - 32) * 5 / 9
	case "K":
		celsius = val - 273.15
	default:
		return 0, false
	}
	switch to {
	case "°C":
		return celsius, true
	case "°F":
		return celsius*9/5 + 32, true
	case "K":
		return celsius + 273.15, true
	}
	return 0, false
}
