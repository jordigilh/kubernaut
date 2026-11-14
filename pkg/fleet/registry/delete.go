package registry

import (
	"sync"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/tools/cache"
)

func handleDeletedCluster(
	obj interface{},
	mu *sync.RWMutex,
	currentClusters func() map[string]ClusterInfo,
	revision *uint64,
	metrics *Metrics,
	logger logr.Logger,
	emit func(ClusterEvent),
) {
	u, ok := obj.(*unstructured.Unstructured)
	if !ok {
		tombstone, tombstoneOK := obj.(cache.DeletedFinalStateUnknown)
		if !tombstoneOK {
			return
		}
		u, ok = tombstone.Obj.(*unstructured.Unstructured)
		if !ok {
			return
		}
	}

	id := u.GetName()
	mu.Lock()
	clusters := currentClusters()
	info, existed := clusters[id]
	if existed && info.RegistrationUID != string(u.GetUID()) {
		mu.Unlock()
		return
	}
	delete(clusters, id)
	if existed {
		*revision++
	}
	clusterCount := len(clusters)
	mu.Unlock()

	if existed {
		metrics.NilSafeSetClusters(float64(clusterCount))
		metrics.NilSafeIncReconcile()
		emit(ClusterEvent{Type: EventDeleted, Cluster: info})
		logger.Info("cluster deleted", "id", id)
	}
}
