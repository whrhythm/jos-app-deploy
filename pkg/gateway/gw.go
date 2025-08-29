package gateway

import (
	"context"
	"fmt"

	apisixv2 "github.com/apache/apisix-ingress-controller/pkg/kube/apisix/apis/config/v2"
	apisixclient "github.com/apache/apisix-ingress-controller/pkg/kube/apisix/client/clientset/versioned"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/rest"
)

type Gateway struct{}

type Backend struct {
	ServiceName string
	ServicePort int32
	Weight      int32
}

type HTTPRoute struct {
	Hosts    string
	Paths    string
	Backends []Backend
}

type StreamRoute struct {
	IngressPort int32
	Backend     Backend
}

// apiVersion: apisix.apache.org/v2
// kind: ApisixRoute
// metadata:
//
//	name: weighted-routing-example
//	namespace: default
//
// spec:
//
//	http:
//	- name: rule1
//	  match:
//	    hosts:
//	    - example.com
//	    paths:
//	    - /api/v1/*
//	  backends:
//	  # 第一个服务 - 权重 70%
//	  - serviceName: service-a
//	    servicePort: 80
//	    weight: 70
//	  # 第二个服务 - 权重 30%
//	  - serviceName: service-b
//	    servicePort: 80
//	    weight: 30
//	  # 可选的插件配置

func NewGateway() *Gateway {
	return &Gateway{}
}

func (g *Gateway) CreateOrUpdateRoute(config *rest.Config,
	arName, namespace string,
	arHttp []apisixv2.ApisixRouteHTTP,
	arStream []apisixv2.ApisixRouteStream) error {
	clientset, err := apisixclient.NewForConfig(config)
	if err != nil {
		return fmt.Errorf("failed to create Apisix clientset: %w", err)
	}

	// 查询是否存在该 ApisixRoute 资源
	existing, err := clientset.ApisixV2().ApisixRoutes(namespace).Get(context.Background(), arName, metav1.GetOptions{})
	if err != nil {
		// 如果不存在，则创建新的 ApisixRoute 资源
		if len(arHttp) == 0 && len(arStream) == 0 {
			return fmt.Errorf("no HTTP or Stream routes provided")
		}

		newAR := &apisixv2.ApisixRoute{
			ObjectMeta: metav1.ObjectMeta{
				Name:      arName,
				Namespace: namespace,
			},
			Spec: apisixv2.ApisixRouteSpec{
				HTTP:   arHttp,
				Stream: arStream,
			},
		}

		_, err = clientset.ApisixV2().ApisixRoutes(namespace).Create(context.Background(), newAR, metav1.CreateOptions{})
		if err != nil {
			return fmt.Errorf("failed to create ApisixRoute: %w", err)
		}
	} else {
		// 如果存在，则更新该 ApisixRoute 资源
		existing.Spec.HTTP = arHttp
		existing.Spec.Stream = arStream
		_, err = clientset.ApisixV2().ApisixRoutes(namespace).Update(context.Background(), existing, metav1.UpdateOptions{})
		if err != nil {
			return fmt.Errorf("failed to update ApisixRoute: %w", err)
		}
	}
	return nil
}

func (g *Gateway) DeleteRoute(config *rest.Config, arName, namespace string) error {
	clientset, err := apisixclient.NewForConfig(config)
	if err != nil {
		return fmt.Errorf("failed to create Apisix clientset: %w", err)
	}

	err = clientset.ApisixV2().ApisixRoutes(namespace).Delete(context.Background(), arName, metav1.DeleteOptions{})
	if err != nil {
		return fmt.Errorf("failed to delete ApisixRoute %s: %w", arName, err)
	}

	return nil
}

func ConvertHTTPRoutes(httpRoutes []HTTPRoute) []apisixv2.ApisixRouteHTTP {
	var arHttps []apisixv2.ApisixRouteHTTP
	for _, httpRoute := range httpRoutes {
		arHttps = append(arHttps, apisixv2.ApisixRouteHTTP{
			Name: fmt.Sprintf("http-route-%s", httpRoute.Hosts),
			Match: apisixv2.ApisixRouteHTTPMatch{
				Hosts: []string{httpRoute.Hosts},
				Paths: []string{httpRoute.Paths},
			},
			Backends: convertHttpBackends(httpRoute.Backends),
		})
	}
	return arHttps
}

func ConvertStreamRoutes(streamRoutes []StreamRoute) []apisixv2.ApisixRouteStream {
	var arStream []apisixv2.ApisixRouteStream
	for _, streamRoute := range streamRoutes {
		arStream = append(arStream, apisixv2.ApisixRouteStream{
			Name: fmt.Sprintf("stream-route-%d", streamRoute.IngressPort),
			Match: apisixv2.ApisixRouteStreamMatch{
				IngressPort: streamRoute.IngressPort,
			},
			Backend: convertStreamBackend(streamRoute.Backend),
		})
	}
	return arStream
}

func convertHttpBackends(backends []Backend) []apisixv2.ApisixRouteHTTPBackend {
	var apiBackends []apisixv2.ApisixRouteHTTPBackend
	for _, b := range backends {
		w := int(b.Weight)
		apiBackend := apisixv2.ApisixRouteHTTPBackend{
			ServiceName: b.ServiceName,
			ServicePort: intstr.FromInt(int(b.ServicePort)),
			Weight:      &w,
		}
		apiBackends = append(apiBackends, apiBackend)
	}
	return apiBackends
}

func convertStreamBackend(backend Backend) apisixv2.ApisixRouteStreamBackend {
	return apisixv2.ApisixRouteStreamBackend{
		ServiceName: backend.ServiceName,
		ServicePort: intstr.FromInt(int(backend.ServicePort)),
	}
}
