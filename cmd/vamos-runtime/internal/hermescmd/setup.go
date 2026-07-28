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
	GatewayURL    string `yaml:"gateway_url"`
	VamosURL      string `yaml:"vamos_url"`
	CallbackToken string `yaml:"callback_token"`
}

func readHostConfig(path string) (hostConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return hostConfig{}, err
	}
	var config hostConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return hostConfig{}, err
	}
	return config, nil
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
	var gatewayURL, vamosURL, callbackToken, configPath string
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
			if strings.TrimSpace(vamosURL) == "" ||
				strings.TrimSpace(callbackToken) == "" {
				return fmt.Errorf(
					"vamos-url and callback-token are required for the Pi completion callback",
				)
			}
			if !strings.HasPrefix(vamosURL, "http://") &&
				!strings.HasPrefix(vamosURL, "https://") {
				return fmt.Errorf("vamos-url must be an http(s) URL")
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
			data, err := yaml.Marshal(hostConfig{
				GatewayURL:    gatewayURL,
				VamosURL:      strings.TrimRight(vamosURL, "/"),
				CallbackToken: callbackToken,
			})
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
				"Gateway verified. Configure Hermes with the saved callback credential and callback URL; set VAMOS_HERMES_CALLBACK_TOKEN to the same credential on Vamos.",
			)
			return nil
		},
	}
	cmd.Flags().StringVar(&gatewayURL, "gateway-url", "", "running Hermes gateway URL")
	cmd.Flags().
		StringVar(&vamosURL, "vamos-url", "", "Vamos server base URL for Pi completion callbacks")
	cmd.Flags().
		StringVar(&callbackToken, "callback-token", "", "machine callback credential shared with Vamos")
	cmd.Flags().StringVar(&configPath, "config", "", "host-local config path")
	_ = cmd.MarkFlagRequired("gateway-url")
	return cmd
}
