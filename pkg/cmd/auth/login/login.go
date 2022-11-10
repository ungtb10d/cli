package login

import (
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/MakeNowJust/heredoc"
	"github.com/ungtb10d/cli/v2/git"
	"github.com/ungtb10d/cli/v2/internal/config"
	"github.com/ungtb10d/cli/v2/internal/ghinstance"
	"github.com/ungtb10d/cli/v2/pkg/cmd/auth/shared"
	"github.com/ungtb10d/cli/v2/pkg/cmdutil"
	"github.com/ungtb10d/cli/v2/pkg/iostreams"
	ghAuth "github.com/cli/go-gh/pkg/auth"
	"github.com/spf13/cobra"
)

type LoginOptions struct {
	IO         *iostreams.IOStreams
	Config     func() (config.Config, error)
	HttpClient func() (*http.Client, error)
	GitClient  *git.Client
	Prompter   shared.Prompt

	MainExecutable string

	Interactive bool

	Hostname    string
	Scopes      []string
	Token       string
	Web         bool
	GitProtocol string
}

func NewCmdLogin(f *cmdutil.Factory, runF func(*LoginOptions) error) *cobra.Command {
	opts := &LoginOptions{
		IO:         f.IOStreams,
		Config:     f.Config,
		HttpClient: f.HttpClient,
		GitClient:  f.GitClient,
		Prompter:   f.Prompter,
	}

	var tokenStdin bool

	cmd := &cobra.Command{
		Use:   "login",
		Args:  cobra.ExactArgs(0),
		Short: "Authenticate with a GitHub host",
		Long: heredoc.Docf(`
			Authenticate with a GitHub host.

			The default authentication mode is a web-based browser flow. After completion, an
			authentication token will be stored internally.

			Alternatively, use %[1]s--with-token%[1]s to pass in a token on standard input.
			The minimum required scopes for the token are: "repo", "read:org".

			Alternatively, gh will use the authentication token found in environment variables.
			This method is most suitable for "headless" use of gh such as in automation. See
			%[1]sgh help environment%[1]s for more info.

			To use gh in GitHub Actions, add %[1]sGH_TOKEN: ${{ github.token }}%[1]s to "env".
		`, "`"),
		Example: heredoc.Doc(`
			# start interactive setup
			$ gh auth login

			# authenticate against github.com by reading the token from a file
			$ gh auth login --with-token < mytoken.txt

			# authenticate with a specific GitHub instance
			$ gh auth login --hostname enterprise.internal
		`),
		RunE: func(cmd *cobra.Command, args []string) error {
			if tokenStdin && opts.Web {
				return cmdutil.FlagErrorf("specify only one of `--web` or `--with-token`")
			}
			if tokenStdin && len(opts.Scopes) > 0 {
				return cmdutil.FlagErrorf("specify only one of `--scopes` or `--with-token`")
			}

			if tokenStdin {
				defer opts.IO.In.Close()
				token, err := io.ReadAll(opts.IO.In)
				if err != nil {
					return fmt.Errorf("failed to read token from standard input: %w", err)
				}
				opts.Token = strings.TrimSpace(string(token))
			}

			if opts.IO.CanPrompt() && opts.Token == "" {
				opts.Interactive = true
			}

			if cmd.Flags().Changed("hostname") {
				if err := ghinstance.HostnameValidator(opts.Hostname); err != nil {
					return cmdutil.FlagErrorf("error parsing hostname: %w", err)
				}
			}

			if opts.Hostname == "" && (!opts.Interactive || opts.Web) {
				opts.Hostname, _ = ghAuth.DefaultHost()
			}

			opts.MainExecutable = f.Executable()
			if runF != nil {
				return runF(opts)
			}

			return loginRun(opts)
		},
	}

	cmd.Flags().StringVarP(&opts.Hostname, "hostname", "h", "", "The hostname of the GitHub instance to authenticate with")
	cmd.Flags().StringSliceVarP(&opts.Scopes, "scopes", "s", nil, "Additional authentication scopes to request")
	cmd.Flags().BoolVar(&tokenStdin, "with-token", false, "Read token from standard input")
	cmd.Flags().BoolVarP(&opts.Web, "web", "w", false, "Open a browser to authenticate")
	cmdutil.StringEnumFlag(cmd, &opts.GitProtocol, "git-protocol", "p", "", []string{"ssh", "https"}, "The protocol to use for git operations")

	return cmd
}

func loginRun(opts *LoginOptions) error {
	cfg, err := opts.Config()
	if err != nil {
		return err
	}

	hostname := opts.Hostname
	if opts.Interactive && hostname == "" {
		var err error
		hostname, err = promptForHostname(opts)
		if err != nil {
			return err
		}
	}

	if src, writeable := shared.AuthTokenWriteable(cfg, hostname); !writeable {
		fmt.Fprintf(opts.IO.ErrOut, "The value of the %s environment variable is being used for authentication.\n", src)
		fmt.Fprint(opts.IO.ErrOut, "To have GitHub CLI store credentials instead, first clear the value from the environment.\n")
		return cmdutil.SilentError
	}

	httpClient, err := opts.HttpClient()
	if err != nil {
		return err
	}

	if opts.Token != "" {
		cfg.Set(hostname, "oauth_token", opts.Token)

		if err := shared.HasMinimumScopes(httpClient, hostname, opts.Token); err != nil {
			return fmt.Errorf("error validating token: %w", err)
		}
		if opts.GitProtocol != "" {
			cfg.Set(hostname, "git_protocol", opts.GitProtocol)
		}
		return cfg.Write()
	}

	existingToken, _ := cfg.AuthToken(hostname)
	if existingToken != "" && opts.Interactive {
		if err := shared.HasMinimumScopes(httpClient, hostname, existingToken); err == nil {
			keepGoing, err := opts.Prompter.Confirm(fmt.Sprintf("You're already logged into %s. Do you want to re-authenticate?", hostname), false)
			if err != nil {
				return err
			}
			if !keepGoing {
				return nil
			}
		}
	}

	return shared.Login(&shared.LoginOptions{
		IO:          opts.IO,
		Config:      cfg,
		HTTPClient:  httpClient,
		Hostname:    hostname,
		Interactive: opts.Interactive,
		Web:         opts.Web,
		Scopes:      opts.Scopes,
		Executable:  opts.MainExecutable,
		GitProtocol: opts.GitProtocol,
		Prompter:    opts.Prompter,
		GitClient:   opts.GitClient,
	})
}

func promptForHostname(opts *LoginOptions) (string, error) {
	options := []string{"GitHub.com", "GitHub Enterprise Server"}
	hostType, err := opts.Prompter.Select(
		"What account do you want to log into?",
		options[0],
		options)
	if err != nil {
		return "", err
	}

	isEnterprise := hostType == 1

	hostname := ghinstance.Default()
	if isEnterprise {
		hostname, err = opts.Prompter.InputHostname()
	}

	return hostname, err
}
