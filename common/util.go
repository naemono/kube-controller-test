package common

import (
	"k8s.io/client-go/pkg/api"
	"k8s.io/client-go/pkg/api/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

func GetClientConfig(kubeconfig string) (*rest.Config, error) {
	if kubeconfig != "" {
		return clientcmd.BuildConfigFromFlags("", kubeconfig)
	}
	return rest.InClusterConfig()
}

func CopyObjToNode(obj interface{}) (*v1.Pod, error) {
	objCopy, err := api.Scheme.Copy(obj.(*v1.Pod))
	if err != nil {
		return nil, err
	}

	pod := objCopy.(*v1.Pod)
	if pod.Annotations == nil {
		pod.Annotations = make(map[string]string)
	}
	return pod, nil
}
