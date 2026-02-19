package logger

import (
	"github.com/sirupsen/logrus"
)

func New() *logrus.Logger {
	lg := logrus.New()
	lg.SetFormatter(&logrus.JSONFormatter{})
	lg.SetLevel(logrus.InfoLevel)
	return lg
}
