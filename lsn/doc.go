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

// Package lsn implements parsing, formatting, and JSON/database scanning for
// PostgreSQL log sequence numbers.
//
// A [LSN] is a 64-bit value composed of two 32-bit halves. PostgreSQL prints
// it as "HIPI/LOPI" (e.g. "0/3", "1/FFFFFFFF"), where each half is
// hexadecimal. This package supports both the "/" and "-" delimiters on input
// and emits the canonical "/" form via [LSN.String].
//
// # Usage
//
// Parse a textual LSN:
//
//	l, err := lsn.ParseLSN("1/3")
//
// Format an LSN:
//
//	s := l.String()            // "1/3"
//	s := l.Expanded()          // "00000001-00000003"
//
// Use with database/sql or encoding/json/v2:
//
//	var l lsn.LSN
//	_ = l.Scan(uint64(3))          // from database/sql
//	b, _ := json.Marshal(l)        // -> "\"0/3\""
//	_ = json.Unmarshal(b, &l)
//
// # Delimiters
//
// [ParseLSN] accepts either "/" or "-" as the separator between the upper and
// lower halves. [LSN.String] always emits "/", while [LSN.Expanded] emits "-"
// for a fixed-width, sortable representation suitable for file names.
package lsn

// Local Variables:
// tab-width: 4
// End:
