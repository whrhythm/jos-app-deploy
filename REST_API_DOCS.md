# REST API 文件上传接口文档

## 接口概述

**路由**: `POST /v1alpha1/chart/upload`  
**功能**: 上传 Helm Chart (.tgz) 文件到默认 Harbor 仓库  
**内容类型**: `multipart/form-data`

## 请求参数

### 必需参数
| 参数名 | 类型 | 说明 | 示例 |
|--------|------|------|------|
| `chart` | File | Helm Chart 文件 (.tgz 格式) | `nginx-1.0.0.tgz` |

### 可选参数
| 参数名 | 类型 | 默认值 | 说明 | 示例 |
|--------|------|--------|------|------|
| `chart_name` | String | `"uploaded-chart"` | Chart 名称 | `"nginx"` |
| `chart_version` | String | `"1.0.0"` | Chart 版本 | `"1.2.3"` |
| `repo_name` | String | `"library"` | 目标 Harbor 项目名 | `"my-project"` |

## 响应格式

### 成功响应 (200 OK)
```json
{
  "success": true,
  "message": "Chart uploaded successfully",
  "chart_url": "https://harbor.joiningos.com/chartrepo/library/charts/nginx-1.2.3.tgz",
  "size_received": 12345,
  "digest": "sha256:abcdef123456789..."
}
```

### 错误响应
```json
{
  "success": false,
  "message": "错误描述信息"
}
```

## HTTP 状态码

| 状态码 | 说明 |
|--------|------|
| 200 | 上传成功 |
| 400 | 请求参数错误 |
| 405 | 请求方法不允许 (仅支持 POST) |
| 500 | 服务器内部错误 |

## 使用示例

### cURL 命令行

#### 基本上传
```bash
curl -X POST \
  -F "chart=@my-chart-1.0.0.tgz" \
  http://localhost:8080/v1alpha1/chart/upload
```

#### 完整参数上传
```bash
curl -X POST \
  -F "chart=@nginx-chart.tgz" \
  -F "chart_name=nginx" \
  -F "chart_version=1.2.3" \
  -F "repo_name=my-project" \
  http://localhost:8080/v1alpha1/chart/upload
```

### JavaScript (Fetch API)

```javascript
async function uploadChart(file, chartName, chartVersion, repoName) {
  const formData = new FormData();
  formData.append('chart', file);
  
  if (chartName) formData.append('chart_name', chartName);
  if (chartVersion) formData.append('chart_version', chartVersion);
  if (repoName) formData.append('repo_name', repoName);

  const response = await fetch('/v1alpha1/chart/upload', {
    method: 'POST',
    body: formData
  });

  return await response.json();
}

// 使用示例
const fileInput = document.getElementById('chartFile');
const file = fileInput.files[0];

uploadChart(file, 'nginx', '1.2.3', 'library')
  .then(result => {
    if (result.success) {
      console.log('上传成功:', result.chart_url);
    } else {
      console.error('上传失败:', result.message);
    }
  });
```

### Python (requests)

```python
import requests

def upload_chart(file_path, chart_name=None, chart_version=None, repo_name=None):
    url = 'http://localhost:8080/v1alpha1/chart/upload'
    
    files = {'chart': open(file_path, 'rb')}
    data = {}
    
    if chart_name:
        data['chart_name'] = chart_name
    if chart_version:
        data['chart_version'] = chart_version
    if repo_name:
        data['repo_name'] = repo_name
    
    response = requests.post(url, files=files, data=data)
    return response.json()

# 使用示例
result = upload_chart(
    'nginx-1.0.0.tgz',
    chart_name='nginx',
    chart_version='1.2.3',
    repo_name='library'
)

if result['success']:
    print(f"上传成功: {result['chart_url']}")
else:
    print(f"上传失败: {result['message']}")
```

### Go 客户端

```go
package main

import (
    "bytes"
    "fmt"
    "io"
    "mime/multipart"
    "net/http"
    "os"
)

func uploadChart(filePath, chartName, chartVersion, repoName string) error {
    file, err := os.Open(filePath)
    if err != nil {
        return err
    }
    defer file.Close()

    var body bytes.Buffer
    writer := multipart.NewWriter(&body)

    // 添加文件
    part, err := writer.CreateFormFile("chart", filepath.Base(filePath))
    if err != nil {
        return err
    }
    io.Copy(part, file)

    // 添加可选参数
    if chartName != "" {
        writer.WriteField("chart_name", chartName)
    }
    if chartVersion != "" {
        writer.WriteField("chart_version", chartVersion)
    }
    if repoName != "" {
        writer.WriteField("repo_name", repoName)
    }

    writer.Close()

    req, err := http.NewRequest("POST", "http://localhost:8080/v1alpha1/chart/upload", &body)
    if err != nil {
        return err
    }
    req.Header.Set("Content-Type", writer.FormDataContentType())

    client := &http.Client{}
    resp, err := client.Do(req)
    if err != nil {
        return err
    }
    defer resp.Body.Close()

    fmt.Printf("Response Status: %s\n", resp.Status)
    return nil
}
```

## 错误处理

### 常见错误

1. **文件格式错误** (400)
   ```json
   {
     "success": false,
     "message": "Only .tgz files are allowed"
   }
   ```

2. **缺少文件** (400)
   ```json
   {
     "success": false,
     "message": "Failed to get chart file: no such file"
   }
   ```

3. **Harbor 推送失败** (500)
   ```json
   {
     "success": false,
     "message": "Failed to push to Harbor: harbor API error: status 401, body: unauthorized"
   }
   ```

### 错误排查

1. **检查文件格式**: 确保上传的是 `.tgz` 文件
2. **检查文件大小**: 默认限制 32MB
3. **检查 Harbor 配置**: 确认 Harbor 地址和认证信息正确
4. **检查网络连接**: 确认能访问 Harbor 服务

## 与 gRPC 接口的区别

| 特性 | REST API | gRPC API |
|------|----------|----------|
| 协议 | HTTP/1.1 | HTTP/2 |
| 数据格式 | multipart/form-data | Protocol Buffers |
| 流式传输 | ❌ | ✅ |
| 浏览器支持 | ✅ | 有限 |
| 性能 | 适中 | 更高 |
| 使用场景 | Web 前端、简单集成 | 微服务、高性能场景 |

## 测试工具

运行提供的测试脚本：

```bash
# Linux/Mac
./test_rest_api.sh

# Windows
test_rest_api.bat
```

或使用 Postman、Insomnia 等 API 测试工具进行测试。
