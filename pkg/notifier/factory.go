package notifier

import "fmt"

type Factory struct {
	URL			string
	Username	string
	Channel		string
}

func NewFactory(url, username, channel string) *Factory {
	return &Factory{
		URL:      url,
		Username: username,
		Channel:  channel,
	}
}

func (f Factory) Notifier(provider string) (Interface, error){
	if f.URL == "" {
		return &NopNotifier{}, nil
	}
	var n Interface
	var err error
	switch provider {
	case "slack" :
		n, err = NewSlack(f.URL, f.Username, f.Channel)
	case "rocket":
		n, err = NewRocket(f.URL, f.Username, f.Channel)
	default:
		err = fmt.Errorf("provider %s not supported", provider)
	}

	if err != nil {
		n = &NopNotifier{}
	}
	return n, err
}
