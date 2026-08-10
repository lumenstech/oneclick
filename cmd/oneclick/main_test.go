package main

import (
	"flag"
	"strings"
	"testing"
)

func TestPlanIdentityFlagsRemainBoundAfterParse(t *testing.T) {
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	identity := addPlanIdentityFlags(fs)
	hash := strings.Repeat("a", 64)
	sha := strings.Repeat("b", 40)
	err := fs.Parse([]string{
		"--source-ref=https://github.com/acme/demo",
		"--source-type=github",
		"--source-revision=" + sha,
		"--expected-plan-hash=" + hash,
	})
	if err != nil {
		t.Fatal(err)
	}
	if identity.SourceRef != "https://github.com/acme/demo" || identity.SourceType != "github" || identity.SourceRevision != sha || identity.ExpectedPlanHash != hash {
		t.Fatalf("parsed identity flags were not retained: %+v", identity)
	}
}

func TestIsHexLen(t *testing.T) {
	if !isHexLen(strings.Repeat("a", 64), 64) {
		t.Fatal("valid hex rejected")
	}
	if isHexLen(strings.Repeat("z", 64), 64) || isHexLen("abcd", 64) {
		t.Fatal("invalid hex accepted")
	}
}
