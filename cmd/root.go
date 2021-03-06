/*
Copyright © 2021 BoxBoat

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package cmd

import (
	"fmt"
	"github.com/boxboat/dockhand-lru-registry/pkg/common"
	log "github.com/sirupsen/logrus"
	"os"
	"strings"

	"github.com/mitchellh/go-homedir"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	CfgFile string
	debug   bool
)

// rootCmdPersistentPreRunE configures logging
func rootCmdPersistentPreRunE(cmd *cobra.Command, args []string) error {
	common.Log.SetOutput(os.Stdout)
	common.Log.SetFormatter(&log.TextFormatter{
		FullTimestamp: true,
	})
	if debug {
		common.Log.SetLevel(log.DebugLevel)
	} else {
		common.Log.SetLevel(log.InfoLevel)
	}
	common.Log.Debugln("rootCmdPersistentPreRunE")
	return nil
}

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:               "dockhand-lru-registry",
	Short:             "ci registry proxy",
	Long:              `ci registry proxy to provide LRU cache to a docker build cache registry`,
	PersistentPreRunE: rootCmdPersistentPreRunE,
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute(version string) {
	rootCmd.Version = version
	if err := rootCmd.Execute(); err != nil {
		common.LogIfError(err)
		os.Exit(1)
	}
}

func init() {
	cobra.OnInitialize(initConfig)

	rootCmd.PersistentFlags().BoolVarP(&debug, "debug", "", false, "debug output")

	// Here you will define your flags and configuration settings.
	// Cobra supports persistent flags, which, if defined here,
	// will be global for your application.
	rootCmd.PersistentFlags().StringVar(
		&CfgFile,
		"config",
		"",
		"config file (default is $HOME/.lru-registry.yaml)")

	_ = viper.BindPFlags(rootCmd.PersistentFlags())
}

// initConfig reads in config file and ENV variables if set.
func initConfig() {
	if CfgFile != "" {
		// Use config file from the flag.
		viper.SetConfigFile(CfgFile)
	} else {
		// Find home directory.
		home, err := homedir.Dir()
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}

		// Search config in home directory with name ".lru-registry" (without extension).
		viper.AddConfigPath(home)
		viper.SetConfigName(".lru-registry")
	}

	viper.SetEnvPrefix("lru")
	replacer := strings.NewReplacer("-", "_")
	viper.SetEnvKeyReplacer(replacer)
	viper.AutomaticEnv() // read in environment variables that match

	// If a config file is found, read it in.
	if err := viper.ReadInConfig(); err == nil {
		common.Log.Infof("Using config file: %s", viper.ConfigFileUsed())
	}
}
