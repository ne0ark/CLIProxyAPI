package gemini

import (
	"fmt"
	"net/http"

	"github.com/router-for-me/CLIProxyAPI/v6/internal/interfaces"
	"github.com/router-for-me/CLIProxyAPI/v6/sdk/api/handlers"
)

func writeGeminiStreamTerminalError(w http.ResponseWriter, alt string, errMsg *interfaces.ErrorMessage) {
	if errMsg == nil {
		return
	}

	status := http.StatusInternalServerError
	if errMsg.StatusCode > 0 {
		status = errMsg.StatusCode
	}

	errText := http.StatusText(status)
	if errMsg.Error != nil && errMsg.Error.Error() != "" {
		errText = errMsg.Error.Error()
	}

	body := handlers.BuildErrorResponseBody(status, errText)
	if alt == "" {
		_, _ = fmt.Fprintf(w, "event: error\ndata: %s\n\n", string(body))
		return
	}

	_, _ = w.Write(body)
}
