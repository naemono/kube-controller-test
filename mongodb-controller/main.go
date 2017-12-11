package main

import (
	"flag"
	"fmt"
	"strings"
	"time"

	"github.com/golang/glog"
	"github.com/naemono/kube-controller-test/common"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	lister_v1 "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/pkg/api/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"

	mongodbreplicasetclientset "github.com/naemono/kube-controller-test/pkg/client/clientset/versioned"
)

// TODO(aaron): make configurable and add MinAvailable
const maxUnavailable = 1

func main() {
	// When running as a pod in-cluster, a kubeconfig is not needed. Instead this will make use of the service account injected into the pod.
	// However, allow the use of a local kubeconfig as this can make local development & testing easier.
	kubeconfig := flag.String("kubeconfig", "", "Path to a kubeconfig file")

	// We log to stderr because glog will default to logging to a file.
	// By setting this debugging is easier via `kubectl logs`
	flag.Set("logtostderr", "true")
	flag.Parse()

	// Build the client config - optionally using a provided kubeconfig file.
	config, err := common.GetClientConfig(*kubeconfig)
	if err != nil {
		glog.Fatalf("Failed to load client config: %v", err)
	}

	// Construct the Kubernetes client
	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		glog.Fatalf("Failed to create kubernetes client: %v", err)
	}

	mclient, err := mongodbreplicasetclientset.NewForConfig(config)
	if err != nil {
		glog.Fatalf("Failed to create mongodb client: %v", err)
	}

	stopCh := make(chan struct{})
	defer close(stopCh)

	newMongodbController(client, mclient).Run(stopCh)
}

type mongodbController struct {
	client      kubernetes.Interface
	mongoClient mongodbreplicasetclientset
	podLister   lister_v1.PodLister
	informer    cache.Controller
	queue       workqueue.RateLimitingInterface
}

func newMongodbController(client kubernetes.Interface, mclient mongodbreplicasetclientset.Interface) *mongodbController {
	rc := &mongodbController{
		client:  client,
		mclient: mclient,
		queue:   workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter()),
	}

	indexer, informer := cache.NewIndexerInformer(
		&cache.ListWatch{
			ListFunc: func(lo meta_v1.ListOptions) (runtime.Object, error) {
				// We do not add any selectors because we want to watch all nodes.
				// This is so we can determine the total count of "unavailable" nodes.
				// However, this could also be implemented using multiple informers (or better, shared-informers)
				return client.Core().Pods("default").List(lo)
			},
			WatchFunc: func(lo meta_v1.ListOptions) (watch.Interface, error) {
				return client.Core().Pods("default").Watch(lo)
			},
		},
		// The types of objects this informer will return
		&v1.Pod{},
		// The resync period of this object. This will force a re-queue of all cached objects at this interval.
		// Every object will trigger the `Updatefunc` even if there have been no actual updates triggered.
		// In some cases you can set this to a very high interval - as you can assume you will see periodic updates in normal operation.
		// The interval is set low here for demo purposes.
		10*time.Second,
		// Callback Functions to trigger on add/update/delete
		cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				if key, err := cache.MetaNamespaceKeyFunc(obj); err == nil {
					rc.queue.Add(key)
				}
			},
			UpdateFunc: func(old, new interface{}) {
				if key, err := cache.MetaNamespaceKeyFunc(new); err == nil {
					rc.queue.Add(key)
				}
			},
			DeleteFunc: func(obj interface{}) {
				if key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj); err == nil {
					rc.queue.Add(key)
				}
			},
		},
		cache.Indexers{},
	)

	rc.informer = informer
	// NodeLister avoids some boilerplate code (e.g. convert runtime.Object to *v1.node)
	rc.podLister = lister_v1.NewPodLister(indexer)

	return rc
}

func (c *mongodbController) Run(stopCh chan struct{}) {
	defer c.queue.ShutDown()
	glog.Info("Starting mongodbController")

	go c.informer.Run(stopCh)

	// Wait for all caches to be synced, before processing items from the queue is started
	if !cache.WaitForCacheSync(stopCh, c.informer.HasSynced) {
		glog.Error(fmt.Errorf("Timed out waiting for caches to sync"))
		return
	}

	// Launching additional goroutines would parallelize workers consuming from the queue (but we don't really need this)
	go wait.Until(c.runWorker, time.Second, stopCh)

	<-stopCh
	glog.Info("Stopping Reboot Cont	roller")
}

func (c *mongodbController) runWorker() {
	for c.processNext() {
	}
}

func (c *mongodbController) processNext() bool {
	// Wait until there is a new item in the working queue
	key, quit := c.queue.Get()
	if quit {
		return false
	}
	// Tell the queue that we are done with processing this key. This unblocks the key for other workers
	// This allows safe parallel processing because two pods with the same key are never processed in
	// parallel.
	defer c.queue.Done(key)
	// Invoke the method containing the business logic
	err := c.process(key.(string))
	// Handle the error if something went wrong during the execution of the business logic
	c.handleErr(err, key)
	return true
}

func (c *mongodbController) process(key string) error {
	result := strings.Split(key, "/")
	if len(result) == 0 {
		return fmt.Errorf("Couldn't split pod %q", key)
	}
	var podName = result[1]
	pod, err := c.podLister.Pods("default").Get(podName)
	if err != nil {
		return fmt.Errorf("failed to retrieve pod by key %q: %v", podName, err)
	}

	glog.V(4).Infof("Received update of pod: %s", pod.GetName())
	if pod.Annotations == nil {
		return nil // If pod has no annotations...
	}

	if _, ok := pod.Annotations[common.RebootNeededAnnotation]; !ok {
		return nil // Node does not need reboot
	}

	// Determine if we should reboot based on maximum number of unavailable nodes
	unavailable, err := c.unavailableNodeCount()
	if err != nil {
		return fmt.Errorf("Failed to determine number of unavailable nodes: %v", err)
	}

	if unavailable >= maxUnavailable {
		// TODO(aaron): We might want this case to retry indefinitely. Could create a specific error an check in handleErr()
		return fmt.Errorf("Too many nodes unvailable (%d/%d). Skipping reboot of %s", unavailable, maxUnavailable, pod.Name)
	}

	// We should not modify the cache object directly, so we make a copy first
	podCopy, err := common.CopyObjToNode(pod)
	if err != nil {
		return fmt.Errorf("Failed to make copy of pod: %v", err)
	}

	glog.Infof("Marking pod %s for reboot", pod.Name)
	podCopy.Annotations[common.RebootAnnotation] = ""
	if _, err := c.client.Core().Pods("default").Update(podCopy); err != nil {
		return fmt.Errorf("Failed to set %s annotation: %v", common.RebootAnnotation, err)
	}
	return nil
}

func (c *mongodbController) handleErr(err error, key interface{}) {
	if err == nil {
		// Forget about the #AddRateLimited history of the key on every successful synchronization.
		// This ensures that future processing of updates for this key is not delayed because of
		// an outdated error history.
		c.queue.Forget(key)
		return
	}

	// This controller retries 5 times if something goes wrong. After that, it stops trying.
	if c.queue.NumRequeues(key) < 5 {
		glog.Infof("Error processing node %v: %v", key, err)

		// Re-enqueue the key rate limited. Based on the rate limiter on the
		// queue and the re-enqueue history, the key will be processed later again.
		c.queue.AddRateLimited(key)
		return
	}

	c.queue.Forget(key)
	glog.Errorf("Dropping node %q out of the queue: %v", key, err)
}

func (c *mongodbController) unavailableNodeCount() (int, error) {
	// nodes, err := c.nodeLister.List(labels.Everything())
	// if err != nil {
	// 	return 0, err
	// }
	// var unavailable int
	// for _, n := range nodes {
	// 	if nodeIsRebooting(n) {
	// 		unavailable++
	// 		continue
	// 	}
	// 	for _, c := range n.Status.Conditions {
	// 		if c.Type == v1.NodeReady && c.Status == v1.ConditionFalse {
	// 			unavailable++
	// 		}
	// 	}
	// }
	var unavailable = 10
	return unavailable, nil
}

func podIsRebooting(p *v1.Pod) bool {
	// Check if node is marked for reeboot-in-progress
	if p.Annotations == nil {
		return false // No annotations - not marked as needing reboot
	}
	if _, ok := p.Annotations[common.RebootInProgressAnnotation]; ok {
		return true
	}
	// Check if node is already marked for immediate reboot
	_, ok := p.Annotations[common.RebootAnnotation]
	return ok
}
