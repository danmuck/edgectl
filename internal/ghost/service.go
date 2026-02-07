package ghost

// Service is the runtime skeleton for Ghost execution.
type Service struct {
	server *Server
}

// NewService creates an empty Ghost runtime service.
func NewService() *Service {
	return &Service{
		server: NewServer(),
	}
}

// Run starts Ghost runtime skeleton.
func (s *Service) Run() error {
	return nil
}

// Server returns the lifecycle/execution boundary owner for Ghost.
func (s *Service) Server() *Server {
	return s.server
}
