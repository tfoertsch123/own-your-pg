package m2

import (
	"time"
	"context"
	"github.com/tfoertsch123/own-your-pg/slot"
	"github.com/tfoertsch123/own-your-pg/lsn"
	"github.com/tfoertsch123/log"
	"github.com/jackc/pgx/v5/pgconn"	
)

type recvStatus struct {
	wpos lsn.LSN
	fpos lsn.LSN
}

type Mon struct {
	settings map[string]string	// clone of the slotinfo
	lg *log.Logger
	relg *log.Logger 
	mlg *log.Logger
	sl *slot.Slot
	maxSize int64				// size_limit
	ci string					// primary_slotname
	sn string					// primary_conninfo
	conn *pgconn.PgConn
	recvStat recvStatus			// written by m2 as it writes the file
	prevStat recvStatus			// written by SendFeedback()
	nextFeedback time.Time		// when to send the next Feedback

	ignMiss map[string]struct{}	// list of tables where we ignore a missing
								// identity, loaded lazily

	// will be cancelled when it's time to exit
	shutdown_ctx context.Context
	shutdown_trg context.CancelCauseFunc

	reload chan struct{}
}

// Local Variables:
// tab-width: 4
// End:
