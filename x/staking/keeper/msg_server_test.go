package keeper_test

import (
	"testing"
	"time"

	"github.com/cosmos/cosmos-sdk/simapp"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/x/bank/testutil"
	disttypes "github.com/cosmos/cosmos-sdk/x/distribution/types"
	"github.com/cosmos/cosmos-sdk/x/staking"
	"github.com/cosmos/cosmos-sdk/x/staking/keeper"
	"github.com/cosmos/cosmos-sdk/x/staking/teststaking"
	"github.com/cosmos/cosmos-sdk/x/staking/types"
	"github.com/stretchr/testify/require"
	tmproto "github.com/tendermint/tendermint/proto/tendermint/types"
)

func TestCreateProbonoValidator_Settlus(t *testing.T) {
	_, app, ctx := createSettlusTestInput(t)

	// reset fee pool
	app.DistrKeeper.SetFeePool(ctx, disttypes.InitialFeePool())

	addrDels := simapp.AddTestAddrsIncremental(app, ctx, 2, app.StakingKeeper.TokensFromConsensusPower(ctx, 1000))
	valAddrs := simapp.ConvertAddrsToValAddrs(addrDels)
	pks := simapp.CreateTestPubKeys(2)
	tstaking := teststaking.NewHelper(t, ctx, app.StakingKeeper)

	// Fund community pool
	communityPoolAmount := sdk.NewInt(100)
	communityPoolReserve := sdk.NewCoins(sdk.NewCoin(sdk.DefaultBondDenom, communityPoolAmount))
	probonoAmount := sdk.NewInt(40)
	normalAmount := sdk.NewInt(30)

	feePool := app.DistrKeeper.GetFeePool(ctx)
	feePool.CommunityPool = sdk.NewDecCoinsFromCoins(communityPoolReserve...)
	app.DistrKeeper.SetFeePool(ctx, feePool)

	// create PROBONO validator with 0% commission
	tstaking.Commission = types.NewCommissionRates(sdk.NewDec(0), sdk.NewDec(0), sdk.NewDec(0))
	tstaking.CreateProbonoValidator(valAddrs[0], pks[0], probonoAmount, true, sdk.NewDecWithPrec(2, 10))

	// create second validator with 0% commission
	tstaking.Commission = types.NewCommissionRates(sdk.NewDec(0), sdk.NewDec(0), sdk.NewDec(0))
	tstaking.CreateValidator(valAddrs[1], pks[1], normalAmount, true)

	staking.EndBlocker(ctx, app.StakingKeeper)
	ctx = ctx.WithBlockHeight(ctx.BlockHeight() + 1)

	probonoVal := app.StakingKeeper.Validator(ctx, valAddrs[0])
	normalVal := app.StakingKeeper.Validator(ctx, valAddrs[1])

	// check if the probono amount is correctly subtracted from the community pool and each validator has correct amount
	require.Equal(t, sdk.DecCoins{{Denom: sdk.DefaultBondDenom, Amount: sdk.NewDecFromInt(communityPoolAmount.Sub(probonoAmount))}}, app.DistrKeeper.GetFeePool(ctx).CommunityPool)
	require.Equal(t, probonoVal.GetTokens(), probonoAmount)
	require.Equal(t, normalVal.GetTokens(), normalAmount)
}

func TestCancelUnbondingDelegation(t *testing.T) {
	// setup the app
	app := simapp.Setup(t, false)
	ctx := app.BaseApp.NewContext(false, tmproto.Header{})
	msgServer := keeper.NewMsgServerImpl(app.StakingKeeper)
	bondDenom := app.StakingKeeper.BondDenom(ctx)

	// set the not bonded pool module account
	notBondedPool := app.StakingKeeper.GetNotBondedPool(ctx)
	startTokens := app.StakingKeeper.TokensFromConsensusPower(ctx, 5)

	require.NoError(t, testutil.FundModuleAccount(app.BankKeeper, ctx, notBondedPool.GetName(), sdk.NewCoins(sdk.NewCoin(app.StakingKeeper.BondDenom(ctx), startTokens))))
	app.AccountKeeper.SetModuleAccount(ctx, notBondedPool)

	moduleBalance := app.BankKeeper.GetBalance(ctx, notBondedPool.GetAddress(), app.StakingKeeper.BondDenom(ctx))
	require.Equal(t, sdk.NewInt64Coin(bondDenom, startTokens.Int64()), moduleBalance)

	// accounts
	delAddrs := simapp.AddTestAddrsIncremental(app, ctx, 2, sdk.NewInt(10000))
	validators := app.StakingKeeper.GetValidators(ctx, 10)
	require.Equal(t, len(validators), 1)

	validatorAddr, err := sdk.ValAddressFromBech32(validators[0].OperatorAddress)
	require.NoError(t, err)
	delegatorAddr := delAddrs[0]

	// setting the ubd entry
	unbondingAmount := sdk.NewInt64Coin(app.StakingKeeper.BondDenom(ctx), 5)
	ubd := types.NewUnbondingDelegation(
		delegatorAddr, validatorAddr, 10,
		ctx.BlockTime().Add(time.Minute*10),
		unbondingAmount.Amount,
	)

	// set and retrieve a record
	app.StakingKeeper.SetUnbondingDelegation(ctx, ubd)
	resUnbond, found := app.StakingKeeper.GetUnbondingDelegation(ctx, delegatorAddr, validatorAddr)
	require.True(t, found)
	require.Equal(t, ubd, resUnbond)

	testCases := []struct {
		Name      string
		ExceptErr bool
		req       types.MsgCancelUnbondingDelegation
	}{
		{
			Name:      "invalid height",
			ExceptErr: true,
			req: types.MsgCancelUnbondingDelegation{
				DelegatorAddress: resUnbond.DelegatorAddress,
				ValidatorAddress: resUnbond.ValidatorAddress,
				Amount:           sdk.NewCoin(app.StakingKeeper.BondDenom(ctx), sdk.NewInt(4)),
				CreationHeight:   0,
			},
		},
		{
			Name:      "invalid coin",
			ExceptErr: true,
			req: types.MsgCancelUnbondingDelegation{
				DelegatorAddress: resUnbond.DelegatorAddress,
				ValidatorAddress: resUnbond.ValidatorAddress,
				Amount:           sdk.NewCoin("dump_coin", sdk.NewInt(4)),
				CreationHeight:   0,
			},
		},
		{
			Name:      "validator not exists",
			ExceptErr: true,
			req: types.MsgCancelUnbondingDelegation{
				DelegatorAddress: resUnbond.DelegatorAddress,
				ValidatorAddress: sdk.ValAddress(sdk.AccAddress("asdsad")).String(),
				Amount:           unbondingAmount,
				CreationHeight:   0,
			},
		},
		{
			Name:      "invalid delegator address",
			ExceptErr: true,
			req: types.MsgCancelUnbondingDelegation{
				DelegatorAddress: "invalid_delegator_addrtess",
				ValidatorAddress: resUnbond.ValidatorAddress,
				Amount:           unbondingAmount,
				CreationHeight:   0,
			},
		},
		{
			Name:      "invalid amount",
			ExceptErr: true,
			req: types.MsgCancelUnbondingDelegation{
				DelegatorAddress: resUnbond.DelegatorAddress,
				ValidatorAddress: resUnbond.ValidatorAddress,
				Amount:           unbondingAmount.Add(sdk.NewInt64Coin(bondDenom, 10)),
				CreationHeight:   10,
			},
		},
		{
			Name:      "success",
			ExceptErr: false,
			req: types.MsgCancelUnbondingDelegation{
				DelegatorAddress: resUnbond.DelegatorAddress,
				ValidatorAddress: resUnbond.ValidatorAddress,
				Amount:           unbondingAmount.Sub(sdk.NewInt64Coin(bondDenom, 1)),
				CreationHeight:   10,
			},
		},
		{
			Name:      "success",
			ExceptErr: false,
			req: types.MsgCancelUnbondingDelegation{
				DelegatorAddress: resUnbond.DelegatorAddress,
				ValidatorAddress: resUnbond.ValidatorAddress,
				Amount:           unbondingAmount.Sub(unbondingAmount.Sub(sdk.NewInt64Coin(bondDenom, 1))),
				CreationHeight:   10,
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.Name, func(t *testing.T) {
			_, err := msgServer.CancelUnbondingDelegation(ctx, &testCase.req)
			if testCase.ExceptErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				balanceForNotBondedPool := app.BankKeeper.GetBalance(ctx, sdk.AccAddress(notBondedPool.GetAddress()), bondDenom)
				require.Equal(t, balanceForNotBondedPool, moduleBalance.Sub(testCase.req.Amount))
				moduleBalance = moduleBalance.Sub(testCase.req.Amount)
			}
		})
	}
}
