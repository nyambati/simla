/*
Copyright © 2025 NAME HERE <EMAIL ADDRESS>
*/
package simla

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/nyambati/simla/internal/config"
	"github.com/nyambati/simla/internal/registry"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var cfg *config.Config
var logger *logrus.Logger
var svcRegistry *registry.ServiceRegistry

// ctx is a signal-aware context. It is cancelled when the process receives
// SIGINT or SIGTERM, triggering graceful shutdown across all subcommands.
var ctx, stopCtx = signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "simla",
	Short: "A brief description of your application",
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		if err := loadConfig(); err != nil {
			logrus.WithError(err).Fatal("failed to load simla config")
		}
		if err := svcRegistry.Load(ctx); err != nil {
			logrus.WithError(err).Fatal("failed to load registry")
		}
	},
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		logrus.WithError(err).Fatal("error occured while running simla")
	}
}

func init() {
	logger = logrus.New()
	logger.SetFormatter(&logrus.TextFormatter{FullTimestamp: true})
	reg, err := registry.NewRegistry(logger.WithField("component", "registry"))
	if err != nil {
		logrus.Fatal(err)
	}
	svcRegistry = reg.(*registry.ServiceRegistry)

}

func loadConfig() error {
	cfg = &config.Config{
		Host:     "127.0.0.1",
		Services: make(map[string]config.Service),
		APIGateway: config.APIGateway{
			Port:  "8080",
			Stage: "v1",
		},
	}

	viper.AddConfigPath(".")
	viper.SetConfigName(".simla")
	viper.SetConfigType("yaml")

	if err := viper.ReadInConfig(); err != nil {
		return err
	}

	return viper.Unmarshal(cfg)
}
