package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// MongodbReplicaset defines the Mongodb Replicaset
type MongodbReplicaset struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Type              string         `json:"type"`
	Spec              ReplicasetSpec `json:"spec"`
	Status            CRDStatus      `json:"status,omitempty"`
}

// CRDStatus is the Custom Resource Definition Status
type CRDStatus struct {
	State   CRDState `json:"state,omitempty"`
	Message string   `json:"message,omitempty"`
}

// CRDState is the state of the Custom Resource Definition
type CRDState string

// ReplicasetSpec defines replicaset options
type ReplicasetSpec struct {
	// DataNodeReplicas defines how many client nodes to have in cluster
	DataNodeReplicas int32 `json:"data-node-replicas"`

	// ArbiterNodeReplicas defines how many client nodes to have in cluster
	ArbiterNodeReplicas int `json:"arbiter-node-replicas"`

	// NodeSelector specifies a map of key-value pairs. For the pod to be eligible
	// to run on a node, the node must have each of the indicated key-value pairs as
	// labels.
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`

	// Zones specifies a map of key-value pairs. Defines which zones
	// to deploy persistent volumes for data nodes
	Zones []string `json:"zones,omitempty"`

	// DataDiskSize specifies how large the persistent volume should be attached
	// to the data nodes in the Mongodb Replicaset
	DataDiskSize string `json:"data-disk-size"`

	// MongodbImage specifies the docker image to use (optional)
	MongodbImage string `json:"mongodb-image"`

	// Storage defines how volumes are provisioned
	Storage Storage `json:"storage"`

	// ImagePullSecrets defines credentials to pull image from private repository (optional)
	ImagePullSecrets []ImagePullSecrets `json:"image-pull-secrets,omitempty"`

	// Resources defines memory / cpu constraints
	Resources Resources `json:"resources"`

	// AdminPassword is the administrative password
	AdminPassword string `json:"admin-password"`

	// AdminUsername is the administrative password
	AdminUsername string `json:"admin-username"`
}

// ImagePullSecrets defines credentials to pull image from private repository
type ImagePullSecrets struct {
	// Name defines the name of the secret file that will be used
	Name string `json:"name"`
}

// Storage defines how dynamic volumes are created
// https://kubernetes.io/docs/user-guide/persistent-volumes/
type Storage struct {
	// StorageType is the type of storage to create
	StorageType string `json:"type"`

	// StorageClassProvisoner is the storage provisioner type
	StorageClassProvisoner string `json:"storage-class-provisioner"`

	// StorageClass to use
	StorageClass string `json:"storage-class"`

	// Volume Reclaim Policy on Persistent Volumes
	VolumeReclaimPolicy string `json:"volume-reclaim-policy"`
}

// Resources defines CPU / Memory restrictions on pods
type Resources struct {
	Limits MemoryCPU `json:"limits"`
}

// MemoryCPU defines memory cpu options
type MemoryCPU struct {
	// Memory defines max amount of memory
	Memory string `json:"memory"`

	// CPU defines max amount of CPU
	CPU string `json:"cpu"`
}
