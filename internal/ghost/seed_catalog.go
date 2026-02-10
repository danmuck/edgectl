package ghost

import "github.com/danmuck/edgectl/internal/seeds"

// SeedCapability captures one seed's identity and full operator-facing capability surface.
type SeedCapability struct {
	Metadata       seeds.SeedMetadata      `json:"metadata"`
	Operations     []seeds.OperationSpec   `json:"operations"`
	CommandCatalog []seeds.CommandTemplate `json:"command_catalog"`
}

// SeedCatalog returns a deterministic snapshot of all registered seed capabilities.
func (s *Server) SeedCatalog() []SeedCapability {
	s.mu.RLock()
	reg := s.registry
	s.mu.RUnlock()
	if reg == nil {
		return []SeedCapability{}
	}

	metaList := reg.ListMetadata()
	out := make([]SeedCapability, 0, len(metaList))
	for i := range metaList {
		meta := metaList[i]
		seed, ok := reg.Resolve(meta.ID)
		if !ok || seed == nil {
			continue
		}
		out = append(out, SeedCapability{
			Metadata:       meta,
			Operations:     cloneOperationSpecs(seed.Operations()),
			CommandCatalog: cloneCommandTemplates(seed.CommandCatalog()),
		})
	}
	return out
}

// cloneOperationSpecs returns a defensive copy of operation specs.
func cloneOperationSpecs(in []seeds.OperationSpec) []seeds.OperationSpec {
	if len(in) == 0 {
		return []seeds.OperationSpec{}
	}
	out := make([]seeds.OperationSpec, len(in))
	copy(out, in)
	return out
}

// cloneCommandTemplates returns a defensive copy of command templates and nested arg specs.
func cloneCommandTemplates(in []seeds.CommandTemplate) []seeds.CommandTemplate {
	if len(in) == 0 {
		return []seeds.CommandTemplate{}
	}
	out := make([]seeds.CommandTemplate, len(in))
	copy(out, in)
	for i := range out {
		out[i].Args = cloneCommandArgSpecs(out[i].Args)
	}
	return out
}

// cloneCommandArgSpecs returns a defensive copy of template argument specs.
func cloneCommandArgSpecs(in []seeds.CommandArgSpec) []seeds.CommandArgSpec {
	if len(in) == 0 {
		return []seeds.CommandArgSpec{}
	}
	out := make([]seeds.CommandArgSpec, len(in))
	copy(out, in)
	return out
}
