package defaults

import (
	"time"
)

const SlotDir = "slots"
const M2SizeLimit = "16MiB"
const M2ErrPause = 5000 * time.Millisecond
var M2ExpectedPlugins map[string][]string = map[string][]string{
	"wal2json": []string{
		`"format-version" '2'`,
		`"include-types" 'true'`,
		`"include-xids" 'true'`,
		`"include-timestamp" 'true'`,
		`"include-lsn" 'true'`,
		`"include-pk" 'true'`,
		`"numeric-data-types-as-string" 'true'`,
	},
	"pgoutput": []string{
		`"proto_version" '2'`,
		`"messages" 'true'`,
		`"publication_names" 'all_tables'`,
	},
}
const M2FeedbackInterval = 10000 * time.Millisecond

// Local Variables:
// tab-width: 4
// End:
