package create

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/MakeNowJust/heredoc"
	"github.com/ungtb10d/cli/v2/api"
	"github.com/ungtb10d/cli/v2/git"
	"github.com/ungtb10d/cli/v2/internal/config"
	"github.com/ungtb10d/cli/v2/internal/ghrepo"
	"github.com/ungtb10d/cli/v2/pkg/cmd/repo/shared"
	"github.com/ungtb10d/cli/v2/pkg/cmdutil"
	"github.com/ungtb10d/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
)

type iprompter interface {
	Input(string, string) (string, error)
	Select(string, string, []string) (int, error)
	Confirm(string, bool) (bool, error)
}

type CreateOptions struct {
	HttpClient func() (*http.Client, error)
	GitClient  *git.Client
	Config     func() (config.Config, error)
	IO         *iostreams.IOStreams
	Prompter   iprompter

	Name               string
	Description        string
	Homepage           string
	Team               string
	Template           string
	Public             bool
	Private            bool
	Internal           bool
	Visibility         string
	Push               bool
	Clone              bool
	Source             string
	Remote             string
	GitIgnoreTemplate  string
	LicenseTemplate    string
	DisableIssues      bool
	DisableWiki        bool
	Interactive        bool
	IncludeAllBranches bool
	AddReadme          bool
}

func NewCmdCreate(f *cmdutil.Factory, runF func(*CreateOptions) error) *cobra.Command {
	opts := &CreateOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
		GitClient:  f.GitClient,
		Config:     f.Config,
		Prompter:   f.Prompter,
	}

	var enableIssues bool
	var enableWiki bool

	cmd := &cobra.Command{
		Use:   "create [<name>]",
		Short: "Create a new repository",
		Long: heredoc.Docf(`
			Create a new GitHub repository.

			To create a repository interactively, use %[1]sgh repo create%[1]s with no arguments.

			To create a remote repository non-interactively, supply the repository name and one of %[1]s--public%[1]s, %[1]s--private%[1]s, or %[1]s--internal%[1]s.
			Pass %[1]s--clone%[1]s to clone the new repository locally.

			To create a remote repository from an existing local repository, specify the source directory with %[1]s--source%[1]s.
			By default, the remote repository name will be the name of the source directory.
			Pass %[1]s--push%[1]s to push any local commits to the new repository.
		`, "`"),
		Example: heredoc.Doc(`
			# create a repository interactively
			gh repo create

			# create a new remote repository and clone it locally
			gh repo create my-project --public --clone

			# create a remote repository from the current directory
			gh repo create my-project --private --source=. --remote=upstream
		`),
		Args:    cobra.MaximumNArgs(1),
		Aliases: []string{"new"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				opts.Name = args[0]
			}

			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				if !opts.IO.CanPrompt() {
					return cmdutil.FlagErrorf("at least one argument required in non-interactive mode")
				}
				opts.Interactive = true
			} else {
				// exactly one visibility flag required
				if !opts.Public && !opts.Private && !opts.Internal {
					return cmdutil.FlagErrorf("`--public`, `--private`, or `--internal` required when not running interactively")
				}
				err := cmdutil.MutuallyExclusive(
					"expected exactly one of `--public`, `--private`, or `--internal`",
					opts.Public, opts.Private, opts.Internal)
				if err != nil {
					return err
				}

				if opts.Public {
					opts.Visibility = "PUBLIC"
				} else if opts.Private {
					opts.Visibility = "PRIVATE"
				} else {
					opts.Visibility = "INTERNAL"
				}
			}

			if opts.Source == "" {
				if opts.Remote != "" {
					return cmdutil.FlagErrorf("the `--remote` option can only be used with `--source`")
				}
				if opts.Push {
					return cmdutil.FlagErrorf("the `--push` option can only be used with `--source`")
				}
				if opts.Name == "" && !opts.Interactive {
					return cmdutil.FlagErrorf("name argument required to create new remote repository")
				}

			} else if opts.Clone || opts.GitIgnoreTemplate != "" || opts.LicenseTemplate != "" || opts.Template != "" {
				return cmdutil.FlagErrorf("the `--source` option is not supported with `--clone`, `--template`, `--license`, or `--gitignore`")
			}

			if opts.Template != "" && (opts.GitIgnoreTemplate != "" || opts.LicenseTemplate != "") {
				return cmdutil.FlagErrorf(".gitignore and license templates are not added when template is provided")
			}

			if opts.Template != "" && opts.AddReadme {
				return cmdutil.FlagErrorf("the `--add-readme` option is not supported with `--template`")
			}

			if cmd.Flags().Changed("enable-issues") {
				opts.DisableIssues = !enableIssues
			}
			if cmd.Flags().Changed("enable-wiki") {
				opts.DisableWiki = !enableWiki
			}
			if opts.Template != "" && (opts.Homepage != "" || opts.Team != "" || opts.DisableIssues || opts.DisableWiki) {
				return cmdutil.FlagErrorf("the `--template` option is not supported with `--homepage`, `--team`, `--disable-issues`, or `--disable-wiki`")
			}

			if opts.Template == "" && opts.IncludeAllBranches {
				return cmdutil.FlagErrorf("the `--include-all-branches` option is only supported when using `--template`")
			}

			if runF != nil {
				return runF(opts)
			}
			return createRun(opts)
		},
	}

	cmd.Flags().StringVarP(&opts.Description, "description", "d", "", "Description of the repository")
	cmd.Flags().StringVarP(&opts.Homepage, "homepage", "h", "", "Repository home page `URL`")
	cmd.Flags().StringVarP(&opts.Team, "team", "t", "", "The `name` of the organization team to be granted access")
	cmd.Flags().StringVarP(&opts.Template, "template", "p", "", "Make the new repository based on a template `repository`")
	cmd.Flags().BoolVar(&opts.Public, "public", false, "Make the new repository public")
	cmd.Flags().BoolVar(&opts.Private, "private", false, "Make the new repository private")
	cmd.Flags().BoolVar(&opts.Internal, "internal", false, "Make the new repository internal")
	cmd.Flags().StringVarP(&opts.GitIgnoreTemplate, "gitignore", "g", "", "Specify a gitignore template for the repository")
	cmd.Flags().StringVarP(&opts.LicenseTemplate, "license", "l", "", "Specify an Open Source License for the repository")
	cmd.Flags().StringVarP(&opts.Source, "source", "s", "", "Specify path to local repository to use as source")
	cmd.Flags().StringVarP(&opts.Remote, "remote", "r", "", "Specify remote name for the new repository")
	cmd.Flags().BoolVar(&opts.Push, "push", false, "Push local commits to the new repository")
	cmd.Flags().BoolVarP(&opts.Clone, "clone", "c", false, "Clone the new repository to the current directory")
	cmd.Flags().BoolVar(&opts.DisableIssues, "disable-issues", false, "Disable issues in the new repository")
	cmd.Flags().BoolVar(&opts.DisableWiki, "disable-wiki", false, "Disable wiki in the new repository")
	cmd.Flags().BoolVar(&opts.IncludeAllBranches, "include-all-branches", false, "Include all branches from template repository")
	cmd.Flags().BoolVar(&opts.AddReadme, "add-readme", false, "Add a README file to the new repository")

	// deprecated flags
	cmd.Flags().BoolP("confirm", "y", false, "Skip the confirmation prompt")
	cmd.Flags().BoolVar(&enableIssues, "enable-issues", true, "Enable issues in the new repository")
	cmd.Flags().BoolVar(&enableWiki, "enable-wiki", true, "Enable wiki in the new repository")

	_ = cmd.Flags().MarkDeprecated("confirm", "Pass any argument to skip confirmation prompt")
	_ = cmd.Flags().MarkDeprecated("enable-issues", "Disable issues with `--disable-issues`")
	_ = cmd.Flags().MarkDeprecated("enable-wiki", "Disable wiki with `--disable-wiki`")

	_ = cmd.RegisterFlagCompletionFunc("gitignore", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		httpClient, err := opts.HttpClient()
		if err != nil {
			return nil, cobra.ShellCompDirectiveError
		}
		cfg, err := opts.Config()
		if err != nil {
			return nil, cobra.ShellCompDirectiveError
		}
		hostname, _ := cfg.DefaultHost()
		results, err := listGitIgnoreTemplates(httpClient, hostname)
		if err != nil {
			return nil, cobra.ShellCompDirectiveError
		}
		return results, cobra.ShellCompDirectiveNoFileComp
	})

	_ = cmd.RegisterFlagCompletionFunc("license", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		httpClient, err := opts.HttpClient()
		if err != nil {
			return nil, cobra.ShellCompDirectiveError
		}
		cfg, err := opts.Config()
		if err != nil {
			return nil, cobra.ShellCompDirectiveError
		}
		hostname, _ := cfg.DefaultHost()
		licenses, err := listLicenseTemplates(httpClient, hostname)
		if err != nil {
			return nil, cobra.ShellCompDirectiveError
		}
		var results []string
		for _, license := range licenses {
			results = append(results, fmt.Sprintf("%s\t%s", license.Key, license.Name))
		}
		return results, cobra.ShellCompDirectiveNoFileComp
	})

	return cmd
}

func createRun(opts *CreateOptions) error {
	fromScratch := opts.Source == ""

	if opts.Interactive {
		selected, err := opts.Prompter.Select("What would you like to do?", "", []string{
			"Create a new repository on GitHub from scratch",
			"Push an existing local repository to GitHub",
		})
		if err != nil {
			return err
		}
		fromScratch = selected == 0
	}

	if fromScratch {
		return createFromScratch(opts)
	}
	return createFromLocal(opts)
}

// create new repo on remote host
func createFromScratch(opts *CreateOptions) error {
	httpClient, err := opts.HttpClient()
	if err != nil {
		return err
	}

	var repoToCreate ghrepo.Interface
	cfg, err := opts.Config()
	if err != nil {
		return err
	}

	host, _ := cfg.DefaultHost()

	if opts.Interactive {
		opts.Name, opts.Description, opts.Visibility, err = interactiveRepoInfo(opts.Prompter, "")
		if err != nil {
			return err
		}
		opts.AddReadme, err = opts.Prompter.Confirm("Would you like to add a README file?", false)
		if err != nil {
			return err
		}
		opts.GitIgnoreTemplate, err = interactiveGitIgnore(httpClient, host, opts.Prompter)
		if err != nil {
			return err
		}
		opts.LicenseTemplate, err = interactiveLicense(httpClient, host, opts.Prompter)
		if err != nil {
			return err
		}

		targetRepo := shared.NormalizeRepoName(opts.Name)
		if idx := strings.IndexRune(targetRepo, '/'); idx > 0 {
			targetRepo = targetRepo[0:idx+1] + shared.NormalizeRepoName(targetRepo[idx+1:])
		}
		confirmed, err := opts.Prompter.Confirm(fmt.Sprintf(`This will create "%s" as a %s repository on GitHub. Continue?`, targetRepo, strings.ToLower(opts.Visibility)), true)
		if err != nil {
			return err
		} else if !confirmed {
			return cmdutil.CancelError
		}
	}

	if strings.Contains(opts.Name, "/") {
		var err error
		repoToCreate, err = ghrepo.FromFullName(opts.Name)
		if err != nil {
			return fmt.Errorf("argument error: %w", err)
		}
	} else {
		repoToCreate = ghrepo.NewWithHost("", opts.Name, host)
	}

	input := repoCreateInput{
		Name:               repoToCreate.RepoName(),
		Visibility:         opts.Visibility,
		OwnerLogin:         repoToCreate.RepoOwner(),
		TeamSlug:           opts.Team,
		Description:        opts.Description,
		HomepageURL:        opts.Homepage,
		HasIssuesEnabled:   !opts.DisableIssues,
		HasWikiEnabled:     !opts.DisableWiki,
		GitIgnoreTemplate:  opts.GitIgnoreTemplate,
		LicenseTemplate:    opts.LicenseTemplate,
		IncludeAllBranches: opts.IncludeAllBranches,
		InitReadme:         opts.AddReadme,
	}

	var templateRepoMainBranch string
	if opts.Template != "" {
		var templateRepo ghrepo.Interface
		apiClient := api.NewClientFromHTTP(httpClient)

		templateRepoName := opts.Template
		if !strings.Contains(templateRepoName, "/") {
			currentUser, err := api.CurrentLoginName(apiClient, host)
			if err != nil {
				return err
			}
			templateRepoName = currentUser + "/" + templateRepoName
		}
		templateRepo, err = ghrepo.FromFullName(templateRepoName)
		if err != nil {
			return fmt.Errorf("argument error: %w", err)
		}

		repo, err := api.GitHubRepo(apiClient, templateRepo)
		if err != nil {
			return err
		}

		input.TemplateRepositoryID = repo.ID
		templateRepoMainBranch = repo.DefaultBranchRef.Name
	}

	repo, err := repoCreate(httpClient, repoToCreate.RepoHost(), input)
	if err != nil {
		return err
	}

	cs := opts.IO.ColorScheme()
	isTTY := opts.IO.IsStdoutTTY()
	if isTTY {
		fmt.Fprintf(opts.IO.Out,
			"%s Created repository %s on GitHub\n",
			cs.SuccessIconWithColor(cs.Green),
			ghrepo.FullName(repo))
	} else {
		fmt.Fprintln(opts.IO.Out, repo.URL)
	}

	if opts.Interactive {
		var err error
		opts.Clone, err = opts.Prompter.Confirm("Clone the new repository locally?", true)
		if err != nil {
			return err
		}
	}

	if opts.Clone {
		protocol, err := cfg.GetOrDefault(repo.RepoHost(), "git_protocol")
		if err != nil {
			return err
		}

		remoteURL := ghrepo.FormatRemoteURL(repo, protocol)

		if opts.LicenseTemplate == "" && opts.GitIgnoreTemplate == "" {
			// cloning empty repository or template
			checkoutBranch := ""
			if opts.Template != "" {
				// use the template's default branch
				checkoutBranch = templateRepoMainBranch
			}
			if err := localInit(opts.GitClient, remoteURL, repo.RepoName(), checkoutBranch); err != nil {
				return err
			}
		} else if _, err := opts.GitClient.Clone(context.Background(), remoteURL, []string{}); err != nil {
			return err
		}
	}

	return nil
}

// create repo on remote host from existing local repo
func createFromLocal(opts *CreateOptions) error {
	httpClient, err := opts.HttpClient()
	if err != nil {
		return err
	}

	cs := opts.IO.ColorScheme()
	isTTY := opts.IO.IsStdoutTTY()
	stdout := opts.IO.Out

	cfg, err := opts.Config()
	if err != nil {
		return err
	}
	host, _ := cfg.DefaultHost()

	if opts.Interactive {
		var err error
		opts.Source, err = opts.Prompter.Input("Path to local repository", ".")
		if err != nil {
			return err
		}
	}

	repoPath := opts.Source
	opts.GitClient.RepoDir = repoPath

	var baseRemote string
	if opts.Remote == "" {
		baseRemote = "origin"
	} else {
		baseRemote = opts.Remote
	}

	absPath, err := filepath.Abs(repoPath)
	if err != nil {
		return err
	}

	isRepo, err := isLocalRepo(opts.GitClient)
	if err != nil {
		return err
	}
	if !isRepo {
		if repoPath == "." {
			return fmt.Errorf("current directory is not a git repository. Run `git init` to initialize it")
		}
		return fmt.Errorf("%s is not a git repository. Run `git -C \"%s\" init` to initialize it", absPath, repoPath)
	}

	committed, err := hasCommits(opts.GitClient)
	if err != nil {
		return err
	}
	if opts.Push {
		// fail immediately if trying to push with no commits
		if !committed {
			return fmt.Errorf("`--push` enabled but no commits found in %s", absPath)
		}
	}

	if opts.Interactive {
		opts.Name, opts.Description, opts.Visibility, err = interactiveRepoInfo(opts.Prompter, filepath.Base(absPath))
		if err != nil {
			return err
		}
	}

	var repoToCreate ghrepo.Interface

	// repo name will be currdir name or specified
	if opts.Name == "" {
		repoToCreate = ghrepo.NewWithHost("", filepath.Base(absPath), host)
	} else if strings.Contains(opts.Name, "/") {
		var err error
		repoToCreate, err = ghrepo.FromFullName(opts.Name)
		if err != nil {
			return fmt.Errorf("argument error: %w", err)
		}
	} else {
		repoToCreate = ghrepo.NewWithHost("", opts.Name, host)
	}

	input := repoCreateInput{
		Name:              repoToCreate.RepoName(),
		Visibility:        opts.Visibility,
		OwnerLogin:        repoToCreate.RepoOwner(),
		TeamSlug:          opts.Team,
		Description:       opts.Description,
		HomepageURL:       opts.Homepage,
		HasIssuesEnabled:  !opts.DisableIssues,
		HasWikiEnabled:    !opts.DisableWiki,
		GitIgnoreTemplate: opts.GitIgnoreTemplate,
		LicenseTemplate:   opts.LicenseTemplate,
	}

	repo, err := repoCreate(httpClient, repoToCreate.RepoHost(), input)
	if err != nil {
		return err
	}

	if isTTY {
		fmt.Fprintf(stdout,
			"%s Created repository %s on GitHub\n",
			cs.SuccessIconWithColor(cs.Green),
			ghrepo.FullName(repo))
	} else {
		fmt.Fprintln(stdout, repo.URL)
	}

	protocol, err := cfg.GetOrDefault(repo.RepoHost(), "git_protocol")
	if err != nil {
		return err
	}

	remoteURL := ghrepo.FormatRemoteURL(repo, protocol)

	if opts.Interactive {
		addRemote, err := opts.Prompter.Confirm("Add a remote?", true)
		if err != nil {
			return err
		}
		if !addRemote {
			return nil
		}

		baseRemote, err = opts.Prompter.Input("What should the new remote be called?", "origin")
		if err != nil {
			return err
		}
	}

	if err := sourceInit(opts.GitClient, opts.IO, remoteURL, baseRemote); err != nil {
		return err
	}

	// don't prompt for push if there are no commits
	if opts.Interactive && committed {
		var err error
		opts.Push, err = opts.Prompter.Confirm(fmt.Sprintf("Would you like to push commits from the current branch to %q?", baseRemote), true)
		if err != nil {
			return err
		}
	}

	if opts.Push {
		err := opts.GitClient.Push(context.Background(), baseRemote, "HEAD")
		if err != nil {
			return err
		}
		if isTTY {
			fmt.Fprintf(stdout, "%s Pushed commits to %s\n", cs.SuccessIcon(), remoteURL)
		}
	}
	return nil
}

func sourceInit(gitClient *git.Client, io *iostreams.IOStreams, remoteURL, baseRemote string) error {
	cs := io.ColorScheme()
	remoteAdd, err := gitClient.Command(context.Background(), "remote", "add", baseRemote, remoteURL)
	if err != nil {
		return err
	}
	_, err = remoteAdd.Output()
	if err != nil {
		return fmt.Errorf("%s Unable to add remote %q", cs.FailureIcon(), baseRemote)
	}
	if io.IsStdoutTTY() {
		fmt.Fprintf(io.Out, "%s Added remote %s\n", cs.SuccessIcon(), remoteURL)
	}
	return nil
}

// check if local repository has committed changes
func hasCommits(gitClient *git.Client) (bool, error) {
	hasCommitsCmd, err := gitClient.Command(context.Background(), "rev-parse", "HEAD")
	if err != nil {
		return false, err
	}
	_, err = hasCommitsCmd.Output()
	if err == nil {
		return true, nil
	}

	var execError *exec.ExitError
	if errors.As(err, &execError) {
		exitCode := int(execError.ExitCode())
		if exitCode == 128 {
			return false, nil
		}
		return false, err
	}
	return false, nil
}

// check if path is the top level directory of a git repo
func isLocalRepo(gitClient *git.Client) (bool, error) {
	projectDir, projectDirErr := gitClient.GitDir(context.Background())
	if projectDirErr != nil {
		var execError *exec.ExitError
		if errors.As(projectDirErr, &execError) {
			if exitCode := int(execError.ExitCode()); exitCode == 128 {
				return false, nil
			}
			return false, projectDirErr
		}
	}
	if projectDir != ".git" {
		return false, nil
	}
	return true, nil
}

// clone the checkout branch to specified path
func localInit(gitClient *git.Client, remoteURL, path, checkoutBranch string) error {
	ctx := context.Background()
	gitInit, err := gitClient.Command(ctx, "init", path)
	if err != nil {
		return err
	}
	_, err = gitInit.Output()
	if err != nil {
		return err
	}

	// Clone the client so we do not modify the original client's RepoDir.
	gc := cloneGitClient(gitClient)
	gc.RepoDir = path

	gitRemoteAdd, err := gc.Command(ctx, "remote", "add", "origin", remoteURL)
	if err != nil {
		return err
	}
	_, err = gitRemoteAdd.Output()
	if err != nil {
		return err
	}

	if checkoutBranch == "" {
		return nil
	}

	refspec := fmt.Sprintf("+refs/heads/%[1]s:refs/remotes/origin/%[1]s", checkoutBranch)
	err = gc.Fetch(ctx, "origin", refspec)
	if err != nil {
		return err
	}

	return gc.CheckoutBranch(ctx, checkoutBranch)
}

func interactiveGitIgnore(client *http.Client, hostname string, prompter iprompter) (string, error) {
	confirmed, err := prompter.Confirm("Would you like to add a .gitignore?", false)
	if err != nil {
		return "", err
	} else if !confirmed {
		return "", nil
	}

	templates, err := listGitIgnoreTemplates(client, hostname)
	if err != nil {
		return "", err
	}
	selected, err := prompter.Select("Choose a .gitignore template", "", templates)
	if err != nil {
		return "", err
	}
	return templates[selected], nil
}

func interactiveLicense(client *http.Client, hostname string, prompter iprompter) (string, error) {
	confirmed, err := prompter.Confirm("Would you like to add a license?", false)
	if err != nil {
		return "", err
	} else if !confirmed {
		return "", nil
	}

	licenses, err := listLicenseTemplates(client, hostname)
	if err != nil {
		return "", err
	}
	licenseNames := make([]string, 0, len(licenses))
	for _, license := range licenses {
		licenseNames = append(licenseNames, license.Name)
	}
	selected, err := prompter.Select("Choose a license", "", licenseNames)
	if err != nil {
		return "", err
	}
	return licenses[selected].Key, nil
}

// name, description, and visibility
func interactiveRepoInfo(prompter iprompter, defaultName string) (string, string, string, error) {
	name, err := prompter.Input("Repository name", defaultName)
	if err != nil {
		return "", "", "", err
	}

	description, err := prompter.Input("Description", defaultName)
	if err != nil {
		return "", "", "", err
	}

	visibilityOptions := []string{"Public", "Private", "Internal"}
	selected, err := prompter.Select("Visibility", "Public", visibilityOptions)
	if err != nil {
		return "", "", "", err
	}

	return name, description, strings.ToUpper(visibilityOptions[selected]), nil
}

func cloneGitClient(c *git.Client) *git.Client {
	return &git.Client{
		GhPath:  c.GhPath,
		RepoDir: c.RepoDir,
		GitPath: c.GitPath,
		Stderr:  c.Stderr,
		Stdin:   c.Stdin,
		Stdout:  c.Stdout,
	}
}
