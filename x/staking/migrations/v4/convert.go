package v4

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
	v046types "github.com/cosmos/cosmos-sdk/x/staking/migrations/v4/types"
	"github.com/cosmos/cosmos-sdk/x/staking/types"
)

func ConvertToNewValidator(oldValidator v046types.Validator) types.Validator {
	var probonoRate sdk.Dec

	if oldValidator.Probono {
		probonoRate = sdk.OneDec()
	} else {
		probonoRate = sdk.ZeroDec()
	}

	return types.Validator{
		OperatorAddress: oldValidator.OperatorAddress,
		ConsensusPubkey: oldValidator.ConsensusPubkey,
		Jailed:          oldValidator.Jailed,
		Status:          types.BondStatus(oldValidator.Status),
		Tokens:          oldValidator.Tokens,
		DelegatorShares: oldValidator.DelegatorShares,
		Description: types.Description{
			Moniker:         oldValidator.Description.Moniker,
			Identity:        oldValidator.Description.Identity,
			Website:         oldValidator.Description.Website,
			SecurityContact: oldValidator.Description.SecurityContact,
			Details:         oldValidator.Description.Details,
		},
		UnbondingHeight: oldValidator.UnbondingHeight,
		Commission: types.Commission{
			CommissionRates: types.CommissionRates{
				Rate:          oldValidator.Commission.Rate,
				MaxRate:       oldValidator.Commission.MaxRate,
				MaxChangeRate: oldValidator.Commission.MaxChangeRate,
			},
			UpdateTime: oldValidator.Commission.UpdateTime,
		},
		MinSelfDelegation: oldValidator.MinSelfDelegation,
		MaxDelegation:     oldValidator.MaxDelegation,
		ProbonoRate:       probonoRate,
	}
}
