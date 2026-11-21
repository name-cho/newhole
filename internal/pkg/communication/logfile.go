package communication

import (
	"io"

	"github.com/name-cho/newhole/config"
)

func SetLogFile(writer io.Writer) {
	if logger, ok := communicationMechanism.(*stdoutLogger); ok {
		logger.messageMutex.Lock()
		defer logger.messageMutex.Unlock()
		logger.file = writer
	}
	if logger, ok := defaultLogger.(*stdoutLogger); ok {
		logger.file = writer
	}
}

func SetVerbose(verbose bool) {
	config.Config.Display.Verbose = verbose
	if logger, ok := communicationMechanism.(*stdoutLogger); ok {
		logger.messageMutex.Lock()
		defer logger.messageMutex.Unlock()
		logger.verbose = verbose
		return
	}
	if logger, ok := defaultLogger.(*stdoutLogger); ok {
		logger.verbose = verbose
	}
}
