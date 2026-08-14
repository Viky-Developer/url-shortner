package handler

import "github.com/vicky/url-shortner/external/logger"

func Hello(log logger.Logger) {
	log.Info("url-shortner server")
}
