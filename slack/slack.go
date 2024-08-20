package slack

import (
	"fmt"

	"github.com/slack-go/slack"
)

type Client struct {
	api *slack.Client
}

func NewClient(token string) *Client {
	return &Client{
		api: slack.New(token),
	}
}

func (c *Client) PostMessage(channel, message string) error {
	_, _, err := c.api.PostMessage(channel, slack.MsgOptionText(message, false))
	if err != nil {
		return fmt.Errorf("error posting message to Slack: %v", err)
	}
	return nil
}