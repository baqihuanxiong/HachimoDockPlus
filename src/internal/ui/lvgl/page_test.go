package lvgl

import (
	"hachimodockplus/internal/core"
	"testing"
)

type testPage string

func (p testPage) ID() string                  { return string(p) }
func (testPage) Mount() error                  { return nil }
func (testPage) Render(core.AppSnapshot) error { return nil }
func (testPage) Unmount()                      {}

func TestPageRegistrationCoverage(t *testing.T) {
	apps := []core.Manifest{{ID: "one"}, {ID: "two"}}
	if _, err := validatePages(apps, []Page{testPage("one")}); err == nil {
		t.Fatal("missing page accepted")
	}
	if _, err := validatePages(apps, []Page{testPage("one"), testPage("one")}); err == nil {
		t.Fatal("duplicate page accepted")
	}
	if pages, err := validatePages(apps, []Page{testPage("one"), testPage("two")}); err != nil || len(pages) != 2 {
		t.Fatal(err)
	}
}
