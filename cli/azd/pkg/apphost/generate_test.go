package apphost

import (
	"context"
	_ "embed"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/azure/azure-dev/cli/azd/pkg/exec"
	"github.com/azure/azure-dev/cli/azd/pkg/osutil"
	"github.com/azure/azure-dev/cli/azd/pkg/tools/dotnet"
	"github.com/azure/azure-dev/cli/azd/test/mocks"
	"github.com/azure/azure-dev/cli/azd/test/snapshot"
	"github.com/stretchr/testify/require"
)

//go:embed testdata/aspire-docker.json
var aspireDockerManifest []byte

func TestAspireDockerGeneration(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping due to EOL issues on Windows with the baselines")
	}

	ctx := context.Background()
	mockCtx := mocks.NewMockContext(ctx)
	mockCtx.CommandRunner.When(func(args exec.RunArgs, command string) bool {
		return args.Cmd == "dotnet" && args.Args[0] == "run" && args.Args[3] == "--publisher" && args.Args[4] == "manifest"
	}).RespondFn(func(args exec.RunArgs) (exec.RunResult, error) {
		err := os.WriteFile(args.Args[6], aspireDockerManifest, osutil.PermissionFile)
		if err != nil {
			return exec.RunResult{
				ExitCode: -1,
				Stderr:   err.Error(),
			}, err
		}
		return exec.RunResult{}, nil
	})

	mockCli := dotnet.NewDotNetCli(mockCtx.CommandRunner)

	m, err := ManifestFromAppHost(ctx, filepath.Join("testdata", "AspireDocker.AppHost.csproj"), mockCli)
	require.NoError(t, err)

	// The App Host manifest does not set the external bit for project resources. Instead, `azd` or whatever tool consumes
	// the manifest should prompt the user to select which services should be exposed. For this test, we manually set the
	// external bit on the resources on the webfrontend resource to simulate the user selecting the webfrontend to be
	// exposed.
	for _, value := range m.Resources["nodeapp"].Bindings {
		value.External = true
	}

	for _, name := range []string{"nodeapp"} {
		t.Run(name, func(t *testing.T) {
			tmpl, err := ContainerAppManifestTemplateForProject(m, name)
			require.NoError(t, err)
			snapshot.SnapshotT(t, tmpl)
		})
	}

	files, err := BicepTemplate(m)
	require.NoError(t, err)

	err = fs.WalkDir(files, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		contents, err := fs.ReadFile(files, path)
		if err != nil {
			return err
		}
		t.Run(path, func(t *testing.T) {
			snapshot.SnapshotT(t, string(contents))
		})
		return nil
	})
	require.NoError(t, err)
}
