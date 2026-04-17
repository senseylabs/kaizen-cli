package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"golang.org/x/term"
)

// isInteractive returns true if stdin is a terminal and --json is not set.
func isInteractive() bool {
	return !cfgJSON && term.IsTerminal(int(os.Stdin.Fd()))
}

// promptColor returns the ANSI color code if the terminal supports colors, otherwise empty string.
func promptColor(code string) string {
	if !term.IsTerminal(int(os.Stdout.Fd())) {
		return ""
	}
	return code
}

// promptReset returns the ANSI reset code if the terminal supports colors.
func promptReset() string {
	if !term.IsTerminal(int(os.Stdout.Fd())) {
		return ""
	}
	return "\033[0m"
}

// SelectOption pairs a display name with an underlying value for select prompts.
type SelectOption struct {
	Display string
	Value   string
}

// promptText prompts for free-text input. Returns empty string if user presses Enter without input.
func promptText(label string) (string, error) {
	reader := bufio.NewReader(os.Stdin)
	cyan := promptColor("\033[36m")
	reset := promptReset()
	_, _ = fmt.Fprintf(os.Stdout, "%s%s:%s ", cyan, label, reset)
	input, err := reader.ReadString('\n')
	if err != nil {
		return "", fmt.Errorf("failed to read input: %w", err)
	}
	return strings.TrimSpace(input), nil
}

// promptTextRequired prompts for text input, repeating until non-empty.
func promptTextRequired(label string) (string, error) {
	for {
		val, err := promptText(label)
		if err != nil {
			return "", err
		}
		if val != "" {
			return val, nil
		}
		_, _ = fmt.Fprintf(os.Stdout, "  %s is required. Please enter a value.\n", label)
	}
}

// promptSingleSelect shows a numbered list and lets user pick one. Returns the selected index.
func promptSingleSelect(label string, options []string) (int, error) {
	cyan := promptColor("\033[36m")
	dim := promptColor("\033[90m")
	reset := promptReset()

	_, _ = fmt.Fprintf(os.Stdout, "%s%s:%s\n", cyan, label, reset)
	for i, opt := range options {
		_, _ = fmt.Fprintf(os.Stdout, "  %s%d%s  %s\n", dim, i+1, reset, opt)
	}

	reader := bufio.NewReader(os.Stdin)
	for {
		_, _ = fmt.Fprintf(os.Stdout, "Select: ")
		input, err := reader.ReadString('\n')
		if err != nil {
			return -1, fmt.Errorf("failed to read input: %w", err)
		}
		input = strings.TrimSpace(input)

		num, parseErr := strconv.Atoi(input)
		if parseErr != nil || num < 1 || num > len(options) {
			_, _ = fmt.Fprintf(os.Stdout, "Invalid selection. Enter a number between 1 and %d.\n", len(options))
			continue
		}
		return num - 1, nil
	}
}

// promptSingleSelectWithValues shows a numbered list with display names but returns the associated value.
func promptSingleSelectWithValues(label string, options []SelectOption) (string, error) {
	displays := make([]string, len(options))
	for i, opt := range options {
		displays[i] = opt.Display
	}
	idx, err := promptSingleSelect(label, displays)
	if err != nil {
		return "", err
	}
	return options[idx].Value, nil
}

// promptMultiSelect shows a numbered list and lets user pick multiple items.
// Returns selected indices.
func promptMultiSelect(label string, options []string) ([]int, error) {
	cyan := promptColor("\033[36m")
	dim := promptColor("\033[90m")
	green := promptColor("\033[32m")
	reset := promptReset()

	_, _ = fmt.Fprintf(os.Stdout, "%s%s:%s\n", cyan, label, reset)
	for i, opt := range options {
		_, _ = fmt.Fprintf(os.Stdout, "  %s%d%s  %s\n", dim, i+1, reset, opt)
	}

	reader := bufio.NewReader(os.Stdin)
	selected := []int{}

	for {
		if len(selected) == 0 {
			_, _ = fmt.Fprintf(os.Stdout, "Select (or 'd' when done): ")
		} else {
			_, _ = fmt.Fprintf(os.Stdout, "Select (or 'd' when done, 'r' to remove last): ")
		}

		input, err := reader.ReadString('\n')
		if err != nil {
			return nil, fmt.Errorf("failed to read input: %w", err)
		}
		input = strings.TrimSpace(input)

		if strings.EqualFold(input, "d") {
			return selected, nil
		}

		if strings.EqualFold(input, "r") && len(selected) > 0 {
			selected = selected[:len(selected)-1]
			if len(selected) > 0 {
				names := make([]string, len(selected))
				for i, idx := range selected {
					names[i] = options[idx]
				}
				_, _ = fmt.Fprintf(os.Stdout, "  %sSelected: %s%s\n", green, strings.Join(names, ", "), reset)
			} else {
				_, _ = fmt.Fprintf(os.Stdout, "  Selection cleared.\n")
			}
			continue
		}

		num, parseErr := strconv.Atoi(input)
		if parseErr != nil || num < 1 || num > len(options) {
			_, _ = fmt.Fprintf(os.Stdout, "Invalid selection. Enter a number between 1 and %d.\n", len(options))
			continue
		}

		idx := num - 1
		// Check for duplicate
		alreadySelected := false
		for _, s := range selected {
			if s == idx {
				alreadySelected = true
				break
			}
		}
		if alreadySelected {
			_, _ = fmt.Fprintf(os.Stdout, "  Already selected. Pick another or 'd' when done.\n")
			continue
		}

		selected = append(selected, idx)
		names := make([]string, len(selected))
		for i, s := range selected {
			names[i] = options[s]
		}
		_, _ = fmt.Fprintf(os.Stdout, "  %sSelected: %s%s\n", green, strings.Join(names, ", "), reset)
	}
}

// promptMultiSelectWithValues shows a numbered list with display names and returns associated values.
func promptMultiSelectWithValues(label string, options []SelectOption) ([]string, error) {
	displays := make([]string, len(options))
	for i, opt := range options {
		displays[i] = opt.Display
	}
	indices, err := promptMultiSelect(label, displays)
	if err != nil {
		return nil, err
	}
	values := make([]string, len(indices))
	for i, idx := range indices {
		values[i] = options[idx].Value
	}
	return values, nil
}

// promptYesNo asks "Label? (y/n): " and returns true if the user answers yes.
func promptYesNo(label string) (bool, error) {
	cyan := promptColor("\033[36m")
	dim := promptColor("\033[90m")
	reset := promptReset()

	reader := bufio.NewReader(os.Stdin)
	for {
		_, _ = fmt.Fprintf(os.Stdout, "%s%s?%s %s(y/n)%s: ", cyan, label, reset, dim, reset)
		input, err := reader.ReadString('\n')
		if err != nil {
			return false, fmt.Errorf("failed to read input: %w", err)
		}
		input = strings.TrimSpace(strings.ToLower(input))
		switch input {
		case "y", "yes":
			return true, nil
		case "n", "no", "":
			return false, nil
		default:
			_, _ = fmt.Fprintf(os.Stdout, "Please enter 'y' or 'n'.\n")
		}
	}
}

// promptDate prompts for a date in YYYY-MM-DD format with validation.
func promptDate(label string) (string, error) {
	reader := bufio.NewReader(os.Stdin)
	cyan := promptColor("\033[36m")
	dim := promptColor("\033[90m")
	reset := promptReset()

	for {
		_, _ = fmt.Fprintf(os.Stdout, "%s%s%s %s(YYYY-MM-DD)%s: ", cyan, label, reset, dim, reset)
		input, err := reader.ReadString('\n')
		if err != nil {
			return "", fmt.Errorf("failed to read input: %w", err)
		}
		input = strings.TrimSpace(input)
		if input == "" {
			return "", nil
		}
		_, parseErr := time.Parse("2006-01-02", input)
		if parseErr != nil {
			_, _ = fmt.Fprintf(os.Stdout, "Invalid date format. Please use YYYY-MM-DD.\n")
			continue
		}
		return input, nil
	}
}

// promptInt prompts for an integer with validation.
func promptInt(label string) (int, error) {
	reader := bufio.NewReader(os.Stdin)
	cyan := promptColor("\033[36m")
	reset := promptReset()

	for {
		_, _ = fmt.Fprintf(os.Stdout, "%s%s:%s ", cyan, label, reset)
		input, err := reader.ReadString('\n')
		if err != nil {
			return 0, fmt.Errorf("failed to read input: %w", err)
		}
		input = strings.TrimSpace(input)
		if input == "" {
			return 0, nil
		}
		num, parseErr := strconv.Atoi(input)
		if parseErr != nil {
			_, _ = fmt.Fprintf(os.Stdout, "Invalid number. Please enter an integer.\n")
			continue
		}
		return num, nil
	}
}
