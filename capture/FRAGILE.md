# Fragile Points in capture/

Patterns that work today but depend on assumptions that could break if
upstream formats change. These should be hardened by decoding JSON into a
well-known fixed structure and re-encoding it, rather than relying on raw
byte positions or key ordering.

## 1. Hardcoded byte offset for B.nextlsn patching

**File:** `file.go`, function `eoc()` (around line 283-289)

```go
_, err := cfg.curr.WriteAt(
    []byte(fmt.Sprintf(
        `%-*s`,
        len(`"PPPPPPPP/QQQQQQQQ",`),
        `"` + lsn.String() + `",`,
    )),
    cfg.eoCommit+int64(len(`{"action":"B","nextlsn":`)))
```

**Why fragile:** The `WriteAt` offset assumes the BEGIN record starts
exactly with `{"action":"B","nextlsn":`. This couples the patching logic
to wal2json's JSON key ordering. If wal2json reorders keys, adds fields
before `action`, or changes its serialization, the offset is wrong and
the file is silently corrupted.

**Mitigation:** Today the wal2json format is pinned via plugin parameters
(`"format-version" '2'`). Long-term fix: decode the JSON into a typed
struct, set the `nextlsn` field, and re-encode in a canonical order.

## 2. Placeholder string for B.nextlsn must match patching format

**File:** `consume.go`, function `parseAndAddLsn()` (around line 76-77)

```go
placeholder := "XXXXXXXX/YYYYYYYY"
if err := json.MarshalEncode(enc, placeholder); err != nil {
```

**File:** `file.go`, function `eoc()` (around line 286)

```go
len(`"PPPPPPPP/QQQQQQQQ",`),
```

**Why fragile:** The placeholder `"XXXXXXXX/YYYYYYYY"` (19 chars) written
by `parseAndAddLsn` must be the same width as the replacement string
`"PPPPPPPP/QQQQQQQQ",` (21 chars including quotes and comma) used by
`eoc()` for the `%-*s` format specifier. These two strings are defined in
different files and have no compile-time link. If one is changed without
the other, the field-width padding silently breaks.

**Mitigation:** Define a single constant for the placeholder and the
expected replacement width in one place. Or better: after re-encoding into
a fixed format, the placeholder is written and replaced at a known
struct offset rather than a computed byte position.

## 3. JSON key string matching in parseAndAddLsn

**File:** `consume.go`, function `parseAndAddLsn()` (around line 68-95)

```go
switch what {
case "action":
    ...
case "transactional":
    ...
```

**Why fragile:** The function matches on raw JSON key strings (`"action"`,
`"transactional"`) and injects new keys (`"nextlsn"`, `"lsn"`) by calling
`enc.WriteToken(jt.String("nextlsn"))`. If wal2json ever emits one of
these keys itself (e.g. if `"include-lsn"` is accidentally enabled), the
output JSON would contain duplicate keys.

**Mitigation:** Decode into a typed struct so that duplicate or
unexpected keys are handled explicitly rather than appended.

## 4. Trailing-data check assumes single JSON object

**File:** `consume.go`, function `parseAndAddLsn()` (around line 104-109)

```go
if _, err := dec.ReadToken(); err != nil {   // consume }
    return nil, err
}
if _, err := dec.ReadToken(); err != io.EOF {
    if err == nil {
        err = fmt.Errorf("unexpected data after JSON object")
    }
    return nil, err
}
```

**Why fragile:** Assumes each wal2json XLD message contains exactly one
JSON object with nothing trailing. If wal2json changes its framing (e.g.
concatenated objects or a trailing newline), this check fails and every
message is rejected.

**Mitigation:** After decoding into a typed struct, trailing data can be
ignored or handled based on the structured representation rather than the
raw token stream.

## 5. parseAndAddLsn preserves all keys in wal2json order

**File:** `consume.go`, function `parseAndAddLsn()` (general design)

**Why fragile:** The function copies all key-value pairs from the input
to the output in whatever order wal2json emits them, only injecting
`nextlsn`/`lsn` after specific keys. This means the output byte layout is
entirely determined by wal2json's serialization choices. The B.nextlsn
patching in `eoc()` (item 1) then depends on this layout.

**Mitigation:** Once JSON is decoded and re-encoded in a canonical fixed
format, all downstream consumers (including `eoc()`) can rely on
deterministic byte positions.

---

## Summary

Items 1-2 and 5 are the core fragility: `eoc()` patches a byte offset into
JSON it did not produce, and the placeholder/replacement width is spread
across two files. Items 3-4 are secondary: the streaming JSON approach
couples tightly to wal2json's exact output.

The unifying fix is to decode wal2json output into a typed struct and
re-encode it in a well-known fixed format in `parseAndAddLsn`. Then `eoc()`
can patch at a known struct offset, the placeholder width is a single
constant, and key-ordering assumptions disappear.
