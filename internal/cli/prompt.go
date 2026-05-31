package cli

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"
)

type promptIO struct {
	in       io.Reader
	out      io.Writer
	language string
}

type menuOption struct {
	Label string
	Value string
}

func newPromptIO(cmdOut io.Writer) promptIO {
	return promptIO{in: inputReader, out: cmdOut, language: "en"}
}

func newPromptIOWithLanguage(cmdOut io.Writer, language string) promptIO {
	if language != "zh" {
		language = "en"
	}
	return promptIO{in: inputReader, out: cmdOut, language: language}
}

func (p promptIO) zh() bool {
	return p.language == "zh"
}

func (p promptIO) msg(en string, zh string) string {
	if p.zh() {
		return zh
	}
	return en
}

func (p promptIO) askRequired(label string, current string) (string, error) {
	if strings.TrimSpace(current) != "" {
		return current, nil
	}
	reader := bufio.NewReader(p.in)
	for {
		fmt.Fprintf(p.out, "%s: ", label)
		value, err := reader.ReadString('\n')
		if err != nil {
			return "", fmt.Errorf("read %s: %w", label, err)
		}
		value = strings.TrimSpace(value)
		if value != "" {
			return value, nil
		}
		fmt.Fprintf(p.out, "%s\n", p.msg(label+" is required", label+" 为必填项"))
	}
}

func (p promptIO) askString(label string, current string, def string) (string, error) {
	if strings.TrimSpace(current) != "" {
		return current, nil
	}
	reader := bufio.NewReader(p.in)
	fmt.Fprintf(p.out, "%s [%s]: ", label, def)
	value, err := reader.ReadString('\n')
	if err != nil {
		return "", fmt.Errorf("read %s: %w", label, err)
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return def, nil
	}
	return value, nil
}

func (p promptIO) askOptionalString(label string, current string) (string, error) {
	if strings.TrimSpace(current) != "" {
		return current, nil
	}
	reader := bufio.NewReader(p.in)
	fmt.Fprintf(p.out, "%s (optional): ", label)
	value, err := reader.ReadString('\n')
	if err != nil {
		return "", fmt.Errorf("read %s: %w", label, err)
	}
	return strings.TrimSpace(value), nil
}

func (p promptIO) askInt(label string, current int, def int) (int, error) {
	if current != 0 {
		return current, nil
	}
	reader := bufio.NewReader(p.in)
	for {
		fmt.Fprintf(p.out, "%s [%d]: ", label, def)
		value, err := reader.ReadString('\n')
		if err != nil {
			return 0, fmt.Errorf("read %s: %w", label, err)
		}
		value = strings.TrimSpace(value)
		if value == "" {
			return def, nil
		}
		n, err := strconv.Atoi(value)
		if err != nil {
			fmt.Fprintln(p.out, p.msg("enter a number", "请输入数字"))
			continue
		}
		return n, nil
	}
}

func (p promptIO) askRequiredInt(label string, current int) (int, error) {
	if current != 0 {
		return current, nil
	}
	reader := bufio.NewReader(p.in)
	for {
		fmt.Fprintf(p.out, "%s: ", label)
		value, err := reader.ReadString('\n')
		if err != nil {
			return 0, fmt.Errorf("read %s: %w", label, err)
		}
		value = strings.TrimSpace(value)
		if value == "" {
			fmt.Fprintf(p.out, "%s\n", p.msg(label+" is required", label+" 为必填项"))
			continue
		}
		n, err := strconv.Atoi(value)
		if err != nil {
			fmt.Fprintln(p.out, p.msg("enter a number", "请输入数字"))
			continue
		}
		return n, nil
	}
}

func (p promptIO) askBool(label string, current bool, def bool) (bool, error) {
	if current {
		return true, nil
	}
	reader := bufio.NewReader(p.in)
	defaultText := "n"
	if def {
		defaultText = "y"
	}
	for {
		fmt.Fprintf(p.out, "%s [y/n, default %s]: ", label, defaultText)
		value, err := reader.ReadString('\n')
		if err != nil {
			return false, fmt.Errorf("read %s: %w", label, err)
		}
		value = strings.ToLower(strings.TrimSpace(value))
		if value == "" {
			return def, nil
		}
		switch value {
		case "y", "yes":
			return true, nil
		case "n", "no":
			return false, nil
		default:
			fmt.Fprintln(p.out, p.msg("enter y or n", "请输入 y 或 n"))
		}
	}
}

func (p promptIO) askMenu(label string, current string, def string, options []menuOption) (string, error) {
	if strings.TrimSpace(current) != "" {
		return current, nil
	}
	reader := bufio.NewReader(p.in)
	defaultIndex := 1
	for i, option := range options {
		if option.Value == def {
			defaultIndex = i + 1
			break
		}
	}
	for {
		fmt.Fprintf(p.out, "%s:\n", label)
		for i, option := range options {
			fmt.Fprintf(p.out, "  %d. %s\n", i+1, option.Label)
		}
		fmt.Fprintf(p.out, "%s [%d]: ", p.msg("Select", "选择"), defaultIndex)
		value, err := reader.ReadString('\n')
		if err != nil {
			return "", fmt.Errorf("read %s: %w", label, err)
		}
		value = strings.TrimSpace(value)
		if value == "" {
			return def, nil
		}
		n, err := strconv.Atoi(value)
		if err != nil || n < 1 || n > len(options) {
			fmt.Fprintln(p.out, p.msg("invalid selection", "无效选择"))
			continue
		}
		return options[n-1].Value, nil
	}
}
