package mirage

// Service is the runtime skeleton for Mirage orchestration.
type Service struct{}

// NewService creates an empty Mirage runtime service.
func NewService() *Service {
	return &Service{}
}

// Run starts Mirage runtime skeleton.
func (s *Service) Run() error {
	return nil
}
