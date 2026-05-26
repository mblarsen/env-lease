package cmd

import (
	"io"
	"log/slog"
	"os"
	"os/exec"

	"github.com/mblarsen/env-lease/internal/presentation"
)

func HandleDirenv(noDirenv bool, out io.Writer) {
	if noDirenv {
		slog.Debug("`--no-direnv` flag is set, skipping direnv execution.")
		presenter.Print(out, presentation.MessageDirenvModified)
		return
	}

	if _, err := exec.LookPath("direnv"); err == nil {
		cmd := exec.Command("direnv", "allow")
		tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
		if err != nil {
			presenter.Print(out, presentation.MessageDirenvTTYFailed, err)
			return
		}
		defer tty.Close()
		cmd.Stdout = tty
		cmd.Stderr = tty
		if err := cmd.Run(); err != nil {
			presenter.Print(out, presentation.MessageDirenvAllowFailed, err)
		}
	}
}
