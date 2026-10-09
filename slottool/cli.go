package slottool

import (
	"fmt"
	"strings"
	"errors"
	"encoding/json/v2"

	shq "github.com/kballard/go-shellquote"

	"github.com/tfoertsch123/own-your-pg/lsn"
	"github.com/tfoertsch123/own-your-pg/slot"
)

// configItem holds a single -c name=value pair, where value is either a
// bare string or a JSON array of strings.
type configItem struct {
	name  string
	value *[]string
}

// UnmarshalText implements encoding.TextUnmarshaler so that kong can decode
// each -c argument into a configItem. Accepted formats:
//
//	name=string
//	name={list of strings}   (shell quoted)
//	name=[list of strings]   (json quoted)
var ErrUnterminatedList = errors.New("missing ] at the end")
func (c *configItem) UnmarshalText(text []byte) error {
	s := string(text)

	// Split on the first '='.
	kv := strings.SplitN(s, "=", 2)
	c.name = strings.Trim(kv[0], " \t\n")
	if len(kv) == 1 {
		// no value given => delete
		c.value = nil
		return nil
	}
	
	s = strings.Trim(kv[1], " \t\n")
	if len(s) > 1 && s[0] == '{' {
		if s[len(s)-1] != '}' {
			return fmt.Errorf("%w of <%s>", ErrUnterminatedList, s)
		}
		val, err := shq.Split(s[1:len(s)-1])
		c.value = &val
		return err
	}
	if len(s) > 1 && s[0] == '[' {
		if s[len(s)-1] != ']' {
			return fmt.Errorf("%w of <%s>", ErrUnterminatedList, s)
		}
		var val []string
		err := json.Unmarshal([]byte(s), &val)
		c.value = &val
		return err
	}
	c.value = &[]string{s}
	return nil
}

type Cli struct {
	// Dir specifies the working directory, required.
	Dir string `arg:"" required:"" help:"Working directory."`

	// Slot is optional, short flag -S.
	Slot *string `short:"S" help:"Slot name."`

	// LSN is optional, short flag -l.
	LSN *lsn.LSN `short:"l" help:"Log sequence number (e.g. 1/FFFFFFFF)."`

	// Type is optional, short flag -t.
	Type *slot.Type `short:"t" help:"Slot type (Change, Archiver, Config, Producer)."`

	// Configs is a repeated flag, short flag -c.
	Config []configItem `short:"c" sep:"none" help:"Config entry: key=string or key=JSON-array or key={shell quoted list of strings}."`
}

// IntroHelp implements help's IntroHelper interface. It returns the
// first half of the help text before the command line parameters.
// The link() function can be used to format certain phrases as
// hyperlinks.
func (_ *Cli) IntroHelp(link func(url, text string) string) string {
	return `
${basename} is the configuration tool for all programs in this system.
Slots are small files residing in the "slots" subdirectory of the working
directory. A slot combines configuration information with runtime status
information.

${basename} creates and modifies these files.
`
}

// ExtraHelp implements help's ExtraHelper interface. It returns the
// first half of the help text before the command line parameters.
// The link() function can be used to format certain phrases as
// hyperlinks.
func (_ *Cli) ExtraHelp(link func(url, text string) string) string {
	return `
Examples:

List all slots in working-directory
 $ slottool working-directory
 {
   "Name": "capture",
   "NextLSN": "4/3607B358",
   "OwnerPid": 1999900,
   "PidActive": true,
   "Type": "Producer",
   "Config": {
     "ignore_missing_identity": [
       "public.table1",
       "schema.table2"
     ],
     "logfile": "file://DEBG5@STDERR?timeformat=2006-01-02+15:04:05.000",
     "primary_conninfo": "postgres:///bench?service=mydb&application_name=m2" ,
     "size_limit": "5KiB",
     "synchronous": "1"
   }
 }

Modify the "capture" slot
 $ slottool tmp -S capture \
            -c ignore_missing_identity='{schema.table1 schema.table2}'
 {
   "Name": "capture",
   "NextLSN": "4/3607B358",
   "OwnerPid": 1999900,
   "PidActive": false,
   "Type": "Producer",
   "Config": {
	 "ignore_missing_identity": [
	   "schema.table1",
	   "schema.table2"
	 ],
	 "logfile": "file://DEBG5@STDERR?timeformat=2006-01-02+15:04:05.000",
	 "primary_conninfo": "postgres:///bench?service=mydb&application_name=m2",
	 "size_limit": "5KiB",
	 "synchronous": "1"
   }
 }
`
}
// Local Variables:
// tab-width: 4
// End:
