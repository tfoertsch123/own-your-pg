package pglogreplsimple

import (
	"time"
	"iter"
	"errors"
	"context"

	"sync"

	"github.com/jackc/pglogrepl"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/tfoertsch123/log"
)

// MsgItem is a type that is either a *pglogrepl.PrimaryKeepaliveMessage or
// a *pglogrepl.XLogData
type MsgItem interface {}

// Logger is an interface type representing the expected logger interface
type Logger interface {
	Error(string)
	Warn(string)
	Info(string)
	Debug(string)
	Debg2(string)
	Debg3(string)
	Debg4(string)
	Debg5(string)
	Errorf(string, ...interface{})
	Warnf(string,  ...interface{})
	Infof(string,  ...interface{})
	Debugf(string, ...interface{})
	Debg2f(string, ...interface{})
	Debg3f(string, ...interface{})
	Debg4f(string, ...interface{})
	Debg5f(string, ...interface{})
}

type Next int8
const (
	Connect Next = iota
	Recv
	Break
	Stop
)

func (nxt Next) String() string {
	return []string{"Connect", "Recv", "Break", "Stop"}[nxt]
}

type Param struct {
	CloseOnActivation chan<- struct{}
	ConnInfo string
	SlotName string
	ErrorRetryInterval time.Duration
	FeedbackInterval time.Duration
	IgnoreMissingIdentity []string
	Logger Logger
}

type recvStatus struct {
	wpos pglogrepl.LSN
	fpos pglogrepl.LSN
	rpos pglogrepl.LSN
}

type reloadRequest struct{}

func(_ reloadRequest) Error() string {
	return "Reload Requested"
}

type Receiver struct {
	producing sync.Mutex		// prevents multiple Produce() calls
	state Next
	plugin string				// the current plugin set after connect
	acceptedPlugins map[string][]string // plugins + options
	p Param						// the current set of params
	lg Logger
	getStartLSN func() pglogrepl.LSN
	conn *pgconn.PgConn
	recvStat recvStatus			// written by m2 as it writes the file
	prevStat recvStatus			// written by SendFeedback()
	nextFeedback time.Time		// when to send the next Feedback

	ignMiss map[string]struct{}	// list of tables where we ignore a missing
								// identity (only wal2json)

	// will be cancelled when it's time to exit
	shutdownCtx context.Context
	shutdownTrg context.CancelCauseFunc

	// the reason for shutting down
	lastErr error

	// ReceiveMessage is called with a context derived from shutdownCtx.
	// This is the cancel function of that derived context. It is called
	// upon Reload and when the deadline exceeds.
	mu sync.Mutex
	reload_p *Param
	cancelCurrent context.CancelCauseFunc
}

type rOpts struct {
	p *Param
	acceptedPlugins map[string][]string
	getStartLSN func() pglogrepl.LSN	
}
type Opt func(*rOpts)
func WithParams(x *Param) Opt {
	return func(o *rOpts) {
		o.p = x
	}
}
func WithAcceptedPlugins(x map[string][]string) Opt {
	return func(o *rOpts) {
		o.acceptedPlugins = x
	}
}
func WithGetStartLSN(x func() pglogrepl.LSN) Opt {
	return func(o *rOpts) {
		o.getStartLSN = x
	}
}

var DefaultPlugins = map[string][]string{
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
		`"proto_version" '1'`,
		`"messages" 'true'`,
		`"publication_names" 'all_tables'`,
	},
}

// r := NewReceiver( opts... )
// for msg := range r.Produce() {
//     ...
//     r.AckLSN(write, flush, replay)
// }
const DefaultErrorRetryInterval = 5*time.Second
const DefaultFeedbackInterval = 10*time.Second
func NewReceiver(p ...Opt) *Receiver {
	opts := rOpts{
		acceptedPlugins: DefaultPlugins,
		getStartLSN: func() pglogrepl.LSN {
			return pglogrepl.LSN(0)
		},
	}
	for _, o := range p {
		o(&opts)
	}

	r := &Receiver{
		state: Connect,
		p: Param{
			ErrorRetryInterval: DefaultErrorRetryInterval,
			FeedbackInterval: DefaultFeedbackInterval,
		},
		reload_p: opts.p,
		lg: log.L(),
		getStartLSN: opts.getStartLSN,
		acceptedPlugins: opts.acceptedPlugins,
	}

	return r
}

func (r *Receiver) Close() error {
	if !r.producing.TryLock() {
		return ErrReceiverLocked
	}
	defer r.producing.Unlock()
	if r.state == Stop {
		return ErrReceiverStopped
	}
	r.state = Stop
	if r.conn != nil {
		err := r.conn.Close(context.Background())
		r.conn = nil
		return err
	}
	return nil
}

func (r *Receiver) configure(nxt Next) Next {
	switch r.state {
	case Break, Stop:			// no change and no reload in these states
		return r.state
	}
	r.mu.Lock()
	if r.reload_p == nil {
		r.mu.Unlock()
		return nxt				// nothing to do
	}
	var p *Param
	p, r.reload_p = r.reload_p, nil
	r.mu.Unlock()

	if p.Logger != nil {
		r.lg = p.Logger			// logger first
	}

	if p.ErrorRetryInterval <= 500 * time.Millisecond {
		r.lg.Debugf("adjusting ErrorRetryInterval from %v to %v",
			p.ErrorRetryInterval, DefaultErrorRetryInterval)
		p.ErrorRetryInterval = DefaultErrorRetryInterval
	}
	if p.FeedbackInterval <= 500 * time.Millisecond {
		r.lg.Debugf("adjusting FeedbackInterval from %v to %v",
			p.FeedbackInterval, DefaultFeedbackInterval)
		p.FeedbackInterval = DefaultFeedbackInterval
	}

	if p.ConnInfo != r.p.ConnInfo {
		r.p.ConnInfo = p.ConnInfo
		nxt = Connect
	}
	if p.SlotName != r.p.SlotName {
		r.p.SlotName = p.SlotName
		nxt = Connect
	}
	r.p.ErrorRetryInterval = p.ErrorRetryInterval
	r.p.FeedbackInterval = p.FeedbackInterval
	r.p.IgnoreMissingIdentity = p.IgnoreMissingIdentity
	r.ignMiss = nil				// lazy init
	if p.CloseOnActivation != nil {
		close(p.CloseOnActivation)
	}
	return nxt
}

func (r *Receiver) setCancelCurrent(c context.CancelCauseFunc) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cancelCurrent = c
}

func (r *Receiver) RequestReload(p Param) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lg.Debugf("Reload requested: %#v", p)
	r.reload_p = &p
	if r.cancelCurrent != nil {
		r.cancelCurrent(reloadRequest{})
	}
}

var ErrReceiverLocked = errors.New("Receiver is locked by another thread")
var ErrReceiverStopped = errors.New("Receiver was stopped before")
func (r *Receiver) Produce(ctx context.Context) (iter.Seq[MsgItem], error) {
	if !r.producing.TryLock() {
		return nil, ErrReceiverLocked
	}
	if r.state == Stop {
		r.producing.Unlock()
		return nil, ErrReceiverStopped
	}
	if ctx == nil {
		ctx = context.Background()
	}
	r.shutdownCtx, r.shutdownTrg = context.WithCancelCause(ctx)
	ret := func(yield func(MsgItem) bool) {
		defer r.producing.Unlock()
		defer r.shutdownTrg(nil)
		r.lastErr = nil
		for {
			r.state = r.configure(r.state)
			switch r.state {
			case Stop:
				if r.conn != nil {
					r.conn.Close(context.Background())
					r.conn = nil
				}
				return
			case Break:
				// This is the result of yield() returning false
				// yield() is only called when a message is to be delivered
				// to the caller. At that state we are in Recv state.
				// Just in case the caller wants to run another
				// for msg := range r.Produce() {...}
				// loop, we need to adjust the state.
				r.state = Recv
				return
			case Connect:
				r.state = r.ConnInit()
			case Recv:
				r.state = r.RecvOne(yield)
			}
		}
	}
	return ret, nil
}

// Local Variables:
// tab-width: 4
// End:
