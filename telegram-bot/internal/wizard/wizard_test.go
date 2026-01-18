package wizard

import "testing"

func TestDefaultExclusions_Contains10Countries(t *testing.T) {
	expected := []string{"ru", "ua", "by", "kz", "de", "fr", "nl", "pl", "tr", "il"}
	if len(DefaultExclusions) != 10 {
		t.Errorf("expected 10 countries, got %d", len(DefaultExclusions))
	}
	for _, code := range expected {
		found := false
		for _, ex := range DefaultExclusions {
			if ex == code {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("missing country code: %s", code)
		}
	}
}

func TestCountryNames_HasAllCodes(t *testing.T) {
	for _, code := range DefaultExclusions {
		if _, ok := CountryNames[code]; !ok {
			t.Errorf("CountryNames missing entry for: %s", code)
		}
	}
}

func TestCountryNames_Values(t *testing.T) {
	tests := map[string]string{
		"ru": "Russia",
		"ua": "Ukraine",
		"by": "Belarus",
		"kz": "Kazakhstan",
		"de": "Germany",
		"fr": "France",
		"nl": "Netherlands",
		"pl": "Poland",
		"tr": "Turkey",
		"il": "Israel",
	}
	for code, expected := range tests {
		if CountryNames[code] != expected {
			t.Errorf("CountryNames[%s] = %s, want %s", code, CountryNames[code], expected)
		}
	}
}
