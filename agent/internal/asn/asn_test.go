package asn

import (
	"context"
	"errors"
	"net"
	"path/filepath"
	"testing"
)

func TestParseOrigin(t *testing.T) {
	rec, err := parseOrigin("15169 | 8.8.8.0/24 | US | arin | 2023-12-28")
	if err != nil {
		t.Fatal(err)
	}
	if rec.ASN != 15169 || rec.Prefix != "8.8.8.0/24" || rec.Country != "US" {
		t.Errorf("got %+v", rec)
	}
}

func TestParseOriginMultiOriginPrefix(t *testing.T) {
	// A prefix announced by more than one AS. Take the first rather than guess.
	rec, err := parseOrigin("8359 12389 | 5.102.176.0/20 | OM | ripencc | 2012-01-01")
	if err != nil {
		t.Fatal(err)
	}
	if rec.ASN != 8359 {
		t.Errorf("ASN = %d, want the first of the multi-origin set", rec.ASN)
	}
}

func TestParseOriginRejectsGarbage(t *testing.T) {
	for _, bad := range []string{"", "nonsense", "x | y | z", "| |"} {
		if _, err := parseOrigin(bad); err == nil {
			t.Errorf("parseOrigin(%q) accepted bad input", bad)
		}
	}
}

func TestParseASName(t *testing.T) {
	got := parseASName("15169 | US | arin | 2000-03-30 | GOOGLE, US")
	if got != "GOOGLE, US" {
		t.Errorf("name = %q", got)
	}
	if parseASName("too | few | fields") != "" {
		t.Error("expected empty name for a short record")
	}
}

func TestPrivateAddressesAreNotLookedUp(t *testing.T) {
	private := []string{"192.168.0.1", "10.0.0.1", "172.16.5.4", "127.0.0.1",
		"169.254.1.1", "100.64.0.1", "0.0.0.0", "224.0.0.1"}
	for _, s := range private {
		if !Private(net.ParseIP(s)) {
			t.Errorf("%s should be treated as private", s)
		}
	}
	for _, s := range []string{"8.8.8.8", "5.102.177.197", "3.5.50.14"} {
		if Private(net.ParseIP(s)) {
			t.Errorf("%s is public", s)
		}
	}
}

func TestOfSkipsPrivateWithoutQuerying(t *testing.T) {
	queried := 0
	l := New("")
	l.TXT = func(ctx context.Context, name string) ([]string, error) {
		queried++
		return nil, errors.New("should not be called")
	}
	if _, ok := l.Of(context.Background(), "192.168.1.1"); ok {
		t.Error("private address returned a record")
	}
	if queried != 0 {
		t.Errorf("made %d DNS queries for a private address", queried)
	}
}

func TestOfCachesResults(t *testing.T) {
	queries := 0
	l := New("")
	l.TXT = func(ctx context.Context, name string) ([]string, error) {
		queries++
		if name == "AS15169.asn.cymru.com" {
			return []string{"15169 | US | arin | 2000-03-30 | GOOGLE, US"}, nil
		}
		return []string{"15169 | 8.8.8.0/24 | US | arin | 2023-12-28"}, nil
	}

	rec, ok := l.Of(context.Background(), "8.8.8.8")
	if !ok || rec.ASN != 15169 || rec.Name != "GOOGLE, US" {
		t.Fatalf("got %+v ok=%v", rec, ok)
	}
	after := queries

	for i := 0; i < 5; i++ {
		l.Of(context.Background(), "8.8.8.8")
	}
	if queries != after {
		t.Errorf("cache missed: %d queries became %d", after, queries)
	}
}

func TestFailedLookupIsCachedToo(t *testing.T) {
	// One unroutable hop must not mean a DNS query on every trace.
	queries := 0
	l := New("")
	l.TXT = func(ctx context.Context, name string) ([]string, error) {
		queries++
		return nil, errors.New("nxdomain")
	}
	for i := 0; i < 3; i++ {
		if _, ok := l.Of(context.Background(), "198.51.100.7"); ok {
			t.Fatal("expected no record")
		}
	}
	if queries > 1 {
		t.Errorf("%d queries for a known-bad address, want 1", queries)
	}
}

func TestCacheSurvivesRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "asn.json")

	first := New(path)
	first.TXT = func(ctx context.Context, name string) ([]string, error) {
		if name == "AS8359.asn.cymru.com" {
			return []string{"8359 | OM | ripencc | 2001-01-01 | OMANTEL, OM"}, nil
		}
		return []string{"8359 | 5.102.176.0/20 | OM | ripencc | 2012-01-01"}, nil
	}
	first.Of(context.Background(), "5.102.177.197")
	if err := first.Save(); err != nil {
		t.Fatal(err)
	}

	second := New(path)
	second.TXT = func(ctx context.Context, name string) ([]string, error) {
		t.Fatal("restarted process re-queried a cached address")
		return nil, nil
	}
	rec, ok := second.Of(context.Background(), "5.102.177.197")
	if !ok || rec.ASN != 8359 || rec.Name != "OMANTEL, OM" {
		t.Errorf("after restart got %+v ok=%v", rec, ok)
	}
}
