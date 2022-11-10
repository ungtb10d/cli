package workflow

import (
	cmdDisable "github.com/ungtb10d/cli/v2/pkg/cmd/workflow/disable"
	cmdEnable "github.com/ungtb10d/cli/v2/pkg/cmd/workflow/enable"
	cmdList "github.com/ungtb10d/cli/v2/pkg/cmd/workflow/list"
	cmdRun "github.com/ungtb10d/cli/v2/pkg/cmd/workflow/run"
	cmdView "github.com/ungtb10d/cli/v2/pkg/cmd/workflow/view"
	"github.com/ungtb10d/cli/v2/pkg/cmdutil"
	"github.com/spf13/cobra"
)

func NewCmdWorkflow(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "workflow <command>",
		Short: "View details about GitHub Actions workflows",
		Long:  "List, view, and run workflows in GitHub Actions.",
		Annotations: map[string]string{
			"IsActions": "true",
		},
	}
	cmdutil.EnableRepoOverride(cmd, f)

	cmd.AddCommand(cmdList.NewCmdList(f, nil))
	cmd.AddCommand(cmdEnable.NewCmdEnable(f, nil))
	cmd.AddCommand(cmdDisable.NewCmdDisable(f, nil))
	cmd.AddCommand(cmdView.NewCmdView(f, nil))
	cmd.AddCommand(cmdRun.NewCmdRun(f, nil))

	return cmd
}
