package msg

//go:generate go run ./gen msg

type M struct {
	CommonFields
	Transactional bool `json:"transactional"`
	Prefix string `json:"prefix"`
	Content string `json:"content"`
}

func (x *M) ToSQL() string {
	return "-- ignoring logical message\n"
}

// Local Variables:
// tab-width: 4
// End:
