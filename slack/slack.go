package slack

import (
	"fmt"

	"github.com/sirupsen/logrus"
	"github.com/slack-go/slack"
)

var log = logrus.New()

func init() {
	log.SetFormatter(&logrus.TextFormatter{
		FullTimestamp: true,
	})
	log.SetLevel(logrus.DebugLevel)
}

type Client struct {
	api *slack.Client
}

func NewClient(token string) *Client {
	return &Client{
		api: slack.New(token),
	}
}

func (c *Client) PostMessage(channel, message string) error {
	log.WithFields(logrus.Fields{
		"channel": channel,
		"message": message,
	}).Debug("Posting message to Slack")

	_, _, err := c.api.PostMessage(channel, slack.MsgOptionText(message, false))
	if err != nil {
		log.WithFields(logrus.Fields{
			"channel": channel,
			"error":   err,
		}).Error("Error posting message to Slack")
		return fmt.Errorf("error posting message to Slack: %w", err)
	}

	log.WithField("channel", channel).Info("Successfully posted message to Slack")
	return nil
}
