package keeper_test

import (
	"math/big"
	"testing"

	tmproto "github.com/tendermint/tendermint/proto/tendermint/types"

	"github.com/cosmos/cosmos-sdk/codec"
	"github.com/cosmos/cosmos-sdk/simapp"
	sdk "github.com/cosmos/cosmos-sdk/types"
	disttypes "github.com/cosmos/cosmos-sdk/x/distribution/types"
	"github.com/cosmos/cosmos-sdk/x/staking/keeper"
	"github.com/cosmos/cosmos-sdk/x/staking/types"
)

var PKs = simapp.CreateTestPubKeys(500)

func init() {
	sdk.DefaultPowerReduction = sdk.NewIntFromBigInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil))
}

// createTestInput Returns a simapp with custom StakingKeeper
// to avoid messing with the hooks.
func createTestInput(t *testing.T) (*codec.LegacyAmino, *simapp.SimApp, sdk.Context) {
	app := simapp.Setup(t, false)
	ctx := app.BaseApp.NewContext(false, tmproto.Header{})

	app.StakingKeeper = keeper.NewKeeper(
		app.AppCodec(),
		app.GetKey(types.StoreKey),
		app.AccountKeeper,
		app.BankKeeper,
		app.GetSubspace(types.ModuleName),
	)

	return app.LegacyAmino(), app, ctx
}

// createSettlusTestInput Returns a simapp with custom StakingKeeper which contains hooks
func createSettlusTestInput(t *testing.T) (*codec.LegacyAmino, *simapp.SimApp, sdk.Context) {
	app := simapp.Setup(t, false)
	ctx := app.BaseApp.NewContext(false, tmproto.Header{})

	stakingKeeper := keeper.NewKeeper(
		app.AppCodec(),
		app.GetKey(types.StoreKey),
		app.AccountKeeper,
		app.BankKeeper,
		app.GetSubspace(types.ModuleName),
	)

	app.StakingKeeper = *stakingKeeper.SetHooks(
		types.NewMultiStakingHooks(app.DistrKeeper.Hooks(), app.SlashingKeeper.Hooks()),
	)

	// Settlus param settings
	app.DistrKeeper.SetParams(ctx, disttypes.Params{
		CommunityTax:        sdk.NewDecWithPrec(2, 1),
		BaseProposerReward:  sdk.ZeroDec(),
		BonusProposerReward: sdk.ZeroDec(),
		WithdrawAddrEnabled: disttypes.DefaultParams().WithdrawAddrEnabled,
	})

	app.StakingKeeper.SetParams(ctx, types.Params{
		UnbondingTime:     types.DefaultUnbondingTime,
		MaxValidators:     40,
		MaxEntries:        7,
		BondDenom:         sdk.DefaultBondDenom,
		HistoricalEntries: types.DefaultHistoricalEntries,
		MinCommissionRate: types.DefaultMinCommissionRate,
	})

	return app.LegacyAmino(), app, ctx
}

// intended to be used with require/assert:  require.True(ValEq(...))
func ValEq(t *testing.T, exp, got types.Validator) (*testing.T, bool, string, types.Validator, types.Validator) {
	return t, exp.MinEqual(&got), "expected:\n%v\ngot:\n%v", exp, got
}

// generateAddresses generates numAddrs of normal AccAddrs and ValAddrs
func generateAddresses(app *simapp.SimApp, ctx sdk.Context, numAddrs int) ([]sdk.AccAddress, []sdk.ValAddress) {
	addrDels := simapp.AddTestAddrsIncremental(app, ctx, numAddrs, sdk.NewInt(10000))
	addrVals := simapp.ConvertAddrsToValAddrs(addrDels)

	return addrDels, addrVals
}
