package cmd

import "github.com/spf13/cobra"

func tokenCmd(config *walletConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use: "token",
		Run: func(cmd *cobra.Command, args []string) {
			consoleWriter.Println("Error: must specify a subcommand like create-type, send etc")
		},
	}
	cmd.AddCommand(tokenCmdCreateType(config))
	cmd.AddCommand(tokenCmdCreateToken(config))
	cmd.AddCommand(tokenCmdSend(config))
	cmd.AddCommand(tokenCmdDC(config))
	cmd.AddCommand(tokenCmdList(config))
	return cmd
}

func tokenCmdCreateType(config *walletConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use: "create-type",
		Run: func(cmd *cobra.Command, args []string) {
			consoleWriter.Println("Error: must specify a subcommand: fungible|non-fungible")
		},
	}
	cmd.AddCommand(tokenCmdCreateTypeFungible(config))
	cmd.AddCommand(tokenCmdCreateTypeNonFungible(config))
	return cmd
}

func tokenCmdCreateTypeFungible(config *walletConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use: "fungible",
		RunE: func(cmd *cobra.Command, args []string) error {
			return execTokenCmdCreateTypeFungible(cmd, config)
		},
	}
	return cmd
}

func execTokenCmdCreateTypeFungible(cmd *cobra.Command, config *walletConfig) error {
	// TODO
	return nil
}

func tokenCmdCreateTypeNonFungible(config *walletConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use: "non-fungible",
		RunE: func(cmd *cobra.Command, args []string) error {
			return execTokenCmdCreateTypeNonFungible(cmd, config)
		},
	}
	return cmd
}

func execTokenCmdCreateTypeNonFungible(cmd *cobra.Command, config *walletConfig) error {
	// TODO
	return nil
}

func tokenCmdCreateToken(config *walletConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use: "create",
		Run: func(cmd *cobra.Command, args []string) {
			consoleWriter.Println("Error: must specify a subcommand: fungible|non-fungible")
		},
	}
	cmd.AddCommand(tokenCmdCreateTokenFungible(config))
	cmd.AddCommand(tokenCmdCreateTokenNonFungible(config))
	return cmd
}

func tokenCmdCreateTokenFungible(config *walletConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use: "fungible",
		RunE: func(cmd *cobra.Command, args []string) error {
			return execTokenCmdCreateTokenFungible(cmd, config)
		},
	}
	return cmd
}

func execTokenCmdCreateTokenFungible(cmd *cobra.Command, config *walletConfig) error {
	// TODO
	return nil
}

func tokenCmdCreateTokenNonFungible(config *walletConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use: "non-fungible",
		RunE: func(cmd *cobra.Command, args []string) error {
			return execTokenCmdCreateTokenNonFungible(cmd, config)
		},
	}
	return cmd
}

func execTokenCmdCreateTokenNonFungible(cmd *cobra.Command, config *walletConfig) error {
	// TODO
	return nil
}

func tokenCmdSend(config *walletConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use: "create",
		Run: func(cmd *cobra.Command, args []string) {
			consoleWriter.Println("Error: must specify a subcommand: fungible|non-fungible")
		},
	}
	cmd.AddCommand(tokenCmdSendFungible(config))
	cmd.AddCommand(tokenCmdSendNonFungible(config))
	return cmd
}

func tokenCmdSendFungible(config *walletConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use: "fungible",
		RunE: func(cmd *cobra.Command, args []string) error {
			return execTokenCmdSendFungible(cmd, config)
		},
	}
	return cmd
}

func execTokenCmdSendFungible(cmd *cobra.Command, config *walletConfig) error {
	// TODO
	return nil
}

func tokenCmdSendNonFungible(config *walletConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use: "non-fungible",
		RunE: func(cmd *cobra.Command, args []string) error {
			return execTokenCmdSendNonFungible(cmd, config)
		},
	}
	return cmd
}

func execTokenCmdSendNonFungible(cmd *cobra.Command, config *walletConfig) error {
	// TODO
	return nil
}

func tokenCmdDC(config *walletConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use: "collect-dust",
		RunE: func(cmd *cobra.Command, args []string) error {
			return execTokenCmdDC(cmd, config)
		},
	}
	return cmd
}

func execTokenCmdDC(cmd *cobra.Command, config *walletConfig) error {
	// TODO
	return nil
}

func tokenCmdList(config *walletConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use: "list",
		Run: func(cmd *cobra.Command, args []string) {
			consoleWriter.Println("Error: must specify a subcommand: fungible|non-fungible")
		},
	}
	cmd.AddCommand(tokenCmdListFungible(config))
	cmd.AddCommand(tokenCmdListNonFungible(config))
	return cmd
}

func tokenCmdListFungible(config *walletConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use: "fungible",
		RunE: func(cmd *cobra.Command, args []string) error {
			return execTokenCmdListFungible(cmd, config)
		},
	}
	return cmd
}

func execTokenCmdListFungible(cmd *cobra.Command, config *walletConfig) error {
	// TODO
	return nil
}

func tokenCmdListNonFungible(config *walletConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use: "non-fungible",
		RunE: func(cmd *cobra.Command, args []string) error {
			return execTokenCmdListNonFungible(cmd, config)
		},
	}
	return cmd
}

func execTokenCmdListNonFungible(cmd *cobra.Command, config *walletConfig) error {
	// TODO
	return nil
}
