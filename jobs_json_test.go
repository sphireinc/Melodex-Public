package main

import (
	"strings"
	"testing"
)

func TestDecodeFirstJSONObjectSkipsLeadingText(t *testing.T) {
	var info ytDLPInfo
	err := decodeFirstJSONObject([]byte("ERROR: ignored line\n{\"title\":\"Tourniquet\",\"id\":\"abc\"}\n"), &info)
	if err != nil {
		t.Fatalf("decodeFirstJSONObject returned error: %v", err)
	}
	if info.Title != "Tourniquet" || info.ID != "abc" {
		t.Fatalf("decoded info mismatch: %#v", info)
	}
}

func TestDecodeFirstJSONObjectSkipsLeadingNumber(t *testing.T) {
	var info ytDLPInfo
	err := decodeFirstJSONObject([]byte("100\n{\"title\":\"Imaginary\",\"id\":\"def\"}\n"), &info)
	if err != nil {
		t.Fatalf("decodeFirstJSONObject returned error: %v", err)
	}
	if info.Title != "Imaginary" || info.ID != "def" {
		t.Fatalf("decoded info mismatch: %#v", info)
	}
}

func TestDecodeFirstJSONObjectReportsMissingObject(t *testing.T) {
	var info ytDLPInfo
	err := decodeFirstJSONObject([]byte("Downloading metadata\nComplete\n"), &info)
	if err == nil {
		t.Fatal("expected decodeFirstJSONObject to fail")
	}
	if !strings.Contains(err.Error(), "no JSON object found") {
		t.Fatalf("unexpected error: %v", err)
	}
}
