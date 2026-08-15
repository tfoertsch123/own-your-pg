package slot

//go:generate msgp -io=false

type Cfg struct {
	Config  map[string][]string `msg:"Config"`
}

// Local Variables:
// tab-width: 4
// End:
