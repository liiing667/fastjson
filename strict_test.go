package fastjson

import (
	"strings"
	"testing"
)

func TestValidateStrictInvalid(t *testing.T) {
	tests := []string{
		// Duplicate object keys.
		`{"a":1,"a":2}`,
		`{"a":1,"b":2,"a":3}`,
		`{"a":{"x":1,"x":2}}`,
		`[{"a":1,"a":2}]`,
		`{"":1,"":2}`,
		// Duplicate object keys after unescaping.
		`{"\u0061":1,"a":2}`,
		`{"ab":1,"a\u0062":2}`,

		// Invalid \u escape sequences.
		`"\u"`,
		`"\u12"`,
		`"\uzzzz"`,
		`"foo\ubarz2134"`,
		`{"\u":1}`,
		`"foo\qw"`,

		// Unpaired surrogates.
		`"\ud800"`,
		`"\ud83d"`,
		`"\udfff"`,
		`"\udc00"`,
		`"\ud83d\u0041"`,
		`"\ud83d\ud83d"`,
		`"\ud83dx"`,
		`"\ud83d\n"`,
		`{"\ud800":1}`,
		`{"a":"x\ude00y"}`,
		`["\udc00"]`,

		// Raw control characters in strings.
		"\"fo\x19bar\"",
		"\"foo\nbar\"",
		"{\"a\x02b\":1}",
		"{\"a\":\"b\x00c\"}",
		"[\"\x7f\x01\"]",

		// Non-standard numbers.
		`nan`,
		`NaN`,
		`inf`,
		`-inf`,
		`+1`,
		`01`,
		`1.`,
		`.5`,
		`1e`,
		`{"a":nan}`,
		`[1.,2]`,
	}
	for _, s := range tests {
		if err := ValidateStrict(s); err == nil {
			t.Errorf("ValidateStrict(%q) unexpectedly passed", s)
		}
		if err := ValidateBytesStrict([]byte(s)); err == nil {
			t.Errorf("ValidateBytesStrict(%q) unexpectedly passed", s)
		}
	}
}

func TestValidateStrictValid(t *testing.T) {
	tests := []string{
		// Normal numbers.
		`0`,
		`123`,
		`-12.5e+6`,
		`0.5`,
		`1e6`,
		`-0.01e+006`,
		`[1, -2, 3.5, 4e-2, 0.25E+10]`,

		// Nested objects and arrays.
		`{}`,
		`{"a":[1,2,{"b":"c"}],"d":{"e":[true,false,null]}}`,
		strings.Repeat(`{"f":`, 100) + "{}" + strings.Repeat("}", 100),

		// Duplicate keys in distinct objects aren't duplicates.
		`{"a":{"x":1},"b":{"x":2}}`,
		`[{"a":1},{"a":2}]`,
		// Duplicate array values aren't duplicate keys.
		`["a","a"]`,

		// Unicode.
		`"привет 🤭"`,
		`"\uD83D\uDE00"`,
		`"\uD83E\uDD2D"`,
		`"п\ud83e\udd2dи"`,
		`{"\u0061":1}`,
		`{"тест":1,"фыва":"\fЗУ\\"}`,
		`{"a":{"a":1},"\u0061\u0062":{"ab":[1,2]}}`,
		`"\n\t\\foo\"bar\u3423x\/\b\f\r\\"`,
	}
	for _, s := range tests {
		if err := ValidateStrict(s); err != nil {
			t.Errorf("ValidateStrict(%q) failed: %s", s, err)
		}
		if err := ValidateBytesStrict([]byte(s)); err != nil {
			t.Errorf("ValidateBytesStrict(%q) failed: %s", s, err)
		}
	}
}

// malformedStrictInputs must be rejected by every strict entry point.
var malformedStrictInputs = []string{
	// Duplicate object keys.
	`{"a":1,"a":2}`,
	`{"a":1,"\u0061":2}`,
	`{"a":{"x":1,"x":2}}`,

	// Invalid \u escape sequences.
	`{"a":"foo\ubarz2134"}`,
	`{"a":"\u12"}`,

	// Unpaired surrogates.
	`{"a":"\ud83d"}`,
	`{"a":"\ude00"}`,
	`{"a":"\ud83d\u0041"}`,
	`{"\ud800":1}`,

	// Raw control characters.
	"{\"a\":\"fo\x19bar\"}",
	"{\"a\x02b\":1}",
}

func TestParseStrictInvalid(t *testing.T) {
	var p Parser
	for _, s := range malformedStrictInputs {
		if v, err := p.ParseStrict(s); err == nil {
			t.Errorf("ParseStrict(%q) unexpectedly passed; got %s", s, v)
		}
		if v, err := p.ParseBytesStrict([]byte(s)); err == nil {
			t.Errorf("ParseBytesStrict(%q) unexpectedly passed; got %s", s, v)
		}

		// The same input must fail via the strict Scanner path.
		var sc Scanner
		sc.Init(s)
		if sc.NextStrict() {
			t.Errorf("NextStrict(%q) unexpectedly passed; got %s", s, sc.Value())
		}
		if err := sc.Error(); err == nil {
			t.Errorf("Scanner.Error(%q) must be non-nil after failed NextStrict", s)
		}
		if sc.NextStrict() {
			t.Errorf("NextStrict(%q) must keep returning false after an error", s)
		}
	}
}

func TestParseStrictValid(t *testing.T) {
	tests := []string{
		`123`,
		`-12.5e+6`,
		`[1, -2, 3.5, 4e-2]`,
		`{"a":[1,2,{"b":"c"}],"d":{"e":[true,false,null]},"n":null}`,
		`{"тест":1,"pair":"\uD83D\uDE00","esc":"a\u0062c"}`,
		`"п\ud83e\udd2dи"`,
	}
	var p Parser
	for _, s := range tests {
		v, err := p.ParseStrict(s)
		if err != nil {
			t.Fatalf("ParseStrict(%q) failed: %s", s, err)
		}
		vLenient, err := p.Parse(s)
		if err != nil {
			t.Fatalf("Parse(%q) failed: %s", s, err)
		}
		if v.String() != vLenient.String() {
			t.Fatalf("ParseStrict(%q)=%s doesn't match Parse result %s", s, v, vLenient)
		}

		var sc Scanner
		sc.Init(s)
		if !sc.NextStrict() {
			t.Fatalf("NextStrict(%q) failed: %s", s, sc.Error())
		}
		if sc.Value().String() != vLenient.String() {
			t.Fatalf("NextStrict(%q)=%s doesn't match Parse result %s", s, sc.Value(), vLenient)
		}
		if sc.NextStrict() {
			t.Fatalf("NextStrict(%q) must return false after the last value", s)
		}
		if err := sc.Error(); err != nil {
			t.Fatalf("unexpected scanner error after %q: %s", s, err)
		}
	}
}

func TestScannerStrictMixedStream(t *testing.T) {
	var sc Scanner
	sc.Init(`1 {"a":"b"} "x"`)
	var values []string
	for sc.NextStrict() {
		values = append(values, sc.Value().String())
	}
	if err := sc.Error(); err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	if got, want := strings.Join(values, " "), `1 {"a":"b"} "x"`; got != want {
		t.Fatalf("unexpected values; got %q; want %q", got, want)
	}

	// The stream must stop at the first malformed value.
	sc.Init(`1 {"a":1,"a":2} 3`)
	if !sc.NextStrict() {
		t.Fatalf("cannot parse the first value: %s", sc.Error())
	}
	if got, want := sc.Value().String(), "1"; got != want {
		t.Fatalf("unexpected first value; got %q; want %q", got, want)
	}
	if sc.NextStrict() {
		t.Fatalf("NextStrict unexpectedly passed on a duplicate key")
	}
	if err := sc.Error(); err == nil {
		t.Fatalf("expecting non-nil error")
	}
}

func TestParseStrictLenientUnaffected(t *testing.T) {
	// The lenient API must keep accepting malformed inputs.
	var p Parser
	for _, s := range malformedStrictInputs {
		if _, err := p.Parse(s); err != nil {
			t.Errorf("Parse(%q) must remain lenient; got error: %s", s, err)
		}
	}

	// Validate must remain lenient towards duplicate keys.
	for _, s := range []string{`{"a":1,"a":2}`, `{"a":{"x":1,"x":2}}`, `{"a":1,"\u0061":2}`} {
		if err := Validate(s); err != nil {
			t.Errorf("Validate(%q) must remain lenient towards duplicate keys; got error: %s", s, err)
		}
	}

	// Object.Get keeps working on values obtained via the strict path.
	v, err := p.ParseStrict(`{"a":{"b":[1,2,{"c":"d"}]},"u":"\uD83D\uDE00"}`)
	if err != nil {
		t.Fatalf("ParseStrict failed: %s", err)
	}
	if got := v.Get("a", "b", "2", "c").GetStringBytes(); string(got) != "d" {
		t.Fatalf("unexpected value for a.b.2.c; got %q; want %q", got, "d")
	}
	if got := v.GetStringBytes("u"); string(got) != "\U0001F600" {
		t.Fatalf("unexpected value for u; got %q; want %q", got, "\U0001F600")
	}
	if !v.Exists("a", "b", "0") {
		t.Fatalf("a.b.0 must exist")
	}
}
