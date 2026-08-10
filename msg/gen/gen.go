package main

import (
	"os"
	"fmt"
	"text/template"
)

type Dat struct {
	Type string
	Opts map[string]bool
}

func main() {
	typ := os.Getenv("GOFILE")
	typ = typ[:len(typ)-3]

	opts := map[string]bool{}
	for _, el := range os.Args[1:] {
		opts[el] = true
	}

	d := Dat{
		Type: typ,
		Opts: opts,
	}

	out, err := os.Create(d.Type + "_gen.go")
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v", err)
		os.Exit(18)
	}
	tmpl := template.Must(template.ParseGlob("common.tmpl"))
	if err := tmpl.Execute(out, d); err != nil {
		fmt.Fprintf(os.Stderr, "%v", err)
		os.Exit(19)
	}
}

// Local Variables:
// tab-width: 4
// End:
