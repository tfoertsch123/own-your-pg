package slot

//go:generate msgp

type Cfg struct {
	Config  map[string][]string `msg:"Config"`
}

// Local Variables:
// tab-width: 4
// End:
