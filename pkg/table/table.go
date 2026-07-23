package table

import (
	"errors"
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"

	"github.com/baalimago/go_away_boilerplate/pkg/ancli"
)

// Sentinel errors returned by Table.Run.
var (
	ErrUserInitiatedExit = errors.New("user exit")
	ErrBack              = errors.New("back")
)

// TableAction describes an interactive action available in the table UI. The
// Format string is displayed in the prompt (e.g. "[n]ext"). Short, Long, and
// AdditionalHotkeys define the keys that trigger the action.
type TableAction struct {
	Format            string
	Short             string
	Long              string
	AdditionalHotkeys string
	Action            func() error
	// Filter, when non-nil, turns this action into a toggleable predicate
	// filter: pressing it filters the table to rows for which Filter reports
	// true. Pressing it again (or enter while any filter is active) clears it.
	// When Filter is set, Action is ignored.
	Filter func(any) bool
	// EmptyMessage is appended in the prompt when this predicate filter is
	// active and yields zero rows.
	EmptyMessage string
}

// Paginator abstracts a pageable data source for the table.
type Paginator[T any] interface {
	totalAm() int
	findPage(start, offset int) ([]T, error)
}

// SlicePaginator wraps a slice as a Paginator.
func SlicePaginator[T any](items []T) Paginator[T] {
	return paginatorFuncs[T]{
		totalFn: func() int { return len(items) },
		findFn: func(start, offset int) ([]T, error) {
			if start < 0 {
				return nil, fmt.Errorf("start index %d below zero", start)
			}
			if offset < 0 {
				return nil, fmt.Errorf("offset %d below zero", offset)
			}
			if start >= len(items) {
				return []T{}, nil
			}
			end := min(start+offset, len(items))
			return items[start:end], nil
		},
	}
}

type paginatorFuncs[T any] struct {
	totalFn func() int
	findFn  func(start, offset int) ([]T, error)
}

func (pf paginatorFuncs[T]) totalAm() int                   { return pf.totalFn() }
func (pf paginatorFuncs[T]) findPage(s, o int) ([]T, error) { return pf.findFn(s, o) }

// Table is a generic, paginated, interactive table UI. Use New to create one,
// chain With* methods to configure it, then call Run.
type Table[T any] struct {
	t table[T]
}

// New creates a Table with the given paginator and row formatter.
// Use the With* methods to configure; call Run to start.
func New[T any](paginator Paginator[T], rowFormater func(int, T) (string, error)) *Table[T] {
	t := &Table[T]{}
	t.t.paginator = paginator
	t.t.originalPaginator = paginator
	t.t.rowFormater = rowFormater
	t.t.theme = DefaultTheme()
	t.t.out = io.Discard
	t.t.clearTermToFn = ClearTermTo
	return t
}

// WithHeader sets the header displayed above the table.
func (t *Table[T]) WithHeader(header string) *Table[T] {
	t.t.header = header
	return t
}

// WithTheme overrides the color palette and default page size.
func (t *Table[T]) WithTheme(theme Theme) *Table[T] {
	t.t.theme = theme
	return t
}

// WithPageSize sets the number of items per page. When not called, falls back
// to the Theme.Items value.
func (t *Table[T]) WithPageSize(n int) *Table[T] {
	t.t.pageSize = n
	return t
}

// WithInput sets the input reader. When nil (the default), input is read from
// /dev/tty (interactive mode). When non-nil, one line is read per iteration
// (macro mode). In macro mode terminal clearing is skipped.
func (t *Table[T]) WithInput(r io.Reader) *Table[T] {
	t.t.input = r
	return t
}

// WithBackLabel overrides the label shown for the back action. Default is "[b]ack".
func (t *Table[T]) WithBackLabel(label string) *Table[T] {
	t.t.backLabel = label
	return t
}

// WithStartPage opens the table at the given page (clamped to valid range).
func (t *Table[T]) WithStartPage(page int) *Table[T] {
	t.t.page = page
	return t
}

// WithWriter sets the output writer. Default is io.Discard.
func (t *Table[T]) WithWriter(w io.Writer) *Table[T] {
	t.t.out = w
	return t
}

// WithActions adds additional table actions beyond the built-in
// prev/next/back/quit.
func (t *Table[T]) WithActions(actions ...TableAction) *Table[T] {
	t.t.tableActions = append(t.t.tableActions, actions...)
	return t
}

// WithSingleSelect restricts selection to a single item. When not called, the
// user may select multiple items.
func (t *Table[T]) WithSingleSelect() *Table[T] {
	t.t.onlyOneSelect = true
	return t
}

// Run executes the interactive or macro loop and returns the user-selected
// indices and final page number.
func (t *Table[T]) Run() ([]int, int, error) {
	if t.t.pageSize == 0 && t.t.theme.Items > 0 {
		t.t.pageSize = t.t.theme.Items
	}
	fmt.Fprintln(t.t.out, Colorize(t.t.theme.Primary, t.t.header))
	headerWidth := visibleRuneCount(t.t.header)
	line := strings.Repeat("-", headerWidth)
	fmt.Fprintf(t.t.out, "%v\n", Colorize(t.t.theme.Primary, line))

	t.t.lastPage = t.t.pageCount()
	t.t.page = min(max(t.t.page, 0), t.t.lastPage)
	baseActions := []TableAction{t.t.prevPage(), t.t.nextPage(), t.t.back(), t.t.quit()}
	if err := validateTableActions(t.t.tableActions, baseActions); err != nil {
		return nil, t.t.page, fmt.Errorf("failed to validate table actions: %w", err)
	}
	if t.t.input == nil {
		defer func() {
			if err := t.t.clearTermToFn(t.t.out, 2); err != nil && t.t.debug {
				ancli.Errf("failed to clear header: %v", err)
			}
		}()
	}
	t.t.tableActions = append(t.t.tableActions, baseActions...)
	var (
		selectedNumbers []int
		err             error
	)
	for {
		selectedNumbers, err = t.t.selectNumbers()
		if err != nil {
			return nil, t.t.page, fmt.Errorf("failed to select number: %w", err)
		}
		if selectedNumbers != nil {
			break
		}
	}

	if t.t.onlyOneSelect && len(selectedNumbers) > 1 {
		return []int{}, t.t.page, fmt.Errorf("only one selected number supported. selected indices: %v", selectedNumbers)
	}

	return selectedNumbers, t.t.page, nil
}

type table[T any] struct {
	debug                       bool
	page                        int
	pageSize                    int
	lastPage                    int
	onlyOneSelect               bool
	backLabel                   string
	paginator                   Paginator[T]
	originalPaginator           Paginator[T]
	filterString                string
	filteredIndices             []int
	predicateActive             bool
	activePredicateEmptyMessage string
	notice                      string
	rowFormater                 func(int, T) (string, error)
	tableActions                []TableAction
	out                         io.Writer
	input                       io.Reader
	theme                       Theme
	header                      string
	clearTermToFn               func(io.Writer, int) error
}

func (t *table[T]) nextPage() TableAction {
	return TableAction{
		Format:            "[n]ext",
		Short:             "n",
		Long:              "next",
		AdditionalHotkeys: "",
		Action: func() error {
			t.page++
			if t.page > t.lastPage {
				t.page = 0
			}
			return nil
		},
	}
}

func (t *table[T]) prevPage() TableAction {
	return TableAction{
		Format: "[p]rev",
		Short:  "p",
		Long:   "prev",
		Action: func() error {
			t.page--
			if t.page < 0 {
				t.page = t.lastPage
			}
			return nil
		},
	}
}

func (t *table[T]) quit() TableAction {
	return TableAction{
		Format: "[q]uit",
		Short:  "q",
		Long:   "quit",
		Action: func() error {
			return ErrUserInitiatedExit
		},
	}
}

func (t *table[T]) back() TableAction {
	label := "[b]ack"
	if t.backLabel != "" {
		label = t.backLabel
	}
	return TableAction{
		Format: label,
		Short:  "b",
		Long:   "back",
		Action: func() error {
			return ErrBack
		},
	}
}

func (t *table[T]) printRow(i int, item T) error {
	formatted, err := t.rowFormater(i, item)
	if err != nil {
		return fmt.Errorf("failed to format row: %w", err)
	}

	_, err = fmt.Fprintln(t.out, Colorize(t.theme.Breadtext, formatted))
	if err != nil {
		return fmt.Errorf("failed to print: %w", err)
	}
	return nil
}

func (t *table[T]) print() (int, error) {
	totalItems := t.paginator.totalAm()
	pageIndex := t.page * t.pageSize
	listToIndex := min(pageIndex+t.pageSize, totalItems)

	amPrinted := 0
	items, err := t.paginator.findPage(pageIndex, t.pageSize)
	if err != nil {
		return 0, fmt.Errorf("failed to find page with pageIndex: %v, pageSize: %v. Error was: %w", pageIndex, t.pageSize, err)
	}
	for rowIndex := pageIndex; rowIndex < listToIndex; rowIndex++ {
		printErr := t.printRow(rowIndex, items[rowIndex-pageIndex])
		if printErr != nil {
			return 0, fmt.Errorf("failed to print row: %w", printErr)
		}
		amPrinted++
	}
	_, err = fmt.Fprint(t.out, Colorize(t.theme.Secondary, t.promptLine()))
	if err != nil {
		return 0, fmt.Errorf("failed to print prompt line: %w", err)
	}
	return amPrinted, nil
}

func (t *table[T]) promptLine() string {
	parts := []string{"select"}
	actions := t.tableActionsString()
	if actions != "" {
		parts = append(parts, actions)
	}
	parts = append(parts, "[/] filter")
	if t.filterString != "" {
		parts = append(parts, fmt.Sprintf("filter: %q", t.filterString))
	}
	if t.predicateActive {
		parts = append(parts, "dir filter")
	}
	if t.notice != "" {
		parts = append(parts, t.notice)
	}
	if t.pageCount() == 0 {
		if t.predicateActive && t.activePredicateEmptyMessage != "" {
			return fmt.Sprintf("(%s, %s): ", strings.Join(parts, ", "), t.activePredicateEmptyMessage)
		}
		if t.filteredIndices != nil && t.paginator.totalAm() == 0 {
			return fmt.Sprintf("(%s, no matches): ", strings.Join(parts, ", "))
		}
		return fmt.Sprintf("(%s): ", strings.Join(parts, ", "))
	}
	parts = append(parts, fmt.Sprintf("page %v/%v", t.page, t.pageCount()))
	return fmt.Sprintf("(%s): ", strings.Join(parts, ", "))
}

func (t *table[T]) selectNumbers() ([]int, error) {
	amPrinted, err := t.print()
	if err != nil {
		return nil, fmt.Errorf("failed to print table: %w", err)
	}
	t.notice = ""

	if t.input == nil && t.clearTermToFn != nil {
		defer func() {
			if err := t.clearTermToFn(t.out, amPrinted+1); err != nil {
				if t.debug {
					ancli.Errf("failed to clear table: %v", err)
				}
			}
		}()
	}

	var choice string
	if t.input != nil {
		choice, err = t.readFromBuf()
	} else {
		choice, err = ReadUserInput()
	}
	if err != nil {
		return nil, fmt.Errorf("failed to read table selection: %w", err)
	}

	if choice == "" && (t.filterString != "" || t.predicateActive) {
		t.clearFilters()
		return nil, nil
	}

	if strings.HasPrefix(choice, "/") {
		if t.predicateActive {
			t.clearPredicateFilter()
		}
		t.filterString = choice[1:]
		if err := t.applyFilter(); err != nil {
			return nil, fmt.Errorf("failed to apply filter: %w", err)
		}
		return nil, nil
	}

	for _, action := range t.tableActions {
		additionalHotkeyMatch := false
		if action.AdditionalHotkeys != "" || (action.Long == "next" && action.Short == "n") {
			additionalHotkeyMatch = slices.Contains(strings.Split(action.AdditionalHotkeys, ","), choice)
		}
		if choice == action.Long || choice == action.Short || additionalHotkeyMatch {
			if action.Filter != nil {
				if err := t.togglePredicateFilter(action.Filter); err != nil {
					return nil, fmt.Errorf("failed to toggle predicate filter: %w", err)
				}
				if t.predicateActive {
					t.activePredicateEmptyMessage = action.EmptyMessage
				}
				return nil, nil
			}
			if action.Action == nil {
				return nil, fmt.Errorf("table action %q has nil action", action.Long)
			}
			if actErr := action.Action(); actErr != nil {
				return nil, actErr
			}
			return nil, nil
		}
	}

	selectedNumbers, err := t.parseNumbersFromString(choice)
	if err != nil {
		t.notice = fmt.Sprintf("invalid selection %q: %s", choice, strings.ReplaceAll(err.Error(), "\n", "; "))
		return nil, nil
	}

	if t.filteredIndices != nil {
		translated := make([]int, 0, len(selectedNumbers))
		for _, num := range selectedNumbers {
			if num < 0 || num >= len(t.filteredIndices) {
				continue
			}
			translated = append(translated, t.filteredIndices[num])
		}
		selectedNumbers = translated
	}
	if len(selectedNumbers) == 0 {
		t.notice = fmt.Sprintf("no selectable index in %q", choice)
		return nil, nil
	}

	return selectedNumbers, nil
}

func (t *table[T]) tableActionsString() string {
	if len(t.tableActions) == 0 {
		return ""
	}
	sb := strings.Builder{}
	for _, ata := range t.tableActions {
		if sb.Len() > 0 {
			sb.WriteString(", ")
		}
		sb.WriteString(ata.Format)
	}
	return sb.String()
}

func validateTableActions(additionalActions, baseActions []TableAction) error {
	seen := map[string]TableAction{}
	for _, action := range baseActions {
		for _, key := range tableActionKeys(action) {
			seen[key] = action
		}
	}
	for _, action := range additionalActions {
		for _, key := range tableActionKeys(action) {
			if existing, found := seen[key]; found {
				return fmt.Errorf("duplicate table action hotkey %q between %q and %q", key, existing.Long, action.Long)
			}
			seen[key] = action
		}
	}
	return nil
}

func tableActionKeys(action TableAction) []string {
	keys := []string{action.Short, action.Long}
	if action.AdditionalHotkeys != "" {
		keys = append(keys, strings.Split(action.AdditionalHotkeys, ",")...)
	}
	ret := make([]string, 0, len(keys))
	for _, key := range keys {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		ret = append(ret, key)
	}
	return ret
}

func (t *table[T]) pageCount() int {
	if t.pageSize <= 0 || t.paginator.totalAm() <= 0 {
		return 0
	}
	return (t.paginator.totalAm() - 1) / t.pageSize
}

func (t *table[T]) multiPartParse(maybeRange string) ([]int, error) {
	parts := strings.Split(maybeRange, ":")
	if len(parts) != 2 {
		return []int{}, fmt.Errorf("expected 2 numbers from range: '%v', got: %v", maybeRange, len(parts))
	}
	start, err := strconv.Atoi(strings.TrimSpace(parts[0]))
	if err != nil {
		return []int{}, fmt.Errorf("failed to parse start: '%v', err: %w", parts[0], err)
	}
	end, err := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err != nil {
		return []int{}, fmt.Errorf("failed to parse end: '%v', err: %w", parts[1], err)
	}

	if end < start {
		return []int{}, fmt.Errorf("start of range: %v, is greater than end: %v", start, end)
	}
	selectedNumbers := make([]int, 0)
	for i := start; i <= end; i++ {
		if i > t.paginator.totalAm() {
			return selectedNumbers, nil
		}
		selectedNumbers = append(selectedNumbers, i)
	}
	return selectedNumbers, nil
}

func (t *table[T]) applyFilter() error {
	if t.filterString == "" {
		t.paginator = t.originalPaginator
		t.filteredIndices = nil
		t.page = 0
		t.lastPage = t.pageCount()
		return nil
	}

	totalAm := t.originalPaginator.totalAm()
	if totalAm == 0 {
		t.filteredIndices = nil
		t.paginator = SlicePaginator([]T{})
		t.page = 0
		t.lastPage = t.pageCount()
		return nil
	}

	allItems, err := t.originalPaginator.findPage(0, totalAm)
	if err != nil {
		return fmt.Errorf("failed to get items for filtering: %w", err)
	}

	lower := strings.ToLower(t.filterString)
	matchedIndices := make([]int, 0, len(allItems))
	matchedItems := make([]T, 0, len(allItems))
	for i, item := range allItems {
		formatted, formatErr := t.rowFormater(i, item)
		if formatErr != nil {
			continue
		}
		if strings.Contains(strings.ToLower(formatted), lower) {
			matchedIndices = append(matchedIndices, i)
			matchedItems = append(matchedItems, item)
		}
	}

	t.filteredIndices = matchedIndices
	t.paginator = SlicePaginator(matchedItems)
	t.page = 0
	t.lastPage = t.pageCount()
	return nil
}

func (t *table[T]) togglePredicateFilter(predicate func(any) bool) error {
	if t.predicateActive {
		t.clearPredicateFilter()
		return nil
	}
	t.filterString = ""

	totalAm := t.originalPaginator.totalAm()
	allItems, err := t.originalPaginator.findPage(0, totalAm)
	if err != nil {
		return fmt.Errorf("failed to get items for predicate filtering: %w", err)
	}
	matchedIndices := make([]int, 0, len(allItems))
	matchedItems := make([]T, 0, len(allItems))
	for i, item := range allItems {
		if predicate(any(item)) {
			matchedIndices = append(matchedIndices, i)
			matchedItems = append(matchedItems, item)
		}
	}
	t.filteredIndices = matchedIndices
	t.paginator = SlicePaginator(matchedItems)
	t.predicateActive = true
	t.activePredicateEmptyMessage = ""
	t.page = 0
	t.lastPage = t.pageCount()
	return nil
}

func (t *table[T]) clearPredicateFilter() {
	t.predicateActive = false
	t.activePredicateEmptyMessage = ""
	t.paginator = t.originalPaginator
	t.filteredIndices = nil
	t.page = 0
	t.lastPage = t.pageCount()
}

func (t *table[T]) clearFilters() {
	t.filterString = ""
	t.clearPredicateFilter()
}

// readFromBuf reads a single line from the table's input reader via
// ReadUserInputFrom, which avoids buffering conflicts when the same reader
// is shared with external callers.
func (t *table[T]) readFromBuf() (string, error) {
	return ReadUserInputFrom(t.input)
}

func (t *table[T]) parseNumbersFromString(choice string) ([]int, error) {
	selectedNumbers := make([]int, 0)
	parseErrors := make([]error, 0)
	tokens := strings.SplitSeq(choice, ",")
	for tok := range tokens {
		tok = strings.TrimSpace(tok)
		if strings.Contains(tok, ":") {
			multiPartParseSelNum, err := t.multiPartParse(tok)
			if err != nil {
				parseErrors = append(parseErrors, fmt.Errorf("failed to parse range selection: %w", err))
				continue
			}
			selectedNumbers = append(selectedNumbers, multiPartParseSelNum...)
			continue
		}
		v, err := strconv.Atoi(tok)
		if err != nil {
			parseErrors = append(parseErrors, fmt.Errorf("token: '%v' failed to parse to int: %w", tok, err))
			continue
		}
		if v < 0 || v > t.paginator.totalAm() {
			parseErrors = append(parseErrors, fmt.Errorf("index: '%v' is outside the range of items", v))
			continue
		}
		selectedNumbers = append(selectedNumbers, v)
	}

	return selectedNumbers, errors.Join(parseErrors...)
}
