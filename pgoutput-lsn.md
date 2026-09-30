# pgoutput: payload LSNs and replication headers

Source inspection: PostgreSQL **master**, checked 2026-09-28. Links follow
master and can change. These are source findings, not results from running
a master build. For wal2json, see [wal2json-lsn.md](wal2json-lsn.md).

## Header versus payload

pgoutput's binary protocol messages travel inside `XLogData`. In the
tables below, **H** means both outer fields, `WALStart` and `ServerWALEnd`.
PostgreSQL's [`WalSndPrepareWrite`][sender] makes them equal. For a buffer
prepared with `last_write=false`, both are `0/0`; otherwise both receive
the decoding context's current `write_location`.

This is not a measurement of the server's latest global WAL end. The
plugin determines when to call `OutputPluginPrepareWrite`; PostgreSQL's
[callback wrappers][logical] determine the context position. The payload
can contain other LSNs with different meanings. Payload byte length has no
arithmetic relationship to WAL length.

## Normal transactions and shared messages

Here, COMMIT start means the WAL reader's `ReadRecPtr` for the COMMIT record;
COMMIT end means its `EndRecPtr`. The end is a boundary after the record,
not start plus one. See `DecodeCommit` in [decode.c][decode] and
`ReorderBufferCommit` in [reorderbuffer.c][reorder].

`txn->first_lsn` is the first position associated with the transaction in
the reorder buffer, which can precede any published row change. It is not
necessarily a WAL record corresponding to SQL `BEGIN`. See
`ReorderBufferTXNByXid` in [reorderbuffer.c][reorder].

| Message | Payload LSNs | H |
| --- | --- | --- |
| `B` — Begin | `final_lsn`: COMMIT start | Position of the first emitted change or transactional message; origin exception below |
| `I`, `U`, `D` | None | `change->lsn`, ordinarily the row-change WAL record's start |
| `T` — Truncate | None | Truncation record's start |
| `R` — Relation | None | `0/0` |
| `Y` — Type | None | `0/0` |
| `M` — Message | `lsn`: logical-message WAL record's end | Same as payload `lsn` |
| `C` — Commit | `commit_lsn`: COMMIT start; `end_lsn`: COMMIT end | Payload `end_lsn` |
| `O` — Origin | `origin_lsn`: position in the originating server's WAL | Current local callback position; see below |

The binary field layouts are documented in the
[development protocol reference][formats] and implemented by the
`logicalrep_write_*` functions in [proto.c][proto]. Message emission and
buffer preparation are in [pgoutput.c][plugin]. Publication filters and
options determine which messages appear; `M` requires `messages=true`.

### BEGIN is delayed

`pgoutput_begin_txn` initially records transaction state without emitting
`B`. `pgoutput_send_begin` runs when a publishable change is first emitted.
Consequently, its header uses the active change/message callback's position,
which need not be `txn->first_lsn`. Transactions with no emitted changes
can have neither `B` nor `C`. See those functions and `pgoutput_commit_txn`
in [pgoutput.c][plugin].

The payload `final_lsn` already identifies the future `C` record's COMMIT
start because this ordinary callback path replays a committed transaction.
It does not mean that the client has already received that transaction.
This differs from wal2json's immediate begin-buffer preparation.

For `M`, `DecodeLogicalMsgOp` passes the record's **end**, even though row
callbacks use record starts. Transactional messages can cause delayed `B`
to be emitted; nontransactional messages need no enclosing transaction.
See [decode.c][decode] and `pgoutput_message` in [pgoutput.c][plugin].

### Origin and zero-header exceptions

When `B`, `b`, or the first `S` requests origin output, pgoutput prepares
that initial message with `last_write=false`, giving it H = `0/0`.
`send_repl_origin` flushes it and prepares `O` with `last_write=true`, so
`O` receives the active callback's local position. If the origin name
cannot be resolved, `O` is omitted but the initial header remains zero.
`send_relation_and_attrs` also uses false for `R` and `Y`.
See [pgoutput.c][plugin].

An `O` payload's `origin_lsn` belongs to the originating server, not
necessarily this sender. Do not compare it numerically with local H.
For ordinary `B` and `b`, the plugin supplies `txn->origin_lsn`; for the
first streaming segment it supplies `InvalidXLogRecPtr` because the origin
commit position is not yet known. Thus `O` can have a valid local header
and a zero payload LSN. See `pgoutput_send_begin`,
`pgoutput_begin_prepare_txn`, and `pgoutput_stream_start` in
[pgoutput.c][plugin].

## Streaming transactions before commit

These messages require negotiated streaming support. A transaction may
span multiple segments, with other transactions between them.

| Message | Payload LSNs | H |
| --- | --- | --- |
| `S` — Stream Start | None | First processed change position in this segment; origin exception above |
| `E` — Stream Stop | None | Last processed change position in this segment |
| `c` — Stream Commit | `commit_lsn`: COMMIT start; `end_lsn`: COMMIT end | Payload `end_lsn` |
| `A` — Stream Abort | With parallel streaming: `abort_lsn`; otherwise no LSN field | Abort position passed to the callback, or `0/0` if unavailable |

For an explicitly decoded abort, the abort position is the abort WAL
record's **end**. An implicit abort during cleanup can lack a known abort
position. When the parallel-streaming payload includes `abort_lsn`, it
matches H. See `DecodeAbort` in [decode.c][decode], `ReorderBufferAbortOld`
in [reorderbuffer.c][reorder], and `logicalrep_write_stream_abort` in
[proto.c][proto].

`S` and `E` track the segment's processed changes, including ones that
produce no published row. They need not equal the first and last visible
row positions. Their values come from `ReorderBufferProcessTXN`, then
`stream_start_cb_wrapper` and `stream_stop_cb_wrapper` in
[logical.c][logical]. The shared `I/U/D/T/R/Y/M` rules above also apply
inside segments. `E` does not mean COMMIT.

## Two-phase transactions

These messages require negotiated two-phase support. PREPARE, COMMIT
PREPARED, and ROLLBACK PREPARED are distinct WAL records with distinct
start/end positions.

| Message | Payload LSNs | H |
| --- | --- | --- |
| `b` — Begin Prepare | `prepare_lsn`: PREPARE start; `end_lsn`: PREPARE end | `txn->first_lsn`; origin exception above |
| `P` — Prepare | `prepare_lsn`: PREPARE start; `end_lsn`: PREPARE end | PREPARE end |
| `p` — Stream Prepare | Same meanings as `P` | PREPARE end |
| `K` — Commit Prepared | `commit_lsn`: COMMIT PREPARED start; `end_lsn`: its end | COMMIT PREPARED end |
| `r` — Rollback Prepared | `prepare_end_lsn`: earlier PREPARE end; `rollback_end_lsn`: ROLLBACK PREPARED end | `rollback_end_lsn` |

Unlike ordinary `B`, `b` is emitted directly by the begin-prepare callback,
so its normal header is `txn->first_lsn`. For `r`, the two payload LSNs
refer to two different records; neither is a record-start LSN.

Follow the prepare/commit-prepared/rollback-prepared wrappers in
[logical.c][logical], the corresponding `pgoutput_*` callbacks in
[pgoutput.c][plugin], and the matching serializers in [proto.c][proto].
The [protocol reference][formats] describes the negotiated message formats.

## Keepalives and acknowledgements

Primary keepalives are separate from all messages above.
[`WalSndKeepalive`][sender] reports an explicit valid position if supplied,
otherwise `sentPtr`. `XLogSendLogical` updates `sentPtr` after processing a
WAL record. `ProcessPendingWrites` can send keepalives between plugin
messages while flushing output.

For a skipped transaction, `pgoutput_commit_txn` reports
`skipped_xact=true`. Under the synchronous-replication conditions in
`WalSndUpdateProgress`, this can produce a keepalive carrying that skipped
transaction's end without a corresponding `B`/`C`. See
[pgoutput.c][plugin] and [walsender.c][sender]. It does not decode a later
transaction concurrently while replaying an earlier committed one.

With streaming, however, uncommitted transactions can remain open across
segments while the sender progresses. A client must track those states;
the next message's header alone does not describe all outstanding work.

For client design, maintain separate received, durable, and applied
positions. Report only progress actually achieved. A payload advertising
a future commit, a keepalive, or an `E` does not itself establish durable
transaction completion. For a transaction-based archive, associate
completion with `C.end_lsn` or `c.end_lsn` after persistence. Supporting
streaming or two-phase acknowledgement also requires preserving the
corresponding pending state across crashes.

Treat header `0/0` as “no valid progress position supplied”, not as a WAL
record. It is [`InvalidXLogRecPtr`][xlogdefs]. Never infer progress by adding
binary payload length to H, or assume every successive header increases.

[plugin]: https://github.com/postgres/postgres/blob/master/src/backend/replication/pgoutput/pgoutput.c
[proto]: https://github.com/postgres/postgres/blob/master/src/backend/replication/logical/proto.c
[logical]: https://github.com/postgres/postgres/blob/master/src/backend/replication/logical/logical.c
[sender]: https://github.com/postgres/postgres/blob/master/src/backend/replication/walsender.c
[decode]: https://github.com/postgres/postgres/blob/master/src/backend/replication/logical/decode.c
[reorder]: https://github.com/postgres/postgres/blob/master/src/backend/replication/logical/reorderbuffer.c
[xlogdefs]: https://github.com/postgres/postgres/blob/master/src/include/access/xlogdefs.h
[formats]: https://www.postgresql.org/docs/devel/protocol-logicalrep-message-formats.html
