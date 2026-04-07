package wizard

// DefaultExclusions lists country codes available for exclusion from proxy
var DefaultExclusions = []string{
	"ru", "ua", "by", "kz", "de",
	"fr", "nl", "pl", "tr", "il",
}

// CountryNames maps country codes to human-readable names
var CountryNames = map[string]string{
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

var RouteOptions = []string{
	"xray",
	"ovpnc1", "ovpnc2", "ovpnc3", "ovpnc4", "ovpnc5",
	"wgc1", "wgc2", "wgc3", "wgc4", "wgc5",
}
