package tui

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/maxcelant/git-synced/internal/config"
	"github.com/maxcelant/git-synced/internal/fetch"
	"github.com/maxcelant/git-synced/internal/providers"
	"github.com/rivo/tview"
)

type TUI struct {
	app   *tview.Application
	pages *tview.Pages
	cfg   config.Config
}

var (
	colorAccent = tcell.NewRGBColor(99, 179, 237)  // soft blue
	colorDim    = tcell.NewRGBColor(120, 120, 140) // muted grey
	colorField  = tcell.NewRGBColor(49, 50, 68)    // dark slate for inputs
)

func Run(cfg config.Config) error {
	tview.Styles.PrimitiveBackgroundColor = tcell.ColorDefault
	tview.Styles.ContrastBackgroundColor = colorField
	tview.Styles.MoreContrastBackgroundColor = colorField
	tview.Styles.BorderColor = colorAccent
	tview.Styles.TitleColor = colorAccent
	tview.Styles.PrimaryTextColor = tcell.ColorWhite
	tview.Styles.SecondaryTextColor = colorDim

	t := &TUI{app: tview.NewApplication(), cfg: cfg}
	t.pages = tview.NewPages()
	t.pages.AddPage("form", t.buildFormPage(), true, true)
	t.pages.AddPage("loading", t.buildLoadingPage(), true, false)

	t.app.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Rune() == 'q' || event.Key() == tcell.KeyEscape {
			if name, _ := t.pages.GetFrontPage(); name == "results" {
				t.pages.SwitchToPage("form")
				return nil
			}
		}
		return event
	})

	return t.app.SetRoot(t.pages, true).EnableMouse(true).Run()
}

func (t *TUI) buildFormPage() tview.Primitive {
	errView := tview.NewTextView().
		SetTextColor(tcell.NewRGBColor(240, 100, 100))

	sinceField := newInput("Since ", defaultSince(), 20)
	untilField := newInput("Until ", defaultUntil(), 20)

	stateDD := tview.NewDropDown().
		SetLabel("State ").
		SetOptions([]string{"opened", "closed", "merged", "all"}, nil).
		SetCurrentOption(0).
		SetFieldBackgroundColor(colorField).
		SetFieldTextColor(tcell.ColorWhite).
		SetLabelColor(colorAccent)

	authorList := tview.NewList().ShowSecondaryText(false)
	authorList.SetMainTextColor(tcell.ColorWhite)
	authorList.SetSelectedTextColor(tcell.ColorBlack)
	authorList.SetSelectedBackgroundColor(colorAccent)
	authorList.SetBorder(true).
		SetTitle(" Authors  [::d]d · Del to remove[-:-:-] ").
		SetTitleAlign(tview.AlignLeft).
		SetBorderColor(colorDim)
	for _, a := range defaultAuthorsList(t.cfg) {
		authorList.AddItem("  "+a, "", 0, nil)
	}

	addField := newInput("Add   ", "", 28)
	addField.SetPlaceholder("username, then Enter").
		SetPlaceholderTextColor(colorDim)

	runBtn := styledButton(" Run  ")
	quitBtn := styledButton(" Quit ")

	// Add author on Enter in addField
	addField.SetDoneFunc(func(key tcell.Key) {
		if key != tcell.KeyEnter {
			return
		}
		text := strings.TrimSpace(addField.GetText())
		if text != "" {
			authorList.AddItem("  "+text, "", 0, nil)
			addField.SetText("")
		}
	})

	// d / Delete removes the selected author
	authorList.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch {
		case event.Key() == tcell.KeyDelete, event.Rune() == 'd':
			if authorList.GetItemCount() > 0 {
				authorList.RemoveItem(authorList.GetCurrentItem())
			}
			return nil
		}
		return event
	})

	runBtn.SetSelectedFunc(func() {
		t.startFetch(sinceField, untilField, stateDD, authorList, errView)
	})
	quitBtn.SetSelectedFunc(t.app.Stop)

	// Tab / Backtab focus chain
	focusOrder := []tview.Primitive{sinceField, untilField, stateDD, authorList, addField, runBtn, quitBtn}
	for i, prim := range focusOrder {
		i := i
		switch p := prim.(type) {
		case *tview.InputField:
			prev := p.GetInputCapture()
			p.SetInputCapture(chainTab(t.app, focusOrder, i, prev))
		case *tview.DropDown:
			prev := p.GetInputCapture()
			p.SetInputCapture(chainTab(t.app, focusOrder, i, prev))
		case *tview.List:
			prev := p.GetInputCapture()
			p.SetInputCapture(chainTab(t.app, focusOrder, i, prev))
		case *tview.Button:
			prev := p.GetInputCapture()
			p.SetInputCapture(chainTab(t.app, focusOrder, i, prev))
		}
	}

	// Layout
	buttonsRow := tview.NewFlex().
		AddItem(runBtn, 8, 0, false).
		AddItem(tview.NewBox(), 2, 0, false).
		AddItem(quitBtn, 8, 0, false).
		AddItem(nil, 0, 1, false)

	inner := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(padV(sinceField), 3, 0, true).
		AddItem(padV(untilField), 3, 0, false).
		AddItem(padV(stateDD), 3, 0, false).
		AddItem(authorList, 6, 0, false).
		AddItem(padV(addField), 3, 0, false).
		AddItem(buttonsRow, 1, 0, false).
		AddItem(tview.NewBox(), 1, 0, false)

	inner.SetBorder(true).SetTitle("  gsync  ").SetTitleAlign(tview.AlignLeft)

	outer := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(nil, 0, 1, false).
		AddItem(tview.NewFlex().
			AddItem(nil, 0, 1, false).
			AddItem(inner, 62, 0, true).
			AddItem(nil, 0, 1, false),
			26, 0, true).
		AddItem(errView, 1, 0, false).
		AddItem(nil, 0, 1, false)

	return outer
}

func (t *TUI) buildLoadingPage() tview.Primitive {
	tv := tview.NewTextView().
		SetText("\n\nFetching pull requests…").
		SetTextAlign(tview.AlignCenter).
		SetTextColor(colorDim)
	return tv
}

func (t *TUI) buildResultsPage(authors []string, entries []providers.Entry) tview.Primitive {
	byAuthor := make(map[string][]providers.Entry)
	for _, e := range entries {
		byAuthor[e.Author()] = append(byAuthor[e.Author()], e)
	}

	list := tview.NewList().ShowSecondaryText(true)
	list.SetMainTextColor(tcell.ColorWhite)
	list.SetSecondaryTextColor(colorDim)
	list.SetSelectedTextColor(tcell.ColorBlack)
	list.SetSelectedBackgroundColor(colorAccent)

	total := 0
	for _, author := range authors {
		authorEntries := byAuthor[author]
		if len(authorEntries) == 0 {
			continue
		}
		list.AddItem(fmt.Sprintf("  %s", author), "", 0, nil)
		for _, e := range authorEntries {
			url := e.URL()
			list.AddItem("    "+e.Title(), "    "+e.Repo()+"  ·  "+e.CreatedAt(), 0, func() {
				openURL(url)
			})
			total++
		}
	}

	if list.GetItemCount() == 0 {
		list.AddItem("  No results found.", "", 0, nil)
	}

	header := tview.NewTextView().
		SetText(fmt.Sprintf("  %d pull request(s) across %d author(s)", total, len(authors))).
		SetTextColor(colorDim)

	footer := tview.NewTextView().
		SetText("  [white]↑↓[-] navigate   [white]Enter[-] open in browser   [white]q/Esc[-] back   [white]Ctrl+C[-] quit").
		SetDynamicColors(true).
		SetTextColor(colorDim)

	return tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(header, 1, 0, false).
		AddItem(list, 0, 1, true).
		AddItem(footer, 1, 0, false)
}

func (t *TUI) startFetch(
	sinceField, untilField *tview.InputField,
	stateDD *tview.DropDown,
	authorList *tview.List,
	errView *tview.TextView,
) {
	since := sinceField.GetText()
	until := untilField.GetText()
	_, state := stateDD.GetCurrentOption()

	var from time.Time
	if since != "" {
		var err error
		from, err = time.Parse("2006-01-02", since)
		if err != nil {
			errView.SetText(fmt.Sprintf(" invalid since date: %v", err))
			return
		}
	}

	var untilTime time.Time
	if until != "" {
		var err error
		untilTime, err = time.Parse("2006-01-02", until)
		if err != nil {
			errView.SetText(fmt.Sprintf(" invalid until date: %v", err))
			return
		}
	}

	var authors []string
	for i := 0; i < authorList.GetItemCount(); i++ {
		main, _ := authorList.GetItemText(i)
		authors = append(authors, strings.TrimSpace(main))
	}

	cfg := t.cfg
	if state != "" && state != "all" {
		for i := range cfg.Providers {
			cfg.Providers[i].State = state
		}
	}
	if len(authors) > 0 {
		for i := range cfg.Providers {
			cfg.Providers[i].Authors = authors
		}
	}

	errView.SetText("")
	t.pages.SwitchToPage("loading")

	go func() {
		entries, resultAuthors, _, err := fetch.Entries(cfg, from, untilTime)
		t.app.QueueUpdateDraw(func() {
			if err != nil {
				t.pages.SwitchToPage("form")
				errView.SetText(fmt.Sprintf(" error: %v", err))
				return
			}
			t.pages.AddAndSwitchToPage("results", t.buildResultsPage(resultAuthors, entries), true)
		})
	}()
}

// chainTab wraps an existing input capture handler to add Tab/Backtab focus cycling.
func chainTab(app *tview.Application, order []tview.Primitive, idx int, prev func(*tcell.EventKey) *tcell.EventKey) func(*tcell.EventKey) *tcell.EventKey {
	return func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyTab:
			app.SetFocus(order[(idx+1)%len(order)])
			return nil
		case tcell.KeyBacktab:
			app.SetFocus(order[(idx-1+len(order))%len(order)])
			return nil
		}
		if prev != nil {
			return prev(event)
		}
		return event
	}
}

// padV wraps a primitive in a Flex that adds one blank row above it.
func padV(p tview.Primitive) tview.Primitive {
	return tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(tview.NewBox(), 1, 0, false).
		AddItem(p, 1, 0, true)
}

func newInput(label, value string, width int) *tview.InputField {
	return tview.NewInputField().
		SetLabel(label).
		SetText(value).
		SetFieldWidth(width).
		SetFieldBackgroundColor(colorField).
		SetFieldTextColor(tcell.ColorWhite).
		SetLabelColor(colorAccent)
}

func styledButton(label string) *tview.Button {
	return tview.NewButton(label).
		SetStyle(tcell.StyleDefault.Background(colorField).Foreground(tcell.ColorWhite)).
		SetActivatedStyle(tcell.StyleDefault.Background(colorAccent).Foreground(tcell.ColorBlack))
}

func defaultSince() string {
	return time.Now().AddDate(0, 0, -7).Format("2006-01-02")
}

func defaultUntil() string {
	return time.Now().Format("2006-01-02")
}

func defaultAuthorsList(cfg config.Config) []string {
	seen := make(map[string]bool)
	var authors []string
	for _, p := range cfg.Providers {
		for _, a := range p.Authors {
			if !seen[a] {
				seen[a] = true
				authors = append(authors, a)
			}
		}
	}
	return authors
}

func openURL(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "linux":
		cmd = exec.Command("xdg-open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	}
	if cmd != nil {
		cmd.Start() //nolint:errcheck
	}
}
