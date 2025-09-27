package cmd

import (
	"fmt"
	"io"
	"os"
	"os/exec"
)

func HandleDirenv(noDirenv bool, out io.Writer) {
	if noDirenv {
		fmt.Fprintln(out, ".envrc modified. Run 'direnv allow' to apply changes.")
		return
	}

	if _, err := exec.LookPath("direnv"); err == nil {
		cmd := exec.Command("direnv", "allow")
		tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
		if err != nil {
			fmt.Fprintf(out, "Failed to open tty: %v\n", err)
			return
		}
		defer tty.Close()
		cmd.Stdout = tty
		cmd.Stderr = tty
		if err := cmd.Run(); err != nil {
			fmt.Fprintf(out, "direnv allow failed: %v\n", err)
		}
	}
}
