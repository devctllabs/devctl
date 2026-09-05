package project

import (
	"context"

	projectdomain "github.com/devctllabs/devctl/internal/domain/project"
	"go.uber.org/zap"
)

const manifestFilename = "devctl.yaml"

//go:generate go tool mockgen -destination mocks/service.go -package mocks -typed . ManifestRepository,ManifestLocator,TargetResolver,ReadinessChecker

// ManifestRepository persists the canonical manifest selected by the project service.
type ManifestRepository interface {
	// Load decodes manifestPath, returning structural issues as data and access failures as errors.
	Load(ctx context.Context, manifestPath string) (projectdomain.LoadManifestResult, error)
	// Save atomically publishes project.Manifest at project.ManifestPath in canonical form and reports whether bytes changed.
	Save(ctx context.Context, project projectdomain.Project) (bool, error)
}

// ManifestLocator exposes only the filesystem facts needed to locate a Project manifest.
type ManifestLocator interface {
	// WorkingDirectory returns the process directory used as the project search origin.
	WorkingDirectory(ctx context.Context) (string, error)
	// RegularFile reports whether relativePath is a regular non-symlink file below root.
	RegularFile(ctx context.Context, root, relativePath string) (bool, error)
}

// TargetResolver attaches the concrete input used to inspect one Target.
type TargetResolver interface {
	// Resolve attaches the concrete input required to execute target in selected Project.
	Resolve(ctx context.Context, selected projectdomain.Project, target projectdomain.Target) (projectdomain.Target, error)
}

// ReadinessChecker evaluates environment-dependent project readiness policy.
type ReadinessChecker interface {
	// Check returns every applicable readiness issue in stable order.
	Check(ctx context.Context, selected projectdomain.Project) ([]projectdomain.Issue, error)
}

type Service struct {
	logger    *zap.Logger
	manifests ManifestRepository
	locator   ManifestLocator
	inputs    TargetResolver
	readiness ReadinessChecker
}

// Dependencies names the required Project service capabilities passed to New.
type Dependencies struct {
	Manifests ManifestRepository
	Locator   ManifestLocator
	Inputs    TargetResolver
	Readiness ReadinessChecker
}

func New(logger *zap.Logger, dependencies Dependencies) *Service {
	return &Service{
		logger: logger, manifests: dependencies.Manifests,
		locator: dependencies.Locator, inputs: dependencies.Inputs, readiness: dependencies.Readiness,
	}
}
