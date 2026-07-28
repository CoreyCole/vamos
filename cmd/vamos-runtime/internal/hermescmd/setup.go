package hermescmd

import (
	"fmt"
	"net/http"
	"os"
	"os/exec"
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
	var gatewayURL, vamosURL, callbackToken, configPath, hermesBin, pluginSource string
	var installPlugin, restartGateway bool
	cmd := &cobra.Command{
		Use:   "setup --gateway-url <url>",
		Short: "Verify a Hermes gateway and save host-local settings",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if installPlugin {
				if strings.TrimSpace(pluginSource) == "" {
					return fmt.Errorf("plugin-source is required with --install-plugin")
				}
				if err := installHermesPlugin(
					cmd,
					hermesBin,
					pluginSource,
					restartGateway,
				); err != nil {
					return err
				}
			}
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
	cmd.Flags().
		BoolVar(&installPlugin, "install-plugin", false, "explicitly install and enable the Vamos Hermes plugin")
	cmd.Flags().StringVar(&hermesBin, "hermes-bin", "hermes", "Hermes executable")
	cmd.Flags().
		StringVar(&pluginSource, "plugin-source", "", "standalone plugin source, for example file:///repo#plugins/hermes-platform")
	cmd.Flags().
		BoolVar(&restartGateway, "restart-gateway", false, "restart Hermes gateway after explicit plugin installation")
	_ = cmd.MarkFlagRequired("gateway-url")
	return cmd
}

func installHermesPlugin(
	cmd *cobra.Command,
	hermesBin, source string,
	restart bool,
) error {
	if strings.TrimSpace(hermesBin) == "" {
		return fmt.Errorf("hermes-bin is required")
	}
	install := exec.CommandContext(
		cmd.Context(),
		hermesBin,
		"plugins",
		"install",
		"--enable",
		source,
	)
	install.Stdout, install.Stderr = cmd.OutOrStdout(), cmd.ErrOrStderr()
	if err := install.Run(); err != nil {
		return fmt.Errorf("install Hermes plugin: %w", err)
	}
	verify := exec.CommandContext(
		cmd.Context(),
		hermesBin,
		"plugins",
		"list",
		"--enabled",
	)
	verify.Stdout, verify.Stderr = cmd.OutOrStdout(), cmd.ErrOrStderr()
	if err := verify.Run(); err != nil {
		return fmt.Errorf("verify enabled Hermes plugin: %w", err)
	}
	if !restart {
		fmt.Fprintln(
			cmd.OutOrStdout(),
			"Plugin installed. Restart Hermes gateway explicitly when ready.",
		)
		return nil
	}
	restartCmd := exec.CommandContext(cmd.Context(), hermesBin, "gateway", "restart")
	restartCmd.Stdout, restartCmd.Stderr = cmd.OutOrStdout(), cmd.ErrOrStderr()
	if err := restartCmd.Run(); err != nil {
		return fmt.Errorf("restart Hermes gateway: %w", err)
	}
	return nil
}
