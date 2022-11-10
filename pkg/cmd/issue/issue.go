package issue

import (
	"github.com/MakeNowJust/heredoc"
	cmdClose "github.com/ungtb10d/cli/v2/pkg/cmd/issue/close"
	cmdComment "github.com/ungtb10d/cli/v2/pkg/cmd/issue/comment"
	cmdCreate "github.com/ungtb10d/cli/v2/pkg/cmd/issue/create"
	cmdDelete "github.com/ungtb10d/cli/v2/pkg/cmd/issue/delete"
	cmdDevelop "github.com/ungtb10d/cli/v2/pkg/cmd/issue/develop"
	cmdEdit "github.com/ungtb10d/cli/v2/pkg/cmd/issue/edit"
	cmdList "github.com/ungtb10d/cli/v2/pkg/cmd/issue/list"
	cmdPin "github.com/ungtb10d/cli/v2/pkg/cmd/issue/pin"
	cmdReopen "github.com/ungtb10d/cli/v2/pkg/cmd/issue/reopen"
	cmdStatus "github.com/ungtb10d/cli/v2/pkg/cmd/issue/status"
	cmdTransfer "github.com/ungtb10d/cli/v2/pkg/cmd/issue/transfer"
	cmdUnpin "github.com/ungtb10d/cli/v2/pkg/cmd/issue/unpin"
	cmdView "github.com/ungtb10d/cli/v2/pkg/cmd/issue/view"
	"github.com/ungtb10d/cli/v2/pkg/cmdutil"
	"github.com/spf13/cobra"
)

func NewCmdIssue(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "issue <command>",
		Short: "Manage issues",
		Long:  `Work with GitHub issues.`,
		Example: heredoc.Doc(`
			$ gh issue list
			$ gh issue create --label bug
			$ gh issue view 123 --web
		`),
		Annotations: map[string]string{
			"IsCore": "true",
			"help:arguments": heredoc.Doc(`
				An issue can be supplied as argument in any of the following formats:
				- by number, e.g. "123"; or
				- by URL, e.g. "https://github.com/OWNER/REPO/issues/123".
			`),
		},
	}

	cmdutil.EnableRepoOverride(cmd, f)

	cmd.AddCommand(cmdClose.NewCmdClose(f, nil))
	cmd.AddCommand(cmdCreate.NewCmdCreate(f, nil))
	cmd.AddCommand(cmdList.NewCmdList(f, nil))
	cmd.AddCommand(cmdReopen.NewCmdReopen(f, nil))
	cmd.AddCommand(cmdStatus.NewCmdStatus(f, nil))
	cmd.AddCommand(cmdView.NewCmdView(f, nil))
	cmd.AddCommand(cmdComment.NewCmdComment(f, nil))
	cmd.AddCommand(cmdDelete.NewCmdDelete(f, nil))
	cmd.AddCommand(cmdEdit.NewCmdEdit(f, nil))
	cmd.AddCommand(cmdTransfer.NewCmdTransfer(f, nil))
	cmd.AddCommand(cmdDevelop.NewCmdDevelop(f, nil))
	cmd.AddCommand(cmdPin.NewCmdPin(f, nil))
	cmd.AddCommand(cmdUnpin.NewCmdUnpin(f, nil))

	return cmd
}
