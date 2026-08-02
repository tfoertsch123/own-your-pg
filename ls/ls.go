// Package ls provides a lightweight directory listing utility built on top of
// the getdents(2) system call. It returns directory entries as an iterator
// sequence, avoiding the overhead of allocating a full slice of results and
// enabling early termination by the caller.
//
// Entries can be filtered by name (via a regular expression) and by type
// (e.g. [unix.DT_REG], [unix.DT_DIR]). Each entry includes the inode number
// and the d_type field, so callers can avoid an additional [unix.Stat] call
// when the filesystem supports d_type (most do, including ext4, xfs, btrfs,
// and ZFS).
package ls

import (
	"bytes"
	"iter"
	"regexp"
	us "unsafe"

	"golang.org/x/sys/unix"
)

// DirItem represents a single directory entry returned by [Ls].
type DirItem struct {
	// Name is the filename of the directory entry, without any path prefix.
	Name string
	// Ino is the inode number of the entry, as reported by the filesystem.
	Ino uint64
	// Type is the d_type field from the directory entry. It is one of the
	// unix.DT_* constants (e.g. [unix.DT_REG], [unix.DT_DIR], [unix.DT_LNK]).
	// Most filesystems populate this field; callers should still handle
	// [unix.DT_UNKNOWN] gracefully.
	Type uint8
}

// Ls returns an iterator over the entries in the directory dir. Each
// iteration yields a *DirItem and an error; when an error occurs the entry
// is nil and iteration stops.
//
// If re is non-nil, only entries whose name matches the regular expression
// are yielded. If one or more types are given, only entries whose d_type
// matches one of the specified values (e.g. [unix.DT_REG], [unix.DT_DIR])
// are yielded. Both filters may be combined.
//
// The iterator opens the directory with [unix.O_RDONLY|unix.O_DIRECTORY] and
// reads entries with [unix.Getdents]. The directory file descriptor is closed
// automatically when iteration completes or is abandoned (via [break] or
// early return from the yield function).
//
// The "." and ".." entries are included in the output unless filtered out by
// re or types.
func Ls(dir string, re *regexp.Regexp, types ...uint8) iter.Seq2[*DirItem, error] {
	var typeCheck func(uint8) bool
	if len(types) > 0 {
		typeCheck = func(t uint8) bool {
			// there are only so many types. I don't think it makes
			// sense to create a hash table here.
			for _, x := range types {
				if x == t {
					return true
				}
			}
			return false
		}
	}
	return func(yield func(*DirItem, error) bool) {
		fd, err := unix.Open(dir, unix.O_RDONLY|unix.O_DIRECTORY, 0)
		if err != nil {
			yield(nil, err)
			return
		}
		defer unix.Close(fd)

		var buf [4096]byte
		for {
			n, err := unix.Getdents(fd, buf[:])
			if err != nil {
				yield(nil, err)
				return
			}

			if n == 0 { // all done
				return
			}

			for off := 0; off < n; {
				entry := (*unix.Dirent)(us.Pointer(&buf[off]))
				off += int(entry.Reclen)

				if typeCheck != nil && !typeCheck(entry.Type) {
					continue
				}

				var name string
				bts := us.Slice(
					(*byte)(us.Pointer(us.SliceData(entry.Name[:]))),
					len(entry.Name[:]),
				)
				idx := bytes.Index(bts, []byte{0})
				if idx < 0 { // no \0 byte
					name = string(bts)
				} else {
					name = string(bts[:idx])
				}

				if re != nil && !re.MatchString(name) {
					continue
				}

				if !yield(&DirItem{
					Name: name,
					Ino:  entry.Ino,
					Type: entry.Type,
				}, nil) {
					return
				}
			}
		}
	}
}

// Local Variables:
// tab-width: 4
// End:
