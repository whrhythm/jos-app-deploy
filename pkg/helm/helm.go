package helm

import (
	"context"
	"jos-deployment/pkg/logger"
	"os"
	"path/filepath"

	pb "jos-deployment/api/v1alpha1/pb"

	"fmt"

	"go.uber.org/zap"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/cli"
	"helm.sh/helm/v3/pkg/getter"
	"helm.sh/helm/v3/pkg/repo"
)

type HelmManagerServer struct {
	pb.UnimplementedHelmManagerServiceServer
}

type RepositoryConfig struct {
	Repositories []RepositoryEntry `yaml:"repositories"`
}

type RepositoryEntry struct {
	Name     string `yaml:"name"`
	URL      string `yaml:"url"`
	Username string `yaml:"username,omitempty"`
	Password string `yaml:"password,omitempty"`
}

type HelmClient struct {
	actionConfig *action.Configuration
	settings     *cli.EnvSettings
}

func initHelmClient() (*cli.EnvSettings, error) {
	settings := cli.New()
	helmHome := filepath.Join(os.Getenv("HOME"), ".helm")

	// 确保目录存在
	if err := os.MkdirAll(helmHome, 0755); err != nil {
		return nil, fmt.Errorf("无法创建 Helm 目录: %v", err)
	}

	f := filepath.Join(helmHome, "repositories.yaml")
	if _, err := os.Stat(f); os.IsNotExist(err) {
		// 如果文件不存在，创建一个新的 repositories.yaml 文件
		_, err := os.Create(f)
		if err != nil {
			return nil, fmt.Errorf("无法创建 repositories.yaml 文件: %v", err)
		}
	}

	settings.RepositoryConfig = filepath.Join(helmHome, "repositories.yaml")
	settings.RepositoryCache = filepath.Join(helmHome, "cache")
	return settings, nil
}

// 实现 ListCharts 方法
func (s *HelmManagerServer) ListCharts(ctx context.Context, req *pb.ListChartsRequest) (*pb.ListChartsResponse, error) {
	logger.L().Info("ListCharts called", zap.String("request", req.String()))
	var repoFile *repo.File
	settings, _ := initHelmClient()

	harborEntry := repo.Entry{
		Name:                  "harbor",
		URL:                   "https://harbor.joiningos.com/chartrepo/library",
		Username:              "admin",
		Password:              "P@88w0rd",
		InsecureSkipTLSverify: true,
	}

	repoFile, err := repo.LoadFile(settings.RepositoryConfig)
	if os.IsNotExist(err) {
		logger.L().Info("Repository file does not exist, creating new one")
		repoFile = repo.NewFile()
	} else if err != nil {
		logger.L().Error("Failed to load repository file", zap.Error(err))
		return nil, err
	}

	if !repoFile.Has(harborEntry.Name) {
		logger.L().Info("Adding new repository", zap.String("name", harborEntry.Name), zap.String("url", harborEntry.URL))
		repoFile.Add(&harborEntry)
	}

	if err := repoFile.WriteFile(settings.RepositoryConfig, 0644); err != nil {
		logger.L().Error("Failed to write repository file", zap.Error(err))
		return nil, err
	} else {
		logger.L().Info("Repository file written successfully", zap.String("path", settings.RepositoryConfig))
	}

	providers := getter.All(cli.New())

	chartRepo, err := repo.NewChartRepository(&harborEntry, providers)
	if err != nil {
		logger.L().Error("Failed to create new chart repository", zap.Error(err))
		return nil, err
	}

	// 打印索引内容
	indexPath := ""

	indexPath, err = chartRepo.DownloadIndexFile()
	if err != nil {
		logger.L().Error("Failed to download index file from chart repository", zap.Error(err))
		return nil, err
	} else {
		logger.L().Info("Index file downloaded successfully", zap.String("repository", harborEntry.Name))
	}

	data, err := os.ReadFile(indexPath)
	if err != nil {
		logger.L().Error("Failed to read index file", zap.Error(err))
		return nil, err
	}
	fmt.Println("=== 原始索引文件内容 ===")
	fmt.Println(string(data))

	// 解析索引文件
	indexFile, err := repo.LoadIndexFile(indexPath)
	if err != nil {
		logger.L().Error("Failed to load index file", zap.Error(err))
		return nil, err
	}

	var chartInfos []*pb.ChartInfo
	// 通过 indexFile 获取所有的 chart 信息
	for _, entries := range indexFile.Entries {
		for _, entry := range entries {
			chartInfo := &pb.ChartInfo{
				Name:         entry.Name,
				ChartVersion: entry.Version,
				AppVersion:   entry.AppVersion,
				Description:  entry.Description,
				IconUrl:      entry.Icon,
			}
			chartInfos = append(chartInfos, chartInfo)
		}
	}

	return &pb.ListChartsResponse{
		Charts: chartInfos, // 返回空列表
	}, nil
}

func New(namespace string) (*HelmClient, error) {
	settings := cli.New()
	actionConfig := new(action.Configuration)
	debugLog := func(format string, v ...interface{}) {
		logger.L().Debug(fmt.Sprintf(format, v...))
	}
	if err := actionConfig.Init(settings.RESTClientGetter(), namespace, "secret", debugLog); err != nil {
		logger.L().Error("Failed to initialize Helm action configuration", zap.Error(err))
		return nil, err
	}

	return &HelmClient{
		actionConfig: actionConfig,
		settings:     settings,
	}, nil
}

// func (c *HelmClient) RemoveRepo(name string) error {
// 	repoFile := filepath.Join(c.settings.RepositoryConfig, "repositories.yaml")
// 	rf, err := repo.LoadFile(repoFile)
// 	if err != nil {
// 		logger.L().Error("Failed to load repositories file", zap.Error(err))
// 		return err
// 	}

// 	if !rf.Has(name) {
// 		logger.L().Info("Repository does not exist", zap.String("name", name))
// 		return nil
// 	}

// 	if err := rf.Remove(name); err != nil {
// 		logger.L().Error("Failed to remove repository", zap.Error(err))
// 		return err
// 	}

// 	if err := rf.WriteFile(repoFile, 0644); err != nil {
// 		logger.L().Error("Failed to write repositories file", zap.Error(err))
// 		return err
// 	}

// 	logger.L().Info("Helm repository removed successfully", zap.String("name", name))
// 	return nil
// }

// func (c *HelmClient) InstallOrUpgrade(chartName, releaseName, namespace string) error {
// 	install := action.NewInstall(c.actionConfig)
// 	install.ReleaseName = releaseName
// 	install.Namespace = namespace

// 	logger.L().Info("Installing or upgrading chart", zap.String("chart", chartName), zap.String("release", releaseName), zap.String("namespace", namespace))

// 	// Load the chart
// 	chartPath, err := filepath.Abs(chartName)
// 	if err != nil {
// 		logger.L().Error("Failed to get absolute path for chart", zap.Error(err))
// 		return err
// 	}

// 	chart, err := install.ChartPathOptions.LocateChart(chartPath, c.settings)
// 	if err != nil {
// 		logger.L().Error("Failed to locate chart", zap.Error(err))
// 		return err
// 	}

// 	if _, err := install.Run(chart, nil); err != nil {
// 		logger.L().Error("Failed to install or upgrade chart", zap.Error(err))
// 		return err
// 	}

// 	logger.L().Info("Chart installed or upgraded successfully", zap.String("chart", chartPath))
// 	return nil
// }

// func (c *HelmClient) Uninstall(releaseName, namespace string) error {
// 	uninstall := action.NewUninstall(c.actionConfig)
// 	uninstall.Namespace = namespace

// 	logger.L().Info("Uninstalling release", zap.String("release", releaseName), zap.String("namespace", namespace))

// 	// Uninstall the release
// 	_, err := uninstall.Run(releaseName)
// 	if err != nil {
// 		logger.L().Error("Failed to uninstall release", zap.Error(err))
// 		return err
// 	}

// 	logger.L().Info("Release uninstalled successfully", zap.String("release", releaseName))
// 	return nil
// }

// func (c *HelmClient) ListReleases(namespace string) ([]*pb.ReleaseInfo, error) {
// 	list := action.NewList(c.actionConfig)
// 	list.Namespace = namespace

// 	logger.L().Info("Listing releases", zap.String("namespace", namespace))

// 	// List the releases
// 	releases, err := list.Run()
// 	if err != nil {
// 		logger.L().Error("Failed to list releases", zap.Error(err))
// 		return nil, err
// 	}

// 	var releaseInfos []*pb.ReleaseInfo
// 	for _, rel := range releases {
// 		releaseInfos = append(releaseInfos, &pb.ReleaseInfo{
// 			Name:      rel.Name,
// 			Namespace: rel.Namespace,
// 			Status:    rel.Info.Status.String(),
// 		})
// 	}

// 	logger.L().Info("Releases listed successfully", zap.Int("count", len(releaseInfos)))
// 	return releaseInfos, nil
// }

// func (c *HelmClient) GetRelease(releaseName, namespace string) (*pb.ReleaseInfo, error) {
// 	get := action.NewGet(c.actionConfig)
// 	get.Namespace = namespace

// 	logger.L().Info("Getting release", zap.String("release", releaseName), zap.String("namespace", namespace))

// 	// Get the release
// 	release, err := get.Run(releaseName)
// 	if err != nil {
// 		logger.L().Error("Failed to get release", zap.Error(err))
// 		return nil, err
// 	}

// 	releaseInfo := &pb.ReleaseInfo{
// 		Name:      release.Name,
// 		Namespace: release.Namespace,
// 		Status:    release.Info.Status.String(),
// 	}

// 	logger.L().Info("Release retrieved successfully", zap.String("release", release.Name))
// 	return releaseInfo, nil
// }

// func (c *HelmClient) WatchPods(namespace string) error {
// 	// 这里可以实现 Pod 监控逻辑
// 	logger.L().Info("Watching pods in namespace", zap.String("namespace", namespace))
// 	// 目前仅返回 nil，实际实现需要使用 Kubernetes 客户端库来监控 Pods
// 	return nil
// }
