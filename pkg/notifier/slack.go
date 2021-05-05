package notifier

import (
	"errors"
	"fmt"
	"net/url"
)

type Slack struct {
	URL			string
	Username	string
	Channel 	string
}

type SlackPayload struct {
	Channel		string				`json:"channel"`
	Username	string				`json:"username"`
	IconUrl		string				`json:"icon_url"`
	IconEmoji	string				`json:"icon_emoji"`
	Text		string				`json:"text,omitempty"`
	Attachments	[]SlackAttachment	`json:"attachments,omitempty"`
}

type SlackAttachment struct {
	Color		string			`json:"color"`
	AuthorName	string			`json:"author_name"`
	Text		string			`json:"text"`
	MrkdwnIn	[]string		`json:"mrkdwn_in"`
	Fields		[]SlackField	`json:"fields"`
}

type SlackField struct {
	Title		string			`json:"title"`
	Value		string			`json:"value"`
	Short		bool			`json:"short"`
}

func NewSlack(hookURL, username, channel string) (*Slack, error) {
	_, err := url.ParseRequestURI(hookURL)
	if err != nil {
		return nil, fmt.Errorf("invalid Slack hook URL %s", hookURL)
	}
	if username == "" {
		return nil, errors.New("empty Slack username")
	}

	if channel == "" {
		return nil, errors.New("empty channel channel")
	}

	return &Slack{
		URL:      hookURL,
		Username: username,
		Channel:  channel,
	}, nil
}

func (s *Slack) Post(workload string, namespace string, message string, field []Field, severity string) error {
	payload := SlackPayload{
		Channel:     s.Channel,
		Username:    s.Username,
		IconEmoji:   ":rocket:",
	}

	color := "good"
	if severity == "error" {
		color = "danger"
	}

	sfield := make([]SlackField, 0, len(field))
	for _, f:= range field {
		sfield = append(sfield, SlackField{
			Title: f.Name,
			Value: f.Value,
			Short: false,
		})
	}
	a := SlackAttachment{
		Color:      color,
		AuthorName: fmt.Sprintf("%s.%s", workload, namespace),
		Text:       message,
		MrkdwnIn:   []string{"text"},
		Fields:     sfield,
	}

	payload.Attachments = []SlackAttachment{a}
	err := postMessage(s.URL, payload)
	if err != nil {
		fmt.Errorf("postMessage failed： %w", err)
	}
	return nil
}