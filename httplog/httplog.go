package httplog

import (
	"io"
	"os"
)

type Logger struct {
	log *os.File
}

// Creates a new Logger
func New(logfile string) (*Logger, error) {
	// TODO: config option for setting logfile perms
	// TODO: need to close this file on exit
	log, err := os.OpenFile(logfile, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 777)
	if err != nil {
		return nil, err
	}
	return &Logger{log}, err
}

func (logger *Logger) Write(s []byte) {
	n, err := logger.log.Write(s)
	if err == nil && n < len(s) {
		err = io.ErrShortWrite
	}
}
