package cmd

import (
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	app          string
	cosmosSdk    bool
	cometbft     bool
	keepBlocks   uint64
	keepVersions uint64
	appName      = "cosmprund"
)

// NewRootCmd returns the root command for relayer.
func NewRootCmd() *cobra.Command {
	// RootCmd represents the base command when called without any subcommands
	var rootCmd = &cobra.Command{
		Use:   appName,
		Short: "cosmprund is meant to prune data base history from a cosmos application, avoiding needing to state sync every couple amount of weeks",
	}

	rootCmd.PersistentPreRunE = func(cmd *cobra.Command, _ []string) error {
		// reads `homeDir/config.yaml` into `var config *Config` before each command
		// if err := initConfig(rootCmd); err != nil {
		// 	return err
		// }

		return nil
	}

	// --keep-blocks flag
	rootCmd.PersistentFlags().Uint64VarP(&keepBlocks, "keep-blocks", "b", 10, "set the amount of blocks to keep")
	if err := viper.BindPFlag("keep-blocks", rootCmd.PersistentFlags().Lookup("keep-blocks")); err != nil {
		panic(err)
	}

	// --keep-versions flag
	rootCmd.PersistentFlags().Uint64VarP(&keepVersions, "keep-versions", "v", 10, "set the amount of versions to keep in the application store")
	if err := viper.BindPFlag("keep-versions", rootCmd.PersistentFlags().Lookup("keep-versions")); err != nil {
		panic(err)
	}

	// --app flag
	rootCmd.PersistentFlags().StringVar(&app, "app", "", "set the app you are pruning (supported apps: osmosis)")
	if err := viper.BindPFlag("app", rootCmd.PersistentFlags().Lookup("app")); err != nil {
		panic(err)
	}

	// --cosmos-sdk flag
	rootCmd.PersistentFlags().BoolVar(&cosmosSdk, "cosmos-sdk", true, "set to false if using only with cometbft")
	if err := viper.BindPFlag("cosmos-sdk", rootCmd.PersistentFlags().Lookup("cosmos-sdk")); err != nil {
		panic(err)
	}

	// --cometbft flag
	rootCmd.PersistentFlags().BoolVar(&cometbft, "cometbft", true, "set to false you dont want to prune cometbft data")
	if err := viper.BindPFlag("cometbft", rootCmd.PersistentFlags().Lookup("cometbft")); err != nil {
		panic(err)
	}

	rootCmd.AddCommand(NewPruneCmd())

	return rootCmd
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	cobra.EnableCommandSorting = false

	rootCmd := NewRootCmd()
	rootCmd.SilenceUsage = true
	rootCmd.CompletionOptions.DisableDefaultCmd = true

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
