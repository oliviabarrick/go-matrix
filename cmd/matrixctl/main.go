package main

import (
	"context"
	"github.com/justinbarrick/go-matrix/pkg/api"
	"github.com/justinbarrick/go-matrix/pkg/matrix"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"log"
	"os"
	"os/user"
	"path/filepath"
)

var rootCmd = &cobra.Command{
	Use:   "matrixctl",
	Short: "matrixctl is a command line tool for interacting with a matrix server",
	Run:   func(cmd *cobra.Command, args []string) {},
}

var registerCmd = &cobra.Command{
	Use:   "register [server] [username] [password]",
	Short: "Register a new user account with a home server.",
	Args:  cobra.ExactArgs(3),
	Run: func(cmd *cobra.Command, args []string) {
		bot, err := matrix.NewBot(args[0])
		if err != nil {
			log.Fatal(err)
		}

		err = bot.Register(context.TODO(), args[1], args[2])
		if err != nil {
			log.Fatal(err)
		}

		if err := matrix.Serialize(bot, viper.Get("config").(string)); err != nil {
			log.Fatal(err)
		}

		log.Println("Registered!")
	},
}

var loginCmd = &cobra.Command{
	Use:   "login [server] [username] [password]",
	Short: "Login to a user account at a home server.",
	Args:  cobra.ExactArgs(3),
	Run: func(cmd *cobra.Command, args []string) {
		bot, err := matrix.NewBot(args[0])
		if err != nil {
			log.Fatal(err)
		}

		if err := bot.Login(context.TODO(), args[1], args[2]); err != nil {
			log.Fatal(err)
		}

		if err := matrix.Serialize(bot, viper.Get("config").(string)); err != nil {
			log.Fatal(err)
		}

		log.Println("Logged in.")
	},
}

var logoutCmd = &cobra.Command{
	Use:   "logout",
	Short: "Logout the provided access token (or all sessions).",
	Run: func(cmd *cobra.Command, args []string) {
		bot, err := matrix.Unserialize(viper.Get("config").(string))
		if err != nil {
			log.Fatal(err)
		}

		if viper.Get("all").(bool) {
			err = bot.LogoutAll(context.TODO())
		} else {
			err = bot.Logout(context.TODO())
		}

		if err != nil {
			log.Fatal(err)
		}

		log.Println("Logged out.")
	},
}

var joinCmd = &cobra.Command{
	Use:   "join [roomId]",
	Short: "Join a room by id.",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		bot, err := matrix.Unserialize(viper.Get("config").(string))
		if err != nil {
			log.Fatal(err)
		}

		if err := bot.JoinRoom(context.TODO(), args[0]); err != nil {
			log.Fatal(err)
		}
	},
}

var msgCmd = &cobra.Command{
	Use:   "msg [roomId] [message]",
	Short: "Send a message to the given roomId.",
	Args:  cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		bot, err := matrix.Unserialize(viper.Get("config").(string))
		if err != nil {
			log.Fatal(err)
		}

		if viper.Get("encrypted").(bool) {
			err = bot.SendEncrypted(context.TODO(), args[0], args[1])
		} else {
			err = bot.Send(context.TODO(), args[0], args[1])
		}

		if err != nil {
			log.Fatal(err)
		}

		log.Println("Sent message to", args[0])
	},
}

var slack2matrixCmd = &cobra.Command{
	Use:   "slack2matrix [default roomId]",
	Short: "Starts a slack2matrix endpoint that can receive slack webhooks and forward them to matrix.",
	Args:  cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		bot, err := matrix.Unserialize(viper.Get("config").(string))
		if err != nil {
			log.Fatal(err)
		}

		channel := os.Getenv("MATRIX_CHAN")
		if len(args) > 0 {
			channel = args[1]
		}
		api.Api(bot, channel, viper.Get("certPath").(string), viper.Get("keyPath").(string))
	},
}

func main() {
	defaultConfig := os.Getenv("MATRIX_CONFIG")
	if defaultConfig == "" {
		usr, err := user.Current()
		if err != nil {
			log.Fatal(err)
		}
		defaultConfig = filepath.Join(usr.HomeDir, ".matrix/config.json")
	}

	rootCmd.PersistentFlags().StringP("config", "c", defaultConfig, "authentication configuration to load")
	logoutCmd.PersistentFlags().BoolP("all", "a", false, "logout all devices")
	msgCmd.PersistentFlags().BoolP("encrypted", "e", false, "send an encrypted message")
	slack2matrixCmd.PersistentFlags().StringP("cert-path", "", "", "path to TLS certificate")
	slack2matrixCmd.PersistentFlags().StringP("key-path", "", "", "path to TLS key")

	viper.BindPFlag("config", rootCmd.PersistentFlags().Lookup("config"))
	viper.BindPFlag("all", logoutCmd.PersistentFlags().Lookup("all"))
	viper.BindPFlag("encrypted", msgCmd.PersistentFlags().Lookup("encrypted"))
	viper.BindPFlag("certPath", slack2matrixCmd.PersistentFlags().Lookup("cert-path"))
	viper.BindPFlag("keyPath", slack2matrixCmd.PersistentFlags().Lookup("key-path"))

	rootCmd.AddCommand(registerCmd)
	rootCmd.AddCommand(loginCmd)
	rootCmd.AddCommand(logoutCmd)
	rootCmd.AddCommand(joinCmd)
	rootCmd.AddCommand(msgCmd)
	rootCmd.AddCommand(slack2matrixCmd)

	if err := rootCmd.Execute(); err != nil {
		log.Fatal(err)
	}
}
