package jval

import (
	"testing"
	"errors"

	"encoding/json/v2"
	"encoding/json/jsontext"
)

func TestMarshalJSON_roundtrip(t *testing.T) {
	type v struct {
		k byte
		s string
		i int64
		f float64
		b bool
	}
	tests := []struct{
		l Val
		s []byte
		v v
	}{
		{Val(jsontext.Null),           []byte(`null`),   v{k: 'n'}},
		{Val(jsontext.String("test")), []byte(`"test"`), v{k: 's', s: "test"}},
		{Val(jsontext.Int(-3456)),     []byte(`-3456`),  v{k: 'i', i: -3456}},
		{Val(jsontext.Float(-346.3)),  []byte(`-346.3`), v{k: 'f', f: -346.3}},
		{Val(jsontext.Bool(true)),     []byte(`true`),   v{k: 'b', b: true}},
		{Val(jsontext.Bool(false)),    []byte(`false`),  v{k: 'b', b: false}},
	}

	for _, x := range tests {
		s, err := json.Marshal(x.l)
		if string(x.s) != string(s) || err != nil {
			t.Errorf("Marshal(%v) s: %s, err: %v", x.l, s, err)
		}

		var v Val
		err = json.Unmarshal(x.s, &v)
		if err != nil {
			t.Errorf("Unmarshal(%v) err: %v", string(x.s), err)
		}
		switch x.v.k {
		case 'n':
		case 's':
			if v_ := jsontext.Token(v).String(); v_ != x.v.s {
				t.Errorf("Unmarshal(%v) exp: %v, got: %v",
					string(x.s), x.v.s, v_)
			}
		case 'i':
			if v_ := jsontext.Token(v).Int(); v_ != x.v.i {
				t.Errorf("Unmarshal(%v) exp: %v, got: %v",
					string(x.s), x.v.s, v_)
			}
		case 'f':
			if v_ := jsontext.Token(v).Float(); v_ != x.v.f {
				t.Errorf("Unmarshal(%v) exp: %v, got: %v",
					string(x.s), x.v.s, v_)
			}
		case 'b':
			if v_ := jsontext.Token(v).Bool(); v_ != x.v.b {
				t.Errorf("Unmarshal(%v) exp: %v, got: %v",
					string(x.s), x.v.s, v_)
			}
		}
	}
}

var errInvalidUTF8 error

func init() {
	_, err := json.Marshal("\xff")
	var serr *jsontext.SyntacticError
	if errors.As(err, &serr) {
		errInvalidUTF8 = serr.Err
	}
}

func TestUnmarshalJSON_fail(t *testing.T) {
	tests := []struct{
		s []byte
		e error
	}{
		{[]byte(`[]`), ErrInvalidValue},
		{[]byte(`{}`), ErrInvalidValue},
		{[]byte("\"ab\xff\""), errInvalidUTF8},
	}

	for _, x := range tests {
		var v Val
		err := json.Unmarshal(x.s, &v)
		if !errors.Is(err, x.e) {
			t.Errorf("Unmarshal(%v) exp: ErrInvalidValue got: %v",
				string(x.s), err)
		}
	}
}

func TestQnullable(t *testing.T) {
	tests := []struct{
		l Val
		t string
		s string
	}{
		{Val(jsontext.Null),           "bigint",  "NULL"},
		{Val(jsontext.String("t''t")), "text",    "'t''''t'"},
		{Val(jsontext.Int(-3456)),     "bigint",  "'-3456'"},
		{Val(jsontext.Float(-346.3)),  "numeric", "'-346.3'"},
		{Val(jsontext.Bool(true)),     "boolean", "TRUE"},
		{Val(jsontext.Bool(false)),    "boolean", "FALSE"},
		{Val(jsontext.String("aabb")), "bytea",   `'\xaabb'`},
	}

	for _, x := range tests {
		if got := x.l.Qnullable(x.t); got != x.s {
			t.Errorf("Qnullable(%v): %v != %v",
				jsontext.Token(x.l).String(), got, x.s)
		}
	}

	t.Run("panic", func(t *testing.T) {
		defer func() {
			r := recover()
			if r == nil {
				t.Fatal("expected panic, but got none")
			}

			exp := `unexpected VAL(kind: "{", string: "{")`
			if r != exp {
				t.Fatalf("expected panic <%s>, got <%v>", exp, r)
			}
		}()
		Val(jsontext.BeginObject).Qnullable("text")
	})
}

func TestIsNull(t *testing.T) {
	tests := []struct{
		l Val
		b bool
	}{
		{Val(jsontext.Null),           true},
		{Val(jsontext.String("t''t")), false},
		{Val(jsontext.Int(-3456)),     false},
		{Val(jsontext.Float(-346.3)),  false},
		{Val(jsontext.Bool(true)),     false},
		{Val(jsontext.Bool(false)),    false},
		{Val(jsontext.String("aabb")), false},
	}

	for _, x := range tests {
		if got := x.l.IsNull(); got != x.b {
			t.Errorf("IsNull(%v): %v != %v",
				jsontext.Token(x.l).String(), got, x.b)
		}
	}
}

func TestIsEqualTo(t *testing.T) {
	tests := []struct{
		l Val
		other Val
		b bool
	}{
		{Val(jsontext.Null),           Val(jsontext.Null),           true},
		{Val(jsontext.Null),           Val(jsontext.Bool(true)),     false},
		{Val(jsontext.Null),           Val(jsontext.Bool(false)),    false},
		{Val(jsontext.Null),           Val(jsontext.String("t''t")), false},
		{Val(jsontext.Null),           Val(jsontext.Int(-3)),        false},
		{Val(jsontext.String("t''t")), Val(jsontext.String("t''t")), true},
		{Val(jsontext.String("t''t")), Val(jsontext.String("tt")),   false},
		{Val(jsontext.Int(-3456)),     Val(jsontext.Int(-3456)),     true},
		{Val(jsontext.Int(-3456)),     Val(jsontext.Int(3456)),      false},
		{Val(jsontext.Float(-346.3)),  Val(jsontext.Float(-346.3)),  true},
		{Val(jsontext.Bool(true)),     Val(jsontext.Bool(true)),     true},
		{Val(jsontext.Bool(false)),    Val(jsontext.Bool(false)),    true},
		{Val(jsontext.String("aabb")), Val(jsontext.String("aabb")), true},
	}

	for _, x := range tests {
		if got := x.l.IsEqualTo(x.other); got != x.b {
			t.Errorf("<%v.>IsEqualTo(%v): %v != %v",
				jsontext.Token(x.l).String(), jsontext.Token(x.other).String(),
				got, x.b)
		}
	}
}

// Local Variables:
// tab-width: 4
// End:
