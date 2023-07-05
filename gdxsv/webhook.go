package main

import (
	"bytes"
	"encoding/json"
	"go.uber.org/zap"
	"io"
	"net/http"
)

func WebhookPostSimpleText(text string) {
	if conf.WebhookUrl == "" {
		return
	}

	body, err := json.Marshal(
		struct {
			Text string `json:"text"`
		}{
			Text: text,
		})
	if err != nil {
		logger.Warn("json.Marshal", zap.Error(err))
		return
	}

	res, err := http.Post(conf.WebhookUrl, "application/x-www-form-urlencoded", bytes.NewBuffer(body))
	if err != nil {
		logger.Warn("http.Post", zap.Error(err))
		return
	}

	defer res.Body.Close()
	read, err := io.ReadAll(res.Body)
	if err != nil {
		logger.Warn("io.ReadAll", zap.Error(err))
		return
	}

	logger.Info(string(read))
}
