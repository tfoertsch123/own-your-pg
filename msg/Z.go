package msg

//go:generate go run ./gen

type Z struct {
	CommonFields
}

func (x *Z) ToSQL() string {
	return `-- EOF`
}

// Local Variables:
// tab-width: 4
// End:
