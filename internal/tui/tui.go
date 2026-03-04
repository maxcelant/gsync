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
	colorAccent     = tcell.NewRGBColor(99, 179, 237)  // soft blue
	colorDim        = tcell.NewRGBColor(120, 120, 140) // muted grey
	colorBackground = tcell.ColorDefault
)

func Run(cfg config.Config) error {
	tview.Styles.PrimitiveBackgroundColor = colorBackground
	tview.Styles.ContrastBackgroundColor = tcell.NewRGBColor(30, 30, 46)
	tview.Styles.MoreContrastBackgroundColor = tcell.NewRGBColor(49, 50, 68)
	tview.Styles.BorderColor = colorAccent
	tview.Styles.TitleColor = colorAccent
	tview.Styles.PrimaryTextColor = tcell.ColorWhite
	tview.Styles.SecondaryTextColor = colorDim
	tview.Styles.TertiaryTextColor = colorDim
	tview.Styles.InverseTextColor = tcell.ColorBlack

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
		SetTextColor(tcell.NewRGBColor(240, 100, 100)).
		SetText("")

	form := tview.NewForm()
	form.SetFieldBackgroundColor(tcell.NewRGBColor(49, 50, 68))
	form.SetFieldTextColor(tcell.ColorWhite)
	form.SetLabelColor(colorAccent)
	form.SetButtonBackgroundColor(tcell.NewRGBColor(49, 50, 68))
	form.SetButtonTextColor(tcell.ColorWhite)
	form.SetButtonActivatedStyle(tcell.StyleDefault.Background(colorAccent).Foreground(tcell.ColorBlack))
	form.AddInputField("Since", defaultSince(), 20, nil, nil)
	form.AddInputField("Until", defaultUntil(), 20, nil, nil)
	form.AddDropDown("State", []string{"opened", "closed", "merged", "all"}, 0, nil)
	form.AddInputField("Authors (comma-separated)", defaultAuthors(t.cfg), 40, nil, nil)
	form.AddButton("Run", func() { t.startFetch(form, errView) })
	form.AddButton("Quit", t.app.Stop)
	form.SetBorder(true).SetTitle("  gsync  ").SetTitleAlign(tview.AlignLeft)

	outer := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(nil, 0, 1, false).
		AddItem(tview.NewFlex().
			AddItem(nil, 0, 1, false).
			AddItem(form, 60, 0, true).
			AddItem(nil, 0, 1, false),
			14, 0, true).
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

	flex := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(header, 1, 0, false).
		AddItem(list, 0, 1, true).
		AddItem(footer, 1, 0, false)

	return flex
}

func (t *TUI) startFetch(form *tview.Form, errView *tview.TextView) {
	since := form.GetFormItemByLabel("Since").(*tview.InputField).GetText()
	until := form.GetFormItemByLabel("Until").(*tview.InputField).GetText()
	_, state := form.GetFormItemByLabel("State").(*tview.DropDown).GetCurrentOption()
	authorsRaw := form.GetFormItemByLabel("Authors (comma-separated)").(*tview.InputField).GetText()

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

	cfg := t.cfg
	if state != "" && state != "all" {
		for i := range cfg.Providers {
			cfg.Providers[i].State = state
		}
	}

	if authorsRaw != "" {
		parts := splitAndTrim(authorsRaw)
		for i := range cfg.Providers {
			cfg.Providers[i].Authors = parts
		}
	}

	errView.SetText("")
	t.pages.SwitchToPage("loading")

	go func() {
		entries, authors, _, err := fetch.Entries(cfg, from, untilTime)
		t.app.QueueUpdateDraw(func() {
			if err != nil {
				t.pages.SwitchToPage("form")
				errView.SetText(fmt.Sprintf(" error: %v", err))
				return
			}
			t.pages.AddAndSwitchToPage("results", t.buildResultsPage(authors, entries), true)
		})
	}()
}

func defaultSince() string {
	return time.Now().AddDate(0, 0, -7).Format("2006-01-02")
}

func defaultUntil() string {
	return time.Now().Format("2006-01-02")
}

func defaultAuthors(cfg config.Config) string {
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
	return strings.Join(authors, ", ")
}

func splitAndTrim(s string) []string {
	parts := strings.Split(s, ",")
	var result []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
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
