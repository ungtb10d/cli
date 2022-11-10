package list

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/ungtb10d/cli/v2/internal/config"
	"github.com/ungtb10d/cli/v2/internal/text"
	"github.com/ungtb10d/cli/v2/pkg/cmd/gist/shared"
	"github.com/ungtb10d/cli/v2/pkg/cmdutil"
	"github.com/ungtb10d/cli/v2/pkg/iostreams"
	"github.com/ungtb10d/cli/v2/utils"
	"github.com/spf13/cobra"
)

type ListOptions struct {
	IO         *iostreams.IOStreams
	Config     func() (config.Config, error)
	HttpClient func() (*http.Client, error)

	Limit      int
	Visibility string // all, secret, public
}

func NewCmdList(f *cmdutil.Factory, runF func(*ListOptions) error) *cobra.Command {
	opts := &ListOptions{
		IO:         f.IOStreams,
		Config:     f.Config,
		HttpClient: f.HttpClient,
	}

	var flagPublic bool
	var flagSecret bool

	cmd := &cobra.Command{
		Use:     "list",
		Short:   "List your gists",
		Aliases: []string{"ls"},
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if opts.Limit < 1 {
				return cmdutil.FlagErrorf("invalid limit: %v", opts.Limit)
			}

			opts.Visibility = "all"
			if flagSecret {
				opts.Visibility = "secret"
			} else if flagPublic {
				opts.Visibility = "public"
			}

			if runF != nil {
				return runF(opts)
			}
			return listRun(opts)
		},
	}

	cmd.Flags().IntVarP(&opts.Limit, "limit", "L", 10, "Maximum number of gists to fetch")
	cmd.Flags().BoolVar(&flagPublic, "public", false, "Show only public gists")
	cmd.Flags().BoolVar(&flagSecret, "secret", false, "Show only secret gists")

	return cmd
}

func listRun(opts *ListOptions) error {
	client, err := opts.HttpClient()
	if err != nil {
		return err
	}

	cfg, err := opts.Config()
	if err != nil {
		return err
	}

	host, _ := cfg.DefaultHost()

	gists, err := shared.ListGists(client, host, opts.Limit, opts.Visibility)
	if err != nil {
		return err
	}

	if len(gists) == 0 {
		return cmdutil.NewNoResultsError("no gists found")
	}

	if err := opts.IO.StartPager(); err == nil {
		defer opts.IO.StopPager()
	} else {
		fmt.Fprintf(opts.IO.ErrOut, "failed to start pager: %v\n", err)
	}

	cs := opts.IO.ColorScheme()

	//nolint:staticcheck // SA1019: utils.NewTablePrinter is deprecated: use internal/tableprinter
	tp := utils.NewTablePrinter(opts.IO)

	for _, gist := range gists {
		fileCount := len(gist.Files)

		visibility := "public"
		visColor := cs.Green
		if !gist.Public {
			visibility = "secret"
			visColor = cs.Red
		}

		description := gist.Description
		if description == "" {
			for filename := range gist.Files {
				if !strings.HasPrefix(filename, "gistfile") {
					description = filename
					break
				}
			}
		}

		gistTime := gist.UpdatedAt.Format(time.RFC3339)
		if tp.IsTTY() {
			gistTime = text.FuzzyAgo(time.Now(), gist.UpdatedAt)
		}

		tp.AddField(gist.ID, nil, nil)
		tp.AddField(text.RemoveExcessiveWhitespace(description), nil, cs.Bold)
		tp.AddField(text.Pluralize(fileCount, "file"), nil, nil)
		tp.AddField(visibility, nil, visColor)
		tp.AddField(gistTime, nil, cs.Gray)
		tp.EndRow()
	}

	return tp.Render()
}
