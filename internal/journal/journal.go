// Package journal implements Tollan's disk-backed ingest journal: an
// append-only, segmented, bounded queue. Inputs Append raw messages and return
// immediately; a single consumer reads them forward in order and commits its
// progress, so a restart resumes roughly where it left off (at-least-once).
//
// The journal is bounded by a maximum total size; when exceeded, the oldest
// segment is evicted. This absorbs ingest bursts while capping disk use, at the
// cost of dropping the oldest un-consumed data under sustained overload — the
// same trade-off Graylog's journal makes. Utilization is exported for alerting.
package journal

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// Record is one raw message as received by an input, before decoding.
type Record struct {
	InputID    string    `json:"input_id"`
	InputType  string    `json:"input_type"`
	Source     string    `json:"source"`
	ReceivedAt time.Time `json:"received_at"`
	Payload    []byte    `json:"payload"`
}

// Default sizing.
const (
	DefaultMaxSegmentBytes int64 = 16 << 20 // 16 MiB
	DefaultMaxTotalBytes   int64 = 1 << 30  // 1 GiB
	headerSize                   = 12       // 8-byte seq + 4-byte length
	segPrefix                    = "seg-"
	segSuffix                    = ".jnl"
	offsetFile                   = "committed.offset"
)

// Options configures a Journal.
type Options struct {
	Dir             string
	MaxSegmentBytes int64
	MaxTotalBytes   int64
}

type segment struct {
	base  uint64 // sequence of the first record that may be written here
	path  string
	bytes int64
}

// Journal is a segmented append-only queue with a single committed cursor.
type Journal struct {
	dir      string
	maxSeg   int64
	maxTotal int64

	mu       sync.Mutex
	segments []segment // ordered oldest -> newest; last is active
	active   *os.File
	nextSeq  uint64 // sequence to assign to the next appended record
	readNext uint64 // next sequence a consumer should read (persisted cursor)
	closed   bool

	notify   chan struct{} // pinged on append so readers wake
	closedCh chan struct{} // closed on Close
}

// Open opens or creates a journal in dir.
func Open(opts Options) (*Journal, error) {
	if opts.MaxSegmentBytes <= 0 {
		opts.MaxSegmentBytes = DefaultMaxSegmentBytes
	}
	if opts.MaxTotalBytes <= 0 {
		opts.MaxTotalBytes = DefaultMaxTotalBytes
	}
	if err := os.MkdirAll(opts.Dir, 0o750); err != nil {
		return nil, fmt.Errorf("creating journal dir: %w", err)
	}
	j := &Journal{
		dir:      opts.Dir,
		maxSeg:   opts.MaxSegmentBytes,
		maxTotal: opts.MaxTotalBytes,
		notify:   make(chan struct{}, 1),
		closedCh: make(chan struct{}),
	}
	if err := j.recover(); err != nil {
		return nil, err
	}
	return j, nil
}

// recover scans existing segments and the committed offset to restore state.
func (j *Journal) recover() error {
	entries, err := os.ReadDir(j.dir)
	if err != nil {
		return fmt.Errorf("reading journal dir: %w", err)
	}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || len(name) < len(segPrefix)+len(segSuffix) ||
			name[:len(segPrefix)] != segPrefix {
			continue
		}
		var base uint64
		if _, err := fmt.Sscanf(name, segPrefix+"%020d"+segSuffix, &base); err != nil {
			continue
		}
		info, err := e.Info()
		if err != nil {
			return err
		}
		j.segments = append(j.segments, segment{
			base:  base,
			path:  filepath.Join(j.dir, name),
			bytes: info.Size(),
		})
	}
	sort.Slice(j.segments, func(a, b int) bool { return j.segments[a].base < j.segments[b].base })

	// Determine nextSeq by scanning the last segment for the highest seq.
	j.nextSeq = 0
	if n := len(j.segments); n > 0 {
		last := j.segments[n-1]
		hi, err := highestSeq(last.path)
		if err != nil {
			return err
		}
		if hi >= last.base {
			j.nextSeq = hi + 1
		} else {
			j.nextSeq = last.base
		}
		f, err := os.OpenFile(last.path, os.O_RDWR|os.O_APPEND, 0o640)
		if err != nil {
			return fmt.Errorf("opening active segment: %w", err)
		}
		j.active = f
	}

	// Restore the read cursor (next sequence to read).
	if data, err := os.ReadFile(filepath.Join(j.dir, offsetFile)); err == nil {
		var c uint64
		if _, err := fmt.Sscanf(string(data), "%d", &c); err == nil {
			j.readNext = c
		}
	}
	// A cursor ahead of nextSeq (drained journal) is normalized.
	if j.readNext > j.nextSeq {
		j.readNext = j.nextSeq
	}
	return nil
}

// highestSeq returns the highest record sequence stored in a segment file, or 0
// if empty. It tolerates a torn trailing record (partial write before a crash).
func highestSeq(path string) (uint64, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	var hi uint64
	var off int64
	hdr := make([]byte, headerSize)
	for {
		if _, err := readAt(f, hdr, off); err != nil {
			break // EOF or torn header: stop
		}
		seq := binary.BigEndian.Uint64(hdr[0:8])
		length := binary.BigEndian.Uint32(hdr[8:12])
		recEnd := off + headerSize + int64(length)
		fi, _ := f.Stat()
		if fi != nil && recEnd > fi.Size() {
			break // torn record
		}
		hi = seq
		off = recEnd
	}
	return hi, nil
}

func readAt(f *os.File, b []byte, off int64) (int, error) {
	n, err := f.ReadAt(b, off)
	if err != nil {
		return n, err
	}
	return n, nil
}

func segName(base uint64) string {
	return fmt.Sprintf("%s%020d%s", segPrefix, base, segSuffix)
}

// Append writes a record and returns its assigned sequence. It is safe for
// concurrent use by many inputs.
func (j *Journal) Append(rec Record) (uint64, error) {
	payload, err := json.Marshal(rec)
	if err != nil {
		return 0, fmt.Errorf("encoding record: %w", err)
	}

	j.mu.Lock()
	defer j.mu.Unlock()
	if j.closed {
		return 0, fmt.Errorf("journal closed")
	}

	if err := j.ensureActive(); err != nil {
		return 0, err
	}
	// Rotate if the active segment would exceed the max segment size.
	if j.active != nil && j.segments[len(j.segments)-1].bytes > 0 &&
		j.segments[len(j.segments)-1].bytes+int64(headerSize+len(payload)) > j.maxSeg {
		if err := j.rotate(); err != nil {
			return 0, err
		}
	}

	seq := j.nextSeq
	frame := make([]byte, headerSize+len(payload))
	binary.BigEndian.PutUint64(frame[0:8], seq)
	binary.BigEndian.PutUint32(frame[8:12], uint32(len(payload)))
	copy(frame[headerSize:], payload)
	if _, err := j.active.Write(frame); err != nil {
		return 0, fmt.Errorf("writing record: %w", err)
	}
	j.nextSeq++
	j.segments[len(j.segments)-1].bytes += int64(len(frame))

	j.evictLocked()
	// Wake a waiting reader (non-blocking; buffered depth 1 coalesces pings).
	select {
	case j.notify <- struct{}{}:
	default:
	}
	return seq, nil
}

// ensureActive makes sure there is an active segment to write to.
func (j *Journal) ensureActive() error {
	if j.active != nil {
		return nil
	}
	base := j.nextSeq
	path := filepath.Join(j.dir, segName(base))
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR|os.O_APPEND, 0o640)
	if err != nil {
		return fmt.Errorf("creating segment: %w", err)
	}
	j.active = f
	j.segments = append(j.segments, segment{base: base, path: path})
	return nil
}

// rotate closes the active segment and starts a new one.
func (j *Journal) rotate() error {
	if j.active != nil {
		if err := j.active.Sync(); err != nil {
			return err
		}
		if err := j.active.Close(); err != nil {
			return err
		}
		j.active = nil
	}
	return j.ensureActive()
}

// evictLocked drops the oldest segments while total size exceeds the cap. The
// active segment is never evicted.
func (j *Journal) evictLocked() {
	for j.totalBytesLocked() > j.maxTotal && len(j.segments) > 1 {
		oldest := j.segments[0]
		_ = os.Remove(oldest.path)
		j.segments = j.segments[1:]
		// Advance the read cursor past evicted data so we never seek into it.
		if newBase := j.segments[0].base; j.readNext < newBase {
			j.readNext = newBase
		}
	}
}

func (j *Journal) totalBytesLocked() int64 {
	var t int64
	for _, s := range j.segments {
		t += s.bytes
	}
	return t
}

// Sync flushes the active segment to disk.
func (j *Journal) Sync() error {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.active != nil {
		return j.active.Sync()
	}
	return nil
}

// Depth returns the number of un-consumed records.
func (j *Journal) Depth() uint64 {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.nextSeq <= j.readNext {
		return 0
	}
	return j.nextSeq - j.readNext
}

// Utilization returns the fraction (0..1) of the max total size in use.
func (j *Journal) Utilization() float64 {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.maxTotal <= 0 {
		return 0
	}
	return float64(j.totalBytesLocked()) / float64(j.maxTotal)
}

// ReadPos returns the next sequence a consumer should read.
func (j *Journal) ReadPos() uint64 {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.readNext
}

// Commit persists progress: it marks every sequence up to and including seq as
// processed, so a restart resumes at seq+1.
func (j *Journal) Commit(seq uint64) error {
	j.mu.Lock()
	if seq+1 > j.readNext {
		j.readNext = seq + 1
	}
	c := j.readNext
	j.mu.Unlock()
	tmp := filepath.Join(j.dir, offsetFile+".tmp")
	if err := os.WriteFile(tmp, []byte(fmt.Sprintf("%d", c)), 0o640); err != nil {
		return err
	}
	return os.Rename(tmp, filepath.Join(j.dir, offsetFile))
}

// Close flushes and closes the journal, waking any blocked reader.
func (j *Journal) Close() error {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.closed {
		return nil
	}
	j.closed = true
	close(j.closedCh)
	if j.active != nil {
		_ = j.active.Sync()
		err := j.active.Close()
		j.active = nil
		return err
	}
	return nil
}
