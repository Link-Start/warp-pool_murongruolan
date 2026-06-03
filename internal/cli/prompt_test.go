package cli

import (
	"bufio"
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

func TestPromptBoolDefaultAndSelection(t *testing.T) {
	inputReader = bytes.NewBufferString("\nn\n")
	t.Cleanup(func() { inputReader = defaultInputReader() })

	var out bytes.Buffer
	p := promptIO{in: bufio.NewReader(bytes.NewBufferString("\nn\n")), out: &out}
	first, err := p.askBool("remove wg", false, true)
	if err != nil {
		t.Fatal(err)
	}
	second, err := p.askBool("remove proxy", false, true)
	if err != nil {
		t.Fatal(err)
	}
	if !first || second {
		t.Fatalf("unexpected bool results: %v %v", first, second)
	}
}

func TestPromptConfirmDefaultNo(t *testing.T) {
	var out bytes.Buffer
	p := promptIO{in: bufio.NewReader(bytes.NewBufferString("\ny\n")), out: &out}
	first, err := p.askConfirmDefaultNo("remove node", false)
	if err != nil {
		t.Fatal(err)
	}
	second, err := p.askConfirmDefaultNo("remove node", false)
	if err != nil {
		t.Fatal(err)
	}
	if first || !second {
		t.Fatalf("unexpected confirm results: %v %v", first, second)
	}
	if !bytes.Contains(out.Bytes(), []byte("[y/N]")) {
		t.Fatalf("missing y/N prompt: %s", out.String())
	}
}

func TestPromptChineseMenuAndRequiredIntMessages(t *testing.T) {
	var out bytes.Buffer
	input := bufio.NewReader(bytes.NewBufferString("x\n2\n\n12345\n"))
	p := promptIO{in: input, out: &out, language: "zh"}

	got, err := p.askMenu("出口模式", "", "direct", []menuOption{
		{Label: "direct", Value: "direct"},
		{Label: "warp", Value: "warp"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got != "warp" {
		t.Fatalf("unexpected menu value: %s", got)
	}

	port, err := p.askRequiredInt("本地代理端口", 0)
	if err != nil {
		t.Fatal(err)
	}
	if port != 12345 {
		t.Fatalf("unexpected port: %d", port)
	}

	text := out.String()
	if !bytes.Contains([]byte(text), []byte("选择 [1]:")) {
		t.Fatalf("missing chinese select prompt: %s", text)
	}
	if !bytes.Contains([]byte(text), []byte("无效选择")) {
		t.Fatalf("missing chinese invalid selection: %s", text)
	}
	if !bytes.Contains([]byte(text), []byte("本地代理端口 为必填项")) {
		t.Fatalf("missing chinese required message: %s", text)
	}
}
