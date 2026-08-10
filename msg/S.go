package msg

//go:generate go run ./gen query

type S struct {
	CommonFields
	Query string `json:"query"`
}

func (x *S) ToSQL() string {
	return x.Query
}

// Local Variables:
// tab-width: 4
// End:
