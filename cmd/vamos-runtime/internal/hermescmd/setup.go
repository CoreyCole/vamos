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
	IngressToken  string `yaml:"ingress_token"`
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

func normalizeGatewayBaseURL(value string) string {
	return strings.TrimRight(strings.TrimSpace(value), "/")
}

func gatewayHealthURL(baseURL string) string {
	return normalizeGatewayBaseURL(baseURL) + "/health"
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
	var gatewayURL, vamosURL, ingressToken, callbackToken, configPath, hermesBin, pluginSource string
	var installPlugin, restartGateway bool
	cmd := &cobra.Command{
		Use:   "setup --gateway-url <adapter-base-url>",
		Short: "Verify a Hermes adapter health endpoint and save host-local settings",
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
			gatewayURL = normalizeGatewayBaseURL(gatewayURL)
			if !strings.HasPrefix(gatewayURL, "http://") &&
				!strings.HasPrefix(gatewayURL, "https://") {
				return fmt.Errorf(
					"gateway-url must be the http(s) Hermes adapter base URL",
				)
			}
			client := &http.Client{Timeout: 5 * time.Second}
			response, err := client.Get(gatewayHealthURL(gatewayURL))
			if err != nil {
				return fmt.Errorf("verify Hermes adapter GET /health: %w", err)
			}
			response.Body.Close()
			if response.StatusCode >= 400 {
				return fmt.Errorf(
					"verify Hermes adapter GET /health: %s",
					response.Status,
				)
			}
			if strings.TrimSpace(vamosURL) == "" ||
				strings.TrimSpace(ingressToken) == "" ||
				strings.TrimSpace(callbackToken) == "" {
				return fmt.Errorf(
					"vamos-url, ingress-token, and callback-token are required",
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
				IngressToken:  ingressToken,
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
	cmd.Flags().StringVar(
		&gatewayURL,
		"gateway-url",
		"",
		"Hermes adapter base URL (/health and /vamos/manager-wake are appended)",
	)
	cmd.Flags().
		StringVar(&vamosURL, "vamos-url", "", "Vamos server base URL for Pi completion callbacks")
	cmd.Flags().
		StringVar(&ingressToken, "ingress-token", "", "machine ingress credential for manager wake delivery")
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
	_ = cmd.MarkFlagRequired("ingress-token")
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
