package msg

import (
	"testing"
	"slices"
	"encoding/json/jsontext"
	// "encoding/json/v2"
	"github.com/tfoertsch123/own-your-pg/jval"
)

func TestCOL(t *testing.T) {
	t.Run("String", func(t *testing.T) {
		v := COL{
			Name: "c.\"1",
			Type: "bigint",
			Value: jval.Val(jsontext.String("123")),
		}
		exp := `"c.""1"='123'::bigint`
		if got := v.String(); got != exp {
			t.Errorf("exp <%v>, got <%v>", exp, got)
		}
	})

	t.Run("Null", func(t *testing.T) {
		v := COL{
			Name: "c.\"1",
			Type: "bigint",
			Value: jval.Val(jsontext.Null),
		}
		exp := `"c.""1"=NULL::bigint`
		if got := v.String(); got != exp {
			t.Errorf("exp <%v>, got <%v>", exp, got)
		}
	})

	t.Run("EncodeColVector", func(t *testing.T) {
		v := []COL{
			COL{
				Name: "c.\"1",
				Type: "bigint",
				Value: jval.Val(jsontext.Null),
			},
			COL{
				Name: "c.\"1",
				Type: "bigint",
				Value: jval.Val(jsontext.String("123")),
			},
		}
		exp := `[{"name":"c.\"1","type":"bigint","value":null},`+
			    `{"name":"c.\"1","type":"bigint","value":"123"}]`

		enc, err := EncodeColVector(v)
		if err != nil {
			t.Errorf("got err <%v>", err)
		}
		if !slices.Equal(enc, jsontext.Value(exp)) {
			t.Errorf("exp <%v>, got <%v>", exp, enc)
		}
	})

	t.Run("EncodeColVector failed", func(t *testing.T) {
		v := []COL{
			COL{
				Name: "c.\"1",
				Type: "bigint\xff",
				Value: jval.Val(jsontext.Null),
			},
			COL{
				Name: "c.\"1",
				Type: "bigint",
				Value: jval.Val(jsontext.String("123")),
			},
		}
		_, err := EncodeColVector(v)
		if err == nil {
			t.Error("expecting error")
		}
	})

	t.Run("ColVectorEqual", func(t *testing.T) {
		if !ColVectorEqual([]COL(nil), []COL(nil)) {
			t.Error("nil != nil")
		}
		if !ColVectorEqual([]COL{}, []COL(nil)) {
			t.Error("[] != nil")
		}
		if !ColVectorEqual([]COL(nil), []COL{}) {
			t.Error("nil != []")
		}
		if !ColVectorEqual([]COL{}, []COL{}) {
			t.Error("[] != []")
		}

		v1 := []COL{
			COL{
				Name: "c.\"1",
				Type: "bigint\xff",
				Value: jval.Val(jsontext.Null),
			},
			COL{
				Name: "c.\"1",
				Type: "bigint",
				Value: jval.Val(jsontext.String("123")),
			},
		}
		v2 := []COL{
			COL{
				Name: "c.\"1",
				Type: "bigint\xff",
				Value: jval.Val(jsontext.Null),
			},
		}
		if ColVectorEqual(v1, v2) {
			t.Error("v1 == v2")
		}
		v3 := append(v2,
			COL{
				Name: "c.\"1",
				Type: "bigint",
				Value: jval.Val(jsontext.String("123")),
			},
		)
		if !ColVectorEqual(v1, v3) {
			t.Error("v1 != v3")
		}
		v4 := append(v2,
			COL{
				Name: "c.\"1",
				Type: "bigint",
				Value: jval.Val(jsontext.String("133")),
			},
		)
		if ColVectorEqual(v1, v4) {
			t.Error("v1 == v4")
		}
	})

	t.Run("EncodeMColVector", func(t *testing.T) {
		v := [][]COL{
			[]COL{},
			[]COL{
				COL{
					Name: "c.\"1",
					Type: "bigint",
					Value: jval.Val(jsontext.Null),
				},
				COL{
					Name: "c.\"1",
					Type: "bigint",
					Value: jval.Val(jsontext.String("123")),
				},
			},
			[]COL{},
		}
		exp := `[[],`+
			`[{"name":"c.\"1","type":"bigint","value":null},`+
			`{"name":"c.\"1","type":"bigint","value":"123"}],`+
			`[]]`
		enc, err := EncodeMColVector(v)
		if err != nil {
			t.Errorf("got err <%v>", err)
		}
		if !slices.Equal(enc, jsontext.Value(exp)) {
			t.Errorf("exp <%v>, got <%v>", exp, enc)
		}
	})

	t.Run("EncodeMColVector failed", func(t *testing.T) {
		v := [][]COL{
			[]COL{},
			[]COL{
				COL{
					Name: "c.\"1",
					Type: "bigint\xff",
					Value: jval.Val(jsontext.Null),
				},
				COL{
					Name: "c.\"1",
					Type: "bigint",
					Value: jval.Val(jsontext.String("123")),
				},
			},
			[]COL{},
		}
		_, err := EncodeMColVector(v)
		if err == nil {
			t.Error("expecting error")
		}
	})

	t.Run("MColVectorEqual", func(t *testing.T) {
		if !MColVectorEqual([][]COL(nil), [][]COL(nil)) {
			t.Error("nil != nil")
		}
		if !MColVectorEqual([][]COL{}, [][]COL(nil)) {
			t.Error("[] != nil")
		}
		if !MColVectorEqual([][]COL(nil), [][]COL{}) {
			t.Error("nil != []")
		}
		if !MColVectorEqual([][]COL{}, [][]COL{}) {
			t.Error("[] != []")
		}

		v1 := [][]COL{
			[]COL{},
			[]COL{
				COL{
					Name: "c.\"1",
					Type: "bigint\xff",
					Value: jval.Val(jsontext.Null),
				},
				COL{
					Name: "c.\"1",
					Type: "bigint",
					Value: jval.Val(jsontext.String("123")),
				},
			},
			[]COL{},
		}
		v2 := [][]COL{
			[]COL{},
			[]COL{
				COL{
					Name: "c.\"1",
					Type: "bigint\xff",
					Value: jval.Val(jsontext.Null),
				},
			},
			[]COL{},
		}
		if MColVectorEqual(v1, v2) {
			t.Error("v1 == v2")
		}
		v2[1] = append(v2[1],
			COL{
				Name: "c.\"1",
				Type: "bigint",
				Value: jval.Val(jsontext.String("123")),
			},
		)
		if !MColVectorEqual(v1, v2) {
			t.Error("v1 != v2")
		}
		if MColVectorEqual(v1[1:], v2) {
			t.Error("v1[1:] == v2")
		}
	})
}

// Local Variables:
// tab-width: 4
// End:
