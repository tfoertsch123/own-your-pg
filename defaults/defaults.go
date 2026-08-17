package defaults

import (
	"time"
)

const SlotDir = "slots"
const M2SizeLimit = "16MiB"
const M2ErrPause = 5000 * time.Millisecond
var M2ExpectedPlugins map[string]bool = map[string]bool{
	"wal2json": true,
	"pgoutput": false,
}

// Local Variables:
// tab-width: 4
// End:
