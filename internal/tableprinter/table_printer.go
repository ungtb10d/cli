package tableprinter

import (
	"strings"
	"time"

	"github.com/ungtb10d/cli/v2/internal/text"
	"github.com/ungtb10d/cli/v2/pkg/iostreams"
	"github.com/cli/go-gh/pkg/tableprinter"
)

type TablePrinter struct {
	tableprinter.TablePrinter
	isTTY bool
}

func (t *TablePrinter) HeaderRow(columns ...string) {
	if !t.isTTY {
		return
	}
	for _, col := range columns {
		t.AddField(strings.ToUpper(col))
	}
	t.EndRow()
}

func (tp *TablePrinter) AddTimeField(t time.Time, c func(string) string) {
	tf := t.Format(time.RFC3339)
	if tp.isTTY {
		// TODO: use a static time.Now
		tf = text.FuzzyAgo(time.Now(), t)
	}
	tp.AddField(tf, tableprinter.WithColor(c))
}

var (
	WithTruncate = tableprinter.WithTruncate
	WithColor    = tableprinter.WithColor
)

func New(ios *iostreams.IOStreams) *TablePrinter {
	maxWidth := 80
	isTTY := ios.IsStdoutTTY()
	if isTTY {
		maxWidth = ios.TerminalWidth()
	}
	tp := tableprinter.New(ios.Out, isTTY, maxWidth)
	return &TablePrinter{
		TablePrinter: tp,
		isTTY:        isTTY,
	}
}
