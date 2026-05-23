package verifycmd

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"
)

type RemoteProofConfig struct {
	SSHHost              string
	ShellTemplate        string
	DNSServer            string
	ExpectIP             string
	RequireRemoteTailnet bool
}

type RemoteCommandResult struct {
	Command string
	Output  string
}

type remoteCommandRunner func(ctx context.Context, template, host string, command []string) (RemoteCommandResult, error)

func RunRemoteCommand(
	ctx context.Context,
	template, host string,
	command []string,
) (RemoteCommandResult, error) {
	if err := validateRemoteSSHHost(host); err != nil {
		return RemoteCommandResult{}, err
	}
	remoteShellCommand := shellJoin(command)
	if strings.TrimSpace(template) == "" || template == "ssh {host} -- {command}" {
		cmd := exec.CommandContext(
			ctx,
			"ssh",
			host,
			"--",
			"sh",
			"-lc",
			remoteShellCommand,
		)
		out, err := cmd.CombinedOutput()
		return RemoteCommandResult{
			Command: "ssh " + shellQuote(
				host,
			) + " -- sh -lc " + shellQuote(
				remoteShellCommand,
			),
			Output: string(out),
		}, err
	}
	shellCommand := strings.ReplaceAll(template, "{host}", shellQuote(host))
	shellCommand = strings.ReplaceAll(shellCommand, "{command}", remoteShellCommand)
	cmd := exec.CommandContext(ctx, "sh", "-c", shellCommand)
	out, err := cmd.CombinedOutput()
	return RemoteCommandResult{Command: shellCommand, Output: string(out)}, err
}

func validateRemoteSSHHost(host string) error {
	if host == "" || strings.HasPrefix(host, "-") ||
		strings.ContainsAny(host, " \t\n\r;&|`$<>(){}[]'\"") {
		return fmt.Errorf("invalid remote SSH host %q", host)
	}
	return nil
}

func shellJoin(args []string) string {
	quoted := make([]string, 0, len(args))
	for _, arg := range args {
		quoted = append(quoted, shellQuote(arg))
	}
	return strings.Join(quoted, " ")
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

func writeRemoteResult(out io.Writer, result RemoteCommandResult) {
	fmt.Fprintf(out, "$ %s\n%s", result.Command, result.Output)
}

func remoteDNSCommand(host string) []string {
	return []string{"getent", "hosts", host}
}

func remoteDNSFallbackCommand(host string) []string {
	return []string{"resolvectl", "query", host}
}

func directDNSCommand(server, host string) []string {
	return []string{"dig", "@" + server, host, "+short"}
}

func remoteHTTPSCommand(rawURL string) []string {
	return []string{"curl", "-I", "--fail", "--show-error", "--silent", rawURL}
}
