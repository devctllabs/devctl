package project_test

import (
	projectdomain "github.com/devctllabs/devctl/internal/domain/project"
	projectservice "github.com/devctllabs/devctl/internal/service/project"
	"github.com/devctllabs/devctl/internal/service/project/mocks"
	"go.uber.org/mock/gomock"
	"go.uber.org/zap"
)

func newValidationService(
	ctrl *gomock.Controller,
	manifests projectservice.ManifestRepository,
	workspace projectservice.ManifestLocator,
	selected projectdomain.Project,
) *projectservice.Service {
	readiness := mocks.NewMockReadinessChecker(ctrl)
	readiness.EXPECT().Check(gomock.Any(), selected).Return(nil, nil)
	return projectservice.New(zap.NewNop(), projectservice.Dependencies{
		Manifests: manifests,
		Locator:   workspace,
		Readiness: readiness,
	})
}
