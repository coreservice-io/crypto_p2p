package peer

import (
	"github.com/coreservice-io/log"
	"github.com/coreservice-io/logrus_log"
)

var llog log.Logger

// uses a specified Logger to output package logging info.
func UseLogger(logger log.Logger) {
	llog = logger
}

// The default amount of logging is none.
func init() {
	elog, _ := logrus_log.New("./logs", 1, 20, 30)

	// elog.SetLevel(log.TraceLevel)
	elog.Infoln("init log")

	llog = elog
}
