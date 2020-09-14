package log

import "go.uber.org/zap"

const (
	DEBUG_LEVEL = "debug"
	INFO_LEVEL  = "info"
)

var (
	//Logger is the default logger
	Logger *zap.Logger
	Sugar  *zap.SugaredLogger
)

func Setup(logLevel string) {

	if logLevel == DEBUG_LEVEL {
		Logger, _ = zap.NewDevelopment()
	} else {
		Logger, _ = zap.NewProduction()
	}
	defer Logger.Sync()
	Sugar = Logger.Sugar()
}
