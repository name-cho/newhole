package inpututil

import (
	"os"

	"golang.org/x/term"
)

func IsUsingPipe() bool {
	info, err := os.Stdin.Stat()
	if err != nil {
		return true
	}
	return (info.Mode() & os.ModeCharDevice) == 0
}

func IsTerminal() bool {
	return term.IsTerminal(int(os.Stdin.Fd()))
}
