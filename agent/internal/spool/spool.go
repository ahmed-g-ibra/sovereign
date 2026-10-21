// Package spool buffers measurements that have not reached the server.
//
// An agent that drops data during an outage is worse than useless: the outage
// is the most interesting thing it will ever see. Samples are written to disk
// before any upload is attempted, survive a restart, and are deleted only once
// the server has acknowledged them.
package spool

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"sovereign/agent/internal/probe"
)

const (
	activeName  = "spool.ndjson"
	sendingName = "spool.sending.ndjson"

	// DefaultMaxBytes caps the spool at roughly two weeks of a 12-target probe
	// reporting every minute.
	DefaultMaxBytes int64 = 64 << 20
)

// Spool is an on-disk queue of pending measurements.
type Spool struct {
	dir      string
	maxBytes int64
	mu       sync.Mutex
}

func New(dir string, maxBytes int64) (*Spool, error) {
	if maxBytes <= 0 {
		maxBytes = DefaultMaxBytes
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	return &Spool{dir: dir, maxBytes: maxBytes}, nil
}

func (s *Spool) path(name string) string { return filepath.Join(s.dir, name) }

// Append writes measurements to the spool, oldest first.
func (s *Spool) Append(results []probe.Result) error {
	if len(results) == 0 {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	f, err := os.OpenFile(s.path(activeName), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	w := bufio.NewWriter(f)
	enc := json.NewEncoder(w)
	for _, r := range results {
		if err := enc.Encode(r); err != nil {
			f.Close()
			return err
		}
	}
	if err := w.Flush(); err != nil {
		f.Close()
		return err
	}
	// Flush to the OS, not to the platter. A power cut can cost the last
	// minute of samples; fsync on every batch would cost an SD card's life on
	// a Raspberry Pi, which is where most community probes will run.
	if err := f.Close(); err != nil {
		return err
	}
	return s.trimLocked()
}

// Claim takes everything currently pending and returns it with a commit
// function. The data stays on disk until commit is called, so a failed upload
// loses nothing.
//
// A previous claim that was never committed is returned again before any newer
// data, which keeps the spool in order across restarts.
func (s *Spool) Claim() ([]probe.Result, func() error, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	sending := s.path(sendingName)
	if _, err := os.Stat(sending); os.IsNotExist(err) {
		if _, err := os.Stat(s.path(activeName)); os.IsNotExist(err) {
			return nil, func() error { return nil }, nil
		}
		if err := os.Rename(s.path(activeName), sending); err != nil {
			return nil, nil, err
		}
	}

	results, err := readAll(sending)
	if err != nil {
		return nil, nil, err
	}
	commit := func() error {
		s.mu.Lock()
		defer s.mu.Unlock()
		return os.Remove(sending)
	}
	return results, commit, nil
}

// Pending reports how many bytes are waiting to be uploaded.
func (s *Spool) Pending() int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	var total int64
	for _, name := range []string{activeName, sendingName} {
		if fi, err := os.Stat(s.path(name)); err == nil {
			total += fi.Size()
		}
	}
	return total
}

// trimLocked enforces the size cap by dropping the oldest half.
//
// Dropping old data rather than refusing new data is the deliberate choice: a
// probe that stops measuring during a long outage produces a gap exactly where
// the interesting event is, and an unbounded spool eventually fills a
// volunteer's disk, which is how you lose a volunteer.
func (s *Spool) trimLocked() error {
	fi, err := os.Stat(s.path(activeName))
	if err != nil || fi.Size() <= s.maxBytes {
		return err
	}

	results, err := readAll(s.path(activeName))
	if err != nil {
		return err
	}
	keep := results[len(results)/2:]

	tmp := s.path(activeName + ".tmp")
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	w := bufio.NewWriter(f)
	enc := json.NewEncoder(w)
	for _, r := range keep {
		if err := enc.Encode(r); err != nil {
			f.Close()
			return err
		}
	}
	if err := w.Flush(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, s.path(activeName))
}

func readAll(path string) ([]probe.Result, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var out []probe.Result
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for line := 1; sc.Scan(); line++ {
		if len(sc.Bytes()) == 0 {
			continue
		}
		var r probe.Result
		if err := json.Unmarshal(sc.Bytes(), &r); err != nil {
			// One corrupt line, from a power cut mid-write, must not strand
			// every other sample in the file.
			fmt.Fprintf(os.Stderr, "spool: skipping unreadable line %d: %v\n", line, err)
			continue
		}
		out = append(out, r)
	}
	return out, sc.Err()
}
