package refresh

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/MakeNowJust/heredoc"
	"github.com/ungtb10d/cli/v2/git"
	"github.com/ungtb10d/cli/v2/internal/authflow"
	"github.com/ungtb10d/cli/v2/internal/config"
	"github.com/ungtb10d/cli/v2/pkg/cmd/auth/shared"
	"github.com/ungtb10d/cli/v2/pkg/cmdutil"
	"github.com/ungtb10d/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
)

type RefreshOptions struct {
	IO         *iostreams.IOStreams
	Config     func() (config.Config, error)
	HttpClient *http.Client
	GitClient  *git.Client
	Prompter   shared.Prompt

	MainExecutable string

	Hostname string
	Scopes   []string
	AuthFlow func(config.Config, *iostreams.IOStreams, string, []string, bool) error

	Interactive bool
}

func NewCmdRefresh(f *cmdutil.Factory, runF func(*RefreshOptions) error) *cobra.Command {
	opts := &RefreshOptions{
		IO:     f.IOStreams,
		Config: f.Config,
		AuthFlow: func(cfg config.Config, io *iostreams.IOStreams, hostname string, scopes []string, interactive bool) error {
			_, err := authflow.AuthFlowWithConfig(cfg, io, hostname, "", scopes, interactive)
			return err
		},
		HttpClient: &http.Client{},
		GitClient:  f.GitClient,
		Prompter:   f.Prompter,
	}

	cmd := &cobra.Command{
		Use:   "refresh",
		Args:  cobra.ExactArgs(0),
		Short: "Refresh stored authentication credentials",
		Long: heredoc.Doc(`Expand or fix the permission scopes for stored credentials.

			The --scopes flag accepts a comma separated list of scopes you want your gh credentials to have. If
			absent, this command ensures that gh has access to a minimum set of scopes.
		`),
		Example: heredoc.Doc(`
			$ gh auth refresh --scopes write:org,read:public_key
			# => open a browser to add write:org and read:public_key scopes for use with gh api

			$ gh auth refresh
			# => open a browser to ensure your authentication credentials have the correct minimum scopes
		`),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Interactive = opts.IO.CanPrompt()

			if !opts.Interactive && opts.Hostname == "" {
				return cmdutil.FlagErrorf("--hostname required when not running interactively")
			}

			opts.MainExecutable = f.Executable()
			if runF != nil {
				return runF(opts)
			}
			return refreshRun(opts)
		},
	}

	cmd.Flags().StringVarP(&opts.Hostname, "hostname", "h", "", "The GitHub host to use for authentication")
	cmd.Flags().StringSliceVarP(&opts.Scopes, "scopes", "s", nil, "Additional authentication scopes for gh to have")

	return cmd
}

func refreshRun(opts *RefreshOptions) error {
	cfg, err := opts.Config()
	if err != nil {
		return err
	}

	candidates := cfg.Hosts()
	if len(candidates) == 0 {
		return fmt.Errorf("not logged in to any hosts. Use 'gh auth login' to authenticate with a host")
	}

	hostname := opts.Hostname
	if hostname == "" {
		if len(candidates) == 1 {
			hostname = candidates[0]
		} else {
			selected, err := opts.Prompter.Select("What account do you want to refresh auth for?", "", candidates)
			if err != nil {
				return fmt.Errorf("could not prompt: %w", err)
			}
			hostname = candidates[selected]
		}
	} else {
		var found bool
		for _, c := range candidates {
			if c == hostname {
				found = true
				break
			}
		}

		if !found {
			return fmt.Errorf("not logged in to %s. use 'gh auth login' to authenticate with this host", hostname)
		}
	}

	if src, writeable := shared.AuthTokenWriteable(cfg, hostname); !writeable {
		fmt.Fprintf(opts.IO.ErrOut, "The value of the %s environment variable is being used for authentication.\n", src)
		fmt.Fprint(opts.IO.ErrOut, "To refresh credentials stored in GitHub CLI, first clear the value from the environment.\n")
		return cmdutil.SilentError
	}

	var additionalScopes []string
	if oldToken, _ := cfg.AuthToken(hostname); oldToken != "" {
		if oldScopes, err := shared.GetScopes(opts.HttpClient, hostname, oldToken); err == nil {
			for _, s := range strings.Split(oldScopes, ",") {
				s = strings.TrimSpace(s)
				if s != "" {
					additionalScopes = append(additionalScopes, s)
				}
			}
		}
	}

	credentialFlow := &shared.GitCredentialFlow{
		Executable: opts.MainExecutable,
		Prompter:   opts.Prompter,
		GitClient:  opts.GitClient,
	}
	gitProtocol, _ := cfg.GetOrDefault(hostname, "git_protocol")
	if opts.Interactive && gitProtocol == "https" {
		if err := credentialFlow.Prompt(hostname); err != nil {
			return err
		}
		additionalScopes = append(additionalScopes, credentialFlow.Scopes()...)
	}

	if err := opts.AuthFlow(cfg, opts.IO, hostname, append(opts.Scopes, additionalScopes...), opts.Interactive); err != nil {
		return err
	}

	cs := opts.IO.ColorScheme()
	fmt.Fprintf(opts.IO.ErrOut, "%s Authentication complete.\n", cs.SuccessIcon())

	if credentialFlow.ShouldSetup() {
		username, _ := cfg.Get(hostname, "user")
		password, _ := cfg.AuthToken(hostname)
		if err := credentialFlow.Setup(hostname, username, password); err != nil {
			return err
		}
	}

	return nil
}
