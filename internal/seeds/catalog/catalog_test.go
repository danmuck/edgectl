package catalog

import (
	"testing"

	"github.com/danmuck/edgectl/internal/seeds"
)

func TestDependencyCatalogContainsDockerAndMongod(t *testing.T) {
	cat := DependencyCatalog()

	docker, ok := cat["seed.docker"]
	if !ok || len(docker) == 0 {
		t.Fatal("expected seed.docker in catalog")
	}
	if docker[0].Binary != "docker" {
		t.Fatalf("expected docker binary, got %q", docker[0].Binary)
	}
	if docker[0].InstallMethod != seeds.InstallMethodApt {
		t.Fatalf("expected apt install method for docker, got %q", docker[0].InstallMethod)
	}

	mongod, ok := cat["seed.mongod"]
	if !ok || len(mongod) == 0 {
		t.Fatal("expected seed.mongod in catalog")
	}
	foundMongod := false
	foundMongosh := false
	for _, dep := range mongod {
		if dep.Binary == "mongod" {
			foundMongod = true
		}
		if dep.Binary == "mongosh" {
			foundMongosh = true
		}
	}
	if !foundMongod {
		t.Fatal("expected mongod binary in seed.mongod deps")
	}
	if !foundMongosh {
		t.Fatal("expected mongosh binary in seed.mongod deps")
	}
}

func TestDependencyCatalogExcludesHostSeed(t *testing.T) {
	cat := DependencyCatalog()
	if _, ok := cat["seed.host"]; ok {
		t.Fatal("seed.host should have no dependencies in catalog")
	}
}

func TestDependencyCatalogAllDepsRequired(t *testing.T) {
	cat := DependencyCatalog()
	for seedID, deps := range cat {
		for _, dep := range deps {
			if !dep.Required {
				t.Errorf("seed %s dep %q is not marked required", seedID, dep.Name)
			}
		}
	}
}

func TestLookupDepsReturnsNilForUnknownSeed(t *testing.T) {
	deps := LookupDeps("seed.nonexistent")
	if deps != nil {
		t.Fatalf("expected nil for unknown seed, got %v", deps)
	}
}
