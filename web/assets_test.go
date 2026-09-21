package web

import (
	"strings"
	"testing"
)

func TestLocationPollingDoesNotOverwriteActiveEdits(t *testing.T) {
	data, err := Assets.ReadFile("static/app.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"let locationFormDirty = false;",
		"locationForm.contains(document.activeElement)",
		"if (forceLocation || !locationFieldsAreBeingEdited())",
		"locationFormDirty = false;",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("app.js missing edit-preservation guard %q", want)
		}
	}
}
