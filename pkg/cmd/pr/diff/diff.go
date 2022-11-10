package diff

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"

	"github.com/MakeNowJust/heredoc"
	"github.com/ungtb10d/cli/v2/api"
	"github.com/ungtb10d/cli/v2/internal/browser"
	"github.com/ungtb10d/cli/v2/internal/ghinstance"
	"github.com/ungtb10d/cli/v2/internal/ghrepo"
	"github.com/ungtb10d/cli/v2/internal/text"
	"github.com/ungtb10d/cli/v2/pkg/cmd/pr/shared"
	"github.com/ungtb10d/cli/v2/pkg/cmdutil"
	"github.com/ungtb10d/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
)

type DiffOptions struct {
	HttpClient func() (*http.Client, error)
	IO         *iostreams.IOStreams
	Browser    browser.Browser

	Finder shared.PRFinder

	SelectorArg string
	UseColor    bool
	Patch       bool
	NameOnly    bool
	BrowserMode bool
}

func NewCmdDiff(f *cmdutil.Factory, runF func(*DiffOptions) error) *cobra.Command {
	opts := &DiffOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
		Browser:    f.Browser,
	}

	var colorFlag string

	cmd := &cobra.Command{
		Use:   "diff [<number> | <url> | <branch>]",
		Short: "View changes in a pull request",
		Long: heredoc.Doc(`
			View changes in a pull request. 

			Without an argument, the pull request that belongs to the current branch
			is selected.
			
			With '--web', open the pull request diff in a web browser instead.
		`),
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Finder = shared.NewFinder(f)

			if repoOverride, _ := cmd.Flags().GetString("repo"); repoOverride != "" && len(args) == 0 {
				return cmdutil.FlagErrorf("argument required when using the `--repo` flag")
			}

			if len(args) > 0 {
				opts.SelectorArg = args[0]
			}

			switch colorFlag {
			case "always":
				opts.UseColor = true
			case "auto":
				opts.UseColor = opts.IO.ColorEnabled()
			case "never":
				opts.UseColor = false
			default:
				return fmt.Errorf("unsupported color %q", colorFlag)
			}

			if runF != nil {
				return runF(opts)
			}
			return diffRun(opts)
		},
	}

	cmdutil.StringEnumFlag(cmd, &colorFlag, "color", "", "auto", []string{"always", "never", "auto"}, "Use color in diff output")
	cmd.Flags().BoolVar(&opts.Patch, "patch", false, "Display diff in patch format")
	cmd.Flags().BoolVar(&opts.NameOnly, "name-only", false, "Display only names of changed files")
	cmd.Flags().BoolVarP(&opts.BrowserMode, "web", "w", false, "Open the pull request diff in the browser")

	return cmd
}

func diffRun(opts *DiffOptions) error {
	findOptions := shared.FindOptions{
		Selector: opts.SelectorArg,
		Fields:   []string{"number"},
	}

	if opts.BrowserMode {
		findOptions.Fields = []string{"url"}
	}

	pr, baseRepo, err := opts.Finder.Find(findOptions)
	if err != nil {
		return err
	}

	if opts.BrowserMode {
		openUrl := fmt.Sprintf("%s/files", pr.URL)
		if opts.IO.IsStdoutTTY() {
			fmt.Fprintf(opts.IO.ErrOut, "Opening %s in your browser.\n", text.DisplayURL(openUrl))
		}
		return opts.Browser.Browse(openUrl)
	}

	httpClient, err := opts.HttpClient()
	if err != nil {
		return err
	}

	if opts.NameOnly {
		opts.Patch = false
	}

	diff, err := fetchDiff(httpClient, baseRepo, pr.Number, opts.Patch)
	if err != nil {
		return fmt.Errorf("could not find pull request diff: %w", err)
	}
	defer diff.Close()

	if err := opts.IO.StartPager(); err == nil {
		defer opts.IO.StopPager()
	} else {
		fmt.Fprintf(opts.IO.ErrOut, "failed to start pager: %v\n", err)
	}

	if opts.NameOnly {
		return changedFilesNames(opts.IO.Out, diff)
	}

	if !opts.UseColor {
		_, err = io.Copy(opts.IO.Out, diff)
		return err
	}

	return colorDiffLines(opts.IO.Out, diff)
}

func fetchDiff(httpClient *http.Client, baseRepo ghrepo.Interface, prNumber int, asPatch bool) (io.ReadCloser, error) {
	url := fmt.Sprintf(
		"%srepos/%s/pulls/%d",
		ghinstance.RESTPrefix(baseRepo.RepoHost()),
		ghrepo.FullName(baseRepo),
		prNumber,
	)
	acceptType := "application/vnd.github.v3.diff"
	if asPatch {
		acceptType = "application/vnd.github.v3.patch"
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Accept", acceptType)

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != 200 {
		return nil, api.HandleHTTPError(resp)
	}

	return resp.Body, nil
}

const lineBufferSize = 4096

var (
	colorHeader   = []byte("\x1b[1;38m")
	colorAddition = []byte("\x1b[32m")
	colorRemoval  = []byte("\x1b[31m")
	colorReset    = []byte("\x1b[m")
)

func colorDiffLines(w io.Writer, r io.Reader) error {
	diffLines := bufio.NewReaderSize(r, lineBufferSize)
	wasPrefix := false
	needsReset := false

	for {
		diffLine, isPrefix, err := diffLines.ReadLine()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return fmt.Errorf("error reading pull request diff: %w", err)
		}

		var color []byte
		if !wasPrefix {
			if isHeaderLine(diffLine) {
				color = colorHeader
			} else if isAdditionLine(diffLine) {
				color = colorAddition
			} else if isRemovalLine(diffLine) {
				color = colorRemoval
			}
		}

		if color != nil {
			if _, err := w.Write(color); err != nil {
				return err
			}
			needsReset = true
		}

		if _, err := w.Write(diffLine); err != nil {
			return err
		}

		if !isPrefix {
			if needsReset {
				if _, err := w.Write(colorReset); err != nil {
					return err
				}
				needsReset = false
			}
			if _, err := w.Write([]byte{'\n'}); err != nil {
				return err
			}
		}
		wasPrefix = isPrefix
	}
	return nil
}

var diffHeaderPrefixes = []string{"+++", "---", "diff", "index"}

func isHeaderLine(l []byte) bool {
	dl := string(l)
	for _, p := range diffHeaderPrefixes {
		if strings.HasPrefix(dl, p) {
			return true
		}
	}
	return false
}

func isAdditionLine(l []byte) bool {
	return len(l) > 0 && l[0] == '+'
}

func isRemovalLine(l []byte) bool {
	return len(l) > 0 && l[0] == '-'
}

func changedFilesNames(w io.Writer, r io.Reader) error {
	diff, err := io.ReadAll(r)
	if err != nil {
		return err
	}

	pattern := regexp.MustCompile(`(?:^|\n)diff\s--git.*\sb/(.*)`)
	matches := pattern.FindAllStringSubmatch(string(diff), -1)

	for _, val := range matches {
		name := strings.TrimSpace(val[1])
		if _, err := w.Write([]byte(name + "\n")); err != nil {
			return err
		}
	}

	return nil
}
