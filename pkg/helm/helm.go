package helm

import (
	"context"
	"encoding/json"
	"jos-deployment/pkg/logger"
	"log"
	"os"
	"path/filepath"

	"google.golang.org/protobuf/types/known/anypb"
	"gopkg.in/yaml.v2"
	"helm.sh/helm/v3/pkg/chart/loader"

	pb "jos-deployment/api/v1alpha1/pb"

	"fmt"

	"go.uber.org/zap"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/cli"
	"helm.sh/helm/v3/pkg/getter"
	"helm.sh/helm/v3/pkg/repo"
)

const (
	defaultRepositoryConfigPath = "/opt/helm/repositories.yaml"
)

// 默认配置
var (
	harborEntry = repo.Entry{
		Name:                  "harbor",
		URL:                   "https://harbor.joiningos.com/chartrepo/library",
		Username:              "admin",
		Password:              "P@88w0rd",
		InsecureSkipTLSverify: true,
	}
	helmClient *HelmClient
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

func init() {
	settings := cli.New()
	if fileExists(defaultRepositoryConfigPath) {
		settings.RepositoryConfig = defaultRepositoryConfigPath
	} else {
		err := createRespositoryConfig(settings)
		if err != nil {
			log.Fatal("Failed to create repository config:", err)
		}
	}

	actionConfig := new(action.Configuration)
	debugLog := func(format string, v ...interface{}) {
		logger.L().Debug(fmt.Sprintf(format, v...))
	}
	if err := actionConfig.Init(settings.RESTClientGetter(), "default", "secret", debugLog); err != nil {
		log.Fatal("Failed to initialize Helm action configuration:", err)
	}

	helmClient = &HelmClient{
		actionConfig: actionConfig,
		settings:     settings,
	}
}

func initHelmClient(s *cli.EnvSettings) error {
	helmHome := filepath.Join(os.Getenv("HOME"), ".helm")

	// 确保目录存在
	if err := os.MkdirAll(helmHome, 0755); err != nil {
		return fmt.Errorf("无法创建 Helm 目录: %v", err)
	}

	f := filepath.Join(helmHome, "repositories.yaml")
	if _, err := os.Stat(f); os.IsNotExist(err) {
		// 如果文件不存在，创建一个新的 repositories.yaml 文件
		_, err := os.Create(f)
		if err != nil {
			return fmt.Errorf("无法创建 repositories.yaml 文件: %v", err)
		}
	}

	s.RepositoryConfig = filepath.Join(helmHome, "repositories.yaml")
	s.RepositoryCache = filepath.Join(helmHome, "cache")
	return nil
}

func createRespositoryConfig(s *cli.EnvSettings) error {
	err := initHelmClient(s)
	if err != nil {
		logger.L().Info("initHelmClient failed")
		return err
	}

	repoFile, err := repo.LoadFile(s.RepositoryConfig)
	if os.IsNotExist(err) {
		logger.L().Info("Repository file does not exist, creating new one")
		repoFile = repo.NewFile()
	} else if err != nil {
		logger.L().Error("Failed to load repository file", zap.Error(err))
		return err
	}

	if !repoFile.Has(harborEntry.Name) {
		logger.L().Info("Adding new repository", zap.String("name", harborEntry.Name), zap.String("url", harborEntry.URL))
		repoFile.Add(&harborEntry)
	}

	if err := repoFile.WriteFile(s.RepositoryConfig, 0644); err != nil {
		logger.L().Error("Failed to write repository file", zap.Error(err))
		return err
	} else {
		logger.L().Info("Repository file written successfully", zap.String("path", s.RepositoryConfig))
	}
	return nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	if err == nil {
		return true // 文件存在
	}
	if os.IsNotExist(err) {
		return false // 文件不存在
	}
	return false // 其他错误（如权限问题）
}

// 实现 ListCharts 方法
func (s *HelmManagerServer) ListCharts(ctx context.Context, req *pb.ListChartsRequest) (*pb.ListChartsResponse, error) {
	logger.L().Info("ListCharts called", zap.String("request", req.String()))
	providers := getter.All(cli.New())

	chartRepo, err := repo.NewChartRepository(&harborEntry, providers)
	if err != nil {
		logger.L().Error("Failed to create new chart repository", zap.Error(err))
		return nil, err
	}

	// 打印索引内容
	indexPath, err := chartRepo.DownloadIndexFile()
	if err != nil {
		logger.L().Error("Failed to download index file from chart repository", zap.Error(err))
		return nil, err
	} else {
		logger.L().Info("Index file downloaded successfully", zap.String("repository", harborEntry.Name))
	}

	// 解析索引文件
	indexFile, err := repo.LoadIndexFile(indexPath)
	if err != nil {
		logger.L().Error("Failed to load index file", zap.Error(err))
		return nil, err
	}

	if req.Limit <= 0 || req.Size <= 0 {
		return &pb.ListChartsResponse{
			Code:    1,
			Message: "Invalid limit or size",
			Success: false,
			Data:    nil,
		}, nil
	}

	var chartInfos []*pb.ChartInfo

	// 通过 indexFile 获取所有的 chart 信息
	for _, entries := range indexFile.Entries {
		for _, entry := range entries {
			chartInfo := &pb.ChartInfo{
				Name:         entry.Name,
				ChartVersion: entry.Version,
				IconUrl:      entry.Icon,
				AppVersion:   entry.AppVersion,
				Description:  entry.Description,
				UpdateDate:   entry.Created.String(),
				UpdateUser:   "admin", // 这里可以根据实际情况修改
			}
			chartInfos = append(chartInfos, chartInfo)
		}
	}

	totalCharts := len(chartInfos)
	pageCount := int32(totalCharts) / req.Size
	if int32(totalCharts)%req.Size != 0 {
		pageCount++
	}
	start := (req.Limit - 1) * req.Size
	end := req.Limit * req.Size
	if int(start) > totalCharts {
		start = int32(totalCharts)
	}
	if int(end) > totalCharts {
		end = int32(totalCharts)
	}
	pagedCharts := chartInfos[int(start):int(end)]

	listChartsData := &pb.ListChartsData{
		Total:       int32(totalCharts),
		PageSize:    req.Size,
		TotalPage:   pageCount,
		CurrentPage: req.Limit,
		Charts:      pagedCharts,
	}
	anyData, err := anypb.New(listChartsData)
	if err != nil {
		logger.L().Error("Failed to marshal ListChartsData to Any", zap.Error(err))
		return &pb.ListChartsResponse{
			Code:    1,
			Message: "Failed to marshal ListChartsData",
			Success: false,
			Data:    nil,
		}, nil
	}

	return &pb.ListChartsResponse{
		Code:    0,
		Message: "Charts retrieved successfully",
		Success: true,
		Data:    anyData,
	}, nil
}

// 实现按照helm chart方法
func (s *HelmManagerServer) InstallChart(ctx context.Context, req *pb.InstallChartRequest) (*pb.InstallChartResponse, error) {
	// 1. 获取参数
	dryRun := req.DryRun
	// 2. 创建 Helm action 配置

	// 3. 解析 values
	var values map[string]interface{}
	if dryRun {
		var message string
		if req.Values != "" {
			if err := unmarshalValues(req.Values, &values); err != nil {
				logger.L().Error("Failed to parse values", zap.Error(err))
				return nil, err
			}
		}
		return &pb.InstallChartResponse{
			Code:    0,
			Message: message,
		}, nil
	} else {
		// 4. 安装 chart
		install := action.NewInstall(helmClient.actionConfig)
		install.ReleaseName = req.GetName()
		install.Namespace = req.GetNamespace()
		if req.Version != "" {
			install.Version = req.Version
		}

		chartRef := fmt.Sprintf("%s/%s", harborEntry.Name, req.Name)
		chartPath, err := install.ChartPathOptions.LocateChart(chartRef, helmClient.settings)
		if err != nil {
			logger.L().Error("Failed to locate chart", zap.Error(err))
			return nil, err
		}

		chart, err := loader.Load(chartPath)
		if err != nil {
			logger.L().Error("Failed to load chart", zap.Error(err))
			return nil, err
		}

		release, err := install.Run(chart, values)
		if err != nil {
			logger.L().Error("Failed to install chart", zap.Error(err))
			return nil, err
		}

		return &pb.InstallChartResponse{
			ReleaseName:   release.Name,
			FirstDeployed: release.Info.FirstDeployed.String(),
			LastDeployed:  release.Info.LastDeployed.String(),
			Deleted:       release.Info.Deleted.String(),
			Message:       release.Info.Description,
			Status:        release.Info.Status.String(),
		}, nil
	}
}

// unmarshalValues tries to unmarshal a YAML or JSON string into a map[string]interface{}.
func unmarshalValues(data string, out *map[string]interface{}) error {
	// Try YAML first
	if err := yaml.Unmarshal([]byte(data), out); err == nil {
		return nil
	}
	// Try JSON
	return json.Unmarshal([]byte(data), out)
}

func (s *HelmManagerServer) UninstallChart(ctx context.Context, req *pb.UninstallChartRequest) (*pb.UninstallChartResponse, error) {
	logger.L().Info("UninstallChart called", zap.String("request", req.String()))

	uninstall := action.NewUninstall(helmClient.actionConfig)
	uninstall.Wait = true

	_, err := uninstall.Run(req.GetName())
	if err != nil {
		logger.L().Error("Failed to uninstall chart", zap.Error(err))
		return &pb.UninstallChartResponse{
			Code:    1,
			Message: "Failed to uninstall chart",
		}, err
	}

	return &pb.UninstallChartResponse{
		Code:    0,
		Message: "Chart uninstalled successfully",
	}, nil
}
