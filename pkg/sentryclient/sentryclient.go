package sentryclient

import (
	"os"
	"time"

	"github.com/getsentry/sentry-go"
	log "github.com/uswitch/heimdall/pkg/log"
)

var (
	sentryActive = false
)

func SentryErr(exception error) {
	if sentryActive {
		sentry.CaptureException(exception)
		log.Sugar.Debug("Error has been sent to Sentry!")
	}
}

func SentryMessage(message string) {
	if sentryActive {
		sentry.CaptureMessage(message)
		log.Sugar.Debug("Message has been sent to Sentry!")
	}
}

// SetupSentry is the function setting up the Sentry access
func SetupSentry() {
	endpoint, exists := os.LookupEnv("sentry_endpoint")

	if exists {
		err := sentry.Init(sentry.ClientOptions{
			Dsn: endpoint,
		})
		if err != nil {
			log.Sugar.Fatalf("sentry.Init: %s", err)
		}
		// Flush buffered events before the program terminates.
		defer sentry.Flush(2 * time.Second)

		log.Sugar.Info("Sentry has been initialised!")
		sentryActive = true
	} else {
		log.Sugar.Info("Secret for Sentry is not present, so we are running without Sentry!")
	}
}
