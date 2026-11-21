package closehandler

import (
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/name-cho/newhole/internal/pkg/communication"
	"github.com/name-cho/newhole/internal/pkg/inpututil"
	"golang.org/x/term"
)

var (
	once          sync.Once
	interruptOnce sync.Once
	quitChannel   = make(chan bool, 1)
)

func SetupCloseHandler(feedbackFormURL string) {
	once.Do(func() {
		signals := make(chan os.Signal, 4)
		signal.Notify(signals, os.Interrupt, syscall.SIGTERM)

		var terminalState *term.State
		if !inpututil.IsUsingPipe() && term.IsTerminal(int(os.Stdin.Fd())) {
			state, err := term.GetState(int(os.Stdin.Fd()))
			if err == nil {
				terminalState = state
			} else {
				communication.Debug("unable to save the terminal state: " + err.Error())
			}
		}

		go func() {
			<-signals
			interruptOnce.Do(func() {
				go func() {
					<-signals
					os.Exit(130)
				}()
			})
			if terminalState != nil {
				_ = term.Restore(int(os.Stdin.Fd()), terminalState)
			}
			communication.ApplicationStop()
			select {
			case quitChannel <- true:
			default:
			}
			os.Exit(0)
		}()
	})
}

func QuitChannel() <-chan bool {
	return quitChannel
}
