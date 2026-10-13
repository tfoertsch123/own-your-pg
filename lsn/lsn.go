/*
   Copyright 2026 Torsten Foertsch

   Licensed under the Apache License, Version 2.0 (the "License");
   you may not use this file except in compliance with the License.
   You may obtain a copy of the License at

       http://www.apache.org/licenses/LICENSE-2.0

   Unless required by applicable law or agreed to in writing, software
   distributed under the License is distributed on an "AS IS" BASIS,
   WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
   See the License for the specific language governing permissions and
   limitations under the License.
 */

package lsn

import (
	"fmt"
	"errors"
	"strings"
	"strconv"
	"encoding/json/v2"
	"encoding/json/jsontext"
)

// LSN represents a PostgreSQL log sequence number as a 64-bit integer.
// It is split internally into an upper 32-bit segment and a lower 32-bit
// segment, matching the textual "HIPI/LOPI" representation used by
// PostgreSQL tools such as pg_receivewal.
type LSN uint64

// ErrCannotScanType is returned by [LSN.Scan] when the source value is
// of a type that Scan does not support.
var ErrCannotScanType error = errors.New("Cannot scan type")

// ErrInvalidLsn is returned by [ParseLSN] and [LSN.Scan] when the input
// string is not a valid LSN representation.
var ErrInvalidLsn error = errors.New("Invalid LSN")

// ParseLSN parses a textual LSN of the form "HIPI/LOPI" or "HIPI-LOPI",
// where both halves are hexadecimal. It returns the parsed [LSN] or an
// error wrapping [ErrInvalidLsn].
func ParseLSN(s string) (LSN, error) {
	e := func() (LSN, error) {
		return 0, fmt.Errorf("%v: %w", s, ErrInvalidLsn)
	}

	delim := strings.IndexAny(s, "/-")
	if delim < 0 {
		return e()
	}

	upper, err := strconv.ParseUint(s[0:delim], 16, 32)
	if err != nil {
		return e()
	}

	lower, err := strconv.ParseUint(s[delim+1:], 16, 32)
	if err != nil {
		return e()
	}

	return LSN((upper<<32) + lower), nil
}

// String formats the LSN as "HIPI/LOPI" using uppercase hexadecimal,
// matching the canonical PostgreSQL representation.
func (l LSN) String() string {
	return fmt.Sprintf("%X/%X", uint32(l>>32), uint32(l))
}

// Expanded formats the LSN as "HHHHHHHH-HHHHHHHH" with both halves
// zero-padded to 8 hex digits, suitable for fixed-width sorting.
func (l LSN) Expanded() string {
	return fmt.Sprintf("%08X-%08X", uint32(l>>32), uint32(l))
}

// Scan implements the database/sql.Scanner interface. It accepts uint64,
// string, or []byte values. String and []byte inputs are parsed via
// [ParseLSN]. A nil receiver is a no-op.
func (l *LSN) Scan(from interface{}) error {
	if l == nil {
		return nil
	}

	switch it := from.(type) {
	case uint64:
		*l = LSN(it)
	case string:
		if lsn, err := ParseLSN(it); err == nil {
			*l = lsn
		} else {
			return err
		}
	case []byte:
		if lsn, err := ParseLSN(string(it)); err == nil {
			*l = lsn
		} else {
			return err
		}
	default:
		return fmt.Errorf("%w: %T", ErrCannotScanType, from)
	}

	return nil
}

// UnmarshalText implements encoding.TextUnmarshaler
func (t *LSN) UnmarshalText(bts []byte) error {
	return t.Scan(bts)
}

// MarshalJSON encodes the LSN as a JSON string in the [LSN.String] format.
func (t LSN) MarshalJSONTo(enc *jsontext.Encoder) error {
	return json.MarshalEncode(enc, t.String())
}

// UnmarshalJSON decodes a JSON string into an [LSN] using [LSN.Scan].
func (t *LSN) UnmarshalJSONFrom(dec *jsontext.Decoder) error {
	var s string
	if err := json.UnmarshalDecode(dec, &s); err != nil {
		return err
	}
	return t.Scan(s)
}

// Local Variables:
// tab-width: 4
// End:
