package cli

import (
	"bytes"
	"testing"
)

func TestPromptMenuDefault(t *testing.T) {
	inputReader = bytes.NewBufferString("\n")
	t.Cleanup(func() { inputReader = defaultInputReader() })

	var out bytes.Buffer
	got, err := (promptIO{in: inputReader, out: &out}).askMenu("mode", "", "direct", []menuOption{
		{Label: "direct", Value: "direct"},
		{Label: "warp", Value: "warp"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got != "direct" {
		t.Fatalf("unexpected value: %s", got)
	}
}

func TestPromptMenuSelection(t *testing.T) {
	inputReader = bytes.NewBufferString("2\n")
	t.Cleanup(func() { inputReader = defaultInputReader() })

	var out bytes.Buffer
	got, err := (promptIO{in: inputReader, out: &out}).askMenu("mode", "", "direct", []menuOption{
		{Label: "direct", Value: "direct"},
		{Label: "warp", Value: "warp"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got != "warp" {
		t.Fatalf("unexpected value: %s", got)
	}
}

func TestPromptRequiredIntRejectsEmptyThenReadsValue(t *testing.T) {
	inputReader = bytes.NewBufferString("\n12345\n")
	t.Cleanup(func() { inputReader = defaultInputReader() })

	var out bytes.Buffer
	got, err := (promptIO{in: inputReader, out: &out}).askRequiredInt("port", 0)
	if err != nil {
		t.Fatal(err)
	}
	if got != 12345 {
		t.Fatalf("unexpected value: %d", got)
	}
}
