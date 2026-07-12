package journal

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"os"
)

// ErrClosed is returned by Reader.Next once the journal is closed and drained.
var ErrClosed = errors.New("journal closed")

// Reader streams records forward from the committed cursor. A journal has a
// single logical consumer; create one Reader and drive it from the processing
// loop. Next blocks until a record is available, the context is cancelled, or
// the journal is closed and fully drained.
type Reader struct {
	j       *Journal
	file    *os.File
	curBase uint64 // base of the currently open segment
	off     int64  // read offset within the open segment
	seq     uint64 // next sequence to deliver
}

// NewReader creates the reader positioned just after the committed cursor.
func (j *Journal) NewReader() *Reader {
	j.mu.Lock()
	start := j.readNext
	if len(j.segments) > 0 && start < j.segments[0].base {
		start = j.segments[0].base
	}
	j.mu.Unlock()
	return &Reader{j: j, seq: start}
}

// Seq returns the next sequence the reader will attempt to deliver.
func (r *Reader) Seq() uint64 { return r.seq }

// Close releases the reader's open file handle.
func (r *Reader) Close() error {
	if r.file != nil {
		err := r.file.Close()
		r.file = nil
		return err
	}
	return nil
}

// Next returns the next record and its sequence. It blocks until data is
// available; it returns ErrClosed when the journal is closed and drained, or
// the context error on cancellation.
func (r *Reader) Next(ctx context.Context) (uint64, Record, error) {
	for {
		if r.file != nil {
			seq, rec, adv, ok, err := r.readFrame()
			if err != nil {
				return 0, Record{}, err
			}
			if ok {
				r.off += adv
				r.seq = seq + 1
				return seq, rec, nil
			}
		}

		j := r.j
		j.mu.Lock()
		closed := j.closed
		nextSeq := j.nextSeq
		segs := append([]segment(nil), j.segments...)
		j.mu.Unlock()

		if closed && r.seq >= nextSeq {
			return 0, Record{}, ErrClosed
		}

		// Our read position was evicted: jump to the earliest available record.
		if len(segs) > 0 && r.seq < segs[0].base {
			r.seq = segs[0].base
			if err := r.openAndSeek(segs, r.seq); err != nil {
				return 0, Record{}, err
			}
			continue
		}

		// More data exists, but not at the current file position: (re)open the
		// segment that contains r.seq and seek to it.
		if r.seq < nextSeq {
			if err := r.openAndSeek(segs, r.seq); err != nil {
				return 0, Record{}, err
			}
			continue
		}

		// Caught up: wait for an append, close, or cancellation.
		select {
		case <-ctx.Done():
			return 0, Record{}, ctx.Err()
		case <-j.closedCh:
		case <-j.notify:
		}
	}
}

// readFrame reads the frame at the current offset. ok is false (with nil error)
// when no complete frame is present (end of segment).
func (r *Reader) readFrame() (seq uint64, rec Record, advance int64, ok bool, err error) {
	hdr := make([]byte, headerSize)
	if _, e := r.file.ReadAt(hdr, r.off); e != nil {
		return 0, Record{}, 0, false, nil
	}
	seq = binary.BigEndian.Uint64(hdr[0:8])
	length := binary.BigEndian.Uint32(hdr[8:12])
	payload := make([]byte, length)
	if _, e := r.file.ReadAt(payload, r.off+headerSize); e != nil {
		return 0, Record{}, 0, false, nil
	}
	if e := json.Unmarshal(payload, &rec); e != nil {
		return 0, Record{}, 0, false, fmt.Errorf("decoding record seq %d: %w", seq, e)
	}
	return seq, rec, headerSize + int64(length), true, nil
}

// openAndSeek opens the segment containing target and positions the read offset
// at the first record with sequence >= target.
func (r *Reader) openAndSeek(segs []segment, target uint64) error {
	var seg *segment
	for i := range segs {
		if segs[i].base <= target {
			seg = &segs[i]
		} else {
			break
		}
	}
	if seg == nil {
		return nil
	}
	f, err := os.Open(seg.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // evicted between snapshot and open; caller retries
		}
		return err
	}

	off := int64(0)
	hdr := make([]byte, headerSize)
	for {
		if _, e := f.ReadAt(hdr, off); e != nil {
			break
		}
		seq := binary.BigEndian.Uint64(hdr[0:8])
		if seq >= target {
			break
		}
		length := binary.BigEndian.Uint32(hdr[8:12])
		off += headerSize + int64(length)
	}

	if r.file != nil {
		_ = r.file.Close()
	}
	r.file = f
	r.curBase = seg.base
	r.off = off
	return nil
}
