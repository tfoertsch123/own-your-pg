package msg

import (
	"testing"
)

func TestDecodePGQualifiedName(t *testing.T) {
	cases := []struct {
		name   string
		input  string
		parts  []string
		rest   string
		err    error
		errMsg string
	}{
		// --- success cases ---
		{
			name:  "single unquoted",
			input: "table",
			parts: []string{"table"},
			rest:  "",
		},
		{
			name:  "two unquoted",
			input: "schema.table",
			parts: []string{"schema", "table"},
			rest:  "",
		},
		{
			name:  "three unquoted",
			input: "db.schema.table",
			parts: []string{"db", "schema", "table"},
			rest:  "",
		},
		{
			name:  "unquoted is lowercased",
			input: "Schema.TABLE",
			parts: []string{"schema", "table"},
			rest:  "",
		},
		{
			name:  "leading and trailing whitespace",
			input: "  schema.table  ",
			parts: []string{"schema", "table"},
			rest:  "",
		},
		{
			name:  "rest after name",
			input: "schema.table rest of input",
			parts: []string{"schema", "table"},
			rest:  "rest of input",
		},
		{
			name:  "simple quoted",
			input: `"Table"`,
			parts: []string{"Table"},
			rest:  "",
		},
		{
			name:  "quoted preserves case",
			input: `"MixedCase"`,
			parts: []string{"MixedCase"},
			rest:  "",
		},
		{
			name:  "quoted with embedded quotes",
			input: `"we""ird"`,
			parts: []string{`we"ird`},
			rest:  "",
		},
		{
			name:  "quoted with spaces and dots",
			input: `"has space.and.dot"`,
			parts: []string{"has space.and.dot"},
			rest:  "",
		},
		{
			name:  "mixed quoted and unquoted",
			input: `"Schema".Table`,
			parts: []string{"Schema", "table"},
			rest:  "",
		},
		{
			name:  "whitespace around dot",
			input: "schema . table",
			parts: []string{"schema", "table"},
			rest:  "",
		},
		{
			name:  "whitespace after dot before quoted",
			input: `schema. "table"`,
			parts: []string{"schema", "table"},
			rest:  "",
		},
		{
			name:  "digits in later positions",
			input: "t1.col2",
			parts: []string{"t1", "col2"},
			rest:  "",
		},
		{
			name:  "dollar in later positions",
			input: "t$.c$1",
			parts: []string{"t$", "c$1"},
			rest:  "",
		},
		{
			name:  "unicode letters unquoted",
			input: "schéma.tablé",
			parts: []string{"schéma", "tablé"},
			rest:  "",
		},
		{
			name:  "underscore start",
			input: "_private",
			parts: []string{"_private"},
			rest:  "",
		},
		{
			name:  "leading whitespace with quoted",
			input: `  "Schema"."Table"  `,
			parts: []string{"Schema", "Table"},
			rest:  "",
		},
		// --- error cases ---
		{
			name:   "empty input",
			input:  "",
			errMsg: "empty name",
		},
		{
			name:   "whitespace only input",
			input:  "   ",
			errMsg: "empty name",
		},
		{
			name:   "dangling dot",
			input:  "schema.",
			errMsg: "empty identifier component",
		},
		{
			name:   "dot only",
			input:  ".",
			errMsg: "empty identifier component",
		},
		{
			name:   "unterminated quote",
			input:  `"unterminated`,
			errMsg: "unterminated quoted identifier",
		},
		{
			name:   "quote in unquoted identifier",
			input:  `bad"quote`,
			errMsg: "quote in unquoted identifier",
		},
		{
			name:   "digit at start of unquoted",
			input:  "9table",
			errMsg: "empty identifier component",
		},
		{
			name:   "dot directly after quote start fails",
			input:  `."x"`,
			errMsg: "empty identifier component",
		},
		{
			name:   "invalid utf8",
			input:  "\xff\xfe",
			errMsg: "invalid utf-8 in identifier",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			parts, rest, err := DecodePGQualifiedName(tc.input)

			if tc.errMsg != "" {
				if err == nil {
					t.Fatalf("expected error %q, got nil (parts=%v, rest=%q)",
						tc.errMsg, parts, rest)
				}
				if err.Error() != tc.errMsg {
					t.Errorf("error: got %q, want %q", err.Error(), tc.errMsg)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if rest != tc.rest {
				t.Errorf("rest: got %q, want %q", rest, tc.rest)
			}
			if len(parts) != len(tc.parts) {
				t.Fatalf("parts length: got %d (%v), want %d (%v)",
					len(parts), parts, len(tc.parts), tc.parts)
			}
			for i := range parts {
				if parts[i] != tc.parts[i] {
					t.Errorf("parts[%d]: got %q, want %q",
						i, parts[i], tc.parts[i])
				}
			}
		})
	}
}

func TestDecodePGQualifiedNames(t *testing.T) {
	cases := []struct {
		name    string
		input   string
		delim   string
		want    [][]string
		wantErr string
	}{
		{
			name:  "blank input",
			input: "",
			delim: ",",
		},
		{
			name:  "whitespace only input",
			input: "   ",
			delim: ",",
		},
		{
			name:  "single name",
			input: "table",
			delim: ",",
			want:  [][]string{{"table"}},
		},
		{
			name:  "comma separated",
			input: "a,b,c",
			delim: ",",
			want:  [][]string{{"a"}, {"b"}, {"c"}},
		},
		{
			name:  "comma separated with spaces",
			input: "a, b , c",
			delim: ",",
			want:  [][]string{{"a"}, {"b"}, {"c"}},
		},
		{
			name:  "multi-component names",
			input: "s.t1, s.t2",
			delim: ",",
			want:  [][]string{{"s", "t1"}, {"s", "t2"}},
		},
		{
			name:  "quoted names in list",
			input: `"A B","C D"`,
			delim: ",",
			want:  [][]string{{"A B"}, {"C D"}},
		},
		{
			name:  "multi-char delimiter",
			input: "a::b::c",
			delim: "::",
			want:  [][]string{{"a"}, {"b"}, {"c"}},
		},
		{
			name:  "trailing delimiter is error",
			input: "a, b,",
			delim: ",",
			wantErr: "empty name",
		},
		{
			name:    "missing delimiter",
			input:   "a b",
			delim:   ",",
			wantErr: "expected list delimted by ,",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var got [][]string
			var gotErr string
			for parts, err := range DecodePGQualifiedNames(tc.input, tc.delim) {
				if err != nil {
					gotErr = err.Error()
					break
				}
				got = append(got, parts)
			}

			if tc.wantErr != "" {
				if gotErr != tc.wantErr {
					t.Errorf("error: got %q, want %q", gotErr, tc.wantErr)
				}
				return
			}

			if gotErr != "" {
				t.Fatalf("unexpected error: %s", gotErr)
			}
			if len(got) != len(tc.want) {
				t.Fatalf("names: got %d (%v), want %d (%v)",
					len(got), got, len(tc.want), tc.want)
			}
			for i := range got {
				if len(got[i]) != len(tc.want[i]) {
					t.Fatalf("names[%d]: got %v, want %v",
						i, got[i], tc.want[i])
				}
				for j := range got[i] {
					if got[i][j] != tc.want[i][j] {
						t.Errorf("names[%d][%d]: got %q, want %q",
							i, j, got[i][j], tc.want[i][j])
					}
				}
			}
		})
	}
}

func TestDecodePGQualifiedNames_EarlyBreak(t *testing.T) {
	// Verify that breaking out of the range loop stops iteration
	// cleanly (the yield-false path in the generator).
	count := 0
	for parts, err := range DecodePGQualifiedNames("a,b,c", ",") {
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		count++
		if count == 2 {
			break
		}
		_ = parts
	}
	if count != 2 {
		t.Errorf("expected to consume 2 names, got %d", count)
	}
}

func TestQident(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{name: "simple", input: "foo", want: `"foo"`},
		{name: "mixed case", input: "MixedCase", want: `"MixedCase"`},
		{name: "empty", input: "", want: `""`},
		{name: "with space", input: "has space", want: `"has space"`},
		{name: "with dot", input: "schema.table", want: `"schema.table"`},
		{name: "embedded quote", input: `we"ird`, want: `"we""ird"`},
		{name: "only a quote", input: `"`, want: `""""`},
		{name: "two embedded quotes", input: `a"b"c`, want: `"a""b""c"`},
		{name: "unicode", input: "schéma", want: `"schéma"`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Qident(tc.input)
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestQliteralString(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{name: "simple", input: "foo", want: `'foo'`},
		{name: "empty", input: "", want: `''`},
		{name: "with space", input: "has space", want: `'has space'`},
		{name: "embedded quote", input: "it's", want: `'it''s'`},
		{name: "only a quote", input: "'", want: `''''`},
		{name: "two embedded quotes", input: "a'b'c", want: `'a''b''c'`},
		{name: "unicode", input: "schéma", want: `'schéma'`},
		{name: "double quote not escaped", input: `say "hi"`, want: `'say "hi"'`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := QliteralString(tc.input)
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// Local Variables:
// tab-width: 4
// End:

