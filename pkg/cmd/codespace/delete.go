package codespace

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/AlecAivazis/survey/v2"
	"github.com/MakeNowJust/heredoc"
	"github.com/ungtb10d/cli/v2/internal/codespaces/api"
	"github.com/ungtb10d/cli/v2/pkg/cmdutil"
	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"
)

type deleteOptions struct {
	deleteAll     bool
	skipConfirm   bool
	codespaceName string
	repoFilter    string
	keepDays      uint16
	orgName       string
	userName      string

	isInteractive bool
	now           func() time.Time
	prompter      prompter
}

//go:generate moq -fmt goimports -rm -skip-ensure -out mock_prompter.go . prompter
type prompter interface {
	Confirm(message string) (bool, error)
}

func newDeleteCmd(app *App) *cobra.Command {
	opts := deleteOptions{
		isInteractive: hasTTY,
		now:           time.Now,
		prompter:      &surveyPrompter{},
	}

	deleteCmd := &cobra.Command{
		Use:   "delete",
		Short: "Delete codespaces",
		Long: heredoc.Doc(`
			Delete codespaces based on selection criteria.

			All codespaces for the authenticated user can be deleted, as well as codespaces for a
			specific repository. Alternatively, only codespaces older than N days can be deleted.

			Organization administrators may delete any codespace billed to the organization.
		`),
		Args: noArgsConstraint,
		RunE: func(cmd *cobra.Command, args []string) error {
			if opts.deleteAll && opts.repoFilter != "" {
				return cmdutil.FlagErrorf("both `--all` and `--repo` is not supported")
			}
			if opts.orgName != "" && opts.codespaceName != "" && opts.userName == "" {
				return cmdutil.FlagErrorf("using `--org` with `--codespace` requires `--user`")
			}
			return app.Delete(cmd.Context(), opts)
		},
	}

	deleteCmd.Flags().StringVarP(&opts.codespaceName, "codespace", "c", "", "Name of the codespace")
	deleteCmd.Flags().BoolVar(&opts.deleteAll, "all", false, "Delete all codespaces")
	deleteCmd.Flags().StringVarP(&opts.repoFilter, "repo", "r", "", "Delete codespaces for a `repository`")
	deleteCmd.Flags().BoolVarP(&opts.skipConfirm, "force", "f", false, "Skip confirmation for codespaces that contain unsaved changes")
	deleteCmd.Flags().Uint16Var(&opts.keepDays, "days", 0, "Delete codespaces older than `N` days")
	deleteCmd.Flags().StringVarP(&opts.orgName, "org", "o", "", "The `login` handle of the organization (admin-only)")
	deleteCmd.Flags().StringVarP(&opts.userName, "user", "u", "", "The `username` to delete codespaces for (used with --org)")

	return deleteCmd
}

func (a *App) Delete(ctx context.Context, opts deleteOptions) (err error) {
	var codespaces []*api.Codespace
	nameFilter := opts.codespaceName
	if nameFilter == "" {
		a.StartProgressIndicatorWithLabel("Fetching codespaces")
		codespaces, err = a.apiClient.ListCodespaces(ctx, api.ListCodespacesOptions{OrgName: opts.orgName, UserName: opts.userName})
		a.StopProgressIndicator()
		if err != nil {
			return fmt.Errorf("error getting codespaces: %w", err)
		}

		if !opts.deleteAll && opts.repoFilter == "" {
			includeUsername := opts.orgName != ""
			c, err := chooseCodespaceFromList(ctx, codespaces, includeUsername)
			if err != nil {
				return fmt.Errorf("error choosing codespace: %w", err)
			}
			nameFilter = c.Name
		}
	} else {
		a.StartProgressIndicatorWithLabel("Fetching codespace")

		var codespace *api.Codespace
		var err error

		if opts.orgName == "" || opts.userName == "" {
			codespace, err = a.apiClient.GetCodespace(ctx, nameFilter, false)
		} else {
			codespace, err = a.apiClient.GetOrgMemberCodespace(ctx, opts.orgName, opts.userName, opts.codespaceName)
		}
		a.StopProgressIndicator()
		if err != nil {
			return fmt.Errorf("error fetching codespace information: %w", err)
		}

		codespaces = []*api.Codespace{codespace}
	}

	codespacesToDelete := make([]*api.Codespace, 0, len(codespaces))
	lastUpdatedCutoffTime := opts.now().AddDate(0, 0, -int(opts.keepDays))
	for _, c := range codespaces {
		if nameFilter != "" && c.Name != nameFilter {
			continue
		}
		if opts.repoFilter != "" && !strings.EqualFold(c.Repository.FullName, opts.repoFilter) {
			continue
		}
		if opts.keepDays > 0 {
			t, err := time.Parse(time.RFC3339, c.LastUsedAt)
			if err != nil {
				return fmt.Errorf("error parsing last_used_at timestamp %q: %w", c.LastUsedAt, err)
			}
			if t.After(lastUpdatedCutoffTime) {
				continue
			}
		}
		if !opts.skipConfirm {
			confirmed, err := confirmDeletion(opts.prompter, c, opts.isInteractive)
			if err != nil {
				return fmt.Errorf("unable to confirm: %w", err)
			}
			if !confirmed {
				continue
			}
		}
		codespacesToDelete = append(codespacesToDelete, c)
	}

	if len(codespacesToDelete) == 0 {
		return errors.New("no codespaces to delete")
	}

	progressLabel := "Deleting codespace"
	if len(codespacesToDelete) > 1 {
		progressLabel = "Deleting codespaces"
	}
	a.StartProgressIndicatorWithLabel(progressLabel)
	defer a.StopProgressIndicator()

	var g errgroup.Group
	for _, c := range codespacesToDelete {
		codespaceName := c.Name
		g.Go(func() error {
			if err := a.apiClient.DeleteCodespace(ctx, codespaceName, opts.orgName, opts.userName); err != nil {
				a.errLogger.Printf("error deleting codespace %q: %v\n", codespaceName, err)
				return err
			}
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return errors.New("some codespaces failed to delete")
	}
	return nil
}

func confirmDeletion(p prompter, apiCodespace *api.Codespace, isInteractive bool) (bool, error) {
	cs := codespace{apiCodespace}
	if !cs.hasUnsavedChanges() {
		return true, nil
	}
	if !isInteractive {
		return false, fmt.Errorf("codespace %s has unsaved changes (use --force to override)", cs.Name)
	}
	return p.Confirm(fmt.Sprintf("Codespace %s has unsaved changes. OK to delete?", cs.Name))
}

type surveyPrompter struct{}

func (p *surveyPrompter) Confirm(message string) (bool, error) {
	var confirmed struct {
		Confirmed bool
	}
	q := []*survey.Question{
		{
			Name: "confirmed",
			Prompt: &survey.Confirm{
				Message: message,
			},
		},
	}
	if err := ask(q, &confirmed); err != nil {
		return false, fmt.Errorf("failed to prompt: %w", err)
	}

	return confirmed.Confirmed, nil
}
