package routes

import (
	"context"
	"fmt"
	pb "jos-deployment/api/v1alpha1/pb_routes"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
)

// ApisixManager provides operations for creating ApisixUpstream and ApisixRoute CRs.
type ApisixManager interface {
	CreateUpstream(ctx context.Context, namespace, upstreamName, host string, port int32) error
	// traffic: optional map of upstreamName->weight. If nil or empty, route will point to the single upstreamName.
	CreateRoute(ctx context.Context, namespace, routeName, host, serviceName string, servicePort int32, upstreamName string, traffic map[string]int) error
	DeleteUpstream(ctx context.Context, namespace, upstreamName string) error
	ListAR(ctx context.Context, namespace string) ([]*unstructured.Unstructured, error)
}

type apisixManagerImpl struct {
	dyn dynamic.Interface
}

// NewApisixManagerForConfig constructs an ApisixManager using the provided rest.Config.
func NewApisixManagerForConfig(cfg *rest.Config) (ApisixManager, error) {
	dyn, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create dynamic client: %v", err)
	}
	return &apisixManagerImpl{dyn: dyn}, nil
}

var (
	upstreamGVR = schema.GroupVersionResource{Group: "apisix.apache.org", Version: "v2", Resource: "apisixupstreams"}
	routeGVR    = schema.GroupVersionResource{Group: "apisix.apache.org", Version: "v2", Resource: "apisixroutes"}
)

func (a *apisixManagerImpl) CreateUpstream(ctx context.Context, namespace, upstreamName, host string, port int32) error {
	upstream := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "apisix.apache.org/v2",
			"kind":       "ApisixUpstream",
			"metadata": map[string]interface{}{
				"name":      upstreamName,
				"namespace": namespace,
			},
			"spec": map[string]interface{}{
				"type": "RoundRobin",
				"nodes": []interface{}{
					map[string]interface{}{"host": host, "port": int64(port), "weight": 1},
				},
			},
		},
	}

	_, err := a.dyn.Resource(upstreamGVR).Namespace(namespace).Create(ctx, upstream, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to create ApisixUpstream: %v", err)
	}
	return nil
}

func (a *apisixManagerImpl) CreateRoute(ctx context.Context, namespace, routeName, host, serviceName string, servicePort int32, upstreamName string, traffic map[string]int) error {
	// Build backend(s). If traffic map is provided and non-empty, create multiple backends with weights.
	var backendObj interface{}
	if len(traffic) == 0 {
		backendObj = map[string]interface{}{"serviceName": serviceName, "servicePort": int64(servicePort), "upstream": upstreamName}
	} else {
		// Build list of backends for traffic splitting
		backends := make([]interface{}, 0, len(traffic))
		for u, w := range traffic {
			// ensure positive weight
			if w <= 0 {
				continue
			}
			backends = append(backends, map[string]interface{}{
				"serviceName": serviceName,
				"servicePort": int64(servicePort),
				"upstream":    u,
				"weight":      int64(w),
			})
		}
		if len(backends) == 0 {
			// fallback to single upstream if traffic map had no positive weights
			backendObj = map[string]interface{}{"serviceName": serviceName, "servicePort": int64(servicePort), "upstream": upstreamName}
		} else {
			backendObj = backends
		}
	}

	route := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "apisix.apache.org/v2",
			"kind":       "ApisixRoute",
			"metadata": map[string]interface{}{
				"name":      routeName,
				"namespace": namespace,
			},
			"spec": map[string]interface{}{
				"http": []interface{}{
					map[string]interface{}{
						"name":  "rule-1",
						"match": map[string]interface{}{"hosts": []interface{}{host}, "paths": []interface{}{"/"}},
						// For single backend backendObj is a map; for split traffic it's a slice of backends.
						"backend": backendObj,
					},
				},
			},
		},
	}

	_, err := a.dyn.Resource(routeGVR).Namespace(namespace).Create(ctx, route, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to create ApisixRoute: %v", err)
	}
	return nil
}

func (a *apisixManagerImpl) DeleteUpstream(ctx context.Context, namespace, upstreamName string) error {
	return a.dyn.Resource(upstreamGVR).Namespace(namespace).Delete(ctx, upstreamName, metav1.DeleteOptions{})
}

func (a *apisixManagerImpl) ListAR(ctx context.Context, namespace string) ([]*unstructured.Unstructured, error) {
	arList, err := a.dyn.Resource(routeGVR).Namespace(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list ApisixRoutes: %v", err)
	}
	result := make([]*unstructured.Unstructured, 0, len(arList.Items))
	for i := range arList.Items {
		result = append(result, &arList.Items[i])
	}
	return result, nil
}

func (s *RoutesManageService) CreateApisixRoute(ctx context.Context, req *pb.CreateApisixRouteRequest) (*pb.CreateApisixRouteResponse, error) {

	return nil, nil
}

func (a *RoutesManageService) DeleteApisixRoute(ctx context.Context, req *pb.DeleteApisixRouteRequest) (*pb.DeleteApisixRouteResponse, error) {

	return nil, nil
}
