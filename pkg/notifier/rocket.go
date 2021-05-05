package notifier

import (
	"errors"
	"fmt"
	"net/url"
)

type Rocket struct {
	URL			string
	Username	string
	Channel		string
}

func NewRocket(hookURL, username, channel string) (*Rocket, error){
	_, err := url.ParseRequestURI(hookURL)
	if err != nil {
		return nil, fmt.Errorf("invalid Rocket hook URL：%s", hookURL)
	}

	if username == "" {
		return nil, errors.New("empty Rocket username")
	}

	if channel == "" {
		return nil, errors.New("empty Rocket channel")
	}

	return &Rocket{
		URL:      hookURL,
		Username: username,
		Channel:  channel,
	}, nil
}

func (r *Rocket) Post(workload string, namespace string, message string, fields []Field, severity string) error {
	payload := SlackPayload{
		Channel:     r.Channel,
		Username:    r.Username,
		IconEmoji:   ":rocket:",
	}

	color := "#0076D7"
	if severity == "error" {
		color = "#FF0000"
	}

	sfields := make([]SlackField, 0, len(fields))
	for _, f := range fields {
		sfields = append(sfields, SlackField{f.Name, f.Value, false})
	}

	a := SlackAttachment{
		Color:      color,
		AuthorName: fmt.Sprintf("%s.%s", workload, namespace),
		Text:       message,
		MrkdwnIn:   []string{"text"},
		Fields:     sfields,
	}

	payload.Attachments = []SlackAttachment{a}

	err := postMessage(r.URL, payload)
	if err != nil {
		return fmt.Errorf("postMessage failed: %w", err)
	}
	return nil
}
