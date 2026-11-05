package cmd

import (
	"github.com/spf13/cobra"

	"github.com/IvanM0206/bearings/cli/internal/notebook"
)

func (a *app) newRoot() *cobra.Command {
	root := &cobra.Command{
		Use:           "bearings",
		Short:         a.t("cmd.root.short"),
		Long:          a.t("cmd.root.long"),
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.ArbitraryArgs,
		PersistentPreRunE: func(c *cobra.Command, _ []string) error {
			return a.prepare(c.Flags().Changed("lang"))
		},
		RunE: func(c *cobra.Command, args []string) error {
			if len(args) > 0 {
				return a.usagef("err.unknown_command", args[0])
			}
			return c.Help()
		},
	}
	root.CompletionOptions.HiddenDefaultCmd = false
	root.SetFlagErrorFunc(func(_ *cobra.Command, err error) error {
		return &exitError{code: ExitUsage, msg: err.Error()}
	})
	root.SetHelpCommand(&cobra.Command{Hidden: true, Use: "no-help"})

	f := root.PersistentFlags()
	f.StringVar(&a.notebookFlag, "notebook", "", a.t("flag.notebook"))
	f.BoolVar(&a.jsonOut, "json", false, a.t("flag.json"))
	f.StringVar(&a.langFlag, "lang", "", a.t("flag.lang"))
	f.BoolVar(&a.noColor, "no-color", false, a.t("flag.no_color"))

	root.AddCommand(
		a.newInit(),
		a.newSection(),
		a.newLs(),
		a.newShow(),
		a.newSearch(),
		a.newJournal(),
		a.newProgress(),
		a.newPath(),
	)
	return root
}

// fixed is a shell completion function that offers a fixed list.
func fixed(values ...string) func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
	return func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
		return values, cobra.ShellCompDirectiveNoFileComp
	}
}

func statusNames() []string {
	out := make([]string, len(notebook.Statuses))
	for i, s := range notebook.Statuses {
		out[i] = string(s)
	}
	return out
}
