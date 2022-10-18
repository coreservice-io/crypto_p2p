package peer

import (
	"github.com/coreservice-io/logrus_log"
)

var log log.Logger

// uses a specified Logger to output package logging info.
func UseLogger(logger log.Logger) {
	log = logger
}

// The default amount of logging is none.
func init() {
	llog, _ := logrus_log.New("./logs", 1, 20, 30)

	log = llog
}
