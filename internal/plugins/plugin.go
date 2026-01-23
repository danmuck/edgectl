package plugins

type Plugin interface {
	Name() string
	Status() (any, error)
	Actions() map[string]Action
}
