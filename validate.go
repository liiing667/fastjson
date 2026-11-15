package fastjson

import (
	"fmt"
	"strconv"
	"strings"
	"unicode/utf16"
)

// Validate validates JSON s.
//
// Validate is lenient towards duplicate object keys and unpaired UTF-16
// surrogates in \u escape sequences. Use ValidateStrict if these
// malformed inputs must be rejected.
func Validate(s string) error {
	s = skipWS(s)

	tail, err := validateValue(s, false)
	if err != nil {
		return fmt.Errorf("cannot parse JSON: %s; unparsed tail: %q", err, startEndString(tail))
	}
	tail = skipWS(tail)
	if len(tail) > 0 {
		return fmt.Errorf("unexpected tail: %q", startEndString(tail))
	}
	return nil
}

// ValidateBytes validates JSON b.
//
// See Validate for the performed checks.
func ValidateBytes(b []byte) error {
	return Validate(b2s(b))
}

// ValidateStrict validates JSON s using strict rules.
//
// Unlike Validate, it fails on duplicate object keys (compared after
// unescaping), unpaired UTF-16 surrogates in \u escape sequences and
// any other malformed input rejected by Validate, such as invalid
// escape sequences, raw control characters in strings and non-standard
// numbers.
//
// Parser.ParseStrict and Scanner.NextStrict perform the same checks,
// so malformed input fails consistently at every entry point.
func ValidateStrict(s string) error {
	s = skipWS(s)

	if err := validateStrictValue(s); err != nil {
		return fmt.Errorf("cannot parse JSON: %s", err)
	}
	return nil
}

// ValidateBytesStrict validates JSON b using strict rules.
//
// See ValidateStrict for the performed checks.
func ValidateBytesStrict(b []byte) error {
	return ValidateStrict(b2s(b))
}

// validateStrictValue validates s containing a single JSON value
// using strict rules. s may contain leading and/or trailing whitespace.
func validateStrictValue(s string) error {
	s = skipWS(s)
	tail, err := validateValue(s, true)
	if err != nil {
		return fmt.Errorf("%s; unparsed tail: %q", err, startEndString(tail))
	}
	tail = skipWS(tail)
	if len(tail) > 0 {
		return fmt.Errorf("unexpected tail: %q", startEndString(tail))
	}
	return nil
}

func validateValue(s string, strict bool) (string, error) {
	if len(s) == 0 {
		return s, fmt.Errorf("cannot parse empty string")
	}

	if s[0] == '{' {
		tail, err := validateObject(s[1:], strict)
		if err != nil {
			return tail, fmt.Errorf("cannot parse object: %s", err)
		}
		return tail, nil
	}
	if s[0] == '[' {
		tail, err := validateArray(s[1:], strict)
		if err != nil {
			return tail, fmt.Errorf("cannot parse array: %s", err)
		}
		return tail, nil
	}
	if s[0] == '"' {
		sv, tail, err := validateString(s[1:], strict)
		if err != nil {
			return tail, fmt.Errorf("cannot parse string: %s", err)
		}
		// Scan the string for control chars.
		for i := range len(sv) {
			if sv[i] < 0x20 {
				return tail, fmt.Errorf("string cannot contain control char 0x%02X", sv[i])
			}
		}
		return tail, nil
	}
	if s[0] == 't' {
		if len(s) < len("true") || s[:len("true")] != "true" {
			return s, fmt.Errorf("unexpected value found: %q", s)
		}
		return s[len("true"):], nil
	}
	if s[0] == 'f' {
		if len(s) < len("false") || s[:len("false")] != "false" {
			return s, fmt.Errorf("unexpected value found: %q", s)
		}
		return s[len("false"):], nil
	}
	if s[0] == 'n' {
		if len(s) < len("null") || s[:len("null")] != "null" {
			return s, fmt.Errorf("unexpected value found: %q", s)
		}
		return s[len("null"):], nil
	}

	tail, err := validateNumber(s)
	if err != nil {
		return tail, fmt.Errorf("cannot parse number: %s", err)
	}
	return tail, nil
}

func validateArray(s string, strict bool) (string, error) {
	s = skipWS(s)
	if len(s) == 0 {
		return s, fmt.Errorf("missing ']'")
	}
	if s[0] == ']' {
		return s[1:], nil
	}

	for {
		var err error

		s = skipWS(s)
		s, err = validateValue(s, strict)
		if err != nil {
			return s, fmt.Errorf("cannot parse array value: %s", err)
		}

		s = skipWS(s)
		if len(s) == 0 {
			return s, fmt.Errorf("unexpected end of array")
		}
		if s[0] == ',' {
			s = s[1:]
			continue
		}
		if s[0] == ']' {
			s = s[1:]
			return s, nil
		}
		return s, fmt.Errorf("missing ',' after array value")
	}
}

func validateObject(s string, strict bool) (string, error) {
	s = skipWS(s)
	if len(s) == 0 {
		return s, fmt.Errorf("missing '}'")
	}
	if s[0] == '}' {
		return s[1:], nil
	}

	var seenKeys map[string]struct{}
	for {
		var err error

		// Parse key.
		s = skipWS(s)
		if len(s) == 0 || s[0] != '"' {
			return s, fmt.Errorf(`cannot find opening '"" for object key`)
		}

		var key string
		rest := s[1:]
		key, s, err = validateKey(rest, strict)
		if err != nil {
			return s, fmt.Errorf("cannot parse object key: %s", err)
		}
		// Scan the key for control chars.
		for i := range len(key) {
			if key[i] < 0x20 {
				return s, fmt.Errorf("object key cannot contain control char 0x%02X", key[i])
			}
		}
		if strict {
			// rest contains the raw key followed by the closing quote and s.
			// The key returned by validateKey cannot be used here, since
			// it may contain only a part of the raw key - see validateString.
			uk := rest[:len(rest)-len(s)-1]
			if strings.IndexByte(uk, '\\') >= 0 {
				// The key has been already validated by validateKey,
				// so it cannot contain invalid escape sequences.
				uk = unescapeStringValidated(uk)
			}
			if seenKeys == nil {
				seenKeys = make(map[string]struct{})
			}
			if _, ok := seenKeys[uk]; ok {
				return s, fmt.Errorf("duplicate object key %q", uk)
			}
			seenKeys[uk] = struct{}{}
		}
		s = skipWS(s)
		if len(s) == 0 || s[0] != ':' {
			return s, fmt.Errorf("missing ':' after object key")
		}
		s = s[1:]

		// Parse value
		s = skipWS(s)
		s, err = validateValue(s, strict)
		if err != nil {
			return s, fmt.Errorf("cannot parse object value: %s", err)
		}
		s = skipWS(s)
		if len(s) == 0 {
			return s, fmt.Errorf("unexpected end of object")
		}
		if s[0] == ',' {
			s = s[1:]
			continue
		}
		if s[0] == '}' {
			return s[1:], nil
		}
		return s, fmt.Errorf("missing ',' after object value")
	}
}

// validateKey is similar to validateString, but is optimized
// for typical object keys, which are quite small and have no escape sequences.
func validateKey(s string, strict bool) (string, string, error) {
	for i := range len(s) {
		if s[i] == '"' {
			// Fast path - the key doesn't contain escape sequences.
			return s[:i], s[i+1:], nil
		}
		if s[i] == '\\' {
			// Slow path - the key contains escape sequences.
			return validateString(s, strict)
		}
	}
	return "", s, fmt.Errorf(`missing closing '"'`)
}

func validateString(s string, strict bool) (string, string, error) {
	// Try fast path - a string without escape sequences.
	if n := strings.IndexByte(s, '"'); n >= 0 && strings.IndexByte(s[:n], '\\') < 0 {
		return s[:n], s[n+1:], nil
	}

	// Slow path - escape sequences are present.
	rs, tail, err := parseRawString(s)
	if err != nil {
		return rs, tail, err
	}
	for {
		n := strings.IndexByte(rs, '\\')
		if n < 0 {
			return rs, tail, nil
		}
		n++
		if n >= len(rs) {
			return rs, tail, fmt.Errorf("BUG: parseRawString returned invalid string with trailing backslash: %q", rs)
		}
		ch := rs[n]
		rs = rs[n+1:]
		switch ch {
		case '"', '\\', '/', 'b', 'f', 'n', 'r', 't':
			// Valid escape sequences - see http://json.org/
			break
		case 'u':
			if len(rs) < 4 {
				return rs, tail, fmt.Errorf(`too short escape sequence: \u%s`, rs)
			}
			xs := rs[:4]
			x, err := strconv.ParseUint(xs, 16, 16)
			if err != nil {
				return rs, tail, fmt.Errorf(`invalid escape sequence \u%s: %s`, xs, err)
			}
			rs = rs[4:]
			if strict && x >= 0xD800 && x <= 0xDFFF {
				// Surrogate.
				// See https://en.wikipedia.org/wiki/Universal_Character_Set_characters#Surrogates
				if x > 0xDBFF {
					return rs, tail, fmt.Errorf(`unexpected unpaired low surrogate \u%s`, xs)
				}
				hs := xs
				if len(rs) < 6 || rs[0] != '\\' || rs[1] != 'u' {
					return rs, tail, fmt.Errorf(`missing low surrogate after high surrogate \u%s`, hs)
				}
				xs = rs[2:6]
				x, err = strconv.ParseUint(xs, 16, 16)
				if err != nil {
					return rs, tail, fmt.Errorf(`invalid escape sequence \u%s: %s`, xs, err)
				}
				if x < 0xDC00 || x > 0xDFFF {
					return rs, tail, fmt.Errorf(`unexpected high surrogate \u%s followed by \u%s`, hs, xs)
				}
				rs = rs[6:]
			}
		default:
			return rs, tail, fmt.Errorf(`unknown escape sequence \%c`, ch)
		}
	}
}

// unescapeStringValidated unescapes s, which has been already validated
// by validateString, into a newly allocated string.
//
// It cannot be used on strings, which haven't been validated yet.
func unescapeStringValidated(s string) string {
	n := strings.IndexByte(s, '\\')
	if n < 0 {
		// Fast path - nothing to unescape.
		return s
	}

	// Slow path - unescape string.
	// Unlike unescapeStringBestEffort, b cannot point to s,
	// since s may point to a read-only memory.
	b := make([]byte, 0, len(s))
	b = append(b, s[:n]...)
	s = s[n+1:]
	for len(s) > 0 {
		ch := s[0]
		s = s[1:]
		switch ch {
		case '"':
			b = append(b, '"')
		case '\\':
			b = append(b, '\\')
		case '/':
			b = append(b, '/')
		case 'b':
			b = append(b, '\b')
		case 'f':
			b = append(b, '\f')
		case 'n':
			b = append(b, '\n')
		case 'r':
			b = append(b, '\r')
		case 't':
			b = append(b, '\t')
		case 'u':
			// s is validated, so it contains exactly 4 hex chars.
			x, _ := strconv.ParseUint(s[:4], 16, 16)
			s = s[4:]
			if !utf16.IsSurrogate(rune(x)) {
				b = append(b, string(rune(x))...)
				break
			}

			// Surrogate pair. s is validated, so it contains
			// a valid low surrogate after the high surrogate.
			x1, _ := strconv.ParseUint(s[2:6], 16, 16)
			r := utf16.DecodeRune(rune(x), rune(x1))
			b = append(b, string(r)...)
			s = s[6:]
		}
		n = strings.IndexByte(s, '\\')
		if n < 0 {
			b = append(b, s...)
			break
		}
		b = append(b, s[:n]...)
		s = s[n+1:]
	}
	return b2s(b)
}

func validateNumber(s string) (string, error) {
	if len(s) == 0 {
		return s, fmt.Errorf("zero-length number")
	}
	if s[0] == '-' {
		s = s[1:]
		if len(s) == 0 {
			return s, fmt.Errorf("missing number after minus")
		}
	}
	i := 0
	for i < len(s) {
		if s[i] < '0' || s[i] > '9' {
			break
		}
		i++
	}
	if i <= 0 {
		return s, fmt.Errorf("expecting 0..9 digit, got %c", s[0])
	}
	if s[0] == '0' && i != 1 {
		return s, fmt.Errorf("unexpected number starting from 0")
	}
	if i >= len(s) {
		return "", nil
	}
	if s[i] == '.' {
		// Validate fractional part
		s = s[i+1:]
		if len(s) == 0 {
			return s, fmt.Errorf("missing fractional part")
		}
		i = 0
		for i < len(s) {
			if s[i] < '0' || s[i] > '9' {
				break
			}
			i++
		}
		if i == 0 {
			return s, fmt.Errorf("expecting 0..9 digit in fractional part, got %c", s[0])
		}
		if i >= len(s) {
			return "", nil
		}
	}
	if s[i] == 'e' || s[i] == 'E' {
		// Validate exponent part
		s = s[i+1:]
		if len(s) == 0 {
			return s, fmt.Errorf("missing exponent part")
		}
		if s[0] == '-' || s[0] == '+' {
			s = s[1:]
			if len(s) == 0 {
				return s, fmt.Errorf("missing exponent part")
			}
		}
		i = 0
		for i < len(s) {
			if s[i] < '0' || s[i] > '9' {
				break
			}
			i++
		}
		if i == 0 {
			return s, fmt.Errorf("expecting 0..9 digit in exponent part, got %c", s[0])
		}
		if i >= len(s) {
			return "", nil
		}
	}
	return s[i:], nil
}
