package extract

import (
	"os"
	"strings"
	"testing"
)

func TestFromHTML_Economist(t *testing.T) {
	f, err := os.Open("testdata/economist.html")
	if err != nil {
		t.Fatalf("failed to open test data: %v", err)
	}
	defer f.Close()

	got, err := FromHTML(f)
	if err != nil {
		t.Fatalf("FromHTML failed: %v", err)
	}

	// Check for common CSS/JS artifacts that shouldn't be there
	unwanted := []string{
		".css-",
		"{margin-left:",
		"display:grid",
		"window.wallInfo",
		"var(--mb-responsive",
	}

	for _, u := range unwanted {
		if strings.Contains(got, u) {
			t.Errorf("output contains unwanted string %q", u)
		}
	}

	// Check for expected structure
	expectedTitle := "## Age gaps in relationships are not as bad as you think"
	if !strings.Contains(got, expectedTitle) {
		t.Errorf("output missing title %q", expectedTitle)
	}
	
	expectedSubtitle := "On screen and in real life, they are getting smaller anyway"
	if !strings.Contains(got, expectedSubtitle) {
		t.Logf("Output: %q", got)
		t.Errorf("output missing subtitle %q", expectedSubtitle)
	}

	// Also ensure we actually got some text content (sanity check)
	if len(got) < 100 {
		t.Logf("Output: %s", got)
		t.Error("output seems too short, maybe nothing was extracted?")
	}
}
