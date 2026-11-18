package table

import (
	"bytes"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func filterTable(items []string, out *bytes.Buffer, input string) *Table[string] {
	return New(
		SlicePaginator(items),
		func(i int, item string) (string, error) { return item, nil },
	).
		WithHeader("header").
		WithWriter(out).
		WithPageSize(3).
		WithInput(strings.NewReader(input))
}

func Test_Builder_withFilterOpensFiltered(t *testing.T) {
	t.Setenv("NO_COLOR", "1")

	var out bytes.Buffer
	got, _, err := filterTable([]string{"Alpha", "Bravo", "Charlie", "alpha-beta"}, &out, "1\n").
		WithFilter("alpha").
		Run()
	if err != nil {
		t.Fatalf("Run() unexpected error: %v", err)
	}
	if !reflect.DeepEqual(got, []int{3}) {
		t.Fatalf("Run() = %v, want [3], the original index of the second match", got)
	}
	if strings.Contains(out.String(), "Bravo") {
		t.Fatalf("the first page should show only the matches:\n%s", out.String())
	}
	if !strings.Contains(out.String(), `filter: "alpha"`) {
		t.Fatalf("the prompt should name the filter:\n%s", out.String())
	}
}

func Test_Builder_withFilterOpensOnTheFilteredStartPage(t *testing.T) {
	t.Setenv("NO_COLOR", "1")

	var items []string
	for i := range 30 {
		items = append(items, fmt.Sprintf("item-%02d", i))
	}
	var out bytes.Buffer
	got, page, err := filterTable(items, &out, "6\n").
		WithFilter("item-1").
		WithStartPage(2).
		Run()
	if err != nil {
		t.Fatalf("Run() unexpected error: %v", err)
	}
	if page != 2 {
		t.Fatalf("page = %d, want 2", page)
	}
	if !strings.Contains(out.String(), "item-16\nitem-17\nitem-18\n") || strings.Contains(out.String(), "item-10") {
		t.Fatalf("filtered page 2 should show item-16 to item-18 only:\n%s", out.String())
	}
	if !reflect.DeepEqual(got, []int{16}) {
		t.Fatalf("Run() = %v, want [16], the original index of filtered row 6", got)
	}
}

func Test_Builder_withFilterClampsTheStartPage(t *testing.T) {
	t.Setenv("NO_COLOR", "1")

	var out bytes.Buffer
	_, page, err := filterTable([]string{"Alpha", "Bravo", "Charlie", "alpha-beta", "delta"}, &out, "q\n").
		WithFilter("alpha").
		WithStartPage(1).
		Run()
	if !errors.Is(err, ErrUserInitiatedExit) {
		t.Fatalf("Run() error = %v, want ErrUserInitiatedExit", err)
	}
	if page != 0 {
		t.Fatalf("page = %d, want 0: two matches fill one page", page)
	}
}

func Test_Builder_withFilterFindPageError(t *testing.T) {
	t.Setenv("NO_COLOR", "1")

	readErr := errors.New("read error")
	_, _, err := New(
		testPaginator{total: 2, findErr: readErr},
		func(i, item int) (string, error) { return fmt.Sprintf("%d", item), nil },
	).
		WithInput(strings.NewReader("0\n")).
		WithFilter("1").
		Run()
	if !errors.Is(err, readErr) {
		t.Fatalf("Run() error = %v, want the paginator's error", err)
	}
}

func Test_Builder_filterReportsTheActiveFilter(t *testing.T) {
	t.Setenv("NO_COLOR", "1")

	items := []string{"Alpha", "Bravo", "alpha-beta"}
	for _, tc := range []struct {
		name    string
		initial string
		input   string
		want    string
	}{
		{"none", "", "q\n", ""},
		{"typed", "", "/alpha\nq\n", "alpha"},
		{"replaced", "", "/alpha\n/bra\nq\n", "bra"},
		{"cleared by enter", "", "/alpha\n\nq\n", ""},
		{"given", "alpha", "q\n", "alpha"},
		{"given then cleared", "alpha", "\nq\n", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			tb := filterTable(items, &out, tc.input).WithFilter(tc.initial)
			if _, _, err := tb.Run(); !errors.Is(err, ErrUserInitiatedExit) {
				t.Fatalf("Run() error = %v, want ErrUserInitiatedExit", err)
			}
			if got := tb.Filter(); got != tc.want {
				t.Fatalf("Filter() = %q, want %q", got, tc.want)
			}
		})
	}
}
