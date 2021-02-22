package proxy

import "github.com/sirupsen/logrus"

func DefaultLogger() *logrus.Logger {
	log := logrus.New()
	log.Formatter = &logrus.JSONFormatter{}
	return log
}
