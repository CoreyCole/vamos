package hermescmd

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

type hostConfig struct {
	GatewayURL string `yaml:"gateway_url"`
}

func defaultConfigPath() (string, error) {
	base := os.Getenv("VAMOS_HERMES_CONFIG")
	if base != "" {
		return base, nil
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "vamos", "hermes.yaml"), nil
}

func newSetupCommand() *cobra.Command {
	var gatewayURL, configPath string
	cmd := &cobra.Command{
		Use:   "setup --gateway-url <url>",
		Short: "Verify a Hermes gateway and save host-local settings",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if !strings.HasPrefix(gatewayURL, "http://") &&
				!strings.HasPrefix(gatewayURL, "https://") {
				return fmt.Errorf("gateway-url must be an http(s) URL")
			}
			client := &http.Client{Timeout: 5 * time.Second}
			response, err := client.Get(gatewayURL)
			if err != nil {
				return fmt.Errorf("verify Hermes gateway: %w", err)
			}
			response.Body.Close()
			if response.StatusCode >= 400 {
				return fmt.Errorf("verify Hermes gateway: %s", response.Status)
			}
			if configPath == "" {
				configPath, err = defaultConfigPath()
				if err != nil {
					return err
				}
			}
			if err := os.MkdirAll(filepath.Dir(configPath), 0o700); err != nil {
				return err
			}
			data, err := yaml.Marshal(hostConfig{GatewayURL: gatewayURL})
			if err != nil {
				return err
			}
			if err := os.WriteFile(configPath, data, 0o600); err != nil {
				return err
			}
			if err := os.Chmod(configPath, 0o600); err != nil {
				return err
			}
			fmt.Fprintln(
				cmd.OutOrStdout(),
				"Gateway verified. Configure the Hermes Vamos platform plugin with this host's callback credential and callback URL.",
			)
			return nil
		},
	}
	cmd.Flags().StringVar(&gatewayURL, "gateway-url", "", "running Hermes gateway URL")
	cmd.Flags().StringVar(&configPath, "config", "", "host-local config path")
	_ = cmd.MarkFlagRequired("gateway-url")
	return cmd
}
