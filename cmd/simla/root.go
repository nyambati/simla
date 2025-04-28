/*
Copyright © 2025 NAME HERE <EMAIL ADDRESS>
*/
package simla

import (
	"context"

	"github.com/nyambati/simla/internal/config"
	reg "github.com/nyambati/simla/internal/registry"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var cfg *config.Config
var logger *logrus.Logger
var svcRegistry *reg.ServiceRegistry
var ctx = context.Background()

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "simla",
	Short: "A brief description of your application",
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		if err := loadConfig(); err != nil {
			logrus.WithError(err).Fatal("failed to load simla config")
		}
		svcRegistry.Load(ctx)
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
	svcRegistry = reg.NewRegistry(logger.WithField("component", "registry")).(*reg.ServiceRegistry)
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
