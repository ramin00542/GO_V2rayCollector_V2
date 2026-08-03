package repository

import "testing"

func TestNormalizeSourceURL(t *testing.T) {
	valid, ok := NormalizeSourceURL("https://example.org/list.txt#label")
	if !ok || valid != "https://example.org/list.txt" {
		t.Fatalf("unexpected normalized URL: %q, %v", valid, ok)
	}
	if _, ok := NormalizeSourceURL("http://example.org/list.txt"); ok {
		t.Fatal("HTTP source should not be accepted")
	}
}
