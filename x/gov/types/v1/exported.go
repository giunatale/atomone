package v1

import sdk "github.com/cosmos/cosmos-sdk/types"

type GovernorI interface {
	GetMoniker() string                                 // moniker of the governor
	GetStatus() GovernorStatus                          // status of the governor
	IsActive() bool                                     // check if has a active status
	IsInactive() bool                                   // check if has status inactive
	GetAddress() GovernorAddress                        // governor address to receive/return governors delegations
	GetDescription() GovernorDescription                // description of the governor
	GetDelegations() []ValidatorDelegation              // get all the x/staking shares delegated to the governor
	GetDelegationShares(valAddr sdk.ValAddress) sdk.Dec // get the x/staking shares delegated to the governor for a specific validator
}
