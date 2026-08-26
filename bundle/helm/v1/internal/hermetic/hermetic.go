package hermetic

import (
	"fmt"
	"text/template"

	"github.com/Masterminds/sprig/v3"
)

// allowed is the explicitly reviewed subset of sprig.TxtFuncMap permitted in
// hermetic templates. New Sprig functions are denied until added here.
var allowed = map[string]struct{}{
	"hello": {},

	// Date functions
	// "ago": {}, // Calls time.Now. See date.go:55.
	// "date": {}, // Sprig's documented non-hermetic functions.
	// "date_in_zone": {}, // Sprig's documented non-hermetic functions.
	// "date_modify": {}, // Sprig's documented non-hermetic functions.
	// "dateInZone": {}, // Sprig's documented non-hermetic functions.
	// "dateModify": {}, // Sprig's documented non-hermetic functions.
	"duration": {},
	// "durationRound": {}, // Calls time.Since for time.Time inputs. See date.go:97.
	// "htmlDate": {}, // Sprig's documented non-hermetic functions.
	// "htmlDateInZone": {}, // Sprig's documented non-hermetic functions.
	"must_date_modify": {},
	"mustDateModify":   {},
	"mustToDate":       {},
	// "now": {}, // Sprig's documented non-hermetic functions.
	"toDate":    {},
	"unixEpoch": {},

	// Strings
	"abbrev":     {},
	"abbrevboth": {},
	"trunc":      {},
	"trim":       {},
	"upper":      {},
	"lower":      {},
	"title":      {},
	"untitle":    {},
	"substr":     {},
	"repeat":     {},
	"trimall":    {},
	"trimAll":    {},
	"trimSuffix": {},
	"trimPrefix": {},
	"nospace":    {},
	"initials":   {},
	// "randAlphaNum": {}, // Sprig's documented non-hermetic functions.
	// "randAlpha": {}, // Sprig's documented non-hermetic functions.
	// "randAscii": {}, // Sprig's documented non-hermetic functions.
	// "randNumeric": {}, // Sprig's documented non-hermetic functions.
	"swapcase": {},
	// "shuffle": {}, // Uses math/rand. See strings.go:355.
	"snakecase":  {},
	"camelcase":  {},
	"kebabcase":  {},
	"wrap":       {},
	"wrapWith":   {},
	"contains":   {},
	"hasPrefix":  {},
	"hasSuffix":  {},
	"quote":      {},
	"squote":     {},
	"cat":        {},
	"indent":     {},
	"nindent":    {},
	"replace":    {},
	"plural":     {},
	"sha1sum":    {},
	"sha256sum":  {},
	"sha512sum":  {},
	"adler32sum": {},
	"toString":   {},
	"atoi":       {},
	"int64":      {},
	"int":        {},
	"float64":    {},
	"seq":        {},
	"toDecimal":  {},
	"split":      {},
	"splitList":  {},
	"splitn":     {},
	"toStrings":  {},
	"until":      {},
	"untilStep":  {},
	"add1":       {},
	"add":        {},
	"sub":        {},
	"div":        {},
	"mod":        {},
	"mul":        {},
	// "randInt": {}, // Uses math/rand. See numeric.go:23.
	"add1f":     {},
	"addf":      {},
	"subf":      {},
	"divf":      {},
	"mulf":      {},
	"biggest":   {},
	"max":       {},
	"min":       {},
	"maxf":      {},
	"minf":      {},
	"ceil":      {},
	"floor":     {},
	"round":     {},
	"join":      {},
	"sortAlpha": {},

	// Defaults
	"default":          {},
	"empty":            {},
	"coalesce":         {},
	"all":              {},
	"any":              {},
	"compact":          {},
	"mustCompact":      {},
	"fromJson":         {},
	"toJson":           {},
	"toPrettyJson":     {},
	"toRawJson":        {},
	"mustFromJson":     {},
	"mustToJson":       {},
	"mustToPrettyJson": {},
	"mustToRawJson":    {},
	"ternary":          {},
	"deepCopy":         {},
	"mustDeepCopy":     {},

	// Reflection
	"typeOf":     {},
	"typeIs":     {},
	"typeIsLike": {},
	"kindOf":     {},
	"kindIs":     {},
	"deepEqual":  {},

	// OS:
	// "env": {}, // Sprig's documented non-hermetic functions.
	// "expandenv": {}, // Sprig's documented non-hermetic functions.

	// Network:
	// "getHostByName": {}, // Sprig's documented non-hermetic functions.

	// Paths:
	"base":  {},
	"dir":   {},
	"clean": {},
	"ext":   {},
	"isAbs": {},

	// Filepaths:
	"osBase":  {},
	"osClean": {},
	"osDir":   {},
	"osExt":   {},
	"osIsAbs": {},

	// Encoding:
	"b64enc": {},
	"b64dec": {},
	"b32enc": {},
	"b32dec": {},

	// Data Structures:
	"tuple":  {},
	"list":   {},
	"dict":   {},
	"get":    {},
	"set":    {},
	"unset":  {},
	"hasKey": {},
	"pluck":  {},
	// "keys": {}, // Iterates maps and returns nondeterministically ordered keys. See dict.go:40.
	"pick":               {},
	"omit":               {},
	"merge":              {},
	"mergeOverwrite":     {},
	"mustMerge":          {},
	"mustMergeOverwrite": {},
	// "values": {}, // Iterates maps and returns nondeterministically ordered values. See dict.go:128.
	"append":      {},
	"push":        {},
	"mustAppend":  {},
	"mustPush":    {},
	"prepend":     {},
	"mustPrepend": {},
	"first":       {},
	"mustFirst":   {},
	"rest":        {},
	"mustRest":    {},
	"last":        {},
	"mustLast":    {},
	"initial":     {},
	"mustInitial": {},
	"reverse":     {},
	"mustReverse": {},
	"uniq":        {},
	"mustUniq":    {},
	"without":     {},
	"mustWithout": {},
	"has":         {},
	"mustHas":     {},
	"slice":       {},
	"mustSlice":   {},
	"concat":      {},
	"dig":         {},
	"chunk":       {},
	"mustChunk":   {},

	// Crypto:
	// "bcrypt": {}, // Generates a random salt. See crypto.go:31.
	// "htpasswd": {}, // Uses bcrypt, which generates a random salt. See crypto.go:56.
	// "genPrivateKey": {}, // Generates random key material. See crypto.go:153.
	"derivePassword":  {},
	"buildCustomCert": {},
	// "genCA": {}, // Generates random key material. See crypto.go:267.
	// "genCAWithKey": {}, // Generates a certificate with the current time. See crypto.go:292.
	// "genSelfSignedCert": {}, // Generates random key material. See crypto.go:356.
	// "genSelfSignedCertWithKey": {}, // Generates a certificate with the current time. See crypto.go:383.
	// "genSignedCert": {}, // Generates random key material. See crypto.go:414.
	// "genSignedCertWithKey": {}, // Generates a certificate with the current time. See crypto.go:445.
	// "encryptAES": {}, // Generates a random IV. See crypto.go:638.
	"decryptAES": {},
	// "randBytes": {}, // Sprig's documented non-hermetic functions.

	// UUIDs:
	// "uuidv4": {}, // Sprig's documented non-hermetic functions.

	// SemVer:
	"semver":        {},
	"semverCompare": {},

	// Flow Control:
	"fail": {},

	// Regex
	"regexMatch":                 {},
	"mustRegexMatch":             {},
	"regexFindAll":               {},
	"mustRegexFindAll":           {},
	"regexFind":                  {},
	"mustRegexFind":              {},
	"regexReplaceAll":            {},
	"mustRegexReplaceAll":        {},
	"regexReplaceAllLiteral":     {},
	"mustRegexReplaceAllLiteral": {},
	"regexSplit":                 {},
	"mustRegexSplit":             {},
	"regexQuoteMeta":             {},

	// URLs:
	"urlParse": {},
	"urlJoin":  {},
}

func isAllowed(name string) bool {
	_, ok := allowed[name]
	return ok
}

// UnsupportedTemplateFunction reports use of a function disallowed in hermetic rendering.
type UnsupportedTemplateFunction struct {
	Name string
}

func (e *UnsupportedTemplateFunction) Error() string {
	return fmt.Sprintf("template function %q is not permitted in hermetic rendering", e.Name)
}

func disallowedFunc(name string) func(...any) (any, error) {
	return func(...any) (any, error) {
		return nil, &UnsupportedTemplateFunction{Name: name}
	}
}

// Overrides replaces every non-allowlisted Sprig function with an error.
func Overrides() template.FuncMap {
	result := template.FuncMap{"lookup": disallowedFunc("lookup")}
	for name := range sprig.TxtFuncMap() {
		if !isAllowed(name) {
			result[name] = disallowedFunc(name)
		}
	}
	return result
}
