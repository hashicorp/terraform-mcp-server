package hashicorp

import (
	"io"

	"github.com/segmentio/analytics-go"

	"github.com/hashicorp/go-hclog"
)

const (
	EventDownload = "download"
	EventRun      = "run"
	EventParams   = "params"
	EventSave     = "save"
	EventLoad     = "load"
)

type Analytics interface {
	io.Closer
	Track(event, userID string, properties map[string]interface{})
}

type segmentAnalytics struct {
	client analytics.Client
	logger hclog.Logger
}

func NewSegmentAnalytics(key string, logger hclog.Logger) Analytics {
	client := analytics.New(key)
	return &segmentAnalytics{
		client: client,
		logger: logger,
	}
}

func (a *segmentAnalytics) Track(event, userID string, properties map[string]interface{}) {
	a.logger.Info("tracking event:", event)
	err := a.client.Enqueue(analytics.Track{
		Event:      event,
		UserId:     userID,
		Properties: analytics.Properties(properties),
	})
	if err != nil {
		a.logger.Error("Error tracking event", "err", err, "event", event)
	}
}

func (a *segmentAnalytics) Close() error {
	return a.client.Close()
}
