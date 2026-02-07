package ghost

// Service is the runtime skeleton for Ghost execution.
type Service struct{}

// NewService creates an empty Ghost runtime service.
func NewService() *Service {
	return &Service{}
}

// Run starts Ghost runtime skeleton.
func (s *Service) Run() error {
	return nil
}
