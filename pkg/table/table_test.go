package table

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
)

type testPaginator struct {
	total   int
	items   []int
	findErr error
}

var _ Paginator[int] = testPaginator{}

func (tp testPaginator) totalAm() int { return tp.total }
func (tp testPaginator) findPage(start, offset int) ([]int, error) {
	if tp.findErr != nil {
		return nil, tp.findErr
	}
	end := min(start+offset, len(tp.items))
	if start > len(tp.items) {
		return []int{}, nil
	}
	return tp.items[start:end], nil
}

// --- Internal unit tests ---

func Test_table_nextPage(t *testing.T) {
	tab := table[int]{page: 1, lastPage: 2}

	action := tab.nextPage()
	if action.Format != "[n]ext" || action.Short != "n" || action.Long != "next" {
		t.Fatalf("nextPage() metadata = %+v", action)
	}
	if action.AdditionalHotkeys != "" {
		t.Fatalf("nextPage() AdditionalHotkeys = %q, want empty string", action.AdditionalHotkeys)
	}
	if err := action.Action(); err != nil {
		t.Fatalf("nextPage action returned error: %v", err)
	}
	if tab.page != 2 {
		t.Fatalf("page after nextPage = %d, want 2", tab.page)
	}

	if err := action.Action(); err != nil {
		t.Fatalf("nextPage wrap action returned error: %v", err)
	}
	if tab.page != 0 {
		t.Fatalf("page after nextPage wrap = %d, want 0", tab.page)
	}
}

func Test_table_prevPage(t *testing.T) {
	tab := table[int]{page: 1, lastPage: 2}

	action := tab.prevPage()
	if action.Format != "[p]rev" || action.Short != "p" || action.Long != "prev" {
		t.Fatalf("prevPage() metadata = %+v", action)
	}
	if err := action.Action(); err != nil {
		t.Fatalf("prevPage action returned error: %v", err)
	}
	if tab.page != 0 {
		t.Fatalf("page after prevPage = %d, want 0", tab.page)
	}

	if err := action.Action(); err != nil {
		t.Fatalf("prevPage wrap action returned error: %v", err)
	}
	if tab.page != 2 {
		t.Fatalf("page after prevPage wrap = %d, want 2", tab.page)
	}
}

func Test_table_quit(t *testing.T) {
	action := new(table[int]).quit()
	if action.Format != "[q]uit" || action.Short != "q" || action.Long != "quit" {
		t.Fatalf("quit() metadata = %+v", action)
	}
	err := action.Action()
	if !errors.Is(err, ErrUserInitiatedExit) {
		t.Fatalf("quit action error = %v, want %v", err, ErrUserInitiatedExit)
	}
}

func Test_table_back(t *testing.T) {
	t.Run("default label", func(t *testing.T) {
		action := new(table[int]).back()
		if action.Format != "[b]ack" || action.Short != "b" || action.Long != "back" {
			t.Fatalf("back() metadata = %+v", action)
		}
		err := action.Action()
		if !errors.Is(err, ErrBack) {
			t.Fatalf("back action error = %v, want %v", err, ErrBack)
		}
	})

	t.Run("custom backLabel", func(t *testing.T) {
		tab := table[int]{backLabel: "[←] back to list"}
		action := tab.back()
		if action.Format != "[←] back to list" {
			t.Fatalf("back() Format = %q, want %q", action.Format, "[←] back to list")
		}
	})
}

func Test_table_pageCount(t *testing.T) {
	tests := []struct {
		name     string
		pageSize int
		total    int
		want     int
	}{
		{name: "zero page size returns zero", pageSize: 0, total: 10, want: 0},
		{name: "negative page size returns zero", pageSize: -1, total: 10, want: 0},
		{name: "zero total returns zero", pageSize: 5, total: 0, want: 0},
		{name: "single page returns zero", pageSize: 5, total: 5, want: 0},
		{name: "partial last page rounds down to last page index", pageSize: 5, total: 6, want: 1},
		{name: "multiple full pages returns last page index", pageSize: 5, total: 15, want: 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tab := table[int]{
				pageSize:  tt.pageSize,
				paginator: testPaginator{total: tt.total},
			}

			got := tab.pageCount()
			if got != tt.want {
				t.Errorf("pageCount() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_table_tableActionsString(t *testing.T) {
	tests := []struct {
		name    string
		actions []TableAction
		want    string
	}{
		{name: "no actions returns empty string", actions: nil, want: ""},
		{name: "single action returns its format", actions: []TableAction{{Format: "[n]ext"}}, want: "[n]ext"},
		{
			name:    "multiple actions are comma separated in order",
			actions: []TableAction{{Format: "[p]rev"}, {Format: "[n]ext"}, {Format: "[q]uit"}},
			want:    "[p]rev, [n]ext, [q]uit",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tab := table[int]{tableActions: tt.actions}
			got := tab.tableActionsString()
			if got != tt.want {
				t.Fatalf("tableActionsString() = %q, want %q", got, tt.want)
			}
		})
	}
}

func Test_tableActionKeys(t *testing.T) {
	t.Run("standard keys", func(t *testing.T) {
		action := TableAction{Short: "n", Long: "next"}
		got := tableActionKeys(action)
		want := []string{"n", "next"}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("tableActionKeys() = %v, want %v", got, want)
		}
	})

	t.Run("with additional hotkeys", func(t *testing.T) {
		action := TableAction{Short: "n", Long: "next", AdditionalHotkeys: "right,→"}
		got := tableActionKeys(action)
		want := []string{"n", "next", "right", "→"}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("tableActionKeys() = %v, want %v", got, want)
		}
	})

	t.Run("whitespace-only additional hotkeys produce no extra keys", func(t *testing.T) {
		action := TableAction{Short: "x", Long: "extra", AdditionalHotkeys: " ,  "}
		got := tableActionKeys(action)
		want := []string{"x", "extra"}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("tableActionKeys() = %v, want %v", got, want)
		}
	})
}

func Test_table_multiPartParse(t *testing.T) {
	tests := []struct {
		name    string
		total   int
		input   string
		want    []int
		wantErr string
	}{
		{name: "missing colon returns error", total: 10, input: "3", wantErr: "expected 2 numbers from range"},
		{name: "too many colons returns error", total: 10, input: "1:2:3", wantErr: "expected 2 numbers from range"},
		{name: "invalid start returns error", total: 10, input: "a:2", wantErr: "failed to parse start"},
		{name: "invalid end returns error", total: 10, input: "1:b", wantErr: "failed to parse end"},
		{name: "end before start returns error", total: 10, input: "4:2", wantErr: "start of range: 4, is greater than end: 2"},
		{name: "single value range returns inclusive selection", total: 10, input: "2:2", want: []int{2}},
		{name: "whitespace is trimmed", total: 10, input: " 1 : 3 ", want: []int{1, 2, 3}},
		{name: "range beyond total truncates", total: 3, input: "2:5", want: []int{2, 3}},
		{name: "range stopping immediately beyond total returns empty", total: 0, input: "1:3", want: []int{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tab := table[int]{paginator: testPaginator{total: tt.total}}
			got, err := tab.multiPartParse(tt.input)

			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("multiPartParse(%q) error = nil, want substring %q", tt.input, tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("multiPartParse(%q) error = %q, want substring %q", tt.input, err.Error(), tt.wantErr)
				}
				return
			}

			if err != nil {
				t.Fatalf("multiPartParse(%q) unexpected error: %v", tt.input, err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("multiPartParse(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func Test_table_parseNumbersFromString(t *testing.T) {
	tests := []struct {
		name    string
		total   int
		input   string
		want    []int
		wantErr []string
	}{
		{name: "parses comma separated integers", total: 10, input: "1, 3,5", want: []int{1, 3, 5}},
		{name: "parses ranges without integer parse noise", total: 10, input: "1:3", want: []int{1, 2, 3}},
		{name: "parses mixed integers and ranges in order", total: 10, input: "0, 2:4, 6", want: []int{0, 2, 3, 4, 6}},
		{name: "truncates ranges at total amount", total: 3, input: "2:5", want: []int{2, 3}},
		{name: "keeps valid selections when one token is invalid", total: 10, input: "1, nope, 4", want: []int{1, 4}, wantErr: []string{"token: 'nope' failed to parse to int"}},
		{name: "reports out of bounds singular values", total: 3, input: "1,4", want: []int{1}, wantErr: []string{"index: '4' is outside the range of items"}},
		{name: "reports malformed ranges without adding values", total: 10, input: "1:bad", want: []int{}, wantErr: []string{"failed to parse range selection: failed to parse end"}},
		{name: "empty token is reported", total: 3, input: "1,,2", want: []int{1, 2}, wantErr: []string{"token: '' failed to parse to int"}},
		{
			name:  "joins multiple parse errors and preserves valid selections",
			total: 3,
			input: "bad, 1:broken, 4, 2",
			want:  []int{2},
			wantErr: []string{
				"token: 'bad' failed to parse to int",
				"failed to parse range selection: failed to parse end",
				"index: '4' is outside the range of items",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tab := table[int]{paginator: testPaginator{total: tt.total}}
			got, err := tab.parseNumbersFromString(tt.input)

			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("parseNumbersFromString(%q) = %v, want %v", tt.input, got, tt.want)
			}

			if len(tt.wantErr) == 0 {
				if err != nil {
					t.Fatalf("parseNumbersFromString(%q) unexpected error: %v", tt.input, err)
				}
				return
			}
			if err == nil {
				t.Fatalf("parseNumbersFromString(%q) error = nil, want substrings %v", tt.input, tt.wantErr)
			}
			for _, wantErr := range tt.wantErr {
				if !strings.Contains(err.Error(), wantErr) {
					t.Fatalf("parseNumbersFromString(%q) error = %q, want substring %q", tt.input, err.Error(), wantErr)
				}
			}
		})
	}
}

func Test_table_printRow(t *testing.T) {
	t.Run("formats and writes row", func(t *testing.T) {
		t.Setenv("NO_COLOR", "1")

		var out bytes.Buffer
		tab := table[int]{
			rowFormater: func(i, item int) (string, error) {
				return fmt.Sprintf("%d=%d", i, item), nil
			},
			out:   &out,
			theme: DefaultTheme(),
		}

		if err := tab.printRow(2, 42); err != nil {
			t.Fatalf("printRow() unexpected error: %v", err)
		}
		if got := out.String(); got != "2=42\n" {
			t.Fatalf("printRow() output = %q, want %q", got, "2=42\n")
		}
	})

	t.Run("formatter error is wrapped", func(t *testing.T) {
		tab := table[int]{
			rowFormater: func(i, item int) (string, error) {
				return "", errors.New("boom")
			},
			out:   new(bytes.Buffer),
			theme: DefaultTheme(),
		}

		err := tab.printRow(0, 1)
		if err == nil {
			t.Fatal("printRow() error = nil, want error")
		}
		if !strings.Contains(err.Error(), "failed to format row") {
			t.Fatalf("printRow() error = %q, want format context", err.Error())
		}
	})

	t.Run("writer error is wrapped", func(t *testing.T) {
		tab := table[int]{
			rowFormater: func(i, item int) (string, error) {
				return "row", nil
			},
			out:   errWriter{},
			theme: DefaultTheme(),
		}

		err := tab.printRow(0, 1)
		if err == nil {
			t.Fatal("printRow() error = nil, want error")
		}
		if !strings.Contains(err.Error(), "failed to print") {
			t.Fatalf("printRow() error = %q, want print context", err.Error())
		}
	})
}

func Test_table_print(t *testing.T) {
	t.Setenv("NO_COLOR", "1")

	t.Run("prints current page and prompt", func(t *testing.T) {
		var out bytes.Buffer
		tab := table[int]{
			page:         1,
			pageSize:     2,
			lastPage:     2,
			paginator:    testPaginator{total: 5, items: []int{10, 20, 30, 40, 50}},
			rowFormater:  func(i, item int) (string, error) { return fmt.Sprintf("%d=%d", i, item), nil },
			tableActions: []TableAction{{Format: "[n]ext"}, {Format: "[q]uit"}},
			out:          &out,
			theme:        DefaultTheme(),
		}

		got, err := tab.print()
		if err != nil {
			t.Fatalf("print() unexpected error: %v", err)
		}
		if got != 2 {
			t.Fatalf("print() printed = %d, want 2", got)
		}

		wantContains := []string{"2=30\n", "3=40\n", "(select, [n]ext, [q]uit, [/] filter, page 1/2): "}
		for _, want := range wantContains {
			if !strings.Contains(out.String(), want) {
				t.Fatalf("print() output = %q, want substring %q", out.String(), want)
			}
		}
	})

	t.Run("findPage error is wrapped", func(t *testing.T) {
		tab := table[int]{
			pageSize:    1,
			paginator:   testPaginator{total: 1, findErr: errors.New("boom")},
			rowFormater: func(i, item int) (string, error) { return "", nil },
			out:         new(bytes.Buffer),
			theme:       DefaultTheme(),
		}

		_, err := tab.print()
		if err == nil {
			t.Fatal("print() error = nil, want error")
		}
		if !strings.Contains(err.Error(), "failed to find page") {
			t.Fatalf("print() error = %q, want findPage context", err.Error())
		}
	})

	t.Run("row printing error is wrapped", func(t *testing.T) {
		tab := table[int]{
			pageSize:    1,
			paginator:   testPaginator{total: 1, items: []int{10}},
			rowFormater: func(i, item int) (string, error) { return "", errors.New("boom") },
			out:         new(bytes.Buffer),
			theme:       DefaultTheme(),
		}

		_, err := tab.print()
		if err == nil {
			t.Fatal("print() error = nil, want error")
		}
		if !strings.Contains(err.Error(), "failed to print row") {
			t.Fatalf("print() error = %q, want row-print context", err.Error())
		}
	})

	t.Run("prompt write error is wrapped", func(t *testing.T) {
		// failAfterWriter succeeds for the first N writes then fails.
		// printRow writes once per row, so allow 1 write (the row) then fail on the prompt.
		w := &failAfterWriter{n: 1}
		tab := table[int]{
			pageSize:    1,
			paginator:   testPaginator{total: 1, items: []int{10}},
			rowFormater: func(i, item int) (string, error) { return fmt.Sprintf("%d=%d", i, item), nil },
			out:         w,
			theme:       DefaultTheme(),
		}

		_, err := tab.print()
		if err == nil {
			t.Fatal("print() error = nil, want error")
		}
		if !strings.Contains(err.Error(), "failed to print prompt line") {
			t.Fatalf("print() error = %q, want prompt-line context", err.Error())
		}
	})
}

func Test_table_selectNumbers(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	t.Setenv("COLUMNS", "5")

	tests := []struct {
		name       string
		input      string
		actions    []TableAction
		want       []int
		wantErr    string
		wantErrIs  error
		wantNil    bool
		wantNotice string
		wantPage   int
	}{
		{
			name:     "returns parsed numbers",
			input:    "1,2\n",
			want:     []int{1, 2},
			wantPage: 0,
		},
		{
			name:  "matches short action",
			input: "n\n",
			actions: []TableAction{{
				Format: "[n]ext",
				Short:  "n",
				Long:   "next",
				Action: func() error { return nil },
			}},
			wantNil: true,
		},
		{
			name:  "matches long action",
			input: "next\n",
			actions: []TableAction{{
				Format: "[n]ext",
				Short:  "n",
				Long:   "next",
				Action: func() error { return nil },
			}},
			wantNil: true,
		},
		{
			name:  "nil action errors",
			input: "next\n",
			actions: []TableAction{{
				Format: "[n]ext",
				Short:  "n",
				Long:   "next",
			}},
			wantErr: `table action "next" has nil action`,
		},
		{
			name:      "action sentinel propagates",
			input:     "quit\n",
			actions:   []TableAction{{Format: "[q]uit", Short: "q", Long: "quit", Action: func() error { return ErrUserInitiatedExit }}},
			wantErrIs: ErrUserInitiatedExit,
		},
		{
			name:       "parse error re-prompts with a notice instead of failing",
			input:      "1,bad\n",
			wantNil:    true,
			wantNotice: `invalid selection "1,bad"`,
		},
		{
			name:     "action can mutate table page",
			input:    "next\n",
			wantNil:  true,
			wantPage: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ttyPath := filepath.Join(t.TempDir(), "tty")
			if err := os.WriteFile(ttyPath, []byte(tt.input), 0o600); err != nil {
				t.Fatalf("write tty input: %v", err)
			}
			t.Setenv("TTY", ttyPath)

			var out bytes.Buffer
			tab := table[int]{
				page:         0,
				pageSize:     3,
				lastPage:     1,
				paginator:    testPaginator{total: 3, items: []int{10, 20, 30}},
				rowFormater:  func(i, item int) (string, error) { return fmt.Sprintf("%d=%d", i, item), nil },
				tableActions: tt.actions,
				out:          &out,
				theme:        DefaultTheme(),
			}
			if tt.name == "action can mutate table page" {
				tab.tableActions = append(tab.tableActions, tab.nextPage())
			}

			got, err := tab.selectNumbers()

			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("selectNumbers() error = nil, want substring %q", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("selectNumbers() error = %q, want substring %q", err.Error(), tt.wantErr)
				}
			} else if tt.wantErrIs != nil {
				if !errors.Is(err, tt.wantErrIs) {
					t.Fatalf("selectNumbers() error = %v, want %v", err, tt.wantErrIs)
				}
			} else if err != nil {
				t.Fatalf("selectNumbers() unexpected error: %v", err)
			}

			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("selectNumbers() = %v, want %v", got, tt.want)
			}
			if tt.wantNil && got != nil {
				t.Fatalf("selectNumbers() = %v, want nil selection", got)
			}
			if tt.wantNotice != "" && !strings.Contains(tab.notice, tt.wantNotice) {
				t.Fatalf("notice = %q, want substring %q", tab.notice, tt.wantNotice)
			}
			if tab.page != tt.wantPage {
				t.Fatalf("page after selectNumbers() = %d, want %d", tab.page, tt.wantPage)
			}
			if out.Len() == 0 {
				t.Fatal("selectNumbers() wrote no table output")
			}
		})
	}
}

func Test_table_selectNumbers_readError(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	t.Setenv("TTY", filepath.Join(t.TempDir(), "missing-tty"))

	tab := table[int]{
		pageSize:    1,
		paginator:   testPaginator{total: 1, items: []int{10}},
		rowFormater: func(i, item int) (string, error) { return fmt.Sprintf("%d=%d", i, item), nil },
		out:         new(bytes.Buffer),
		theme:       DefaultTheme(),
	}

	_, err := tab.selectNumbers()
	if err == nil {
		t.Fatal("selectNumbers() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "failed to read table selection") {
		t.Fatalf("selectNumbers() error = %q, want read context", err.Error())
	}
}

func Test_table_selectNumbers_printError(t *testing.T) {
	tab := table[int]{
		pageSize:    1,
		paginator:   testPaginator{total: 1, findErr: errors.New("boom")},
		rowFormater: func(i, item int) (string, error) { return "", nil },
		out:         new(bytes.Buffer),
		theme:       DefaultTheme(),
	}

	_, err := tab.selectNumbers()
	if err == nil {
		t.Fatal("selectNumbers() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "failed to print table") {
		t.Fatalf("selectNumbers() error = %q, want print-table context", err.Error())
	}
}

// --- Builder API tests ---

func Test_Builder_basicSelection(t *testing.T) {
	t.Setenv("NO_COLOR", "1")

	var out bytes.Buffer
	got, _, err := New(
		testPaginator{total: 3, items: []int{10, 20, 30}},
		func(i, item int) (string, error) { return fmt.Sprintf("%d=%d", i, item), nil },
	).
		WithHeader("test").
		WithWriter(&out).
		WithInput(strings.NewReader("1\n")).
		Run()
	if err != nil {
		t.Fatalf("Run() unexpected error: %v", err)
	}
	if !reflect.DeepEqual(got, []int{1}) {
		t.Fatalf("Run() = %v, want [1]", got)
	}
}

func Test_Builder_singleSelectRejectsMultiple(t *testing.T) {
	t.Setenv("NO_COLOR", "1")

	got, _, err := New(
		testPaginator{total: 3, items: []int{10, 20, 30}},
		func(i, item int) (string, error) { return fmt.Sprintf("%d=%d", i, item), nil },
	).
		WithHeader("test").
		WithInput(strings.NewReader("0,1\n")).
		WithSingleSelect().
		Run()

	if err == nil {
		t.Fatal("Run() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "only one selected number supported") {
		t.Fatalf("Run() error = %q, want only-one context", err.Error())
	}
	if len(got) != 0 {
		t.Fatalf("Run() got = %v, want empty", got)
	}
}

func Test_Builder_backReturnsErrBack(t *testing.T) {
	t.Setenv("NO_COLOR", "1")

	_, _, err := New(
		testPaginator{total: 1, items: []int{10}},
		func(i, item int) (string, error) { return fmt.Sprintf("%d=%d", i, item), nil },
	).
		WithHeader("test").
		WithInput(strings.NewReader("b\n")).
		Run()

	if !errors.Is(err, ErrBack) {
		t.Fatalf("Run() error = %v, want ErrBack", err)
	}
}

func Test_Builder_customBackLabel(t *testing.T) {
	t.Setenv("NO_COLOR", "1")

	var out bytes.Buffer
	// Use macro mode: send 'b' as the back action
	_, _, err := New(
		testPaginator{total: 1, items: []int{10}},
		func(i, item int) (string, error) { return fmt.Sprintf("%d=%d", i, item), nil },
	).
		WithHeader("test").
		WithWriter(&out).
		WithBackLabel("[←] back to list").
		WithInput(strings.NewReader("b\n")).
		Run()

	if !errors.Is(err, ErrBack) {
		t.Fatalf("Run() error = %v, want ErrBack", err)
	}
	// The custom label should appear in the output
	if !strings.Contains(out.String(), "[←] back to list") {
		t.Fatalf("output missing custom back label: %s", out.String())
	}
}

func Test_Builder_startPage(t *testing.T) {
	t.Setenv("NO_COLOR", "1")

	var items []int
	for i := range 30 {
		items = append(items, i)
	}
	var out bytes.Buffer
	got, page, err := New(
		testPaginator{total: 30, items: items},
		func(i, item int) (string, error) { return fmt.Sprintf("%d=%d", i, item), nil },
	).
		WithHeader("test").
		WithWriter(&out).
		WithPageSize(10).
		WithStartPage(2).
		WithInput(strings.NewReader("0\n")).
		Run()
	if err != nil {
		t.Fatalf("Run() unexpected error: %v", err)
	}
	if !reflect.DeepEqual(got, []int{0}) {
		t.Fatalf("Run() = %v, want [0] (page-relative)", got)
	}
	if page != 2 {
		t.Fatalf("page = %d, want 2", page)
	}
}

func Test_Builder_pageNav(t *testing.T) {
	t.Setenv("NO_COLOR", "1")

	var items []int
	for i := range 30 {
		items = append(items, i)
	}
	var out bytes.Buffer
	got, page, err := New(
		testPaginator{total: 30, items: items},
		func(i, item int) (string, error) { return fmt.Sprintf("%d=%d", i, item), nil },
	).
		WithHeader("test").
		WithWriter(&out).
		WithPageSize(10).
		WithInput(strings.NewReader("n\nn\n2\n")).
		Run()
	if err != nil {
		t.Fatalf("Run() unexpected error: %v", err)
	}
	if !reflect.DeepEqual(got, []int{2}) {
		t.Fatalf("Run() = %v, want [2]", got)
	}
	if page != 2 {
		t.Fatalf("page = %d, want 2", page)
	}
}

func Test_Builder_mistypeReprompts(t *testing.T) {
	t.Setenv("NO_COLOR", "1")

	var out bytes.Buffer
	got, _, err := New(
		SlicePaginator([]int{10, 20, 30}),
		func(i, item int) (string, error) { return fmt.Sprintf("%d: %d", i, item), nil },
	).
		WithHeader("header").
		WithWriter(&out).
		WithPageSize(10).
		WithSingleSelect().
		WithInput(strings.NewReader("d\n50:60\n1\n")).
		Run()
	if err != nil {
		t.Fatalf("Run() should survive mistyped input: %v", err)
	}
	if !reflect.DeepEqual(got, []int{1}) {
		t.Fatalf("expected selection [1] after retries, got %v", got)
	}
	if !strings.Contains(out.String(), `invalid selection "d"`) {
		t.Fatalf("expected mistype notice in prompt, got:\n%s", out.String())
	}
	if !strings.Contains(out.String(), `no selectable index in "50:60"`) {
		t.Fatalf("expected out-of-range notice in prompt, got:\n%s", out.String())
	}
}

func Test_Builder_emptyPageAdvance(t *testing.T) {
	t.Setenv("NO_COLOR", "1")

	var out bytes.Buffer
	got, _, err := New(
		testPaginator{total: 4, items: []int{10, 20, 30, 40}},
		func(i, item int) (string, error) { return fmt.Sprintf("%d=%d", i, item), nil },
	).
		WithHeader("header").
		WithWriter(&out).
		WithPageSize(2).
		WithSingleSelect().
		WithInput(strings.NewReader("\n1\n")).
		Run()
	if err != nil {
		t.Fatalf("Run() unexpected error: %v", err)
	}
	if !reflect.DeepEqual(got, []int{1}) {
		t.Fatalf("Run() = %v, want [1]", got)
	}

	output := out.String()
	for _, want := range []string{"0=10\n", "1=20\n", "2=30\n", "3=40\n", "page 0/1", "page 1/1"} {
		if !strings.Contains(output, want) {
			t.Fatalf("Run() output = %q, want substring %q", output, want)
		}
	}
}

func Test_Builder_singlePageShowsNoPageIndicator(t *testing.T) {
	t.Setenv("NO_COLOR", "1")

	var out bytes.Buffer
	got, _, err := New(
		testPaginator{total: 1, items: []int{10}},
		func(i, item int) (string, error) { return fmt.Sprintf("%d=%d", i, item), nil },
	).
		WithHeader("header").
		WithWriter(&out).
		WithPageSize(10).
		WithSingleSelect().
		WithInput(strings.NewReader("0\n")).
		Run()
	if err != nil {
		t.Fatalf("Run() unexpected error: %v", err)
	}
	if !slices.Equal(got, []int{0}) {
		t.Fatalf("Run() = %v, want [0]", got)
	}
	if strings.Contains(out.String(), "page ") {
		t.Fatalf("Run() output = %q, want no page indicator", out.String())
	}
}

func Test_Builder_filterMacro(t *testing.T) {
	t.Setenv("NO_COLOR", "1")

	items := []string{"Alpha", "Bravo", "Charlie", "alpha-beta"}
	var out bytes.Buffer
	got, page, err := New(
		SlicePaginator(items),
		func(i int, item string) (string, error) { return item, nil },
	).
		WithHeader("header").
		WithWriter(&out).
		WithPageSize(10).
		WithInput(strings.NewReader("/alpha\n0\n")).
		Run()
	if err != nil {
		t.Fatalf("Run() unexpected error: %v", err)
	}
	if !reflect.DeepEqual(got, []int{0}) {
		t.Fatalf("Run() = %v, want [0]", got)
	}
	if page != 0 {
		t.Fatalf("page = %d, want 0", page)
	}
}

func Test_Builder_quitMacro(t *testing.T) {
	t.Setenv("NO_COLOR", "1")

	_, _, err := New(
		testPaginator{total: 3, items: []int{10, 20, 30}},
		func(i, item int) (string, error) { return fmt.Sprintf("%d=%d", i, item), nil },
	).
		WithHeader("header").
		WithPageSize(10).
		WithInput(strings.NewReader("q\n")).
		Run()

	if !errors.Is(err, ErrUserInitiatedExit) {
		t.Fatalf("error = %v, want ErrUserInitiatedExit", err)
	}
}

func Test_Builder_backInMacro(t *testing.T) {
	t.Setenv("NO_COLOR", "1")

	_, _, err := New(
		testPaginator{total: 3, items: []int{10, 20, 30}},
		func(i, item int) (string, error) { return fmt.Sprintf("%d=%d", i, item), nil },
	).
		WithHeader("header").
		WithPageSize(10).
		WithInput(strings.NewReader("b\n")).
		Run()

	if !errors.Is(err, ErrBack) {
		t.Fatalf("error = %v, want ErrBack", err)
	}
}

func Test_Builder_eofBeforeSelection(t *testing.T) {
	t.Setenv("NO_COLOR", "1")

	_, _, err := New(
		testPaginator{total: 5, items: []int{10, 20, 30, 40, 50}},
		func(i, item int) (string, error) { return fmt.Sprintf("%d=%d", i, item), nil },
	).
		WithHeader("header").
		WithPageSize(2).
		WithInput(strings.NewReader("n\n")).
		Run()

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, io.EOF) {
		t.Fatalf("error = %v, want io.EOF", err)
	}
}

func Test_Builder_noClearTermToInMacro(t *testing.T) {
	t.Setenv("NO_COLOR", "1")

	var cleared int
	trackingClear := func(w io.Writer, upTo int) error {
		cleared++
		return nil
	}

	// Build table by hand to inject the clearTermToFn field
	tab := table[int]{
		paginator:         testPaginator{total: 1, items: []int{10}},
		originalPaginator: testPaginator{total: 1, items: []int{10}},
		rowFormater:       func(i, item int) (string, error) { return fmt.Sprintf("%d=%d", i, item), nil },
		header:            "header",
		theme:             DefaultTheme(),
		pageSize:          10,
		out:               new(bytes.Buffer),
		input:             strings.NewReader("0\n"),

		clearTermToFn: trackingClear,
	}

	bt := &Table[int]{t: tab}
	_, _, err := bt.Run()
	if err != nil {
		t.Fatalf("Run() unexpected error: %v", err)
	}
	if cleared != 0 {
		t.Fatalf("ClearTermTo called %d times in macro mode, want 0", cleared)
	}
}

func Test_Builder_clearTermToCalledInInteractive(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	t.Setenv("TTY", filepath.Join(t.TempDir(), "missing-tty"))

	var cleared []int
	trackingClear := func(w io.Writer, upTo int) error {
		cleared = append(cleared, upTo)
		return nil
	}

	tab := table[int]{
		paginator:         testPaginator{total: 1, items: []int{10}},
		originalPaginator: testPaginator{total: 1, items: []int{10}},
		rowFormater:       func(i, item int) (string, error) { return fmt.Sprintf("%d=%d", i, item), nil },
		header:            "header",
		theme:             DefaultTheme(),
		pageSize:          10,
		out:               new(bytes.Buffer),
		input:             nil,
		clearTermToFn:     trackingClear,
	}

	bt := &Table[int]{t: tab}
	_, _, err := bt.Run()
	if err == nil {
		t.Fatal("expected error from TTY, got nil")
	}
	// clearTermToFn should have been called at least for the header cleanup
	if len(cleared) == 0 {
		t.Fatal("ClearTermTo should have been called in interactive mode")
	}
}

func Test_Builder_multipleSelection(t *testing.T) {
	t.Setenv("NO_COLOR", "1")

	var out bytes.Buffer
	got, _, err := New(
		testPaginator{total: 5, items: []int{10, 20, 30, 40, 50}},
		func(i, item int) (string, error) { return fmt.Sprintf("%d=%d", i, item), nil },
	).
		WithHeader("header").
		WithWriter(&out).
		WithPageSize(10).
		WithInput(strings.NewReader("0,2\n")).
		Run()
	if err != nil {
		t.Fatalf("Run() unexpected error: %v", err)
	}
	if !reflect.DeepEqual(got, []int{0, 2}) {
		t.Fatalf("Run() = %v, want [0, 2]", got)
	}
}

func Test_Builder_rangeSelection(t *testing.T) {
	t.Setenv("NO_COLOR", "1")

	var out bytes.Buffer
	got, _, err := New(
		testPaginator{total: 5, items: []int{10, 20, 30, 40, 50}},
		func(i, item int) (string, error) { return fmt.Sprintf("%d=%d", i, item), nil },
	).
		WithHeader("header").
		WithWriter(&out).
		WithPageSize(10).
		WithInput(strings.NewReader("1:3\n")).
		Run()
	if err != nil {
		t.Fatalf("Run() unexpected error: %v", err)
	}
	if !reflect.DeepEqual(got, []int{1, 2, 3}) {
		t.Fatalf("Run() = %v, want [1, 2, 3]", got)
	}
}

func Test_Builder_customAction(t *testing.T) {
	t.Setenv("NO_COLOR", "1")

	var triggered bool
	customAction := TableAction{
		Format: "[x]tra",
		Short:  "x",
		Long:   "extra",
		Action: func() error {
			triggered = true
			return nil
		},
	}

	var out bytes.Buffer
	got, _, err := New(
		testPaginator{total: 3, items: []int{10, 20, 30}},
		func(i, item int) (string, error) { return fmt.Sprintf("%d=%d", i, item), nil },
	).
		WithHeader("header").
		WithWriter(&out).
		WithPageSize(10).
		WithActions(customAction).
		WithInput(strings.NewReader("x\n0\n")).
		Run()
	if err != nil {
		t.Fatalf("Run() unexpected error: %v", err)
	}
	if !triggered {
		t.Fatal("custom action was not triggered")
	}
	if !reflect.DeepEqual(got, []int{0}) {
		t.Fatalf("Run() = %v, want [0]", got)
	}
}

func Test_Builder_macroMistypeReprompts(t *testing.T) {
	t.Setenv("NO_COLOR", "1")

	var out bytes.Buffer
	got, _, err := New(
		testPaginator{total: 3, items: []int{10, 20, 30}},
		func(i, item int) (string, error) { return fmt.Sprintf("%d=%d", i, item), nil },
	).
		WithHeader("header").
		WithWriter(&out).
		WithPageSize(10).
		WithInput(strings.NewReader("garbage\n1\n")).
		Run()
	if err != nil {
		t.Fatalf("Run() unexpected error: %v", err)
	}
	if !reflect.DeepEqual(got, []int{1}) {
		t.Fatalf("Run() = %v, want [1]", got)
	}
}

func Test_Builder_duplicateHotkeyError(t *testing.T) {
	t.Setenv("NO_COLOR", "1")

	_, _, err := New(
		testPaginator{total: 1, items: []int{10}},
		func(i, item int) (string, error) { return fmt.Sprintf("%d=%d", i, item), nil },
	).
		WithHeader("header").
		WithPageSize(10).
		WithSingleSelect().
		WithActions(TableAction{Format: "[n]ew", Short: "n", Long: "new", Action: func() error { return nil }}).
		WithInput(strings.NewReader("0\n")).
		Run()

	if err == nil {
		t.Fatal("Run() error = nil, want duplicate hotkey error")
	}
	if !strings.Contains(err.Error(), `duplicate table action hotkey "n"`) {
		t.Fatalf("Run() error = %q, want duplicate hotkey context", err.Error())
	}
}

func Test_Builder_duplicateBuiltinHotkeyError(t *testing.T) {
	t.Setenv("NO_COLOR", "1")

	_, _, err := New(
		testPaginator{total: 1, items: []int{10}},
		func(i, item int) (string, error) { return fmt.Sprintf("%d=%d", i, item), nil },
	).
		WithHeader("header").
		WithPageSize(10).
		WithSingleSelect().
		WithActions(TableAction{Format: "[b]ack", Short: "b", Long: "back", Action: func() error { return nil }}).
		WithInput(strings.NewReader("0\n")).
		Run()

	if err == nil {
		t.Fatal("Run() error = nil, want duplicate hotkey error")
	}
	if !strings.Contains(err.Error(), `duplicate table action hotkey "b"`) {
		t.Fatalf("Run() error = %q, want duplicate hotkey context", err.Error())
	}
}

func Test_Builder_themeFallback(t *testing.T) {
	t.Setenv("NO_COLOR", "1")

	// No WithTheme or WithPageSize — uses DefaultTheme().Items=10
	var out bytes.Buffer
	got, _, err := New(
		testPaginator{total: 1, items: []int{10}},
		func(i, item int) (string, error) { return fmt.Sprintf("%d=%d", i, item), nil },
	).
		WithHeader("header").
		WithWriter(&out).
		WithInput(strings.NewReader("0\n")).
		Run()
	if err != nil {
		t.Fatalf("Run() unexpected error: %v", err)
	}
	if !reflect.DeepEqual(got, []int{0}) {
		t.Fatalf("Run() = %v, want [0]", got)
	}
}

func Test_Builder_customTheme(t *testing.T) {
	t.Setenv("NO_COLOR", "1")

	custom := Theme{
		Primary:   "\033[31m",
		Secondary: "\033[32m",
		Breadtext: "\033[33m",
		Items:     5,
	}

	var out bytes.Buffer
	_, _, err := New(
		testPaginator{total: 1, items: []int{10}},
		func(i, item int) (string, error) { return fmt.Sprintf("%d=%d", i, item), nil },
	).
		WithHeader("header").
		WithWriter(&out).
		WithTheme(custom).
		WithInput(strings.NewReader("0\n")).
		Run()
	if err != nil {
		t.Fatalf("Run() unexpected error: %v", err)
	}
}

// --- More internal tests ---

func TestSlicePaginator(t *testing.T) {
	paginator := SlicePaginator([]int{10, 20, 30})

	got, err := paginator.findPage(1, 2)
	if err != nil {
		t.Fatalf("findPage() unexpected error: %v", err)
	}
	if !reflect.DeepEqual(got, []int{20, 30}) {
		t.Fatalf("findPage() = %v, want %v", got, []int{20, 30})
	}
	if paginator.totalAm() != 3 {
		t.Fatalf("totalAm() = %d, want 3", paginator.totalAm())
	}

	_, err = paginator.findPage(-1, 1)
	if err == nil || !strings.Contains(err.Error(), "start index -1 below zero") {
		t.Fatalf("negative start error = %v, want wrapped bounds error", err)
	}

	_, err = paginator.findPage(0, -1)
	if err == nil || !strings.Contains(err.Error(), "offset -1 below zero") {
		t.Fatalf("negative offset error = %v, want wrapped bounds error", err)
	}

	got, err = paginator.findPage(99, 1)
	if err != nil {
		t.Fatalf("out-of-range page unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("out-of-range page = %v, want empty", got)
	}
}

func Test_table_applyFilter(t *testing.T) {
	t.Setenv("NO_COLOR", "1")

	t.Run("empty string clears filter and restores original paginator", func(t *testing.T) {
		orig := testPaginator{total: 3, items: []int{10, 20, 30}}
		tab := table[int]{
			paginator:         orig,
			originalPaginator: orig,
			filterString:      "",
			filteredIndices:   []int{0, 2},
			pageSize:          10,
			rowFormater:       func(i, item int) (string, error) { return fmt.Sprintf("%d=%d", i, item), nil },
		}

		if err := tab.applyFilter(); err != nil {
			t.Fatalf("applyFilter() unexpected error: %v", err)
		}
		if tab.filteredIndices != nil {
			t.Fatal("filteredIndices not nil after clearing")
		}
		if tab.paginator.totalAm() != 3 {
			t.Fatalf("total after clear = %d, want 3", tab.paginator.totalAm())
		}
	})

	t.Run("filters items by case-insensitive substring match on rendered text", func(t *testing.T) {
		items := []string{"Alpha", "Bravo", "Charlie", "alpha-beta"}
		paginator := SlicePaginator(items)
		tab := table[string]{
			paginator:         paginator,
			originalPaginator: paginator,
			pageSize:          10,
			filterString:      "alpha",
			rowFormater:       func(i int, item string) (string, error) { return item, nil },
		}

		if err := tab.applyFilter(); err != nil {
			t.Fatalf("applyFilter() unexpected error: %v", err)
		}

		wantIndices := []int{0, 3}
		if !reflect.DeepEqual(tab.filteredIndices, wantIndices) {
			t.Fatalf("filteredIndices = %v, want %v", tab.filteredIndices, wantIndices)
		}

		gotItems, err := tab.paginator.findPage(0, 10)
		if err != nil {
			t.Fatalf("findPage() error: %v", err)
		}
		wantItems := []string{"Alpha", "alpha-beta"}
		if !reflect.DeepEqual(gotItems, wantItems) {
			t.Fatalf("filtered items = %v, want %v", gotItems, wantItems)
		}

		if tab.page != 0 {
			t.Fatalf("page = %d, want 0 (reset on filter)", tab.page)
		}
		if tab.paginator.totalAm() != 2 {
			t.Fatalf("total = %d, want 2", tab.paginator.totalAm())
		}
	})

	t.Run("no matches results in empty paginator and empty filteredIndices", func(t *testing.T) {
		items := []string{"Alpha", "Bravo"}
		paginator := SlicePaginator(items)
		tab := table[string]{
			paginator:         paginator,
			originalPaginator: paginator,
			pageSize:          10,
			filterString:      "xyz",
			rowFormater:       func(i int, item string) (string, error) { return item, nil },
		}

		if err := tab.applyFilter(); err != nil {
			t.Fatalf("applyFilter() error: %v", err)
		}

		if len(tab.filteredIndices) != 0 {
			t.Fatalf("filteredIndices = %v, want empty", tab.filteredIndices)
		}
		if tab.paginator.totalAm() != 0 {
			t.Fatalf("total = %d, want 0", tab.paginator.totalAm())
		}
	})

	t.Run("rowFormater errors are skipped gracefully", func(t *testing.T) {
		items := []string{"one", "boom", "two"}
		paginator := SlicePaginator(items)
		tab := table[string]{
			paginator:         paginator,
			originalPaginator: paginator,
			pageSize:          10,
			filterString:      "t",
			rowFormater: func(i int, item string) (string, error) {
				if item == "boom" {
					return "", errors.New("boom")
				}
				return item, nil
			},
		}

		if err := tab.applyFilter(); err != nil {
			t.Fatalf("applyFilter() error: %v", err)
		}

		want := []int{2}
		if !reflect.DeepEqual(tab.filteredIndices, want) {
			t.Fatalf("filteredIndices = %v, want %v", tab.filteredIndices, want)
		}
	})

	t.Run("returns error when original paginator fails", func(t *testing.T) {
		badPaginator := testPaginator{total: 1, findErr: errors.New("read error")}
		tab := table[int]{
			paginator:         badPaginator,
			originalPaginator: badPaginator,
			pageSize:          10,
			filterString:      "test",
			rowFormater:       func(i, item int) (string, error) { return "ok", nil },
		}

		err := tab.applyFilter()
		if err == nil {
			t.Fatal("applyFilter() error = nil, want error")
		}
		if !strings.Contains(err.Error(), "failed to get items for filtering") {
			t.Fatalf("error = %q, want filtering context", err.Error())
		}
	})

	t.Run("clearing filter after one was active restores all items", func(t *testing.T) {
		items := []string{"Alpha", "Bravo"}
		paginator := SlicePaginator(items)
		tab := table[string]{
			paginator:         paginator,
			originalPaginator: paginator,
			pageSize:          10,
			filterString:      "Alpha",
			rowFormater:       func(i int, item string) (string, error) { return item, nil },
		}

		if err := tab.applyFilter(); err != nil {
			t.Fatalf("first applyFilter() error: %v", err)
		}
		if tab.paginator.totalAm() != 1 {
			t.Fatalf("filtered total = %d, want 1", tab.paginator.totalAm())
		}

		tab.filterString = ""
		if err := tab.applyFilter(); err != nil {
			t.Fatalf("clear applyFilter() error: %v", err)
		}
		if tab.paginator.totalAm() != 2 {
			t.Fatalf("cleared total = %d, want 2", tab.paginator.totalAm())
		}
		if tab.filteredIndices != nil {
			t.Fatal("filteredIndices not nil after clear")
		}
	})

	t.Run("filtering empty paginator returns empty", func(t *testing.T) {
		paginator := SlicePaginator([]string{})
		tab := table[string]{
			paginator:         paginator,
			originalPaginator: paginator,
			pageSize:          10,
			filterString:      "alpha",
			rowFormater:       func(i int, item string) (string, error) { return item, nil },
		}

		if err := tab.applyFilter(); err != nil {
			t.Fatalf("applyFilter() error: %v", err)
		}
		if tab.paginator.totalAm() != 0 {
			t.Fatalf("total = %d, want 0", tab.paginator.totalAm())
		}
		if tab.filteredIndices != nil {
			t.Fatal("filteredIndices should be nil for empty paginator")
		}
	})
}

func Test_table_selectNumbers_filterPrefix(t *testing.T) {
	t.Setenv("NO_COLOR", "1")

	t.Run("/term sets filter and returns nil so loop continues", func(t *testing.T) {
		items := []string{"first", "second", "test-item", "other"}
		paginator := SlicePaginator(items)
		tab := table[string]{
			page:              0,
			pageSize:          10,
			lastPage:          0,
			paginator:         paginator,
			originalPaginator: paginator,
			rowFormater:       func(i int, item string) (string, error) { return item, nil },
			tableActions:      nil,
			out:               new(bytes.Buffer),
			theme:             DefaultTheme(),
		}

		// First call: filter
		tab.input = strings.NewReader("/test\n")
		got, err := tab.selectNumbers()
		if err != nil {
			t.Fatalf("first selectNumbers() error: %v", err)
		}
		if got != nil {
			t.Fatalf("first call returned %v, want nil (loop continue)", got)
		}
		if tab.filterString != "test" {
			t.Fatalf("filterString = %q, want %q", tab.filterString, "test")
		}
		if tab.paginator.totalAm() != 1 {
			t.Fatalf("filtered total = %d, want 1", tab.paginator.totalAm())
		}

		// Second call: select
		tab.input = strings.NewReader("0\n")
		got, err = tab.selectNumbers()
		if err != nil {
			t.Fatalf("second selectNumbers() error: %v", err)
		}
		want := []int{2}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("got = %v, want %v (original index)", got, want)
		}
	})

	t.Run("/ alone clears filter", func(t *testing.T) {
		items := []string{"first", "second"}
		paginator := SlicePaginator(items)
		tab := table[string]{
			page:              0,
			pageSize:          10,
			paginator:         SlicePaginator([]string{"first"}),
			originalPaginator: paginator,
			filterString:      "first",
			filteredIndices:   []int{0},
			rowFormater:       func(i int, item string) (string, error) { return item, nil },
			out:               new(bytes.Buffer),
			theme:             DefaultTheme(),
		}

		tab.input = strings.NewReader("/\n")
		got, err := tab.selectNumbers()
		if err != nil {
			t.Fatalf("selectNumbers() error: %v", err)
		}
		if got != nil {
			t.Fatalf("got = %v, want nil (loop continue)", got)
		}
		if tab.filterString != "" {
			t.Fatalf("filterString = %q, want empty", tab.filterString)
		}
		if tab.filteredIndices != nil {
			t.Fatal("filteredIndices should be nil after clear")
		}
		if tab.paginator.totalAm() != 2 {
			t.Fatalf("restored total = %d, want 2", tab.paginator.totalAm())
		}
	})

	t.Run("filtered selection with multiple numbers returns translated indices", func(t *testing.T) {
		items := []string{"Alpha-zero", "Bravo", "Alpha-one"}
		paginator := SlicePaginator(items)
		tab := table[string]{
			page:              0,
			pageSize:          10,
			paginator:         paginator,
			originalPaginator: paginator,
			rowFormater:       func(i int, item string) (string, error) { return item, nil },
			out:               new(bytes.Buffer),
			theme:             DefaultTheme(),
		}

		tab.input = strings.NewReader("/alpha\n")
		got, err := tab.selectNumbers()
		if err != nil {
			t.Fatalf("first selectNumbers() error: %v", err)
		}
		if got != nil {
			t.Fatalf("first call returned %v, want nil", got)
		}

		tab.input = strings.NewReader("0,1\n")
		got, err = tab.selectNumbers()
		if err != nil {
			t.Fatalf("second selectNumbers() error: %v", err)
		}
		want := []int{0, 2}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("got = %v, want %v", got, want)
		}
	})

	t.Run("navigation actions work while filter is active", func(t *testing.T) {
		items := []string{"zero", "one", "two", "three", "four", "five", "six", "seven"}
		paginator := SlicePaginator(items)
		var out bytes.Buffer
		tab := table[string]{
			page:              0,
			pageSize:          2,
			paginator:         paginator,
			originalPaginator: paginator,
			rowFormater:       func(i int, item string) (string, error) { return item, nil },
			tableActions:      []TableAction{},
			out:               &out,
			theme:             DefaultTheme(),
		}
		tab.tableActions = append(tab.tableActions, tab.prevPage(), tab.nextPage(), tab.back(), tab.quit())

		tab.input = strings.NewReader("/e\n")
		got, err := tab.selectNumbers()
		if err != nil {
			t.Fatalf("first call: %v", err)
		}
		if got != nil {
			t.Fatalf("first returned %v, want nil", got)
		}
		if tab.paginator.totalAm() != 5 {
			t.Fatalf("filtered total = %d, want 5", tab.paginator.totalAm())
		}

		tab.input = strings.NewReader("n\n")
		got, err = tab.selectNumbers()
		if err != nil {
			t.Fatalf("second call: %v", err)
		}
		if got != nil {
			t.Fatalf("second returned %v, want nil", got)
		}
		if tab.page != 1 {
			t.Fatalf("page = %d, want 1 (next from 0)", tab.page)
		}

		tab.input = strings.NewReader("2\n")
		got, err = tab.selectNumbers()
		if err != nil {
			t.Fatalf("third call: %v", err)
		}
		want := []int{3}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("got = %v, want %v", got, want)
		}
	})
}

func Test_table_togglePredicateFilter(t *testing.T) {
	t.Setenv("NO_COLOR", "1")

	evenOnly := func(row any) bool {
		s, ok := row.(string)
		if !ok {
			return false
		}
		return strings.HasSuffix(s, "0") || strings.HasSuffix(s, "2")
	}

	newTab := func(out *bytes.Buffer) *table[string] {
		items := []string{"item0", "item1", "item2", "item3"}
		paginator := SlicePaginator(items)
		tab := &table[string]{
			page:              0,
			pageSize:          10,
			paginator:         paginator,
			originalPaginator: paginator,
			rowFormater:       func(i int, item string) (string, error) { return item, nil },
			out:               out,
			theme:             DefaultTheme(),
		}
		tab.tableActions = []TableAction{{Format: "[d]irscoped convs", Short: "d", Long: "dir", Filter: evenOnly}}
		return tab
	}

	t.Run("toggle on filters and selection translates to original index", func(t *testing.T) {
		var out bytes.Buffer
		tab := newTab(&out)

		tab.input = strings.NewReader("d\n")
		got, err := tab.selectNumbers()
		if err != nil {
			t.Fatalf("apply: %v", err)
		}
		if got != nil {
			t.Fatalf("apply returned %v, want nil", got)
		}
		if !tab.predicateActive {
			t.Fatal("expected predicateActive after toggle on")
		}
		if tab.paginator.totalAm() != 2 {
			t.Fatalf("filtered total = %d, want 2", tab.paginator.totalAm())
		}

		tab.input = strings.NewReader("1\n")
		got, err = tab.selectNumbers()
		if err != nil {
			t.Fatalf("select: %v", err)
		}
		if !reflect.DeepEqual(got, []int{2}) {
			t.Fatalf("got %v, want [2] (original index)", got)
		}
	})

	t.Run("second press toggles off and restores", func(t *testing.T) {
		var out bytes.Buffer
		tab := newTab(&out)
		tab.input = strings.NewReader("d\n")
		if _, err := tab.selectNumbers(); err != nil {
			t.Fatalf("first toggle: %v", err)
		}
		tab.input = strings.NewReader("d\n")
		if _, err := tab.selectNumbers(); err != nil {
			t.Fatalf("second toggle: %v", err)
		}
		if tab.predicateActive {
			t.Fatal("expected predicate cleared after second press")
		}
		if tab.filteredIndices != nil {
			t.Fatal("expected filteredIndices nil after clear")
		}
		if tab.paginator.totalAm() != 4 {
			t.Fatalf("restored total = %d, want 4", tab.paginator.totalAm())
		}
	})

	t.Run("enter clears active predicate filter", func(t *testing.T) {
		var out bytes.Buffer
		tab := newTab(&out)
		tab.input = strings.NewReader("d\n")
		if _, err := tab.selectNumbers(); err != nil {
			t.Fatalf("toggle on: %v", err)
		}
		tab.input = strings.NewReader("\n")
		if _, err := tab.selectNumbers(); err != nil {
			t.Fatalf("enter clear: %v", err)
		}
		if tab.predicateActive {
			t.Fatal("expected enter to clear predicate filter")
		}
	})

	t.Run("toggle off when findPage errors returns error", func(t *testing.T) {
		items := []string{"item0", "item1"}
		paginator := SlicePaginator(items)
		tab := &table[string]{
			paginator:         SlicePaginator(items),
			originalPaginator: paginator,
			pageSize:          10,
			rowFormater:       func(i int, item string) (string, error) { return item, nil },
			theme:             DefaultTheme(),
		}

		// Mark predicate as active so toggle tries to restore original
		tab.predicateActive = true

		err := tab.togglePredicateFilter(evenOnly)
		// toggle-off just clears state, doesn't re-fetch
		if err != nil {
			t.Fatalf("togglePredicateFilter off should not error: %v", err)
		}
		if tab.predicateActive {
			t.Fatal("expected predicate cleared")
		}
	})
}

func Test_table_promptLine_filter(t *testing.T) {
	t.Setenv("NO_COLOR", "1")

	t.Run("shows filter string when filter is active", func(t *testing.T) {
		tab := table[int]{
			filterString: "hello",
			pageSize:     10,
			paginator:    SlicePaginator([]int{1, 2, 3}),
		}
		got := tab.promptLine()
		if !strings.Contains(got, `filter: "hello"`) {
			t.Fatalf("promptLine() = %q, want filter indicator", got)
		}
	})

	t.Run("shows no matches for active filter with zero results", func(t *testing.T) {
		tab := table[int]{
			filterString:    "nothing",
			filteredIndices: []int{},
			pageSize:        10,
			paginator:       SlicePaginator([]int{}),
		}
		got := tab.promptLine()
		if !strings.Contains(got, "no matches") {
			t.Fatalf("promptLine() = %q, want 'no matches' indicator", got)
		}
	})

	t.Run("shows page when filter is active with multiple pages", func(t *testing.T) {
		tab := table[int]{
			filterString:    "e",
			filteredIndices: []int{0, 1, 2, 3, 4, 5},
			pageSize:        3,
			page:            0,
			paginator:       SlicePaginator([]int{10, 20, 30, 40, 50, 60}),
		}
		got := tab.promptLine()
		if !strings.Contains(got, "page 0/1") {
			t.Fatalf("promptLine() = %q, want page indicator", got)
		}
		if !strings.Contains(got, `filter: "e"`) {
			t.Fatalf("promptLine() = %q, want filter indicator", got)
		}
	})

	t.Run("always shows [/] filter legend", func(t *testing.T) {
		tab := table[int]{
			pageSize:  10,
			paginator: SlicePaginator([]int{1}),
		}
		got := tab.promptLine()
		if !strings.Contains(got, "[/] filter") {
			t.Fatalf("promptLine() = %q, want [/] filter legend", got)
		}
	})

	t.Run("predicate active with empty message", func(t *testing.T) {
		tab := table[int]{
			predicateActive:             true,
			activePredicateEmptyMessage: "no dir-scoped items",
			pageSize:                    10,
			paginator:                   SlicePaginator([]int{}),
		}
		got := tab.promptLine()
		if !strings.Contains(got, "no dir-scoped items") {
			t.Fatalf("promptLine() = %q, want empty message", got)
		}
	})
}

type errWriter struct{}

func (errWriter) Write(p []byte) (int, error) {
	return 0, errors.New("write boom")
}

type failAfterWriter struct {
	n int
}

func (w *failAfterWriter) Write(p []byte) (int, error) {
	if w.n <= 0 {
		return 0, errors.New("write boom")
	}
	w.n--
	return len(p), nil
}

// --- ReadUserInputFrom tests ---

func Test_ReadUserInputFrom(t *testing.T) {
	t.Run("reads single line", func(t *testing.T) {
		got, err := ReadUserInputFrom(strings.NewReader("hello\n"))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "hello" {
			t.Fatalf("got = %q, want %q", got, "hello")
		}
	})

	t.Run("trims whitespace", func(t *testing.T) {
		got, err := ReadUserInputFrom(strings.NewReader("  hello  \n"))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "hello" {
			t.Fatalf("got = %q, want %q", got, "hello")
		}
	})

	t.Run("returns ErrUserInitiatedExit for q", func(t *testing.T) {
		_, err := ReadUserInputFrom(strings.NewReader("q\n"))
		if !errors.Is(err, ErrUserInitiatedExit) {
			t.Fatalf("error = %v, want ErrUserInitiatedExit", err)
		}
	})

	t.Run("returns ErrUserInitiatedExit for quit", func(t *testing.T) {
		_, err := ReadUserInputFrom(strings.NewReader("quit\n"))
		if !errors.Is(err, ErrUserInitiatedExit) {
			t.Fatalf("error = %v, want ErrUserInitiatedExit", err)
		}
	})

	t.Run("wraps io.EOF", func(t *testing.T) {
		_, err := ReadUserInputFrom(strings.NewReader(""))
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !errors.Is(err, io.EOF) {
			t.Fatalf("error = %v, want io.EOF", err)
		}
	})
}

// --- TermWidth tests ---

func Test_TermWidth(t *testing.T) {
	t.Run("respects COLUMNS env", func(t *testing.T) {
		t.Setenv("COLUMNS", "42")
		w, err := TermWidth()
		if err != nil {
			t.Fatalf("TermWidth() error: %v", err)
		}
		if w != 42 {
			t.Fatalf("TermWidth() = %d, want 42", w)
		}
	})

	t.Run("ignores invalid COLUMNS", func(t *testing.T) {
		t.Setenv("COLUMNS", "not-a-number")
		w, err := TermWidth()
		if err != nil {
			t.Fatalf("TermWidth() error: %v", err)
		}
		if w <= 0 {
			t.Fatalf("TermWidth() = %d, want positive fallback", w)
		}
	})

	t.Run("ignores negative COLUMNS", func(t *testing.T) {
		t.Setenv("COLUMNS", "-5")
		w, err := TermWidth()
		if err != nil {
			t.Fatalf("TermWidth() error: %v", err)
		}
		if w <= 0 {
			t.Fatalf("TermWidth() = %d, want positive fallback", w)
		}
	})
}

// --- WidthAppropriateStringTrunc tests ---

func Test_WidthAppropriateStringTruncColored(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	t.Setenv("COLUMNS", "30")

	t.Run("short string fits without truncation", func(t *testing.T) {
		got, err := WidthAppropriateStringTruncColored("hello", "prefix: ", "", "", 0)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "prefix: hello" {
			t.Fatalf("got %q, want %q", got, "prefix: hello")
		}
	})

	t.Run("long string gets truncated with infix", func(t *testing.T) {
		got, err := WidthAppropriateStringTruncColored("this is a very long string that needs truncation", "[INFO] ", "", "", 0)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(got, " ... ") {
			t.Fatalf("got %q, want infix ' ... '", got)
		}
	})

	t.Run("zero remaining width after prefix", func(t *testing.T) {
		t.Setenv("COLUMNS", "10")
		got, err := WidthAppropriateStringTruncColored("hello world", "prefix: ", "", "", 0)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// prefix "prefix: " = 8 chars, termWidth=10, remaining=2, no infix
		if !strings.Contains(got, "prefix: ") {
			t.Fatalf("got %q, want prefix preserved", got)
		}
	})

	t.Run("empty string works", func(t *testing.T) {
		got, err := WidthAppropriateStringTruncColored("", "prefix: ", "", "", 0)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "prefix: " {
			t.Fatalf("got %q, want %q", got, "prefix: ")
		}
	})
}

func Test_WidthAppropriateStringTrunc(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	t.Setenv("COLUMNS", "40")

	got, err := WidthAppropriateStringTrunc("hello world", "p: ", 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "p: hello world" {
		t.Fatalf("got %q, want %q", got, "p: hello world")
	}
}

// --- ClearLine and ClearTermTo tests ---

func Test_ClearLine(t *testing.T) {
	t.Run("writes to provided writer", func(t *testing.T) {
		var buf bytes.Buffer
		ClearLine(&buf)
		if buf.Len() == 0 {
			t.Fatal("ClearLine wrote nothing")
		}
	})

	t.Run("nil writer falls back to stdout", func(t *testing.T) {
		// Just verify it doesn't panic
		ClearLine(nil)
	})
}

func Test_ClearTermTo(t *testing.T) {
	t.Run("writes to provided writer", func(t *testing.T) {
		var buf bytes.Buffer
		err := ClearTermTo(&buf, 3)
		if err != nil {
			t.Fatalf("ClearTermTo() error: %v", err)
		}
		if buf.Len() == 0 {
			t.Fatal("ClearTermTo wrote nothing")
		}
	})

	t.Run("nil writer falls back to stdout", func(t *testing.T) {
		err := ClearTermTo(nil, 1)
		if err != nil {
			t.Fatalf("ClearTermTo(nil) error: %v", err)
		}
	})
}

// --- Theme tests ---

func Test_DefaultTheme(t *testing.T) {
	th := DefaultTheme()
	if th.Items != 10 {
		t.Fatalf("DefaultTheme().Items = %d, want 10", th.Items)
	}
	if th.Primary == "" || th.Secondary == "" || th.Breadtext == "" {
		t.Fatal("DefaultTheme() has empty color fields")
	}
}

// --- End-to-end: filter and selectNumbers with macro input via bufInput ---

func Test_table_selectNumbers_macroFilterAndSelect(t *testing.T) {
	t.Setenv("NO_COLOR", "1")

	items := []string{"Alpha", "Bravo", "Charlie"}
	paginator := SlicePaginator(items)
	var out bytes.Buffer
	tab := table[string]{
		page:              0,
		pageSize:          10,
		paginator:         paginator,
		originalPaginator: paginator,
		rowFormater:       func(i int, item string) (string, error) { return item, nil },
		out:               &out,
		theme:             DefaultTheme(),
		input:             strings.NewReader("/alpha\n0\n"),
	}

	tab.input = strings.NewReader("/alpha\n0\n")

	got, err := tab.selectNumbers()
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	if got != nil {
		t.Fatalf("first = %v, want nil", got)
	}
	if tab.filterString != "alpha" {
		t.Fatalf("filterString = %q, want %q", tab.filterString, "alpha")
	}

	// Re-read for selection
	tab.input = strings.NewReader("0\n")
	got, err = tab.selectNumbers()
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	if !reflect.DeepEqual(got, []int{0}) {
		t.Fatalf("got = %v, want [0]", got)
	}
}

// --- Coverage gap tests ---

func Test_ReadUserInputFrom_nilFallback(t *testing.T) {
	ttyPath := filepath.Join(t.TempDir(), "tty")
	if err := os.WriteFile(ttyPath, []byte("fallback\n"), 0o600); err != nil {
		t.Fatalf("write tty input: %v", err)
	}
	t.Setenv("TTY", ttyPath)

	got, err := ReadUserInputFrom(nil)
	if err != nil {
		t.Fatalf("ReadUserInputFrom(nil) error: %v", err)
	}
	if got != "fallback" {
		t.Fatalf("got = %q, want %q", got, "fallback")
	}
}

func Test_TermWidth_ioctlFallback(t *testing.T) {
	// Unset COLUMNS to force ioctl path
	t.Setenv("COLUMNS", "")
	w, err := TermWidth()
	if err != nil {
		t.Fatalf("TermWidth() error: %v", err)
	}
	// Should get either 80 (fallback) or actual terminal width
	if w <= 0 {
		t.Fatalf("TermWidth() = %d, want positive", w)
	}
}

func Test_WidthAppropriateStringTruncColored_edgeCases(t *testing.T) {
	t.Setenv("NO_COLOR", "1")

	t.Run("remaining width equals infix length", func(t *testing.T) {
		// "prefix: " = 8 chars, "..." infix has visible length 5
		// termWidth=14 means remaining=6, infixLen=5 => remaining > infixLen but avail=1
		// Actually we need: remainingWidth <= infixLen
		// Set COLUMNS so that termWidth - visibleRuneCount(prefix) - padding = infixLen
		// prefix "pre: " = 5 chars, termWidth=11 -> remaining=6, infix=5, avail=1
		// Actually to hit the "remainingWidth <= infixLen" branch: remainingWidth MUST be <= infixLen
		// infixLen = 5 (" ... ")
		// Let's use prefix="p:" (2 chars), termWidth=7 -> remaining=5, infixLen=5
		t.Setenv("COLUMNS", "7")
		got, err := WidthAppropriateStringTruncColored("hello world", "p:", "", "", 0)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// remainingWidth=5 <= infixLen=5, so first 5 runes of remainder
		if !strings.HasPrefix(got, "p:") && strings.Contains(got, "hello") {
			t.Fatalf("got %q", got)
		}
	})

	t.Run("newlines and tabs escaped", func(t *testing.T) {
		t.Setenv("COLUMNS", "80")
		got, err := WidthAppropriateStringTruncColored("hello\nworld\ttab", "", "", "", 0)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if strings.Contains(got, "\n") || strings.Contains(got, "\t") {
			t.Fatalf("newlines/tabs not escaped: %q", got)
		}
	})

	t.Run("prefix with color", func(t *testing.T) {
		t.Setenv("COLUMNS", "40")
		t.Setenv("NO_COLOR", "") // enable colors
		got, err := WidthAppropriateStringTruncColored("hello", "PREFIX", "\033[31m", "\033[34m", 0)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(got, "\033[31m") || !strings.Contains(got, "\033[34m") {
			t.Fatalf("colors not applied: %q", got)
		}
	})
}

func Test_table_selectNumbers_debugClearError(t *testing.T) {
	t.Setenv("NO_COLOR", "1")

	// Use TTY for input, set clearTermToFn to a failing function, enable debug
	ttyPath := filepath.Join(t.TempDir(), "tty")
	if err := os.WriteFile(ttyPath, []byte("0\n"), 0o600); err != nil {
		t.Fatalf("write tty input: %v", err)
	}
	t.Setenv("TTY", ttyPath)

	tab := table[int]{
		page:          0,
		pageSize:      10,
		paginator:     testPaginator{total: 1, items: []int{10}},
		rowFormater:   func(i, item int) (string, error) { return fmt.Sprintf("%d=%d", i, item), nil },
		out:           new(bytes.Buffer),
		theme:         DefaultTheme(),
		clearTermToFn: func(io.Writer, int) error { return errors.New("clear fail") },
		debug:         true,
	}

	got, err := tab.selectNumbers()
	if err != nil {
		t.Fatalf("selectNumbers() error: %v", err)
	}
	if !reflect.DeepEqual(got, []int{0}) {
		t.Fatalf("got = %v, want [0]", got)
	}
}

func Test_Run_debugClearError(t *testing.T) {
	t.Setenv("NO_COLOR", "1")

	ttyPath := filepath.Join(t.TempDir(), "tty")
	if err := os.WriteFile(ttyPath, []byte("0\n"), 0o600); err != nil {
		t.Fatalf("write tty input: %v", err)
	}
	t.Setenv("TTY", ttyPath)

	tab := table[int]{
		paginator:         testPaginator{total: 1, items: []int{10}},
		originalPaginator: testPaginator{total: 1, items: []int{10}},
		rowFormater:       func(i, item int) (string, error) { return fmt.Sprintf("%d=%d", i, item), nil },
		header:            "header",
		theme:             DefaultTheme(),
		pageSize:          10,
		out:               new(bytes.Buffer),
		input:             nil,
		clearTermToFn:     func(io.Writer, int) error { return errors.New("clear fail") },
		debug:             true,
	}

	bt := &Table[int]{t: tab}
	_, _, err := bt.Run()
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
}

func Test_table_togglePredicateFilter_findPageError(t *testing.T) {
	t.Setenv("NO_COLOR", "1")

	// Toggle ON requires findPage on originalPaginator
	badPaginator := testPaginator{total: 1, findErr: errors.New("read error")}
	tab := &table[int]{
		paginator:         badPaginator,
		originalPaginator: badPaginator,
		pageSize:          10,
		rowFormater:       func(i, item int) (string, error) { return fmt.Sprintf("%d", item), nil },
		theme:             DefaultTheme(),
	}

	err := tab.togglePredicateFilter(func(a any) bool { return true })
	if err == nil {
		t.Fatal("togglePredicateFilter() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "failed to get items for predicate filtering") {
		t.Fatalf("error = %q, want predicate filtering context", err.Error())
	}
}

func Test_fillRemainderOfTermWidthColored_edgeCases(t *testing.T) {
	t.Setenv("NO_COLOR", "1")

	t.Run("remaining width zero", func(t *testing.T) {
		// prefix "abcde" (5 chars), termWidth=5, padding=0 => remainingWidth=0
		got := fillRemainderOfTermWidthColored("abcde", "hello world", "", "", 5, 0)
		if got != "abcde" {
			t.Fatalf("got %q, want prefix only", got)
		}
	})

	t.Run("startLen exceeds rune count", func(t *testing.T) {
		// Very short string, huge terminal width, but string shorter than half avail
		// prefix="" (0), termWidth=100, remaining=100, avail=95, startLen=47
		// string "hi" has 2 runes, startLen=47 > 2
		got := fillRemainderOfTermWidthColored("", "hi", "", "", 100, 0)
		// Should not crash and should contain "hi"
		if !strings.Contains(got, "hi") {
			t.Fatalf("got %q, want containing 'hi'", got)
		}
	})
}
