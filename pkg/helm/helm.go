package helm

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"io"
	"jos-deployment/pkg/logger"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/anypb"
	"gopkg.in/yaml.v2"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/release"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

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
		PassCredentialsAll:    true,
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
	os.Setenv("XDG_CACHE_HOME", filepath.Join(os.Getenv("HOME"), ".helm"))
	settings := cli.New()
	if fileExists(defaultRepositoryConfigPath) {
		settings.RepositoryConfig = defaultRepositoryConfigPath
		// 修改默认 的harborEntry，从新赋值
		repoEntry, err := repo.LoadFile(settings.RepositoryConfig)
		if err != nil {
			log.Fatal("Failed to load repository config:", err)
		}
		if e := repoEntry.Get("harbor"); e != nil {
			harborEntry = *e
		}

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
	cacheDir := filepath.Join(helmHome, "helm", "repository")
	// 确保目录存在
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return fmt.Errorf("无法创建 Helm Cache 目录: %v", err)
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
	s.RepositoryCache = cacheDir
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

	// 按照 chartInfos.Name 首写字母排序
	sort.Slice(chartInfos, func(i, j int) bool {
		return strings.ToLower(chartInfos[i].Name)[0] < strings.ToLower(chartInfos[j].Name)[0]
	})

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

	if req.Keyword != "" {
		// 过滤包含关键词的 charts
		filteredCharts := make([]*pb.ChartInfo, 0)
		for _, chart := range pagedCharts {
			if strings.Contains(strings.ToLower(chart.Name), strings.ToLower(req.Keyword)) ||
				strings.Contains(strings.ToLower(chart.Description), strings.ToLower(req.Keyword)) {
				filteredCharts = append(filteredCharts, chart)
			}
		}
		listChartsData.Charts = filteredCharts
		listChartsData.Total = int32(len(filteredCharts))
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
	logger.L().Info("InstallChart called", zap.String("request", req.String()))
	namespace := req.GetNamespace()
	if namespace == "" {
		return nil, status.Errorf(codes.InvalidArgument, "namespace is required")
	}
	// 1. 获取参数
	dryRun := req.DryRun
	// 2. 创建 Helm action 配置

	// 3. 解析 values
	var values map[string]any
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
		releaseName := req.GetReleaseName()
		// 为指定 namespace 创建专用的 action configuration
		actionConfig := new(action.Configuration)
		debugLog := func(format string, v ...interface{}) {
			logger.L().Debug(fmt.Sprintf(format, v...))
		}
		if err := actionConfig.Init(helmClient.settings.RESTClientGetter(), namespace, "secret", debugLog); err != nil {
			logger.L().Error("Failed to initialize Helm action configuration", zap.Error(err))
			return nil, err
		}

		// 创建 Install action，使用专用的 actionConfig
		install := action.NewInstall(actionConfig)
		install.ReleaseName = releaseName
		install.Namespace = namespace
		if req.Version != "" {
			install.Version = req.Version
		}
		install.ChartPathOptions.InsecureSkipTLSverify = true
		install.CreateNamespace = true // 确保 namespace 存在，如果不存在则创建

		err := refreshChartRepository()
		if err != nil {
			logger.L().Error("Failed to refresh chart repository", zap.Error(err))
			return nil, err
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

		var release *release.Release
		get := action.NewGet(actionConfig)

		if _, err := get.Run(releaseName); err == nil {
			// Release 已经存在则退出安装
			logger.L().Info("Release already exists, skipping installation", zap.String("release_name", releaseName))
			return &pb.InstallChartResponse{
				Code:        0,
				Message:     "Release already exists, skipping installation",
				ReleaseName: releaseName,
			}, nil
		} else {
			// Release 不存在，执行安装
			release, err = install.Run(chart, values)
			if err != nil {
				logger.L().Error("Failed to install chart", zap.Error(err))
				// 安装失败，调用uninstall方法清理已安装的资源
				uninstall := action.NewUninstall(actionConfig)
				uninstall.Wait = true
				uninstall.Run(releaseName)
				return &pb.InstallChartResponse{
					Code:        1,
					Message:     fmt.Sprintf("Failed to install chart: %v", err),
					ReleaseName: releaseName,
				}, err
			}

			logger.L().Info("Chart installed successfully", zap.String("release", release.Name))
		}

		// 获取release info 中的 k8s 资源信息
		parseAndPrintManifest(release.Manifest)

		return &pb.InstallChartResponse{
			Code:          0,
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
	nameSpace := req.GetNamespace()
	if nameSpace == "" {
		return nil, status.Errorf(codes.InvalidArgument, "namespace is required")
	}

	// 为指定 namespace 创建新的 action configuration
	actionConfig := new(action.Configuration)
	debugLog := func(format string, v ...interface{}) {
		logger.L().Debug(fmt.Sprintf(format, v...))
	}
	if err := actionConfig.Init(helmClient.settings.RESTClientGetter(), nameSpace, "secret", debugLog); err != nil {
		logger.L().Error("Failed to initialize Helm action configuration for uninstall", zap.Error(err))
		return &pb.UninstallChartResponse{
			Code:    1,
			Message: "Failed to initialize Helm configuration",
		}, err
	}

	// 创建 Uninstall action
	uninstall := action.NewUninstall(actionConfig)
	uninstall.Wait = true

	_, err := uninstall.Run(req.GetReleaseName())
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

// PushChartToHarbor 将 chart 文件推送到 Harbor 仓库
func PushChartToHarbor(filePath, repoName, fileName string) (string, error) {
	// 打开文件
	file, err := os.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to open file: %v", err)
	}
	defer file.Close()

	// 创建 multipart form
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	// 添加文件字段
	part, err := writer.CreateFormFile("chart", fileName)
	if err != nil {
		return "", fmt.Errorf("failed to create form file: %v", err)
	}

	if _, err := io.Copy(part, file); err != nil {
		return "", fmt.Errorf("failed to copy file to form: %v", err)
	}

	writer.Close()

	// 构建 Harbor API URL
	harborApiUrl := fmt.Sprintf("%s/api/chartrepo/%s/charts",
		strings.TrimSuffix(harborEntry.URL, "/chartrepo/library"), repoName)

	// 创建 HTTP 请求
	req, err := http.NewRequest("POST", harborApiUrl, &body)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %v", err)
	}

	// 设置请求头
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.SetBasicAuth(harborEntry.Username, harborEntry.Password)

	// 发送请求
	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to send request: %v", err)
	}
	defer resp.Body.Close()

	// 检查响应状态
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("harbor API error: status %d, body: %s", resp.StatusCode, string(bodyBytes))
	}

	logger.L().Info("Chart pushed to Harbor successfully",
		zap.String("fileName", fileName),
		zap.String("repoName", repoName))

	// 返回 chart URL
	return fmt.Sprintf("%s/charts/%s", harborEntry.URL, fileName), nil
}

func refreshChartRepository() error {
	logger.L().Info("RefreshChartRepository called")

	repoFile := helmClient.settings.RepositoryConfig
	repoObj, err := repo.LoadFile(repoFile)
	if err == nil {
		for _, entry := range repoObj.Repositories {
			chartRepo, err := repo.NewChartRepository(entry, getter.All(helmClient.settings))
			if err == nil {
				path, err := chartRepo.DownloadIndexFile()
				if err != nil {
					logger.L().Error("Failed to download index file from chart repository", zap.Error(err))
					return err
				} else {
					logger.L().Info("Index file downloaded successfully", zap.String("repository", entry.Name), zap.String("path", path))
				}
			}
		}
	}

	return err
}

func parseAndPrintManifest(manifest string) error {
	// 按 "---" 分割多个资源
	resources := strings.Split(manifest, "---")
	for _, res := range resources {
		if strings.TrimSpace(res) == "" {
			continue
		}

		// 将 YAML 解析为 unstructured.Unstructured 对象
		obj := &unstructured.Unstructured{}
		if err := yaml.Unmarshal([]byte(res), obj); err != nil {
			return fmt.Errorf("failed to decode YAML: %v", err)
		}

		// 打印资源关键信息
		fmt.Printf(
			"Resource: %s/%s (Kind: %s, API: %s)\n",
			obj.GetNamespace(),
			obj.GetName(),
			obj.GetKind(),
			obj.GetAPIVersion(),
		)
	}
	return nil
}

func (s *HelmManagerServer) UpgradeChart(ctx context.Context, req *pb.UpgradeChartRequest) (*pb.UpgradeChartResponse, error) {
	logger.L().Info("UpgradeChart called", zap.String("request", req.String()))
	nameSpace := req.GetNamespace()
	if nameSpace == "" {
		nameSpace = "default"
	}

	// 创建 Upgrade Action
	actionConfig := new(action.Configuration)
	debugLog := func(format string, v ...interface{}) {
		logger.L().Debug(fmt.Sprintf(format, v...))
	}
	if err := actionConfig.Init(helmClient.settings.RESTClientGetter(), nameSpace, "secret", debugLog); err != nil {
		logger.L().Error("Failed to initialize Helm action configuration for upgrade", zap.Error(err))
		return nil, status.Errorf(codes.Internal, "initialize helm action failed: %v", err)
	}

	upgrade := action.NewUpgrade(actionConfig)
	upgrade.Namespace = nameSpace
	upgrade.Force = req.Force
	upgrade.ChartPathOptions.InsecureSkipTLSverify = true
	upgrade.ChartPathOptions.Version = req.Chart.ChartVersion

	// 2. 获取 Chart
	chartRef := fmt.Sprintf("%s/%s", harborEntry.Name, req.Chart.ChartName)

	chartPath, err := upgrade.ChartPathOptions.LocateChart(chartRef, helmClient.settings)
	if err != nil {
		logger.L().Error("Failed to locate chart", zap.Error(err))
		return nil, err
	}

	chart, err := loader.Load(chartPath)
	if err != nil {
		logger.L().Error("Failed to load chart", zap.Error(err))
		return nil, status.Errorf(codes.Internal, "load chart failed: %v", err)
	}

	// 3. 转换 values 类型
	stringValues := req.GetChart().GetValues()
	values := make(map[string]interface{})
	for k, v := range stringValues {
		values[k] = v
	}

	// 4. 执行升级
	release, err := upgrade.Run(req.GetReleaseName(), chart, values)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "upgrade failed: %v", err)
	}

	return &pb.UpgradeChartResponse{
		Status:   release.Info.Status.String(),
		Revision: strconv.Itoa(release.Version),
	}, nil
}

func (s *HelmManagerServer) RollbackChart(ctx context.Context, req *pb.RollbackChartRequest) (*pb.RollbackChartResponse, error) {
	// 1. 创建 Rollback Action
	rollback := action.NewRollback(helmClient.actionConfig)
	// rollback.Recreate = false // Default value, can be set based on available fields in req

	// Convert revision string to int and set it
	revision, err := strconv.Atoi(req.GetRevision())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid revision: %v", err)
	}
	rollback.Version = revision

	// 2. 执行回滚
	if err := rollback.Run(req.GetReleaseName()); err != nil {
		return nil, status.Errorf(codes.Internal, "rollback failed: %v", err)
	}

	return &pb.RollbackChartResponse{}, nil
}

func (s *HelmManagerServer) ListInstalledCharts(ctx context.Context, req *pb.ListInstalledChartsRequest) (*pb.ListInstalledChartsResponse, error) {
	logger.L().Info("ListInstalledCharts called", zap.String("request", req.String()))
	namespace := req.GetNamespace()
	if namespace == "" {
		return nil, status.Errorf(codes.Internal, "namespace is required")
	}

	// 为指定 namespace 创建专用的 action configuration
	actionConfig := new(action.Configuration)
	debugLog := func(format string, v ...interface{}) {
		logger.L().Debug(fmt.Sprintf(format, v...))
	}
	if err := actionConfig.Init(helmClient.settings.RESTClientGetter(), namespace, "secret", debugLog); err != nil {
		logger.L().Error("Failed to initialize Helm action configuration for list", zap.Error(err))
		return nil, status.Errorf(codes.Internal, "initialize helm action failed: %v", err)
	}

	list := action.NewList(actionConfig)
	list.All = true
	// 如果请求中没有指定 namespace，则查询所有 namespace
	list.AllNamespaces = req.GetNamespace() == ""
	releaseName := req.GetReleaseName()

	logger.L().Info("Listing installed charts", zap.String("namespace", namespace), zap.String("release_name", releaseName))

	releases, err := list.Run()
	if len(releases) == 0 {
		logger.L().Info("No installed charts found")
		return &pb.ListInstalledChartsResponse{
			Code:    0,
			Message: "No installed charts found",
			Success: true,
			Data:    nil,
		}, nil
	}

	if err != nil {
		logger.L().Error("Failed to list installed charts", zap.Error(err))
		return nil, status.Errorf(codes.Internal, "list installed charts failed: %v", err)
	}

	var chartInfos []*pb.InstalledChart
	for _, rel := range releases {
		if req.GetNamespace() != "" && rel.Namespace != req.GetNamespace() {
			continue
		}

		info := &pb.InstalledChart{
			Name:         rel.Name,
			Namespace:    rel.Namespace,
			ChartVersion: rel.Chart.Metadata.Version,
			AppVersion:   rel.Chart.Metadata.AppVersion,
			ChartName:    rel.Chart.Metadata.Name,
			Status:       rel.Info.Status.String(),
		}

		if req.WithManifest {
			info.Manifest = rel.Manifest
		}

		chartInfos = append(chartInfos, info)
	}

	// Convert to []*anypb.Any
	var anyData []*anypb.Any
	for _, chart := range chartInfos {
		anyChart, err := anypb.New(chart)
		if err != nil {
			logger.L().Error("Failed to marshal InstalledChart to Any", zap.Error(err))
			return nil, status.Errorf(codes.Internal, "marshal chart data failed: %v", err)
		}
		anyData = append(anyData, anyChart)
	}

	return &pb.ListInstalledChartsResponse{
		Code:    0,
		Message: fmt.Sprintf("Found %d installed charts", len(chartInfos)),
		Success: true,
		Data:    anyData,
	}, nil
}

func GetPodList(ctx context.Context, namespace, releaseName string) (*v1.PodList, error) {
	logger.L().Info("GetPodList called", zap.String("namespace", namespace), zap.String("releaseName", releaseName))

	restConfig, err := rest.InClusterConfig()
	if err != nil {
		logger.L().Error("Failed to get REST config", zap.Error(err))
		return nil, status.Errorf(codes.Internal, "get REST config failed: %v", err)
	}

	clientset, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		logger.L().Error("Failed to create Kubernetes clientset", zap.Error(err))
		return nil, status.Errorf(codes.Internal, "create Kubernetes clientset failed: %v", err)
	}

	podList, err := clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: fmt.Sprintf("app.kubernetes.io/instance=%s", releaseName),
	})
	if err != nil {
		logger.L().Error("Failed to list pods", zap.Error(err))
		return nil, status.Errorf(codes.Internal, "list pods failed: %v", err)
	}

	return podList, nil
}

func calculatePodStatus(pod *v1.Pod) string {
	// 规则 1：检查容器状态（优先于 Phase）
	for _, containerStatus := range pod.Status.ContainerStatuses {
		// 1.1 容器处于 Waiting 且原因非空（如 ImagePullBackOff）
		if containerStatus.State.Waiting != nil && containerStatus.State.Waiting.Reason != "" {
			return containerStatus.State.Waiting.Reason
		}
		// 1.2 容器异常退出（如 CrashLoopBackOff）
		if containerStatus.State.Terminated != nil && containerStatus.State.Terminated.ExitCode != 0 {
			if containerStatus.State.Terminated.Reason != "" {
				return containerStatus.State.Terminated.Reason
			}
			return "Error"
		}
	}

	// 规则 2：所有容器 Running → 返回 Phase（通常是 Running）
	// 规则 3：Pod 未被调度 → 返回 Pending
	return string(pod.Status.Phase)
}

func (s *HelmManagerServer) ListPodStatus(ctx context.Context, req *pb.ListPodStatusRequest) (*pb.ListPodStatusResponse, error) {
	logger.L().Info("ListPodStatus called", zap.String("request", req.String()))

	namespace := req.GetNamespace()
	if namespace == "" {
		namespace = "default"
	}
	podList, err := GetPodList(ctx, namespace, req.GetReleaseName())
	if err != nil {
		logger.L().Error("Failed to list pods", zap.Error(err))
		return nil, status.Errorf(codes.Internal, "list pods failed: %v", err)
	}

	// 构建 PodStatus 列表
	var podStatuses []*pb.PodStatus
	for _, pod := range podList.Items {
		podStatus := &pb.PodStatus{
			Name:       pod.Name,
			Ip:         pod.Status.PodIP,
			Phase:      string(pod.Status.Phase),
			Node:       pod.Spec.NodeName,
			Labels:     pod.Labels,
			Containers: make([]*pb.ContainerStatus, 0, len(pod.Status.ContainerStatuses)),
		}

		readyCount := 0
		for _, container := range pod.Status.ContainerStatuses {
			containerStatus := &pb.ContainerStatus{
				Name:  container.Name,
				Image: container.Image,
				State: container.State.String(),
				Ready: container.Ready,
			}
			if container.Ready {
				readyCount++
			}
			podStatus.Containers = append(podStatus.Containers, containerStatus)
		}
		readStatus := fmt.Sprintf("%d/%d", readyCount, len(pod.Status.ContainerStatuses))
		podStatus.Ready = readStatus
		age := metav1.Now().Sub(pod.CreationTimestamp.Time) // 计算 Pod 的年龄
		// 根据age的大小，分别以秒/小时/天来统计
		switch {
		case age < time.Minute:
			podStatus.Age = fmt.Sprintf("%d秒", int(age.Seconds()))
		case age < time.Hour:
			podStatus.Age = fmt.Sprintf("%d分钟", int(age.Minutes()))
		default:
			podStatus.Age = fmt.Sprintf("%d小时", int(age.Hours()))
		}
		podStatus.Status = calculatePodStatus(&pod)
		podStatuses = append(podStatuses, podStatus)
	}

	return &pb.ListPodStatusResponse{
		Code:    0,
		Message: fmt.Sprintf("Found %d pods", len(podStatuses)),
		Success: true,
		Data:    &pb.PodsStatusList{Pods: podStatuses},
	}, nil
}
