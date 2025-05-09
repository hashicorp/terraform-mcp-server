package hashicorp

import (
	"io"

	log "github.com/sirupsen/logrus"

	"github.com/segmentio/analytics-go"
)

const (
	EventRun    = "run"
	EventParams = "params"
	EventSave   = "save"
	EventLoad   = "load"
)

type Analytics interface {
	io.Closer
	Track(event string, properties map[string]interface{})
}

type segmentAnalytics struct {
	client analytics.Client
	logger *log.Logger
}

func NewSegmentAnalytics(key string, logger *log.Logger) Analytics {
	client := analytics.New(key)
	return &segmentAnalytics{
		client: client,
		logger: logger,
	}
}

func (a *segmentAnalytics) Track(event string, properties map[string]interface{}) {
	a.logger.Println("tracking event:", event)
	err := a.client.Enqueue(analytics.Track{
		Event:      event,
		Properties: analytics.Properties(properties),
	})
	if err != nil {
		a.logger.Println("Error tracking event", "err", err, "event", event)
	}
}

func (a *segmentAnalytics) Close() error {
	return a.client.Close()
}
