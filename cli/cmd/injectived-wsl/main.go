// injectived-wsl forwards Windows CLI calls to the Injective binary installed
// in WSL without routing JSON transaction arguments through cmd.exe.
package main

import (
	"encoding/base64"
	"errors"
	"os"
	"os/exec"
	"strings"
)

func main() {
	// wsl.exe re-parses Windows command lines and corrupts JSON arguments that
	// contain quotes. Encode each argument into a shell-safe alphabet, then
	// reconstruct the original argv inside WSL before execing injectived.
	const decoder = `set -e
args=()
for encoded in "$@"; do
  args+=("$(printf %s "$encoded" | base64 -d)")
done
exec injectived "${args[@]}"`
	encoded := make([]string, 0, len(os.Args)-1)
	for _, arg := range os.Args[1:] {
		encoded = append(encoded, base64.StdEncoding.EncodeToString([]byte(arg)))
	}
	command := "printf %s " + base64.StdEncoding.EncodeToString([]byte(decoder)) +
		" | base64 -d | bash -s -- " + strings.Join(encoded, " ")
	args := []string{"-d", "Ubuntu-24.04", "--", "bash", "-lc", command}
	cmd := exec.Command("wsl.exe", args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			os.Exit(exitErr.ExitCode())
		}
		os.Exit(1)
	}
}
