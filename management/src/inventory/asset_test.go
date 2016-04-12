// +build unittest

package inventory

import (
	"testing"

	"github.com/contiv/cluster/management/src/mock"
	"github.com/contiv/errored"
	"github.com/golang/mock/gomock"
	. "gopkg.in/check.v1"
)

// Hook up gocheck into the "go test" runner.
func Test(t *testing.T) { TestingT(t) }

type inventorySuite struct {
}

var _ = Suite(&inventorySuite{})

func (s *inventorySuite) TestNewAsset(c *C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mClient := mock.NewMockSubsysClient(ctrl)
	eAsset := &Asset{
		client:     mClient,
		name:       "foo",
		status:     Unallocated,
		prevStatus: Incomplete,
		state:      Discovered,
		prevState:  Unknown,
	}
	mClient.EXPECT().CreateAsset(eAsset.name, eAsset.status.String())
	mClient.EXPECT().SetAssetStatus(eAsset.name, eAsset.status.String(),
		eAsset.state.String(), StateDescription[eAsset.state])
	rAsset, err := NewAsset(mClient, eAsset.name)
	c.Assert(err, IsNil)
	c.Assert(rAsset, DeepEquals, eAsset)
}

func (s *inventorySuite) TestNewAssetCreateFailure(c *C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mClient := mock.NewMockSubsysClient(ctrl)
	eAsset := &Asset{
		client:     mClient,
		name:       "foo",
		status:     Unallocated,
		prevStatus: Incomplete,
		state:      Discovered,
		prevState:  Unknown,
	}
	mClient.EXPECT().CreateAsset(eAsset.name,
		eAsset.status.String()).Return(errored.Errorf("test error"))
	_, err := NewAsset(mClient, eAsset.name)
	c.Assert(err, NotNil)
}

func (s *inventorySuite) TestNewAssetSetStatusFailure(c *C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mClient := mock.NewMockSubsysClient(ctrl)
	eAsset := &Asset{
		client:     mClient,
		name:       "foo",
		status:     Unallocated,
		prevStatus: Incomplete,
		state:      Discovered,
		prevState:  Unknown,
	}
	mClient.EXPECT().CreateAsset(eAsset.name, eAsset.status.String())
	mClient.EXPECT().SetAssetStatus(eAsset.name, eAsset.status.String(),
		eAsset.state.String(), StateDescription[eAsset.state]).Return(errored.Errorf("test error"))
	_, err := NewAsset(mClient, eAsset.name)
	c.Assert(err, NotNil)
}

func (s *inventorySuite) TestSetStatus(c *C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mClient := mock.NewMockSubsysClient(ctrl)
	asset := &Asset{
		client:     mClient,
		name:       "foo",
		status:     Unallocated,
		prevStatus: Incomplete,
		state:      Discovered,
		prevState:  Unknown,
	}
	eAsset := &Asset{
		client:     mClient,
		name:       "foo",
		status:     Provisioning,
		prevStatus: Unallocated,
		state:      Disappeared,
		prevState:  Discovered,
	}
	mClient.EXPECT().SetAssetStatus(asset.name, eAsset.status.String(),
		eAsset.state.String(), StateDescription[eAsset.state])
	err := asset.SetStatus(eAsset.status, eAsset.state)
	c.Assert(err, IsNil)
	c.Assert(asset, DeepEquals, eAsset)
}

func (s *inventorySuite) TestSetStatusNoTransition(c *C) {
	asset := &Asset{
		client:     nil,
		name:       "foo",
		status:     Unallocated,
		prevStatus: Incomplete,
		state:      Discovered,
		prevState:  Unknown,
	}
	eAsset := &Asset{
		client:     nil,
		name:       "foo",
		status:     Unallocated,
		prevStatus: Incomplete,
		state:      Discovered,
		prevState:  Unknown,
	}
	err := asset.SetStatus(eAsset.status, eAsset.state)
	c.Assert(err, IsNil)
	c.Assert(asset, DeepEquals, eAsset)
}

func (s *inventorySuite) TestSetStatusInvalidStatusTransition(c *C) {
	asset := &Asset{
		client:     nil,
		name:       "foo",
		status:     Unallocated,
		prevStatus: Incomplete,
		state:      Discovered,
		prevState:  Unknown,
	}
	eAsset := &Asset{
		client:     nil,
		name:       "foo",
		status:     Cancelled,
		prevStatus: Unallocated,
		state:      Disappeared,
		prevState:  Discovered,
	}
	errStr := "transition from.*is not allowed"
	err := asset.SetStatus(eAsset.status, eAsset.state)
	c.Assert(err, ErrorMatches, errStr)
}

func (s *inventorySuite) TestSetStatusUnallowedState(c *C) {
	asset := &Asset{
		client:     nil,
		name:       "foo",
		status:     Incomplete,
		prevStatus: Incomplete,
		state:      Discovered,
		prevState:  Unknown,
	}
	eAsset := &Asset{
		client:     nil,
		name:       "foo",
		status:     Incomplete,
		prevStatus: Unallocated,
		state:      Disappeared,
		prevState:  Discovered,
	}
	errStr := ".*is not a valid state when asset is in.*status"
	err := asset.SetStatus(eAsset.status, eAsset.state)
	c.Assert(err, ErrorMatches, errStr)
}

func (s *inventorySuite) TestSetStatusSetFailure(c *C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mClient := mock.NewMockSubsysClient(ctrl)
	asset := &Asset{
		client:     mClient,
		name:       "foo",
		status:     Unallocated,
		prevStatus: Incomplete,
		state:      Discovered,
		prevState:  Unknown,
	}
	eAsset := &Asset{
		client:     mClient,
		name:       "foo",
		status:     Unallocated,
		prevStatus: Incomplete,
		state:      Discovered,
		prevState:  Unknown,
	}
	mClient.EXPECT().SetAssetStatus(asset.name, Provisioning.String(),
		Discovered.String(), StateDescription[asset.state]).Return(errored.Errorf("test failure"))
	err := asset.SetStatus(Provisioning, Discovered)
	c.Assert(err, NotNil)
	c.Assert(asset, DeepEquals, eAsset)
}
