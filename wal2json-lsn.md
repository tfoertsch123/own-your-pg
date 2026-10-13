# wal2json: payload LSNs and replication headers

Source inspection: PostgreSQL **master** and wal2json **master**, checked
2026-09-28. Links deliberately follow master and can change. These are source
findings, not results from running a master build. For pgoutput, see
[pgoutput-lsn.md](pgoutput-lsn.md).

## Two layers of LSNs

wal2json supplies JSON inside PostgreSQL's `XLogData` replication message.
The surrounding message contains `WALStart` and `ServerWALEnd`, as named by
pglogrepl. JSON `lsn` and `nextlsn` are separate, plugin-defined fields.

In logical replication, both header fields have the same value. PostgreSQL's
[`WalSndPrepareWrite`][sender] puts its `lsn` argument into both. If
`last_write` is false, it substitutes `InvalidXLogRecPtr`, or `0/0`.
Consequently, `ServerWALEnd` here is neither the server's current global WAL
end nor an upper bound calculated from the JSON's size. This equality is
intentional; it is not a pglogrepl parsing bug.

[`OutputPluginPrepareWrite`][logical] supplies `ctx->write_location` to that
function. PostgreSQL sets this context in its decoding callback wrappers;
wal2json chooses when to prepare an output buffer. Thus the header depends
on both the callback and the plugin's buffering decisions. Sending a
previously prepared buffer does not recalculate its LSN header.

## The underlying positions

| Position | Meaning |
| --- | --- |
| `txn->first_lsn` | First WAL position associated with this transaction in the reorder buffer. It can precede the first emitted row change. |
| `change->lsn` | WAL record position associated with a decoded change; for ordinary row changes and truncation, the record's start. |
| `txn->final_lsn` | For a normally committed transaction, the start of its COMMIT WAL record. |
| `commit_lsn` | The COMMIT record's start, passed to the commit callback. |
| `txn->end_lsn` | Position after the COMMIT WAL record, obtained from the WAL reader's `EndRecPtr`. |
| Message callback `lsn` | End position of the logical-message WAL record. |

The decoder distinguishes record start (`ReadRecPtr`/`origptr`) from record
end (`EndRecPtr`/`endptr`). Follow `LogicalDecodingProcessRecord`,
`DecodeCommit`, and `DecodeLogicalMsgOp` in [decode.c][decode], then
`ReorderBufferCommit` and `ReorderBufferTXNByXid` in
[reorderbuffer.c][reorder]. The callback-to-header assignments are in
`begin_cb_wrapper`, `change_cb_wrapper`, `truncate_cb_wrapper`,
`commit_cb_wrapper`, and `message_cb_wrapper` in [logical.c][logical].

An end LSN is a record boundary, not the record's start plus one byte.
Nor should it universally be described as the start of the next record:
WAL alignment and page boundaries matter.

## Format version 2

This table assumes `include-lsn=true`; `include-transaction` controls whether
`B` and `C` are emitted. Let **H** mean both `WALStart` and `ServerWALEnd`.

| JSON action | JSON `lsn` | JSON `nextlsn` | H |
| --- | --- | --- | --- |
| `B` — begin | COMMIT start | COMMIT end | `txn->first_lsn` |
| `I`, `U`, `D` | `change->lsn` | Absent | Same as JSON `lsn` |
| `T` — truncate | `change->lsn` | Absent | Same as JSON `lsn` |
| `M` — logical message | Message record end | Absent | Same as JSON `lsn` |
| `C` — commit | COMMIT start | COMMIT end | Same as JSON `nextlsn` |

The relevant functions in [wal2json.c][plugin] are
`pg_decode_begin_txn_v2`, `pg_decode_change_v2`, `pg_decode_write_change`,
`pg_decode_truncate_v2`, `pg_decode_message_v2`, and
`pg_decode_commit_txn_v2`.

`B` and `C` intentionally describe the same commit in their JSON fields.
wal2json has no callbacks for streaming uncommitted transactions: the
decoder already knows the commit when it invokes begin. Nevertheless, the
begin callback's *header* uses the transaction's first position.

Logical messages can be transactional or nontransactional; the latter do
not need enclosing `B`/`C` records. An optional JSON `origin` is an origin
identifier, not an origin LSN. See the message and origin serialization in
[wal2json.c][plugin].

### Why the first change can differ from BEGIN

SQL `BEGIN` need not itself produce WAL. Transaction bookkeeping can first
encounter a sequence update, catalog activity, or another record that never
becomes a visible JSON row. Multiple emitted changes can also share a WAL
position, for example with a multi-insert record. See transaction creation
and change replay in [reorderbuffer.c][reorder].

Your earlier PostgreSQL 14 `pg_waldump` observation illustrates these
positions; it is not a separate test of master:

| Event | Header H | JSON `lsn` | JSON `nextlsn` |
| --- | --- | --- | --- |
| `B`; first transaction WAL was a sequence LOG record | `4/269C7D38` | `4/269C7E88` | `4/269C7EB8` |
| `I`; heap INSERT record | `4/269C7DA0` | `4/269C7DA0` | Absent |
| `C`; COMMIT record | `4/269C7EB8` | `4/269C7E88` | `4/269C7EB8` |

Here the COMMIT starts at `4/269C7E88` and ends at `4/269C7EB8`, a difference
of 48 bytes. The switch from record start for `I` to record end for `C`
comes from PostgreSQL's different callback wrappers.

## Format version 1 differs

With default buffering, one transaction JSON object is prepared in the
begin callback and sent in the commit callback. Its header therefore stays
at `txn->first_lsn`, while optional JSON `nextlsn` identifies COMMIT end.
With `write-in-chunks`, opening, change, and closing chunks are prepared in
their respective callbacks: H is `txn->first_lsn`, `change->lsn`, and COMMIT
end, respectively. Chunking is not streaming before commit.
See `pg_decode_begin_txn_v1`, `pg_decode_change_v1`, and
`pg_decode_commit_txn_v1` in [wal2json.c][plugin].

## Keepalives and client progress

A primary keepalive is a separate replication message, not JSON or
`XLogData`. [`WalSndKeepalive`][sender] uses an explicitly supplied valid
position, or otherwise `sentPtr`. Ordinary logical sending advances
`sentPtr` after decoding a WAL record. Keepalives can occur between JSON
messages while pending output is being flushed; see
`ProcessPendingWrites` and `XLogSendLogical` in the same file.

That does not mean another backend's newer commit is decoded concurrently
in the middle of emitting this committed transaction. In this ordinary
wal2json path, such concurrent activity does not make a keepalive leap
past the transaction's COMMIT end before its `C` arrives. An intermediate
keepalive can still be ahead of earlier *change* positions.

`WalSndUpdateProgress` has a special keepalive path for skipped transactions
under synchronous replication. Current wal2json calls
`OutputPluginUpdateProgress` with `skipped_xact=false`, so that branch does
not explain wal2json's intermediate keepalives. See
[wal2json.c][plugin] and [walsender.c][sender].

For a commit-oriented archive, JSON `C.nextlsn` is the transaction's end
position to associate with durable completion. Do not acknowledge it just
because `B` already advertises it. Maintain a durable progress position
and report that in feedback; receiving a keepalive does not itself make
pending output durable. Nontransactional messages need their own handling.
These are client-design consequences of the positions above.

Never add JSON byte length to a header LSN to calculate WAL progress. Also,
`0/0` is an invalid-position sentinel, not a real WAL record location; see
[`InvalidXLogRecPtr` in xlogdefs.h][xlogdefs].

[plugin]: https://github.com/eulerto/wal2json/blob/master/wal2json.c
[sender]: https://github.com/postgres/postgres/blob/master/src/backend/replication/walsender.c
[logical]: https://github.com/postgres/postgres/blob/master/src/backend/replication/logical/logical.c
[decode]: https://github.com/postgres/postgres/blob/master/src/backend/replication/logical/decode.c
[reorder]: https://github.com/postgres/postgres/blob/master/src/backend/replication/logical/reorderbuffer.c
[xlogdefs]: https://github.com/postgres/postgres/blob/master/src/include/access/xlogdefs.h
