package capture

type Cli struct {
	Dir string `arg:"" required:"" help:"Working directory."`
	Slot string `short:"S" help:"Slot name." default:"${basename}"`
}

// IntroHelp implements help's IntroHelper interface. It returns the
// first half of the help text before the command line parameters.
// The link() function can be used to format certain phrases as
// hyperlinks.
func (_ *Cli) IntroHelp(link func(url, text string) string) string {
	return `
${basename} consumes a stream of logical changes from a Postgres database.
The changes are first recorded in "incomplete/current" the "working directory".
Once the size of the "current" file exceeds a certain limit, it is renamed
and placed directly in the working directory. After that a new "current"
file is opened and the process is repeated.

The capture files in the working directory are called "history files". A
history file always contains complete transactions. In other words, a
transaction is never broken into several files.

The name of a history file follows the this pattern:

 XXXXXXXX-YYYYYYYY..xxxxxxxx-yyyyyyyy

Both parts, before and after the 2 dots, represent LSNs expanded with leading
zeroes. That means the files can be alphabetically sorted in the natural order
given by the LSN. The first LSN in the file name, XXXXXXXX-YYYYYYYY, is the
LSN right after the COMMIT record of the last transaction in the file. The
second LSN, xxxxxxxx-yyyyyyyy, is the COMMIT LSN of the first transaction in
the file.

${basename} reads its configuration from the "slot". Slots are files stored
in the "slots" subdirectory of the working directory. Slots are created and
modified using "slottool".
`
}

// IntroHelp implements help's ExtraHelper interface. It returns the
// second half of the help text after the command line parameters.
// The link() function can be used to format certain phrases as
// hyperlinks.
func (_ *Cli) ExtraHelp(link func(url, text string) string) string {
	return `
Slot configuration:

  primary_conninfo         libpq connection string (required)
  primary_slotname         PG replication slot name (default: slot name)
  logfile                  log destination URL (e.g. //stderr, file:///path)
  size_limit               file size limit to trigger rotation (default: 16MiB)
  synchronous              on/true/1 for synchronous mode (default: off)
  ignore_missing_identity  tables to ignore missing replication identity

For more information about the logfile specification, see
` + link("https://pkg.go.dev/github.com/tfoertsch123/log#ParseURL",
		"the log package documentation") + `.

The size_limit if given as a simple integer number specifies the size in bytes.
A unit can be appended according to
` + link("https://pkg.go.dev/github.com/alecthomas/units#ParseStrictBytes",
		"the ParseStrictBytes function in alecthomas/units package") + `.

The actual file size can significantly exceed size_limit. The capture process
never breaks up a DB transaction into several files. So, a large change in
one transaction can create GB-sized files even if size_limit=10KiB.

Use slottool to set these parameters.`
}

// Local Variables:
// tab-width: 4
// End:
