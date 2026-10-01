// Bound the memory a fetch can use: an in-memory repository that checks the packfile before inflating anything.

package git

import (
	"bytes"
	"errors"
	"fmt"
	"io"

	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/format/packfile"
	"github.com/go-git/go-git/v5/storage/memory"
)

// Limits bound what fetching a library may hold in memory, so a repository can't exhaust it.
type Limits struct {
	// Tags bounds the release tags.
	Tags int
	// TagBytes bounds one release tag object, its message and any signature included.
	TagBytes int64
	// PackBytes bounds the packfile the remote sends, which is held while it's checked.
	PackBytes int64
	// Objects bounds the objects the packfile holds.
	Objects int
	// ObjectBytes bounds one object, inflated, and TotalBytes all of them together.
	ObjectBytes, TotalBytes int64
}

// boundedStorage is an in-memory repository that refuses objects past its limits. It receives a fetch's
// packfile itself, as a storer.PackfileWriter, so it can check the pack's size, object count, and every object's
// declared size before go-git inflates any object into memory. Its object count and bytes also bound every object
// stored, as a second check.
type boundedStorage struct {
	*memory.Storage
	limits Limits
	// objects and bytes count the objects stored so far, and their inflated bytes.
	objects int
	bytes   int64
}

func newBoundedStorage(limits Limits) *boundedStorage {
	return &boundedStorage{Storage: memory.NewStorage(), limits: limits}
}

// SetEncodedObject stores o, unless it's larger than one object may be or it would pass the object or byte
// limits.
func (s *boundedStorage) SetEncodedObject(o plumbing.EncodedObject) (plumbing.Hash, error) {
	s.objects++
	s.bytes += o.Size()
	switch {
	case o.Size() > s.limits.ObjectBytes:
		return plumbing.ZeroHash, objectTooLarge(o.Size(), s.limits)
	case s.objects > s.limits.Objects:
		return plumbing.ZeroHash, tooManyObjects(s.limits)
	case s.bytes > s.limits.TotalBytes:
		return plumbing.ZeroHash, tooManyBytes(s.limits)
	}
	return s.Storage.SetEncodedObject(o)
}

// PackfileWriter returns a writer that takes the packfile a fetch receives. It refuses more than packBytes, and
// when it's closed it checks the pack with checkPack and then stores the pack's objects.
func (s *boundedStorage) PackfileWriter() (io.WriteCloser, error) {
	return &packWriter{storage: s}, nil
}

// packWriter holds a fetch's packfile until it's complete.
type packWriter struct {
	storage *boundedStorage
	pack    bytes.Buffer
}

func (w *packWriter) Write(p []byte) (int, error) {
	if int64(w.pack.Len()+len(p)) > w.storage.limits.PackBytes {
		return 0, fmt.Errorf("the repository sent more than %d bytes, which ingestion won't hold", w.storage.limits.PackBytes)
	}
	return w.pack.Write(p)
}

func (w *packWriter) Close() error {
	if w.pack.Len() == 0 {
		return nil
	}
	if err := checkPack(w.pack.Bytes(), w.storage.limits); err != nil {
		return err
	}
	parser, err := packfile.NewParserWithStorage(packfile.NewScanner(bytes.NewReader(w.pack.Bytes())), w.storage)
	if err != nil {
		return fmt.Errorf("read packfile: %v", err)
	}
	if _, err := parser.Parse(); err != nil {
		return fmt.Errorf("read packfile: %v", err)
	}
	return nil
}

// checkPack checks a packfile against limits before any of its objects is inflated into memory: its object
// count, each object's size, and their total. A delta's size is that of the object it produces, which it declares
// first; the delta itself is inflated, within the one-object limit, to read it.
func checkPack(pack []byte, limits Limits) error {
	scanner := packfile.NewScanner(bytes.NewReader(pack))
	_, count, err := scanner.Header()
	if err != nil {
		return fmt.Errorf("read packfile header: %v", err)
	}
	if int64(count) > int64(limits.Objects) {
		return tooManyObjects(limits)
	}
	var total int64
	for range count {
		header, err := scanner.NextObjectHeader()
		if err != nil {
			return fmt.Errorf("read packfile: %v", err)
		}
		if header.Length > limits.ObjectBytes {
			return objectTooLarge(header.Length, limits)
		}
		size := header.Length
		if header.Type.IsDelta() {
			var delta bytes.Buffer
			if _, _, err := scanner.NextObject(&delta); err != nil {
				return fmt.Errorf("read packfile: %v", err)
			}
			if size, err = deltaTargetSize(delta.Bytes()); err != nil {
				return err
			}
			if size > limits.ObjectBytes {
				return objectTooLarge(size, limits)
			}
		} else if _, _, err := scanner.NextObject(io.Discard); err != nil {
			return fmt.Errorf("read packfile: %v", err)
		}
		total += size
		if total > limits.TotalBytes {
			return tooManyBytes(limits)
		}
	}
	return nil
}

// deltaTargetSize returns the size of the object a delta produces: the second of the two sizes a delta begins
// with, each a little-endian base-128 number.
func deltaTargetSize(delta []byte) (int64, error) {
	_, rest, ok := deltaSize(delta)
	if ok {
		var target int64
		if target, _, ok = deltaSize(rest); ok {
			return target, nil
		}
	}
	return 0, errors.New("read packfile: a delta's header is malformed")
}

// deltaSize reads one size from the start of b, and returns it with the bytes after it.
func deltaSize(b []byte) (size int64, rest []byte, ok bool) {
	for i, c := range b {
		if i >= 9 {
			return 0, nil, false
		}
		size |= int64(c&0x7f) << (7 * i)
		if c&0x80 == 0 {
			return size, b[i+1:], true
		}
	}
	return 0, nil, false
}

func objectTooLarge(size int64, limits Limits) error {
	return fmt.Errorf("the repository holds a %d-byte object, more than the %d bytes ingestion accepts for one", size, limits.ObjectBytes)
}

func tooManyObjects(limits Limits) error {
	return fmt.Errorf("the repository's release tags reach more than %d objects, which ingestion won't hold", limits.Objects)
}

func tooManyBytes(limits Limits) error {
	return fmt.Errorf("the repository's release tags reach more than %d bytes of objects, which ingestion won't hold", limits.TotalBytes)
}
