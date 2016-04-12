//go:generate mockgen -destination ../mock/inventory_client_mock.go -package=mock github.com/contiv/cluster/management/src/inventory SubsysClient

package inventory

import "encoding/json"

// Subsys provides the following services to the cluster manager:
// - Interface to perform CRUD operations on the asset inventory.
type Subsys interface {
	//AddAsset adds an asset discovered for first time
	AddAsset(name string) error
	//SetAssetDiscovered sets an asset state to discovered
	SetAssetDiscovered(name string) error
	//SetAssetDisappeared sets an asset state to disappeared
	SetAssetDisappeared(name string) error
	//SetAssetProvisioning sets an asset state to provisioning
	SetAssetProvisioning(name string) error
	//SetAssetCommissioned sets an asset state to commissioned (aka allocated)
	SetAssetCommissioned(name string) error
	//SetAssetCancelled sets an asset state to cancelled
	SetAssetCancelled(name string) error
	//SetAssetDecommissioned sets an asset state to decommissioned
	SetAssetDecommissioned(name string) error
	//SetAssetInMaintenance sets an asset state to maintenance
	SetAssetInMaintenance(name string) error
	//SetAssetUnallocated sets an asset status to unallocated
	SetAssetUnallocated(name string) error
	//GetAsset finds and returns the asset in inventory
	GetAsset(name string) SubsysAsset
	//GetAllAssets returns all the assets in inventory
	GetAllAssets() SubsysAssets
}

// SubsysClient provides the client interface for the inventory subsystem
type SubsysClient interface {
	GetAllAssets() (interface{}, error)
	CreateAsset(tag, status string) error
	CreateState(name, description, status string) error
	AddAssetLog(tag, mtype, message string) error
	SetAssetStatus(tag, status, state, reason string) error
}

// SubsysAsset denotes a single asset in inventory subsystem
type SubsysAsset interface {
	//GetStatus returns the current status of the asset
	GetStatus() (AssetStatus, AssetState)
	//GetTag returns the inventory tag of the asset
	GetTag() string
	//SubsysAsset shall satisfy the json marshaller interface to encode asset's info in json
	json.Marshaler
}

// SubsysAssets denotes a collection of assets in the inventory subsystem
type SubsysAssets interface{}
