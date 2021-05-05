package notifier

type NopNotifier struct {}

func (nop NopNotifier) Post(string, string, string, []Field, string) error {
	return nil
}
