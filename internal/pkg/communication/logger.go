package communication

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/briandowns/spinner"
	"github.com/logrusorgru/aurora"
	"github.com/mattn/go-colorable"
	"github.com/mdp/qrterminal"
	"github.com/name-cho/newhole/config"
	coreModels "github.com/name-cho/newhole/internal/app/newhole/models"
	authModels "github.com/name-cho/newhole/internal/pkg/token/models"
	"github.com/name-cho/newhole/internal/pkg/urlmaker"
)

type stdoutLogger struct {
	output       io.Writer
	file         io.Writer
	loader       *spinner.Spinner
	loaderActive bool
	messageMutex sync.Mutex
	verbose      bool
	printed      int
	remoteSiteID string
}

func (l *stdoutLogger) writeFile(level string, message string) {
	if l.file == nil {
		return
	}
	fmt.Fprintf(l.file, "%s %s %s\n", time.Now().Format(time.RFC3339), level, message)
}

func NewStdOutLogger() Mechanism {
	logger := &stdoutLogger{
		output: colorable.NewColorableStdout(),
	}
	logger.loader = spinner.New(spinner.CharSets[9], 100*time.Millisecond)
	logger.loader.Writer = logger.output
	return logger
}

func (l *stdoutLogger) stopLoader() {
	if l.loaderActive {
		l.loader.Stop()
		l.loaderActive = false
	}
}

func (l *stdoutLogger) line(level string, message string) {
	l.stopLoader()
	l.writeFile(level, message)
	if l.printed > 0 {
		fmt.Fprintln(l.output)
	}
	l.printed++
	timestamp := time.Now().Format("15:04:05")
	prefix := level
	switch level {
	case "DBG":
		prefix = aurora.Gray(10, level).String()
	case "INF":
		prefix = aurora.Blue(level).String()
	case "WRN":
		prefix = aurora.Yellow(level).String()
	case "ERR", "FTL":
		prefix = aurora.Red(level).String()
	}
	fmt.Fprintf(l.output, "%s %s %s\n", aurora.Gray(8, timestamp), prefix, message)
}

func (l *stdoutLogger) TunnelDebug(tunnelID string, message string) {
	if !l.verbose && !config.Config.Display.Verbose {
		l.writeFile("DBG", message)
		return
	}
	l.messageMutex.Lock()
	defer l.messageMutex.Unlock()
	l.line("DBG", message)
}

func (l *stdoutLogger) TunnelInfo(tunnelID string, message string) {
	l.messageMutex.Lock()
	defer l.messageMutex.Unlock()
	l.line("INF", message)
}

func (l *stdoutLogger) TunnelWarn(tunnelID string, message string) {
	l.messageMutex.Lock()
	defer l.messageMutex.Unlock()
	l.line("WRN", message)
}

func (l *stdoutLogger) TunnelError(tunnelID string, message string) {
	l.messageMutex.Lock()
	defer l.messageMutex.Unlock()
	l.line("ERR", message)
}

func (l *stdoutLogger) Debug(message string) {
	l.TunnelDebug("", message)
}

func (l *stdoutLogger) Info(message string) {
	l.TunnelInfo("", message)
}

func (l *stdoutLogger) Warn(message string) {
	l.TunnelWarn("", message)
}

func (l *stdoutLogger) Error(message string) {
	l.TunnelError("", message)
}

func (l *stdoutLogger) Fatal(message string) {
	l.messageMutex.Lock()
	l.line("FTL", message)
	l.messageMutex.Unlock()
	os.Exit(1)
}

func (l *stdoutLogger) HostnameHistory() {}

func (l *stdoutLogger) ApplicationStart(loggedIn bool, idToken string) {
	l.messageMutex.Lock()
	defer l.messageMutex.Unlock()
	l.stopLoader()
	fmt.Fprint(l.output, aurora.Cyan("Newhole"))
	fmt.Fprint(l.output, aurora.Italic(" - end to end TLS encrypted TCP tunnels from your machine to the world"))
	fmt.Fprintln(l.output)
	fmt.Fprintln(l.output)
	if config.Config.TLS.Insecure {
		fmt.Fprintln(l.output, aurora.Yellow("TLS verification is disabled, do not use this in production"))
	}
}

func (l *stdoutLogger) ApplicationStop() {
	l.messageMutex.Lock()
	defer l.messageMutex.Unlock()
	l.stopLoader()
	fmt.Fprintln(l.output)
	fmt.Fprintln(l.output, "Goodbye")
	fmt.Fprintln(l.output, aurora.Cyan(fmt.Sprintf("Thank you for using Newhole. Report problems or suggestions at %s", config.Config.FeedbackFormURL)))
}

func (l *stdoutLogger) TunnelStart(tunnelID string) {
	l.messageMutex.Lock()
	defer l.messageMutex.Unlock()
	if config.Config.Display.Verbose {
		l.line("DBG", "Tunnel starting up...")
	}
}

func (l *stdoutLogger) TunnelStartSuccess(remoteConfig coreModels.RemoteEndpointSpecs, localEndpoint string) {
	l.messageMutex.Lock()
	defer l.messageMutex.Unlock()
	l.stopLoader()
	l.remoteSiteID = remoteConfig.SiteID

	siteAddr := urlmaker.GetSiteURL("https", remoteConfig.SiteID, remoteConfig.Domain)
	fmt.Fprintln(l.output)
	fmt.Fprint(l.output, "Forwarding ")
	fmt.Fprint(l.output, aurora.Green(siteAddr))
	fmt.Fprint(l.output, " -> ")
	fmt.Fprint(l.output, aurora.Green(localEndpoint))
	fmt.Fprintln(l.output)

	if config.Config.Display.QR {
		fmt.Fprintln(l.output)
		qrterminal.GenerateHalfBlock(siteAddr, qrterminal.L, l.output)
	}

	fmt.Fprintln(l.output)
	fmt.Fprint(l.output, l.lifetimeHint())
	fmt.Fprintln(l.output)
	fmt.Fprint(l.output, aurora.Cyan("Press CTRL + C to stop the service"))
	fmt.Fprintln(l.output)
	l.printed++
	l.line("INF", "Awaiting connections...")
}

func (l *stdoutLogger) lifetimeHint() interface{} {
	custom := strings.TrimSpace(l.remoteSiteID)
	if custom != "" {
		return aurora.Yellow("A custom hostname stays reserved for 30 days after the last tunnel and every start extends it")
	}
	return aurora.Yellow("A random hostname stays valid for 60 minutes, restart the tunnel with --hostname to keep it")
}

func (l *stdoutLogger) TunnelStartFailure(tunnelID string, err error) {
	l.LoginFailure(err)
}

func (l *stdoutLogger) TunnelStopSuccess(tunnelID string) {
	l.messageMutex.Lock()
	defer l.messageMutex.Unlock()
	if config.Config.Display.Verbose {
		l.line("DBG", "Tunnel shut down")
	}
}

func (l *stdoutLogger) LoginStart(deviceCodeSpec authModels.DeviceCodeSpec) {
	l.messageMutex.Lock()
	defer l.messageMutex.Unlock()
	l.stopLoader()
	uri := deviceCodeSpec.VerificationURIComplete
	if uri == "" {
		uri = deviceCodeSpec.VerificationURI
	}
	fmt.Fprintf(l.output, "Please open %s and use %s code to log in\n", aurora.Yellow(uri), aurora.Yellow(deviceCodeSpec.UserCode))
	l.printed++
}

func (l *stdoutLogger) LoginSuccess(idToken string) {
	l.messageMutex.Lock()
	defer l.messageMutex.Unlock()
	l.line("INF", "Logged in successfully")
}

func (l *stdoutLogger) LoginFailure(err error) {
	l.messageMutex.Lock()
	l.line("FTL", err.Error())
	l.messageMutex.Unlock()
	os.Exit(1)
}

func (l *stdoutLogger) LogoutSuccess() {
	l.messageMutex.Lock()
	defer l.messageMutex.Unlock()
	l.line("INF", "Logged out successfully")
}

func (l *stdoutLogger) LogoutFailure(err error) {
	l.LoginFailure(err)
}

func (l *stdoutLogger) LoadingStart(tunnelID string, loaderMessage string) {
	l.LoadingStartWithRequest(tunnelID, loaderMessage)
}

func (l *stdoutLogger) LoadingStartWithRequest(tunnelID string, loaderMessage string) {
	l.messageMutex.Lock()
	defer l.messageMutex.Unlock()
	if config.Config.Display.Verbose {
		l.line("DBG", loaderMessage)
		return
	}
	l.loader.Prefix = fmt.Sprintf("%s ", loaderMessage)
	l.loader.FinalMSG = ""
	l.loader.Start()
	l.loaderActive = true
}

func (l *stdoutLogger) LoadingSuccess(tunnelID string) {
	l.LoadingSuccessWithRequest(tunnelID)
}

func (l *stdoutLogger) LoadingSuccessWithRequest(tunnelID string) {
	l.messageMutex.Lock()
	defer l.messageMutex.Unlock()
	if config.Config.Display.Verbose {
		return
	}
	if l.loaderActive {
		l.loader.FinalMSG = fmt.Sprintf("%s%s\n", l.loader.Prefix, aurora.Green("Success!"))
		l.loader.Stop()
		l.loaderActive = false
		l.printed++
	}
}

func (l *stdoutLogger) LoadingFailure(tunnelID string, err error) {
	l.LoadingFailureWithRequest(tunnelID, err)
}

func (l *stdoutLogger) LoadingFailureWithRequest(tunnelID string, err error) {
	l.messageMutex.Lock()
	defer l.messageMutex.Unlock()
	if config.Config.Display.Verbose {
		l.line("DBG", err.Error())
		return
	}
	if l.loaderActive {
		l.loader.FinalMSG = fmt.Sprintf("%s%s %s\n", l.loader.Prefix, aurora.Red("Error!"), err.Error())
		l.loader.Stop()
		l.loaderActive = false
		l.printed++
	}
}

func (l *stdoutLogger) NewVersionAvailable(availableVersion string) {
	l.messageMutex.Lock()
	defer l.messageMutex.Unlock()
	l.line("INF", fmt.Sprintf("A newer version (%s) is available: https://github.com/name-cho/newhole/releases", availableVersion))
}
